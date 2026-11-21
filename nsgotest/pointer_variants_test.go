package nsgotest

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
)

func TestPointerVariantsByIdiomAndEnvironment(t *testing.T) {
	root := func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			core.View{
				TestID: "target",
				Class:  "h-9 bg-white pointer-coarse:h-11 pointer-fine:h-7 dark:pointer-coarse:bg-zinc-900 pointer-fine:hover:opacity-50",
			},
			core.View{
				TestID: "plain",
				Class:  "h-9",
			},
		}}
	}

	for _, tc := range []struct {
		name   string
		opts   []Option
		height core.Points
	}{
		{"default phone", nil, 44},
		{"tablet", []Option{Idiom(core.IdiomTablet)}, 44},
		{"desktop", []Option{Idiom(core.IdiomDesktop)}, 28},
		{"tv", []Option{Idiom(core.IdiomTV)}, 36},
		{"explicit fine on phone", []Option{Environment(core.Environment{OS: core.OSWeb, Idiom: core.IdiomPhone, Pointer: core.PointerFine})}, 28},
		{"explicit coarse on desktop", []Option{Environment(core.Environment{OS: core.OSWindows, Idiom: core.IdiomDesktop, Pointer: core.PointerCoarse})}, 44},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := Start(t, root, tc.opts...)
			if got := app.Find("id:target").Frame.Size.Height; got != tc.height {
				t.Fatalf("target height = %v, want %v", got, tc.height)
			}
			if got := app.Find("id:plain").Frame.Size.Height; got != 36 {
				t.Fatalf("plain height = %v, want 36 (core must not alter size by idiom)", got)
			}
		})
	}

	// Dynamic environment switching updates layout, appearance composition and hover state styles.
	app := Start(t, root, Idiom(core.IdiomDesktop))
	if got := app.Find("id:target").Frame.Size.Height; got != 28 {
		t.Fatalf("initial desktop height = %v, want 28", got)
	}
	app.Hover("id:target")
	if got := app.Find("id:target").Style.Opacity; got != 0.5 {
		t.Fatalf("desktop hover opacity = %v, want 0.5", got)
	}
	app.HoverOut()

	app.SetEnvironment(core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone, Dark: true})
	target := app.Find("id:target")
	if target.Frame.Size.Height != 44 {
		t.Fatalf("phone height = %v, want 44", target.Frame.Size.Height)
	}
	if target.Style.Background == core.RGB(0xffffff) || target.Style.Background == 0 {
		t.Fatalf("dark phone background = %#x, want zinc-900", target.Style.Background)
	}
	app.Hover("id:target")
	if got := app.Find("id:target").Style.Opacity; got != 1 {
		t.Fatalf("coarse hover opacity = %v, want 1 (pointer-fine:hover must not apply)", got)
	}
	app.HoverOut()

	app.SetEnvironment(core.Environment{OS: core.OSIOS, Idiom: core.IdiomTV})
	if got := app.Find("id:target").Frame.Size.Height; got != 36 {
		t.Fatalf("tv height = %v, want 36", got)
	}
}
