package ui

import (
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/base/locales"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

const (
	emojiPickerWidth  core.Points = 320
	emojiPickerHeight core.Points = 360
	emojiCellSize     core.Points = 34
	emojiCategorySize core.Points = 28
	emojiSwatchSize   core.Points = 16
)

// EmojiPicker is a styled emoji picker: a search field matching every Unicode
// emoji in the app's languages, a category icon bar, a virtualized emoji grid,
// a highlighted emoji preview and a skin tone selector, 320 × 360 pt by
// default. Behavior, search and accessibility come from base.EmojiPicker, and
// all default words come from base/locales.
type EmojiPicker struct {
	Query           func() *string
	DefaultQuery    string
	OnQueryChange   func(query string)
	NoSearch        bool
	Placeholder     string
	PlaceholderFunc func() string
	SearchLabel     string
	SearchLabelFunc func() string
	AutoFocus       bool
	InputSource     string
	KeepInputSource bool

	Category         func() base.EmojiCategory
	DefaultCategory  base.EmojiCategory
	OnCategoryChange func(base.EmojiCategory)
	Categories       []base.EmojiCategory
	NoCategories     bool

	SkinTone         func() base.EmojiSkinTone
	DefaultSkinTone  base.EmojiSkinTone
	OnSkinToneChange func(base.EmojiSkinTone)
	NoSkinTone       bool
	// NoPreview hides the highlighted emoji preview in the footer.
	NoPreview bool

	// Recent supplies recently used emoji strings (e.g. "🔥", "👍").
	Recent func() []string

	// Languages overrides the app's languages searched for emoji names and
	// keywords.
	Languages     []string
	LanguagesFunc func() []string

	// Columns is how many emoji sit in each row; 8 when <= 0.
	Columns int

	Disabled          func() bool
	Invalid           func() bool
	OnSelect          func(base.Emoji)
	OnHighlightChange func(*base.Emoji)
	OnCancel          func()
	Ref               *base.EmojiPickerRef

	Label     string
	LabelFunc func() string
	TestID    string
	Style     core.Style
	StyleFunc func() core.Style
	OnLayout  func(core.Bounds)
	Tooltip   core.Node
}

// Render implements core.Node.
func (p EmojiPicker) Render(cx *core.Scope) core.Node {
	colors := ColorsOf(cx)
	ref := p.Ref
	if ref == nil {
		ref = new(base.EmojiPickerRef)
	}

	clearBtn := core.Show{
		When: func() bool { return ref.Query() != "" && !isDisabled(p.Disabled) },
		Then: func(cx *core.Scope) core.Node {
			return base.Button{
				LabelFunc: func() string { return locales.ListClear(cx) },
				Style: core.Style{
					Width:          core.Pt(20),
					Height:         core.Pt(20),
					AlignItems:     core.AlignCenter,
					JustifyContent: core.JustifyCenter,
					CornerRadius:   RadiusControl - 2,
					FlexShrink:     0,
				},
				StyleFunc: func() core.Style {
					return core.Style{
						Hover:        &core.Style{Background: colors.Hover()},
						Active:       &core.Style{Background: colors.Pressed()},
						FocusVisible: FocusRing(cx, -1),
					}
				},
				OnPress:  func(core.PressEvent) { ref.SetQuery("") },
				Children: []core.Node{Icon(lucide.X, 13, colors.Muted)},
			}
		},
	}

	b := base.EmojiPicker{
		Query:             p.Query,
		DefaultQuery:      p.DefaultQuery,
		OnQueryChange:     p.OnQueryChange,
		NoSearch:          p.NoSearch,
		Placeholder:       p.Placeholder,
		PlaceholderFunc:   p.PlaceholderFunc,
		SearchLabel:       p.SearchLabel,
		SearchLabelFunc:   p.SearchLabelFunc,
		AutoFocus:         p.AutoFocus,
		InputSource:       p.InputSource,
		KeepInputSource:   p.KeepInputSource,
		Category:          p.Category,
		DefaultCategory:   p.DefaultCategory,
		OnCategoryChange:  p.OnCategoryChange,
		Categories:        p.Categories,
		NoCategories:      p.NoCategories,
		SkinTone:          p.SkinTone,
		DefaultSkinTone:   p.DefaultSkinTone,
		OnSkinToneChange:  p.OnSkinToneChange,
		NoSkinTone:        p.NoSkinTone,
		Recent:            p.Recent,
		Languages:         p.Languages,
		LanguagesFunc:     p.LanguagesFunc,
		Columns:           p.Columns,
		Disabled:          p.Disabled,
		Invalid:           p.Invalid,
		OnSelect:          p.OnSelect,
		OnHighlightChange: p.OnHighlightChange,
		OnCancel:          p.OnCancel,
		Ref:               ref,
		Label:             p.Label,
		LabelFunc:         p.LabelFunc,
		TestID:            p.TestID,
		OnLayout:          p.OnLayout,
		Tooltip:           p.Tooltip,

		Style: core.Style{
			Width:        core.Pt(emojiPickerWidth),
			Height:       core.Pt(emojiPickerHeight),
			MaxWidth:     core.Pct(100),
			CornerRadius: RadiusCard,
			BorderWidth:  1,
			Overflow:     core.Hidden,
			Disabled:     Dimmed(),
		}.Merge(p.Style),
		StyleFunc: func() core.Style {
			s := core.Style{
				Background:   colors.Surface(),
				BorderColor:  colors.Border(),
				FocusVisible: FocusRing(cx, 1),
			}
			if p.StyleFunc != nil {
				s = s.Merge(p.StyleFunc())
			}
			return s
		},

		SearchBarStyle: core.Style{
			Direction:    core.Row,
			AlignItems:   core.AlignCenter,
			Gap:          6,
			Height:       core.Pt(ButtonHeight + 4),
			Margin:       core.Edges{Top: 8, Right: 8, Bottom: 4, Left: 8},
			Padding:      core.Symmetric(0, 8),
			CornerRadius: RadiusControl,
			BorderWidth:  1,
			FlexShrink:   0,
			Transition:   quick,
		},
		SearchBarStyleFunc: func() core.Style {
			return core.Style{
				Background:  colors.Field(),
				BorderColor: colors.FieldBorder(),
				Hover:       &core.Style{BorderColor: Mix(colors.FieldBorder(), colors.Text(), 0.25)},
				FocusWithin: &core.Style{BorderColor: colors.Accent()},
			}
		},
		SearchPrefix: Icon(lucide.Search, 14, colors.Muted),
		SearchSuffix: clearBtn,
		SearchField: func(in core.Input) core.Node {
			in.Style = TypeBody.Merge(core.Style{
				FlexGrow:    1,
				FlexShrink:  1,
				MinWidth:    core.Pt(0),
				Padding:     core.All(0),
				BorderWidth: 0,
				Background:  core.Transparent,
			})
			in.StyleFunc = func() core.Style {
				return core.Style{
					Color:            colors.Text(),
					PlaceholderColor: colors.Faint(),
					CaretColor:       colors.Accent(),
					SelectionColor:   colors.Now().Selection,
				}
			}
			return in
		},

		CategoriesStyle: core.Style{
			Padding:    core.Symmetric(4, 8),
			Borders:    core.Edges{Bottom: 1},
			FlexShrink: 0,
		},
		CategoriesStyleFunc: func() core.Style {
			return core.Style{BorderColors: core.EdgeColors{Bottom: colors.Line()}}
		},
		CategoryStyle: core.Style{
			FlexGrow:       1,
			Height:         core.Pt(emojiCategorySize),
			AlignItems:     core.AlignCenter,
			JustifyContent: core.JustifyCenter,
			CornerRadius:   RadiusControl - 2,
		},
		CategoryStyleFunc: func() core.Style {
			return core.Style{
				Color:        colors.Muted(),
				Hover:        &core.Style{Background: colors.Hover(), Color: colors.Text()},
				Active:       &core.Style{Background: colors.Pressed()},
				Selected:     &core.Style{Background: Alpha(colors.Accent(), 0.14), Color: colors.Accent()},
				FocusVisible: FocusRing(cx, -1),
			}
		},
		RenderCategory: func(cat base.EmojiCategory, _ string) core.Node {
			return core.Icon{SVG: emojiCategoryIcon(cat)(), Style: core.Style{Width: core.Pt(15), Height: core.Pt(15)}}
		},

		ListStyle: core.Style{
			Padding: core.Edges{Top: 4, Right: 8, Bottom: 8, Left: 8},
		},
		SectionStyle: core.Style{
			Padding: core.Edges{Top: 8, Right: 4, Bottom: 4, Left: 4},
		},
		RenderSection: func(_ base.EmojiCategory, label string) core.Node {
			return core.Text{
				Content:   label,
				MaxLines:  1,
				Style:     TypeCaption.Merge(core.Style{FontWeight: Medium}),
				StyleFunc: Colored(colors.Muted),
			}
		},
		RowStyle: core.Style{
			Gap:    2,
			Margin: core.Edges{Bottom: 2},
		},
		ItemStyle: core.Style{
			Height:         core.Pt(emojiCellSize),
			AlignItems:     core.AlignCenter,
			JustifyContent: core.JustifyCenter,
			CornerRadius:   RadiusControl - 2,
		},
		ItemStyleFunc: func() core.Style {
			return core.Style{
				Selected: &core.Style{Background: Alpha(colors.Accent(), 0.14)},
			}
		},
		RenderItem: func(e base.Emoji) core.Node {
			return core.Text{
				Content: e.Emoji,
				Style:   core.Style{FontSize: 20, LineHeight: 24, TextAlign: core.TextAlignCenter},
			}
		},

		Empty: core.View{
			Style: core.Style{
				FlexGrow:       1,
				MinHeight:      core.Pt(0),
				Padding:        core.All(16),
				AlignItems:     core.AlignCenter,
				JustifyContent: core.JustifyCenter,
			},
			Children: []core.Node{
				core.Text{
					LiveRegion:  core.LiveRegionPolite,
					ContentFunc: func() string { return locales.EmojiNoMatches(cx) },
					Style:       TypeBody.Merge(core.Style{TextAlign: core.TextAlignCenter}),
					StyleFunc:   Colored(colors.Muted),
				},
			},
		},

		FooterBarStyle: core.Style{
			Direction:  core.Row,
			AlignItems: core.AlignCenter,
			Gap:        8,
			Padding:    core.Symmetric(6, 10),
			Borders:    core.Edges{Top: 1},
			FlexShrink: 0,
		},
		FooterBarStyleFunc: func() core.Style {
			return core.Style{BorderColors: core.EdgeColors{Top: colors.Line()}}
		},
		PreviewStyle: core.Style{
			Direction:  core.Row,
			AlignItems: core.AlignCenter,
			FlexGrow:   1,
			FlexShrink: 1,
			MinWidth:   core.Pt(0),
		},
		SkinToneBarStyle: core.Style{
			Direction:  core.Row,
			AlignItems: core.AlignCenter,
			Gap:        4,
			FlexShrink: 0,
		},
		SkinToneStyle: core.Style{
			Width:          core.Pt(emojiSwatchSize + 4),
			Height:         core.Pt(emojiSwatchSize + 4),
			AlignItems:     core.AlignCenter,
			JustifyContent: core.JustifyCenter,
			CornerRadius:   (emojiSwatchSize + 4) / 2,
			BorderWidth:    1.5,
			BorderColor:    core.Transparent,
		},
		SkinToneStyleFunc: func() core.Style {
			return core.Style{
				Hover:        &core.Style{Background: colors.Hover()},
				Selected:     &core.Style{BorderColor: colors.Accent()},
				FocusVisible: FocusRing(cx, 1),
			}
		},
		RenderSkinTone: func(tone base.EmojiSkinTone, _ string) core.Node {
			return core.View{
				Style: core.Style{
					Width:        core.Pt(emojiSwatchSize - 4),
					Height:       core.Pt(emojiSwatchSize - 4),
					CornerRadius: (emojiSwatchSize - 4) / 2,
					Background:   emojiSkinToneColor(tone),
				},
			}
		},
	}

	if !p.NoPreview {
		b.RenderPreview = func(e *base.Emoji) core.Node {
			if e == nil {
				return core.View{Style: core.Style{Height: core.Pt(22)}}
			}
			return core.View{
				Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)},
				Children: []core.Node{
					core.Text{Content: e.Emoji, Style: core.Style{FontSize: 18, LineHeight: 22, FlexShrink: 0}},
					core.Text{Content: e.Name, MaxLines: 1, Style: TypeCaption.Merge(core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}), StyleFunc: Colored(colors.Muted)},
				},
			}
		}
	}

	return b
}

