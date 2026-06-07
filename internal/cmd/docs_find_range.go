package cmd

import (
	"context"
	"fmt"
	"html"
	"os"
	"strings"
	"unicode"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DocsFindRangeCmd struct {
	DocID               string `arg:"" name:"docId" help:"Doc ID"`
	Text                string `arg:"" name:"text" help:"Text to find"`
	MatchCase           bool   `name:"match-case" help:"Use case-sensitive matching"`
	All                 bool   `name:"all" help:"Return all matches"`
	Occurrence          *int   `name:"occurrence" help:"Return the Nth occurrence (1-based; default first)"`
	NormalizeWhitespace bool   `name:"normalize-whitespace" help:"Collapse whitespace while matching" default:"true" negatable:""`
	FailEmpty           bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no matches"`
	Tab                 string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	TabID               string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
}

type docsFindRangeResult struct {
	Matches []docsTextRangeMatch `json:"matches"`
}

type docsTextRangeOptions struct {
	MatchCase           bool
	NormalizeWhitespace bool
	TabID               string
}

type docsTextRangeMatch struct {
	StartIndex     int64  `json:"startIndex"`
	EndIndex       int64  `json:"endIndex"`
	ParagraphIndex int    `json:"paragraphIndex"`
	TabID          string `json:"tabId"`
}

type docsTextUnit struct {
	StartByte  int
	EndByte    int
	StartIndex int64
	EndIndex   int64
}

func (c *DocsFindRangeCmd) Run(ctx context.Context, flags *RootFlags) error {
	id := strings.TrimSpace(c.DocID)
	if id == "" {
		return usage("empty docId")
	}
	if c.Text == "" {
		return usage("empty text")
	}
	if c.All && c.Occurrence != nil {
		return usage("--all and --occurrence are mutually exclusive")
	}
	if c.Occurrence != nil && *c.Occurrence <= 0 {
		return usage("--occurrence must be > 0")
	}

	tab, tabErr := resolveTabArg(ctx, c.Tab, c.TabID)
	if tabErr != nil {
		return tabErr
	}
	c.Tab = tab

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	doc, tabID, err := c.loadTargetDoc(ctx, svc, id)
	if err != nil {
		return err
	}
	matches := findDocsTextRanges(doc, c.Text, docsTextRangeOptions{
		MatchCase:           c.MatchCase,
		NormalizeWhitespace: c.NormalizeWhitespace,
		TabID:               tabID,
	})
	matches = c.selectMatches(matches)

	return c.writeResult(ctx, matches)
}

func (c *DocsFindRangeCmd) loadTargetDoc(ctx context.Context, svc *docs.Service, docID string) (*docs.Document, string, error) {
	getCall := svc.Documents.Get(docID).Context(ctx)
	if c.Tab != "" {
		getCall = getCall.IncludeTabsContent(true)
	}
	doc, err := getCall.Do()
	if err != nil {
		if isDocsNotFound(err) {
			return nil, "", fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return nil, "", err
	}
	doc, err = requireRawResponse(doc, "doc not found")
	if err != nil {
		return nil, "", err
	}

	if c.Tab == "" {
		return doc, "", nil
	}
	tab, tabErr := findTab(flattenTabs(doc.Tabs), c.Tab)
	if tabErr != nil {
		return nil, "", tabErr
	}
	tabID := ""
	if tab.TabProperties != nil {
		tabID = tab.TabProperties.TabId
	}
	targetDoc := &docs.Document{}
	if tab.DocumentTab != nil {
		targetDoc.Body = tab.DocumentTab.Body
	}
	return targetDoc, tabID, nil
}

func (c *DocsFindRangeCmd) selectMatches(matches []docsTextRangeMatch) []docsTextRangeMatch {
	if c.All {
		return matches
	}
	occurrence := 1
	if c.Occurrence != nil {
		occurrence = *c.Occurrence
	}
	if occurrence > len(matches) {
		return nil
	}
	return matches[occurrence-1 : occurrence]
}

func (c *DocsFindRangeCmd) writeResult(ctx context.Context, matches []docsTextRangeMatch) error {
	if matches == nil {
		matches = []docsTextRangeMatch{}
	}
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, docsFindRangeResult{Matches: matches}); err != nil {
			return err
		}
		return failEmptyIfNoDocsRange(c.FailEmpty, matches)
	}

	u := ui.FromContext(ctx)
	for _, match := range matches {
		u.Out().Linef("%d\t%d\t%d\t%s", match.StartIndex, match.EndIndex, match.ParagraphIndex, match.TabID)
	}
	return failEmptyIfNoDocsRange(c.FailEmpty, matches)
}

func failEmptyIfNoDocsRange(failEmpty bool, matches []docsTextRangeMatch) error {
	if len(matches) == 0 {
		return failEmptyExit(failEmpty)
	}
	return nil
}

