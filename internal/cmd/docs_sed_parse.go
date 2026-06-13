package cmd

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/steipete/gogcli/internal/docssed"
)

func parseSedExpr(raw string) (pattern, replacement string, global bool, err error) {
	expression, err := docssed.ParseSubstitution(raw)
	if err != nil {
		return "", "", false, err
	}
	return expression.Pattern, expression.Replacement, expression.Global, nil
}

// parseTableRef checks if a pattern is a bare table reference like |1|, |2|, |-1|, |*|.
func parseTableRef(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if len(value) < 3 || value[0] != '|' || value[len(value)-1] != '|' {
		return 0, false
	}
	inner := value[1 : len(value)-1]
	if strings.ContainsAny(inner, "xX") {
		return 0, false
	}
	if inner == "*" {
		return math.MinInt32, true
	}
	index, err := strconv.Atoi(inner)
	if err != nil || index == 0 {
		return 0, false
	}
	return index, true
}

func parseSedExprWithCell(raw string) (pattern, replacement string, global bool, cellRef *tableCellRef, err error) {
	expression, err := docssed.ParseSubstitution(raw)
	if err != nil {
		return "", "", false, nil, err
	}
	pattern = expression.Pattern
	cellRef = parseTableCellRef(pattern)
	if cellRef != nil {
		if cellRef.subPattern != "" {
			pattern = cellRef.subPattern
		} else {
			pattern = ""
		}
	}
	return pattern, expression.Replacement, expression.Global, cellRef, nil
}

// parseMarkdownReplacement extracts text and formatting from markdown-style replacement.
func parseMarkdownReplacement(replacement string) (text string, formats []string) {
	text = escapeMarkdown(replacement)
	defer func() { text = unescapeMarkdown(text) }()

	trimmed := strings.TrimSpace(text)
	if trimmed == literalMarkdownTripleDash || trimmed == "***" || trimmed == "___" {
		return "\n", []string{"hrule"}
	}
	if strings.HasPrefix(text, "```") && strings.HasSuffix(text, "```") && len(text) > 6 {
		inner := text[3 : len(text)-3]
		if index := strings.Index(inner, "\n"); index >= 0 {
			inner = inner[index+1:]
		}
		return inner, append(formats, "codeblock")
	}
	if strings.HasPrefix(text, "> ") {
		return text[2:], append(formats, "blockquote")
	}
	if strings.HasPrefix(text, "[^") && strings.HasSuffix(text, "]") && len(text) > 3 {
		return text[2 : len(text)-1], []string{"footnote"}
	}

	indentLevel := 0
	listText := text
	for strings.HasPrefix(listText, "  ") {
		indentLevel++
		listText = listText[2:]
	}
	listFormat := ""
	switch {
	case strings.HasPrefix(listText, "- "):
		text = listText[2:]
		listFormat = "bullet"
	case strings.HasPrefix(listText, "* ") && !strings.HasSuffix(listText, "*"):
		text = listText[2:]
		listFormat = "bullet"
	case len(listText) > 2 && listText[0] >= '0' && listText[0] <= '9' && listText[1] == '.' && listText[2] == ' ':
		text = listText[3:]
		listFormat = "numbered"
	}
	if listFormat != "" {
		formats = append(formats, listFormat)
		if indentLevel > 0 {
			text = strings.Repeat("\t", indentLevel) + text
		}
	}

	if strings.HasPrefix(text, "***") && strings.HasSuffix(text, "***") && len(text) > 6 {
		return text[3 : len(text)-3], append(formats, "bold", "italic")
	}
	if strings.HasPrefix(text, "**") && strings.HasSuffix(text, "**") && len(text) > 4 {
		return text[2 : len(text)-2], append(formats, "bold")
	}
	if strings.HasPrefix(text, "*") && strings.HasSuffix(text, "*") && len(text) > 2 {
		return text[1 : len(text)-1], append(formats, "italic")
	}
	if strings.HasPrefix(text, "~~") && strings.HasSuffix(text, "~~") && len(text) > 4 {
		return text[2 : len(text)-2], append(formats, "strikethrough")
	}
	if strings.HasPrefix(text, "`") && strings.HasSuffix(text, "`") && len(text) > 2 {
		return text[1 : len(text)-1], append(formats, "code")
	}
	if index := strings.Index(text, "]("); index > 0 && strings.HasPrefix(text, "[") {
		closeParen := strings.LastIndex(text, ")")
		if closeParen > index+2 {
			linkText := text[1:index]
			linkURL := strings.ReplaceAll(text[index+2:closeParen], "\\/", "/")
			return linkText, append(formats, "link:"+linkURL)
		}
	}
	if strings.HasPrefix(text, "#") {
		level := 0
		for index := 0; index < len(text) && index < 6; index++ {
			if text[index] != '#' {
				break
			}
			level++
		}
		if level > 0 {
			stripped := strings.TrimPrefix(text[level:], " ")
			return stripped, append(formats, fmt.Sprintf("heading%d", level))
		}
	}
	return text, formats
}

