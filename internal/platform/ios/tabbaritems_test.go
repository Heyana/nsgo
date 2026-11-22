package ios

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func svgDrawing(t *testing.T, doc string) *driver.Drawing {
	t.Helper()
	svg, err := driver.ParseSVG([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	d := &driver.Drawing{}
	svg.Draw(d, driver.Size{Width: tabBarIconPoints, Height: tabBarIconPoints}, driver.RGB(0xffffff))
	return d
}

func TestResolveTabBarImagePrecedence(t *testing.T) {
	home := svgDrawing(t, `<svg viewBox="0 0 24 24"><path d="M3 12L12 3L21 12" stroke="currentColor" stroke-width="2"/></svg>`)

	t.Run("SF Symbol wins over SVG", func(t *testing.T) {
		svgCalls := 0
		got := resolveTabBarImage(
			tabBarItemSpec{title: "Home", icon: "house", drawing: home},
			func(name string) uintptr {
				if name != "house" {
					t.Fatalf("unexpected symbol %q", name)
				}
				return 101
			},
			func(*driver.Drawing) uintptr {
				svgCalls++
				return 202
			},
		)
		if got != 101 || svgCalls != 0 {
			t.Fatalf("got %d (svgCalls=%d), want SF Symbol 101 with 0 SVG rasterizations", got, svgCalls)
		}
	})

	t.Run("SVG fallback when icon names no SF Symbol", func(t *testing.T) {
		got := resolveTabBarImage(
			tabBarItemSpec{title: "Search", icon: "android:ic_menu_search", drawing: home},
			func(string) uintptr { return 0 },
			func(d *driver.Drawing) uintptr {
				if d != home {
					t.Fatal("wrong drawing passed to svgImage")
				}
				return 202
			},
		)
		if got != 202 {
			t.Fatalf("got %d, want SVG fallback 202", got)
		}
	})

	t.Run("SVG only when icon is empty", func(t *testing.T) {
		sysCalls := 0
		got := resolveTabBarImage(
			tabBarItemSpec{title: "Home", drawing: home},
			func(string) uintptr { sysCalls++; return 101 },
			func(*driver.Drawing) uintptr { return 202 },
		)
		if got != 202 || sysCalls != 0 {
			t.Fatalf("got %d (sysCalls=%d), want SVG 202 with 0 symbol lookups", got, sysCalls)
		}
	})

	t.Run("neither icon nor drawing returns 0", func(t *testing.T) {
		got := resolveTabBarImage(
			tabBarItemSpec{title: "Plain"},
			func(string) uintptr { return 101 },
			func(*driver.Drawing) uintptr { return 202 },
		)
		if got != 0 {
			t.Fatalf("got %d, want 0", got)
		}
	})
}

func TestTabBarIconsEqual(t *testing.T) {
	const dashedDoc = `<svg viewBox="0 0 24 24"><defs><linearGradient id="g"><stop offset="0" stop-color="#fff"/><stop offset="1" stop-color="#888"/></linearGradient></defs><path d="M2 12H22" stroke="url(#g)" stroke-width="2" stroke-dasharray="4 2"/></svg>`
	const circleDoc = `<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="8" fill="currentColor"/></svg>`

	a := parseTabBarItems([]any{
		map[string]any{"title": "Home", "drawing": svgDrawing(t, dashedDoc)},
		map[string]any{"title": "Search", "icon": "magnifyingglass"},
	})
	// Re-drawn identical SVG with a new title must compare equal so titles update in place.
	b := parseTabBarItems([]any{
		map[string]any{"title": "Accueil", "drawing": svgDrawing(t, dashedDoc)},
		map[string]any{"title": "Rechercher", "icon": "magnifyingglass"},
	})
	if !tabBarIconsEqual(a, b) {
		t.Fatal("identical re-drawn SVGs with different titles were not considered equal")
	}
	if !tabBarHasDrawings(a) {
		t.Fatal("tabBarHasDrawings reported false for SVG items")
	}

	// Different SVG operations must trigger a rebuild.
	c := parseTabBarItems([]any{
		map[string]any{"title": "Home", "drawing": svgDrawing(t, circleDoc)},
		map[string]any{"title": "Search", "icon": "magnifyingglass"},
	})
	if tabBarIconsEqual(a, c) {
		t.Fatal("different SVG drawings were considered equal")
	}

	// Removing an SVG must trigger a rebuild.
	d := parseTabBarItems([]any{
		map[string]any{"title": "Home"},
		map[string]any{"title": "Search", "icon": "magnifyingglass"},
	})
	if tabBarIconsEqual(a, d) {
		t.Fatal("removing an SVG was considered equal")
	}
	if tabBarHasDrawings(d) {
		t.Fatal("tabBarHasDrawings reported true when no item has a drawing")
	}
}