func findDocsTextRanges(doc *docs.Document, searchText string, opts docsTextRangeOptions) []docsTextRangeMatch {
	if doc == nil || doc.Body == nil {
		return nil
	}
	find := prepareDocsFindNeedle(searchText, opts)
	if find == "" {
		return nil
	}

	var matches []docsTextRangeMatch
	paragraphIndex := 0
	findDocsTextRangesInElements(doc.Body.Content, find, opts, &paragraphIndex, &matches)
	return matches
}

func findDocsTextRangesInElements(elements []*docs.StructuralElement, find string, opts docsTextRangeOptions, paragraphIndex *int, matches *[]docsTextRangeMatch) {
	for _, el := range elements {
		if el == nil {
			continue
		}
		switch {
		case el.Paragraph != nil:
			findDocsTextRangesInParagraph(el.Paragraph, find, opts, *paragraphIndex, matches)
			*paragraphIndex++
		case el.Table != nil:
			for _, row := range el.Table.TableRows {
				for _, cell := range row.TableCells {
					findDocsTextRangesInElements(cell.Content, find, opts, paragraphIndex, matches)
				}
			}
		}
	}
}

func findDocsTextRangesInParagraph(para *docs.Paragraph, find string, opts docsTextRangeOptions, paragraphIndex int, matches *[]docsTextRangeMatch) {
	text, units := buildDocsComparableParagraphText(para, opts)
	if text == "" {
		return
	}

	offset := 0
	for {
		idx := strings.Index(text[offset:], find)
		if idx < 0 {
			return
		}
		startByte := offset + idx
		endByte := startByte + len(find)
		startIndex, endIndex, ok := docsOriginalRangeForComparableBytes(units, startByte, endByte)
		if ok {
			*matches = append(*matches, docsTextRangeMatch{
				StartIndex:     startIndex,
				EndIndex:       endIndex,
				ParagraphIndex: paragraphIndex,
				TabID:          opts.TabID,
			})
		}
		offset = endByte
	}
}

func prepareDocsFindNeedle(text string, opts docsTextRangeOptions) string {
	text = html.UnescapeString(text)
	var b strings.Builder
	lastWasWhitespace := false
	for _, r := range text {
		if opts.NormalizeWhitespace && unicode.IsSpace(r) {
			if !lastWasWhitespace {
				b.WriteRune(' ')
				lastWasWhitespace = true
			}
			continue
		}
		if !opts.MatchCase {
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
		lastWasWhitespace = false
	}
	return b.String()
}

func buildDocsComparableParagraphText(para *docs.Paragraph, opts docsTextRangeOptions) (string, []docsTextUnit) {
	if para == nil {
		return "", nil
	}
	var b strings.Builder
	var units []docsTextUnit
	lastWhitespaceUnit := -1
	for _, pe := range para.Elements {
		if pe == nil || pe.TextRun == nil {
			continue
		}
		runOffset := int64(0)
		for _, r := range pe.TextRun.Content {
			startIndex := pe.StartIndex + runOffset
			endIndex := startIndex + utf16RuneLen(r)
			runOffset = endIndex - pe.StartIndex

			if opts.NormalizeWhitespace && unicode.IsSpace(r) {
				if lastWhitespaceUnit >= 0 {
					units[lastWhitespaceUnit].EndIndex = endIndex
					continue
				}
				appendDocsTextUnit(&b, &units, ' ', startIndex, endIndex)
				lastWhitespaceUnit = len(units) - 1
				continue
			}

			if !opts.MatchCase {
				r = unicode.ToLower(r)
			}
			appendDocsTextUnit(&b, &units, r, startIndex, endIndex)
			lastWhitespaceUnit = -1
		}
	}
	return b.String(), units
}

func appendDocsTextUnit(b *strings.Builder, units *[]docsTextUnit, r rune, startIndex, endIndex int64) {
	startByte := b.Len()
	b.WriteRune(r)
	*units = append(*units, docsTextUnit{
		StartByte:  startByte,
		EndByte:    b.Len(),
		StartIndex: startIndex,
		EndIndex:   endIndex,
	})
}

func docsOriginalRangeForComparableBytes(units []docsTextUnit, startByte, endByte int) (int64, int64, bool) {
	first := -1
	last := -1
	for i, unit := range units {
		if first < 0 && unit.EndByte > startByte {
			first = i
		}
		if unit.StartByte < endByte {
			last = i
			continue
		}
		break
	}
	if first < 0 || last < first {
		return 0, 0, false
	}
	return units[first].StartIndex, units[last].EndIndex, true
}

func utf16RuneLen(r rune) int64 {
	if r >= 0x10000 {
		return 2
	}
	return 1
}
