package core

import (
	"fmt"
	"maps"

	"gopkg.in/yaml.v3"
)

// Palette is a theme's colors in one appearance. Its roles are Onevra's;
// Colors holds the ones a component library or an app adds, by name.
type Palette struct {
	// Background is the page; Surface a card or control on it, under
	// the pointer and held down.
	Background, Surface, SurfaceHover, SurfacePressed Color
	Border                                            Color
	Text, TextMuted                                   Color
	// Accent is the accent fill, AccentText what is drawn on it.
	Accent, AccentHover, AccentPressed, AccentText Color
	Danger, Warning, Success                       Color
	// Selection is selected text's, Focus a focus ring's and Scrollbar a
	// scroll indicator's.
	Selection, Focus, Scrollbar Color
	// Colors are roles by name past the ones above, such as "inset".
	Colors map[string]Color
}

// paletteRoles names the fields of Palette, as theme files and host
// tokens do.
var paletteRoles = map[string]func(*Palette) *Color{
	"background":     func(p *Palette) *Color { return &p.Background },
	"surface":        func(p *Palette) *Color { return &p.Surface },
	"surfaceHover":   func(p *Palette) *Color { return &p.SurfaceHover },
	"surfacePressed": func(p *Palette) *Color { return &p.SurfacePressed },
	"border":         func(p *Palette) *Color { return &p.Border },
	"text":           func(p *Palette) *Color { return &p.Text },
	"textMuted":      func(p *Palette) *Color { return &p.TextMuted },
	"accent":         func(p *Palette) *Color { return &p.Accent },
	"accentHover":    func(p *Palette) *Color { return &p.AccentHover },
	"accentPressed":  func(p *Palette) *Color { return &p.AccentPressed },
	"accentText":     func(p *Palette) *Color { return &p.AccentText },
	"danger":         func(p *Palette) *Color { return &p.Danger },
	"warning":        func(p *Palette) *Color { return &p.Warning },
	"success":        func(p *Palette) *Color { return &p.Success },
	"selection":      func(p *Palette) *Color { return &p.Selection },
	"focus":          func(p *Palette) *Color { return &p.Focus },
	"scrollbar":      func(p *Palette) *Color { return &p.Scrollbar },
}

// Color returns the role named name: a field's name in lower camel case
// ("surfaceHover") or one of Colors, and whether the palette has it.
func (p *Palette) Color(name string) (Color, bool) {
	if f := paletteRoles[name]; f != nil {
		return *f(p), true
	}
	c, ok := p.Colors[name]
	return c, ok
}

// SetColor sets the role named name, as Color names it.
func (p *Palette) SetColor(name string, c Color) {
	if f := paletteRoles[name]; f != nil {
		*f(p) = c
		return
	}
	if p.Colors == nil {
		p.Colors = map[string]Color{}
	}
	p.Colors[name] = c
}

// clone returns p with a Colors of its own.
func (p Palette) clone() Palette {
	p.Colors = maps.Clone(p.Colors)
	return p
}

// Theme is a look in both appearances: a palette for each, the corner
// radius components start from and font families, empty for the
// system's. Scope.SetTheme gives it to a subtree; treat a theme given as
// read-only.
type Theme struct {
	Light, Dark Palette
	Radius      Points
	Sans, Mono  string
}

// Clone returns a copy of t that shares nothing with it.
func (t *Theme) Clone() *Theme {
	c := *t
	c.Light, c.Dark = t.Light.clone(), t.Dark.clone()
	return &c
}

// DefaultTheme returns Onevra's theme, which a tree shows until the app
// sets one: warm neutrals with a terracotta accent, lifted in dark.
func DefaultTheme() *Theme {
	text, dark := RGB(0x33332f), RGB(0xeeeae2)
	return &Theme{
		Light: Palette{
			Background: RGB(0xfcfbf9), Surface: RGB(0xfefefd), SurfaceHover: RGB(0xeceae4), SurfacePressed: RGB(0xd9d7d2),
			Border: RGB(0xdeded5), Text: text, TextMuted: RGB(0x72726a),
			Accent: RGB(0xbd481b), AccentHover: RGB(0xa63f18), AccentPressed: RGB(0x8e3614), AccentText: RGB(0xffffff),
			Danger: RGB(0xb4444c), Warning: RGB(0xb45309), Success: RGB(0x059669),
			Selection: RGBA(0xe6652b, 0.22), Focus: RGBA(0xb94716, 0.55), Scrollbar: text.Faded(0.30),
		},
		Dark: Palette{
			Background: RGB(0x211f1c), Surface: RGB(0x2b2925), SurfaceHover: RGB(0x36332e), SurfacePressed: RGB(0x403c36),
			Border: RGB(0x504b43), Text: dark, TextMuted: RGB(0xbdb6aa),
			Accent: RGB(0xed986e), AccentHover: RGB(0xefa47f), AccentPressed: RGB(0xd18661), AccentText: RGB(0x291b14),
			Danger: RGB(0xe58f95), Warning: RGB(0xe6b567), Success: RGB(0x79bea1),
			Selection: RGBA(0xed986e, 0.24), Focus: RGBA(0xed986e, 0.8), Scrollbar: dark.Faded(0.35),
		},
		Radius: 8,
	}
}

