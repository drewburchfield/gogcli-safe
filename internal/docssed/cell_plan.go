//nolint:wsl_v5 // Cell matching, replacement normalization, and plan construction stay adjacent.
package docssed

import "strings"

// CellInput is the indexed text content of one table cell.
type CellInput struct {
	Text           string
	TextStartIndex int64
	TextEndIndex   int64
}

// CellPlanner owns one compiled cell replacement expression.
type CellPlanner struct {
	expression Expression
	matcher    *MatchPlanner
}

// NewCellPlanner validates one cell replacement expression.
func NewCellPlanner(expression Expression) (*CellPlanner, error) {
	planner := &CellPlanner{expression: expression}
	if expression.Pattern == "" {
		return planner, nil
	}
	matcher, err := NewMatchPlanner(expression)
	if err != nil {
		return nil, err
	}
	planner.matcher = matcher
	return planner, nil
}

// PlanCellReplacement plans one whole-cell or sub-pattern replacement.
func PlanCellReplacement(input CellInput, expression Expression) (TextPlan, error) {
	planner, err := NewCellPlanner(expression)
	if err != nil {
		return TextPlan{}, err
	}
	return planner.Plan(input), nil
}

// Plan plans one cell using the planner's compiled expression.
func (p *CellPlanner) Plan(input CellInput) TextPlan {
	if p.matcher == nil {
		return PlanWholeCellReplacement(input, p.expression.Replacement)
	}

	actions := p.matcher.PlanSegment(DocumentSegment{TextRuns: []DocumentTextRun{{
		Text:       input.Text,
		StartIndex: input.TextStartIndex,
		EndIndex:   input.TextEndIndex,
	}}})
	for index := range actions {
		expanded := actions[index].Replacement.ExpandedText
		actions[index].Replacement = Replacement{
			Kind:         ReplacementText,
			ExpandedText: expanded,
			Text:         expanded,
		}
	}
	return PlanTextMutations(actions)
}

// PlanWholeCellReplacement replaces cell text while preserving its terminal newline.
func PlanWholeCellReplacement(input CellInput, replacement string) TextPlan {
	cellText := strings.TrimRight(input.Text, "\n")
	expanded := strings.ReplaceAll(strings.ReplaceAll(replacement, "$$", "$"), "${0}", cellText)
	markdown := ParseMarkdownReplacement(expanded)

	deleteEnd := input.TextEndIndex
	if strings.HasSuffix(input.Text, "\n") && deleteEnd > input.TextStartIndex {
		deleteEnd--
	}
	plan := TextPlan{
		MatchCount: 1,
		TextEdits: []TextEdit{{
			StartIndex: input.TextStartIndex,
			EndIndex:   deleteEnd,
			InsertText: markdown.Text,
		}},
	}
	if markdown.Text == "" || len(markdown.Formats) == 0 {
		return plan
	}

	end := input.TextStartIndex + utf16Length(markdown.Text)
	plan.Formatting = []FormatIntent{{
		StartIndex:           input.TextStartIndex,
		EndIndex:             end,
		StructuralStartIndex: input.TextStartIndex,
		StructuralEndIndex:   end,
		Formats:              append([]string(nil), markdown.Formats...),
		LeadingTab:           strings.HasPrefix(markdown.Text, "\t"),
	}}
	return plan
}
