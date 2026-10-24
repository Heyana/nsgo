package main

import (
	"fmt"

	"github.com/oneo-me/nsgo/core"
)

// galleryKey gives sections the shell's state.
var galleryKey = core.NewKey[*gallery]("gallery", nil)

// windowPage shows the window the gallery asks for: a material behind
// the sidebar and its own title row.
func windowPage(cx *core.Scope, t *theme) core.Node {
	env := cx.Env()
	return core.Show{When: func() bool { return env.Idiom() == core.IdiomDesktop },
		Else: func(*core.Scope) core.Node {
			return t.demo("Desktop only", "Title bars and materials belong to desktop windows; phones, tablets and the web have neither.",
				t.note("Run the gallery on macOS, Windows or Linux to see them."))
		},
		Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
				t.demo("Background material", "The gallery asks for WindowMaterialSidebar: macOS draws its sidebar material, and Windows 11 draws Mica, through the translucent sidebar beside you.",
					t.live(func() string {
						if env.WindowMaterial() != core.WindowMaterialNone {
							return "This window has a material behind its views."
						}
						return "This window is opaque: this platform or Windows version has no window materials."
					}),
				),
				t.demo("Title bar", "The gallery hides the system's title bar with SetTitleBar: the title row floats over the content as a WindowDrag handle clear of the window controls, and Linux draws those controls itself.",
					flow(
						t.primary("Hide title bar", func() { cx.SetTitleBar(core.TitleBarHidden, headerHeight) }),
						t.plain("Show title bar", func() { cx.SetTitleBar(core.TitleBarDefault, 0) }),
					),
					t.live(func() string {
						tb := env.TitleBar()
						if tb == (core.Edges{}) {
							return "The system draws the title bar."
						}
						return fmt.Sprintf("Hidden: %g pt tall, controls keep %g pt on the left and %g pt on the right.", tb.Top, tb.Left, tb.Right)
					}),
				),
			}}
		},
	}
}