// ParseTheme reads a theme file over DefaultTheme: radius, sans and
// mono, and light and dark maps of role names, as Palette.Color names
// them, to CSS colors. YAML reads JSON as well.
//
//	radius: 8
//	light:
//	  accent: "#bd481b"
//	  inset: "#f2efea"
//	dark:
//	  accent: "#ed986e"
func ParseTheme(data []byte) (*Theme, error) {
	var f struct {
		Radius *float32          `yaml:"radius"`
		Sans   string            `yaml:"sans"`
		Mono   string            `yaml:"mono"`
		Light  map[string]string `yaml:"light"`
		Dark   map[string]string `yaml:"dark"`
	}
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("core: theme: %w", err)
	}
	t := DefaultTheme()
	if f.Radius != nil {
		t.Radius = Points(*f.Radius)
	}
	if f.Sans != "" {
		t.Sans = f.Sans
	}
	if f.Mono != "" {
		t.Mono = f.Mono
	}
	for _, m := range []struct {
		p      *Palette
		colors map[string]string
		name   string
	}{{&t.Light, f.Light, "light"}, {&t.Dark, f.Dark, "dark"}} {
		for name, v := range m.colors {
			c, err := ParseColor(v)
			if err != nil {
				return nil, fmt.Errorf("core: theme: %s.%s: %w", m.name, name, err)
			}
			m.p.SetColor(name, c)
		}
	}
	return t, nil
}

// scopeTheme and scopeAppearance key the scope values SetTheme and
// SetSubtreeAppearance keep.
type (
	scopeTheme      struct{}
	scopeAppearance struct{}
)

// SetTheme gives cx and everything it builds from now on theme t; a
// nil t takes back what an earlier call gave, so the theme above shows.
// Called at the app's root it is the app's theme; a deeper call wins
// for its subtree, so a sidebar or a panel can take another.
func (cx *Scope) SetTheme(t *Theme) {
	o := cx
	if s, ok := o.values[scopeTheme{}].(*Signal[*Theme]); ok {
		s.Set(t)
		return
	}
	if o.values == nil {
		o.values = map[any]any{}
	}
	o.values[scopeTheme{}] = NewSignal(o, t)
}

// SetSubtreeAppearance shows cx and everything it builds from now on
// light or dark whatever the appearance above it, the platform's views
// in it too where the platform scopes appearance to a view;
// AppearanceSystem follows the one above again. Scope.SetAppearance
// forces the app's.
func (cx *Scope) SetSubtreeAppearance(a Appearance) {
	o := cx
	if s, ok := o.values[scopeAppearance{}].(*Signal[Appearance]); ok {
		s.Set(a)
		return
	}
	if o.values == nil {
		o.values = map[any]any{}
	}
	o.values[scopeAppearance{}] = NewSignal(o, a)
	o.rt.subtreeAppearance = true
}

// Theme returns the theme of cx's subtree: the nearest SetTheme's, else
// DefaultTheme. Host tokens reach its colors through Palette. A binding
// that reads it runs again when it changes.
func (cx *Scope) Theme() *Theme {
	for s := cx.current(); s != nil; s = s.parent {
		if sig, ok := s.values[scopeTheme{}].(*Signal[*Theme]); ok {
			if t := sig.Get(); t != nil {
				return t
			}
		}
	}
	return cx.rt.defaultTheme()
}

// Dark reports whether cx's subtree shows dark: as the nearest
// SetSubtreeAppearance forces it, else as the app shows (Env.Dark).
func (cx *Scope) Dark() bool {
	switch subtreeAppearance(cx.current()) {
	case AppearanceLight:
		return false
	case AppearanceDark:
		return true
	}
	return cx.Env().Dark()
}

// subtreeAppearance is the appearance the nearest SetSubtreeAppearance
// above o forces, tracked, or AppearanceSystem.
func subtreeAppearance(o *Scope) Appearance {
	if !o.rt.subtreeAppearance {
		return AppearanceSystem
	}
	for s := o; s != nil; s = s.parent {
		if sig, ok := s.values[scopeAppearance{}].(*Signal[Appearance]); ok {
			if a := sig.Get(); a != AppearanceSystem {
				return a
			}
		}
	}
	return AppearanceSystem
}

// Palette returns the colors of cx's subtree: its theme's palette in the
// appearance it shows, with the host's tokens over it. A binding that
// reads it runs again when any of them changes.
func (cx *Scope) Palette() Palette {
	t, dark := cx.Theme(), cx.Dark()
	p := t.Light
	if dark {
		p = t.Dark
	}
	if tok := cx.Env().tokens.Get(); tok.any() {
		p = p.clone()
		tok.apply(&p, dark)
	}
	return p
}

func (rt *runtime) defaultTheme() *Theme {
	if rt.theme == nil {
		rt.theme = DefaultTheme()
	}
	return rt.theme
}

// hostColors are a host's Tokens read as colors.
type hostColors struct {
	values, dark map[string]Color
}

func (h hostColors) any() bool { return len(h.values) > 0 || len(h.dark) > 0 }

func (h hostColors) apply(p *Palette, dark bool) {
	for name, c := range h.values {
		p.SetColor(name, c)
	}
	if dark {
		for name, c := range h.dark {
			p.SetColor(name, c)
		}
	}
}

// parseTokens reads t's colors; a value that is no color is dropped.
func parseTokens(t Tokens) hostColors {
	read := func(m map[string]string) map[string]Color {
		var out map[string]Color
		for name, v := range m {
			c, err := ParseColor(v)
			if err != nil {
				continue
			}
			if out == nil {
				out = map[string]Color{}
			}
			out[name] = c
		}
		return out
	}
	return hostColors{values: read(t.Values), dark: read(t.Dark)}
}
