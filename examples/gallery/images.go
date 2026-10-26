package main

import (
	_ "embed"
	"fmt"
	"slices"

	"github.com/oneo-me/nsgo/core"
)

//go:embed assets/bounce.gif
var bounceGIF []byte

//go:embed assets/spinner.webp
var spinnerWebP []byte

//go:embed assets/mark.svg
var markSVG []byte

//go:embed assets/star.svg
var starSVG []byte

// Fixed photos, in color, large enough to stay sharp at 3x.
const widePhoto = "https://picsum.photos/id/1015/1200/600"

func photoURL(id, w, h int) string { return fmt.Sprintf("https://picsum.photos/id/%d/%d/%d", id, w, h) }

// remoteSVG is the W3C's SVG logo.
const remoteSVG = "https://www.w3.org/Icons/SVG/svg-logo.svg"

// logoSource is the gallery's logo at 64 points.
func logoSource() core.ImageSource {
	return core.ImageSource{Data: logoPNG, Size: core.Size{Width: 64, Height: 64}}
}

// imageFrame is every photo's frame: rounded, a hairline border, a raised
// fill while it loads.
func imageFrame(s core.Style) core.Style {
	return core.Style{CornerRadius: radiusControl, BorderCurve: core.BorderCurveContinuous, BorderWidth: 1, BorderColor: colors.border, Background: colors.raised}.Merge(s)
}

// imageNote is a quiet label under a tile, code for API values.
func imageNote(s string, isCode bool) core.Node {
	st := core.Style{FontSize: 13, Color: colors.muted, TextAlign: core.TextAlignCenter}
	if isCode {
		st.FontFamily = mono
	}
	return core.Text{Content: s, MaxLines: 1, Style: st}
}

// imageTile is something centered on a raised square, a note below.
func imageTile(noteText string, isCode bool, height core.Points, child core.Node) core.Node {
	if height == 0 {
		height = 76
	}
	return core.View{Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), Gap: 8}, Children: []core.Node{
		core.View{Style: fin.raised.over(core.Style{Height: core.Pt(height), CornerRadius: radiusControl, BorderCurve: core.BorderCurveContinuous, Background: colors.raised,
			BorderWidth: 1, BorderColor: colors.border, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Overflow: core.Hidden}), Children: []core.Node{child}},
		imageNote(noteText, isCode),
	}}
}

func resizeModes(cx *core.Scope) core.Node {
	modes := []struct {
		name string
		mode core.ResizeMode
	}{{"Cover", core.Cover}, {"Contain", core.Contain}, {"Stretch", core.Stretch}, {"Center", core.Center}}
	var rows []core.Node
	for i := 0; i < len(modes); i += 2 {
		var cells []core.Node
		for _, m := range modes[i : i+2] {
			cells = append(cells, core.View{Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), Gap: 8}, Children: []core.Node{
				core.Image{Source: core.Src(widePhoto), ResizeMode: m.mode, Label: m.name, Style: imageFrame(core.Style{Height: core.Pt(140)})},
				imageNote(m.name, true),
			}})
		}
		rows = append(rows, core.View{Style: core.Style{Direction: core.Row, Gap: 12}, Children: cells})
	}
	return card(cardProps{icon: iconImage, title: "Resize modes", subtitle: "A 2:1 image in square frames"}, rows...)
}

func imageCorners(cx *core.Scope) core.Node {
	sq := imageFrame(core.Style{Width: core.Pt(88), Height: core.Pt(88)})
	item := func(s core.Style, name string) core.Node {
		return core.View{Style: core.Style{AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{core.Image{Source: core.Src(widePhoto), Style: sq.Merge(s)}, imageNote(name, false)}}
	}
	return card(cardProps{icon: iconStyles, title: "Corners and borders", subtitle: "Radii per corner, borders on the image"},
		core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifySpaceBetween}, Children: []core.Node{
			item(core.Style{CornerRadius: 44}, "Circle"),
			item(core.Style{CornerRadius: 18, BorderWidth: 2, BorderColor: colors.accent}, "Border"),
			item(core.Style{CornerRadii: core.Corners{TopLeft: 40, BottomRight: 40}}, "Two corners"),
		}})
}