func parseFullExpr(raw string) (sedExpr, error) {
	program, err := docssed.Parse(raw)
	if err != nil {
		return sedExpr{}, err
	}
	core := program.Expressions[0]
	expression := sedExpr{
		pattern:     core.Pattern,
		replacement: core.Replacement,
		global:      core.Global,
		nthMatch:    core.NthMatch,
		command:     byte(core.Command),
		addr:        core.Address,
	}
	if core.Command != docssed.CommandSubstitute {
		return expression, nil
	}
	return enrichSedExpression(expression)
}

func enrichSedExpression(expression sedExpr) (sedExpr, error) {
	expression.cellRef = parseTableCellRef(expression.pattern)
	if expression.cellRef != nil {
		if expression.cellRef.subPattern != "" {
			expression.pattern = expression.cellRef.subPattern
		} else {
			expression.pattern = ""
		}
	}

	if expression.cellRef == nil && strings.HasPrefix(expression.pattern, "{") {
		remaining, tableRef, imageRef, err := detectBracePattern(expression.pattern)
		if err != nil {
			return sedExpr{}, fmt.Errorf("brace pattern: %w", err)
		}
		if tableRef != nil {
			braceTableToSedExpr(tableRef, &expression)
			expression.pattern = remaining
			if tableRef.IsCreate {
				if spec := braceTableToTableCreateSpec(tableRef); spec != nil {
					if spec.header {
						expression.replacement = fmt.Sprintf("|%dx%d:header|", spec.rows, spec.cols)
					} else {
						expression.replacement = fmt.Sprintf("|%dx%d|", spec.rows, spec.cols)
					}
				}
			}
		}
		if imageRef != nil {
			if imagePattern := braceImgToImageRefPattern(imageRef); imagePattern != nil {
				switch {
				case imagePattern.AllImages:
					expression.pattern = "!(*)"
				case imagePattern.ByPosition:
					expression.pattern = fmt.Sprintf("!(%d)", imagePattern.Position)
				case imagePattern.ByAlt && imagePattern.AltRegex != nil:
					expression.pattern = fmt.Sprintf("![%s]", imageRef.Pattern)
				}
			}
		}
	}

	if expression.cellRef == nil && expression.tableRef == 0 {
		if tableIndex, ok := parseTableRef(expression.pattern); ok {
			expression.tableRef = tableIndex
			expression.pattern = ""
		}
	}

	if hasBraceFormatting(expression.replacement) {
		cleanedText, spans := findBraceExprs(expression.replacement)
		if len(spans) > 0 {
			expression.replacement = cleanedText
			expression.braceSpans = spans
			if len(spans) == 1 && spans[0].IsGlobal {
				expression.brace = spans[0].Expr
			} else {
				expression.brace = mergeBraceSpans(spans)
			}
			if expression.brace != nil && expression.brace.TableRef != "" {
				tableRef, err := parseBraceTableRef(expression.brace.TableRef)
				if err == nil && tableRef.IsCreate {
					if spec := braceTableToTableCreateSpec(tableRef); spec != nil {
						if spec.header {
							expression.replacement = fmt.Sprintf("|%dx%d:header|", spec.rows, spec.cols)
						} else {
							expression.replacement = fmt.Sprintf("|%dx%d|", spec.rows, spec.cols)
						}
						expression.brace = nil
						expression.braceSpans = nil
					}
				}
			}
		}
	}
	return expression, nil
}

func parseAddress(raw string) (*sedAddress, string, error) {
	return docssed.ParseAddress(raw)
}

func parseNthFlag(raw string) int {
	return docssed.NthFlag(raw)
}

func parseDCommand(raw string) (sedExpr, error) {
	expression, err := docssed.ParseDelete(raw)
	return sedExprFromCore(expression), err
}

func parseAICommand(raw string, command byte) (sedExpr, error) {
	expression, err := docssed.ParseInsertAppend(raw, docssed.Command(command))
	return sedExprFromCore(expression), err
}

func parseYCommand(raw string) (sedExpr, error) {
	expression, err := docssed.ParseTransliterate(raw)
	return sedExprFromCore(expression), err
}

func sedExprFromCore(expression docssed.Expression) sedExpr {
	return sedExpr{
		pattern:     expression.Pattern,
		replacement: expression.Replacement,
		global:      expression.Global,
		nthMatch:    expression.NthMatch,
		command:     byte(expression.Command),
		addr:        expression.Address,
	}
}
