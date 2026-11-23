package ui

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
)

// TestField draws a field's box, sized by its style, and turns the box off
// with transparent colors.
func TestField(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		app, p := start(t, e, func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Width: core.Pt(240), Gap: 8}, Children: []core.Node{
				Field{TestID: "boxed", Style: core.Style{Height: core.Pt(36), Padding: core.Symmetric(0, 12)}},
				Field{TestID: "bare", Style: core.Style{Height: core.Pt(36), Background: core.Transparent, BorderWidth: core.Zero}},
			}}
		})
		boxed := app.Find("boxed")
		if boxed.Frame.Size.Height != 36 || boxed.Style.Background != p.Field || boxed.Style.BorderColor != p.FieldBorder {
			t.Fatalf("boxed field: %v tall, fill %x, edge %x; want 36 and the field's colors", boxed.Frame.Size.Height, boxed.Style.Background, boxed.Style.BorderColor)
		}
		if x := boxed.Input.Content.Origin.X; x < 12 {
			t.Fatalf("boxed text starts %v in, want the style's 12", x)
		}
		bare := app.Find("bare")
		if bare.Style.Background != core.Transparent || bare.Style.BorderWidth != 0 || bare.Input.Style.Color != p.Text {
			t.Fatalf("bare field: fill %x, border %v, text %x; want no box and the text color", bare.Style.Background, bare.Style.BorderWidth, bare.Input.Style.Color)
		}
		if x := bare.Input.Content.Origin.X; x != 0 {
			t.Fatalf("bare text starts %v in, want at the edge", x)
		}
	})
}