func isDisabled(fn func() bool) bool { return fn != nil && fn() }

func emojiCategoryIcon(cat base.EmojiCategory) func() *core.SVG {
	switch cat {
	case base.EmojiRecent:
		return lucide.Clock
	case base.EmojiSmileys:
		return lucide.FaceSlightlySmiling
	case base.EmojiPeople:
		return lucide.Hand
	case base.EmojiAnimals:
		return lucide.PawPrint
	case base.EmojiFood:
		return lucide.Utensils
	case base.EmojiTravel:
		return lucide.Plane
	case base.EmojiActivities:
		return lucide.Trophy
	case base.EmojiObjects:
		return lucide.Lightbulb
	case base.EmojiSymbols:
		return lucide.Heart
	case base.EmojiFlags:
		return lucide.Flag
	default:
		return lucide.FaceSlightlySmiling
	}
}

func emojiSkinToneColor(tone base.EmojiSkinTone) core.Color {
	switch tone {
	case base.SkinToneLight:
		return core.RGB(0xfadcbc)
	case base.SkinToneMediumLight:
		return core.RGB(0xe0bb95)
	case base.SkinToneMedium:
		return core.RGB(0xbf8f68)
	case base.SkinToneMediumDark:
		return core.RGB(0x9b643d)
	case base.SkinToneDark:
		return core.RGB(0x594539)
	default:
		return core.RGB(0xf7d046)
	}
}