func assetsCard(cx *core.Scope) core.Node {
	tile := func(name string, tint core.Color) core.Node {
		return imageTile(name, false, 0, core.Image{Source: logoSource(), Style: core.Style{TintColor: tint, Width: core.Pt(56), Height: core.Pt(56)}})
	}
	return card(cardProps{icon: iconStar, title: "Embedded images and tint", subtitle: "logo.png from go:embed, tinted natively"},
		core.View{Style: core.Style{Direction: core.Row, Gap: 12}, Children: []core.Node{
			tile("Original", 0), tile("Accent", colors.accent), tile("Info", colors.info), tile("Danger", colors.danger),
		}})
}

// svgCard is SVG images: a one-color star tinted at three sizes and as
// drawn, a two-color logo in two resize modes, and an SVG from a URL.
func svgCard(cx *core.Scope) core.Node {
	star := core.ImageSource{Data: starSVG, Size: core.Size{Width: 24, Height: 24}}
	mark := core.ImageSource{Data: markSVG, Size: core.Size{Width: 120, Height: 60}}
	var sizes []core.Node
	for _, s := range []core.Points{18, 36, 72} {
		sizes = append(sizes, imageTile(fmt.Sprintf("%.0f pt", s), false, 96, core.Image{Source: star, TestID: fmt.Sprintf("svg-star-%.0f", s),
			Style: core.Style{Width: core.Pt(s), Height: core.Pt(s), TintColor: colors.accent}}))
	}
	sizes = append(sizes, imageTile("As drawn", false, 96, core.Image{Source: star, TestID: "svg-star-plain", Style: core.Style{Width: core.Pt(36), Height: core.Pt(36)}}))
	cell := func(child core.Node, n string, isCode bool) core.Node {
		return core.View{Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), Gap: 8}, Children: []core.Node{child, imageNote(n, isCode)}}
	}
	return card(cardProps{icon: iconStar, title: "SVG images", subtitle: "star.svg, mark.svg and a remote SVG, sharp at every size"},
		core.View{Style: core.Style{Direction: core.Row, Gap: 12}, Children: sizes},
		core.View{Style: core.Style{Direction: core.Row, Gap: 12}, Children: []core.Node{
			cell(core.Image{Source: mark, ResizeMode: core.Contain, TestID: "svg-mark-contain", Style: imageFrame(core.Style{Height: core.Pt(96)})}, "Contain", true),
			cell(core.Image{Source: mark, ResizeMode: core.Cover, TestID: "svg-mark-cover", Style: imageFrame(core.Style{Height: core.Pt(96)})}, "Cover", true),
			cell(core.Image{Source: core.Src(remoteSVG), ResizeMode: core.Contain, TestID: "svg-remote", Style: imageFrame(core.Style{Height: core.Pt(96)})}, "Remote", false),
		}},
	)
}

func animatedImages(cx *core.Scope) core.Node {
	gif := core.ImageSource{Data: bounceGIF, Size: core.Size{Width: 72, Height: 72}}
	webp := core.ImageSource{Data: spinnerWebP, Size: core.Size{Width: 72, Height: 72}}
	return card(cardProps{icon: iconAnimation, title: "Animated GIF and WebP", subtitle: "Decoded and played natively"},
		core.View{Style: core.Style{Direction: core.Row, Gap: 12}, Children: []core.Node{
			imageTile("bounce.gif", true, 96, core.Image{Source: gif, TestID: "gif"}),
			imageTile("spinner.webp", true, 96, core.Image{Source: webp, TestID: "webp"}),
			imageTile("Scaled, round", false, 96, core.Image{Source: gif, Style: core.Style{Width: core.Pt(48), Height: core.Pt(48), CornerRadius: 24}}),
		}})
}

