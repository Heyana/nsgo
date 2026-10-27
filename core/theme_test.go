package core

import (
	"testing"

	"github.com/oneo-me/nsgo/internal/driver"
)

func TestThemeSubtrees(t *testing.T) {
	side := DefaultTheme()
	side.Light.Background, side.Dark.Background = RGB(0x111111), RGB(0x222222)
	var appBg, sideBg, lightBg Color
	var forced *Signal[Appearance]
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		forced = NewSignal(cx, AppearanceDark)
		return View{TestID: "app", StyleFunc: func() Style { appBg = cx.Palette().Background; return Style{} },
			Children: []Node{Func(func(cx *Scope) Node {
				cx.SetTheme(side)
				return View{TestID: "side", StyleFunc: func() Style { sideBg = cx.Palette().Background; return Style{} },
					Children: []Node{Func(func(cx *Scope) Node {
						cx.SetSubtreeAppearance(forced.GetUntracked())
						cx.Effect(func() { cx.SetSubtreeAppearance(forced.Get()) })
						return View{TestID: "forced", StyleFunc: func() Style { lightBg = cx.Palette().Background; return Style{} }}
					})}}
			})}}
	})
	h.commit()
	def := DefaultTheme()
	if appBg != def.Light.Background || sideBg != RGB(0x111111) || lightBg != RGB(0x222222) {
		t.Fatalf("backgrounds = %v %v %v", appBg, sideBg, lightBg)
	}
	if a := h.find("forced").appearance; a != AppearanceDark {
		t.Fatalf("forced view's appearance = %v", a)
	}

	// The app turns dark; the forced subtree goes light when asked.
	h.tree.SetEnvironment(Environment{Dark: true})
	forced.Set(AppearanceLight)
	h.drain()
	b := h.commit()
	if appBg != def.Dark.Background || sideBg != RGB(0x222222) || lightBg != RGB(0x111111) {
		t.Fatalf("dark backgrounds = %v %v %v", appBg, sideBg, lightBg)
	}
	id := h.find("forced").id
	sent := false
	for _, it := range b.Items {
		if it.Kind == driver.ItemViewProps && it.ID == id {
			sent = it.ViewProps.Appearance == AppearanceLight
		}
	}
	if !sent || h.find("side").appearance != AppearanceSystem {
		t.Fatalf("props sent = %v, side = %v", sent, h.find("side").appearance)
	}
}

func TestParseTheme(t *testing.T) {
	th, err := ParseTheme([]byte("radius: 12\nlight:\n  accent: \"#112233\"\n  inset: \"#445566\"\ndark:\n  text: white\n"))
	if err != nil {
		t.Fatal(err)
	}
	if th.Radius != 12 || th.Light.Accent != RGB(0x112233) || th.Light.Colors["inset"] != RGB(0x445566) ||
		th.Dark.Text != RGB(0xffffff) || th.Dark.Accent != DefaultTheme().Dark.Accent {
		t.Fatalf("theme = %+v", th)
	}
	if _, err := ParseTheme([]byte(`{"light": {"accent": "nope"}}`)); err == nil {
		t.Fatal("a bad color parsed")
	}
}
