package ui

import "github.com/nsgo-dev/nsgo/core"

// FormSection is related rows on a rounded card, divided by hairlines from the start of the titles above them, as an inset grouped list of iOS Settings: Title above the card and Footer below it, in the muted color. Its rows are FormRow and FormField, or the app's own nodes, which pad themselves; a core.Each among them gives each of its rows a hairline of its own.
type FormSection struct {
	Title, Footer string
	Children      []core.Node
	TestID        string
	Style         core.Style
	// CardStyle styles the rows' surface. CardStyleFunc merges over it
	// and follows the signals it reads.
	CardStyle     core.Style
	CardStyleFunc func() core.Style
	// DividerInset overrides a row's hairline inset when it returns true.
	// Zero is full width; false keeps the title alignment or 12-point default.
	// It receives the actual row for each child of a core.Each.
	DividerInset func(row core.Node) (inset core.Points, override bool)
}

// Render implements core.Node.
func (s FormSection) Render(cx *core.Scope) core.Node {
	colors := ColorsOf(cx)
	// Each row has its hairline below it, so a row from core.Each takes and keeps its own as rows come, go and move; the card's last view hides its hairline.
	var rows []core.Node
	for _, child := range s.Children {
		if each, ok := core.AfterEachRow(child, func(_ *core.Scope, row core.Node) core.Node { return s.formDivider(colors, row) }); ok {
			rows = append(rows, each)
			continue
		}
		rows = append(rows, child, s.formDivider(colors, child))
	}
	var nodes []core.Node
	if s.Title != "" {
		title := Text(s.Title, 12, Medium, colors.Muted)
		title.Role = core.RoleHeader
		title.Style.Padding = core.Symmetric(0, formRowPad)
		nodes = append(nodes, title)
	}
	nodes = append(nodes, core.View{Style: core.Style{CornerRadius: RadiusCard, BorderCurve: core.BorderCurveContinuous, Overflow: core.Hidden},
		StyleFunc: func() core.Style {
			style := core.Style{Background: Alpha(colors.Text(), 0.045)}.Merge(s.CardStyle)
			if s.CardStyleFunc != nil {
				style = style.Merge(s.CardStyleFunc())
			}
			return style
		}, Children: rows})
	if s.Footer != "" {
		footer := Text(s.Footer, 11.5, Regular, colors.Muted)
		footer.Style.LineHeight, footer.Style.Padding = 16, core.Symmetric(0, formRowPad)
		nodes = append(nodes, footer)
	}
	return core.View{TestID: s.TestID, Style: core.Style{Gap: 6}.Merge(s.Style), Children: nodes}
}

// formDivider is the hairline below row, which hides as the card's last view.
func (s FormSection) formDivider(colors Colors, row core.Node) core.Node {
	inset := formRowLead(row)
	if s.DividerInset != nil {
		if value, override := s.DividerInset(row); override {
			inset = value
		}
	}
	return core.View{Class: "last:hidden", Style: core.Style{Height: core.Pt(1), Margin: core.Edges{Left: inset}}, StyleFunc: Filled(colors.Line)}
}

// The rows' metrics: their least height, their padding at the sides and
// the tile an icon sits on.
const (
	formRowHeight core.Points = 40
	formRowPad    core.Points = 12
	formRowGap    core.Points = 10
	formIconTile  core.Points = 24
)

// formRowHeightFor is a row's least height: formRowHeight, or 48 points where it is reached by touch.
func formRowHeightFor(cx *core.Scope) core.Points {
	if Touch(cx) {
		return 48
	}
	return formRowHeight
}

// formRowLead is where the hairline below row starts, aligned with the
// title after the row's leading slot.
func formRowLead(row core.Node) core.Points {
	var width core.Points
	switch r := row.(type) {
	case FormRow:
		width = r.leadingWidth()
	case *FormRow:
		if r != nil {
			width = r.leadingWidth()
		}
	}
	if width != 0 {
		return formRowPad + width + formRowGap
	}
	return formRowPad
}