// placeholders are network images behind a placeholder, fading in;
// Reload asks for images that are not cached yet.
func placeholders(cx *core.Scope) core.Node {
	seed := core.NewSignal(cx, 1)
	fades := []struct{ id, ms int }{{1043, 0}, {1080, 300}, {1069, 1000}}
	var cells []core.Node
	for _, f := range fades {
		cells = append(cells, core.View{Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), Gap: 8}, Children: []core.Node{
			// A new size per reload: a URL not in the cache yet. A
			// FadeDuration of 0 is the platform's; a nanosecond is none.
			core.Image{SourceFunc: func() core.ImageSource { return core.Src(photoURL(f.id, 600+seed.Get(), 600+seed.Get())) },
				Placeholder: logoSource(), FadeDuration: ms(f.ms) + 1, ResizeMode: core.Cover, Style: imageFrame(core.Style{Height: core.Pt(100)})},
			imageNote(fmt.Sprintf("%d ms", f.ms), false),
		}})
	}
	return card(cardProps{icon: iconPhotos, title: "Placeholder and fade", subtitle: "A placeholder, then a fade",
		right: button(buttonProps{label: "Reload", variant: secondary, small: true}, func() { seed.Update(func(n int) int { return n + 1 }) })},
		core.View{Style: core.Style{Direction: core.Row, Gap: 12}, Children: cells},
		caption("Loads", func() string { return itoa(seed.Get()) }),
	)
}

func blurCard(cx *core.Scope) core.Node {
	var cells []core.Node
	for _, r := range []core.Points{0, 2, 6, 16} {
		n := "None"
		if r > 0 {
			n = fmt.Sprintf("%.0f pt", r)
		}
		cells = append(cells, core.View{Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), Gap: 8}, Children: []core.Node{
			core.Image{Source: core.Src(widePhoto), BlurRadius: r, Style: imageFrame(core.Style{Height: core.Pt(76)})}, imageNote(n, false),
		}})
	}
	return card(cardProps{icon: iconSpark, title: "Blur radius", subtitle: "BlurRadius, applied while decoding"},
		core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: cells})
}

func imageInfo(cx *core.Scope) core.Node {
	big := photoURL(1018, 2400, 1600)
	size, prefetched := core.NewSignal(cx, "—"), core.NewSignal(cx, "—")
	progress := core.NewSignal(cx, 0.0)
	status := core.NewSignal(cx, "not shown")
	show := core.NewSignal(cx, false)
	return card(cardProps{icon: iconSearch, title: "Size, prefetch, progress", subtitle: "ImageSize · PrefetchImage · OnProgress"},
		core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
			cell(button(buttonProps{label: "Size", variant: secondary}, func() {
				cx.ImageSize(core.Src(big), func(px core.Size, err error) {
					if err != nil {
						size.Set(err.Error())
						return
					}
					size.Set(fmt.Sprintf("%.0f×%.0f px", px.Width, px.Height))
				})
			})),
			cell(button(buttonProps{label: "Prefetch", variant: secondary}, func() {
				prefetched.Set("loading…")
				cx.PrefetchImage(core.Src(big), func(ok bool) {
					if ok {
						prefetched.Set("in the cache")
					} else {
						prefetched.Set("failed")
					}
				})
			})),
			cell(core.Dynamic(func(*core.Scope) core.Node {
				if show.Get() {
					return button(buttonProps{label: "Hide", variant: secondary}, func() { show.Set(false) })
				}
				return button(buttonProps{label: "Show", variant: accentButton}, func() { show.Set(true) })
			})),
		}},
		core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
			caption("ImageSize", size.Get),
			caption("PrefetchImage", prefetched.Get),
			caption("OnProgress", func() string { return fmt.Sprintf("%.0f%% · %s", progress.Get()*100, status.Get()) }),
			core.Dynamic(func(*core.Scope) core.Node {
				t := toneAccent
				if progress.Get() >= 1 {
					t = toneUp
				}
				return meter(progress.Get, t, -1)
			}),
		}},
		core.Show{When: show.Get, Else: func(*core.Scope) core.Node { return imageNote("Prefetch, then show: no download, no progress", false) },
			Then: func(*core.Scope) core.Node {
				return appear(0, false, core.Image{Source: core.Src(big), Style: imageFrame(core.Style{Height: core.Pt(170)}),
					OnLoadStart: func() { status.Set("loading") },
					OnProgress: func(loaded, total int64) {
						progress.Set(float64(loaded) / float64(max(1, total)))
					},
					OnLoad: func(core.Size) {
						if progress.GetUntracked() == 0 {
							status.Set("loaded (no download)")
						} else {
							status.Set("loaded")
						}
						progress.Set(1)
					}})
			}},
	)
}

func imageEvents(cx *core.Scope) core.Node {
	events := core.NewSignalFunc(cx, []string(nil), slices.Equal)
	log := func(l string) { events.Update(func(e []string) []string { return append(slices.Clone(e), l) }) }
	loaded, failed := core.NewSignal(cx, "…"), core.NewSignal(cx, "Loading…")
	thumb := imageFrame(core.Style{Width: core.Pt(96), Height: core.Pt(64)})
	return card(cardProps{icon: iconBell, title: "Events", subtitle: "Load start, load, error, load end"},
		core.View{Style: core.Style{Direction: core.Row, Gap: 12, AlignItems: core.AlignCenter}, Children: []core.Node{
			core.Image{Source: core.Src(photoURL(1084, 600, 400)), Style: thumb,
				OnLoadStart: func() { log("OnLoadStart") },
				OnLoad:      func(px core.Size) { log("OnLoad"); loaded.Set(fmt.Sprintf("%.0f×%.0f", px.Width, px.Height)) },
				OnError:     func(err error) { loaded.Set(err.Error()) },
				OnLoadEnd:   func() { log("OnLoadEnd") }},
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 4}, Children: []core.Node{
				text("picsum.photos", 15, regular, colors.text),
				textf(func() string { return "Loaded, " + loaded.Get() + " px" }, 13, regular, colors.up),
			}},
		}},
		core.View{Style: core.Style{Direction: core.Row, Gap: 12, AlignItems: core.AlignCenter, Borders: core.Edges{Top: 1}, BorderColors: core.EdgeColors{Top: colors.line}, Padding: core.Edges{Top: 12}}, Children: []core.Node{
			core.View{Style: thumb.Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, BorderColor: alpha(colors.danger, 0.4)}), Children: []core.Node{
				core.Image{Source: core.Src("https://example.invalid/missing.png"), OnError: func(err error) { failed.Set(err.Error()) },
					Style: thumb.Merge(core.Style{Position: core.Absolute, Left: core.Pt(-1), Top: core.Pt(-1), BorderWidth: 0, Background: transparent})},
				text("×", 20, regular, colors.danger),
			}},
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 4}, Children: []core.Node{
				text("example.invalid", 15, regular, colors.text),
				core.Text{ContentFunc: failed.Get, MaxLines: 3, Style: core.Style{FontSize: 13, Color: colors.danger}},
			}},
		}},
		core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 8, AlignItems: core.AlignCenter}, Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			e := events.Get()
			if len(e) == 0 {
				return imageNote("No load events yet", false)
			}
			var badges []core.Node
			for _, l := range e {
				t := toneNeutral
				if l == "OnLoad" {
					t = toneUp
				}
				badges = append(badges, badge(l, t))
			}
			return core.Group(badges)
		})}},
	)
}

// imagesDemo is core.Image: resize modes, corners, embedded and tinted
// images, SVG, animated GIF and WebP, placeholders and fades, blur, sizes,
// prefetching, progress and the load events.
func imagesDemo(cx *core.Scope) core.Node {
	return demoPage(cx, pageProps{columns: 3}, resizeModes, imageCorners, assetsCard, svgCard, animatedImages, placeholders, blurCard, imageInfo, imageEvents)
}
