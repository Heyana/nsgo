package main

import (
	"fmt"
	"strings"

	"github.com/oneo-me/nsgo/core"
)

// servicesPage shows what the platform does for the app: the clipboard,
// opening links, the photo picker, toasts and the app's appearance.
func servicesPage(cx *core.Scope, t *theme) core.Node {
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		clipboardDemo(cx, t),
		linksDemo(cx, t),
		pickerDemo(cx, t),
		appearanceDemo(cx, t),
		toastDemo(cx, t),
	}}
}

func clipboardDemo(cx *core.Scope, t *theme) core.Node {
	clip := core.NewSignal(cx, "Hello from NSGo")
	status := core.NewSignal(cx, "Nothing read yet.")
	image := core.NewSignal(cx, (*core.ClipboardImage)(nil))
	return t.demo("Clipboard", "Text, images and the files a file manager copied: SetClipboard and SetClipboardImage write, "+
		"ReadClipboard, ReadClipboardImage and ReadClipboardFiles read, and HasClipboardText and HasClipboardImage ask without reading.",
		core.Input{Value: clip.Get, OnChange: clip.Set, Style: t.field(), Label: "Clipboard text"},
		flow(
			t.primary("Copy text", func() { cx.SetClipboard(clip.GetUntracked()) }),
			t.plain("Paste text", func() { cx.ReadClipboard(func(s string) { status.Set("Text: " + s) }) }),
			t.plain("Copy logo", func() { cx.SetClipboardImage(logo) }),
			t.plain("Paste image", func() {
				cx.ReadClipboardImage(func(img core.ClipboardImage, ok bool) {
					if !ok {
						image.Set(nil)
						status.Set("The clipboard holds no image.")
						return
					}
					image.Set(&img)
					status.Set(fmt.Sprintf("Image: %g × %g px, %d bytes of PNG", img.Size.Width, img.Size.Height, len(img.PNG)))
				})
			}),
			t.plain("Paste files", func() {
				cx.ReadClipboardFiles(func(uris []string) {
					if len(uris) == 0 {
						status.Set("The clipboard holds no files.")
						return
					}
					status.Set("Files: " + strings.Join(uris, ", "))
				})
			}),
			t.plain("What is there?", func() {
				cx.HasClipboardText(func(text bool) {
					cx.HasClipboardImage(func(img bool) { status.Set(fmt.Sprintf("Text: %v, image: %v", text, img)) })
				})
			}),
		),
		core.Text{ContentFunc: status.Get, Style: t.caption(), TestID: "clipboard-status"},
		core.Show{When: func() bool { return image.Get() != nil }, Then: func(*core.Scope) core.Node {
			return core.Image{SourceFunc: func() core.ImageSource { return core.ImageSource{Data: image.Get().PNG} }, ResizeMode: core.Contain,
				Label: "Pasted image", TestID: "pasted-image",
				Style: core.Style{Width: core.Pt(120), Height: core.Pt(120), CornerRadius: radius, Background: t.sunken}}
		}},
	)
}

func linksDemo(cx *core.Scope, t *theme) core.Node {
	opened := core.NewSignal(cx, "")
	open := func(u string) func() {
		return func() {
			cx.OpenURL(u, func(ok bool) {
				if ok {
					opened.Set("Opened " + u)
				} else {
					opened.Set("Nothing opened " + u)
				}
			})
		}
	}
	return t.demo("Links", "OpenURL opens a link outside the app: a web page in the browser, a mailto: or tel: link in its app. "+
		"It refuses javascript:, data: and file: URLs.",
		flow(
			t.primary("Open the website", open("https://github.com/oneo-me/nsgo")),
			t.plain("Write an email", open("mailto:hello@example.com?subject=Hello%20from%20NSGo")),
			t.plain("Refused URL", open("javascript:alert(1)")),
		),
		t.live(func() string {
			if opened.Get() == "" {
				return "Nothing opened yet."
			}
			return opened.Get()
		}),
	)
}

func pickerDemo(cx *core.Scope, t *theme) core.Node {
	picked := core.NewSignal(cx, (*[]core.PickedFile)(nil))
	asked := core.NewSignal(cx, false)
	pick := func(o core.PickOptions) func() {
		return func() {
			cx.PickImages(o, func(files []core.PickedFile) {
				asked.Set(true)
				picked.Set(&files)
			})
		}
	}
	return t.demo("Photo picker", "PickImages shows the system's picker and gives the photos chosen as files, "+
		"HEIC as JPEG, large ones scaled down when asked.",
		flow(
			t.primary("Pick a photo", pick(core.PickOptions{})),
			t.plain("Pick up to 4, 800 px", pick(core.PickOptions{Multiple: true, Limit: 4, MaxDimension: 800})),
		),
		core.Dynamic(func(cx *core.Scope) core.Node {
			var files []core.PickedFile
			if p := picked.Get(); p != nil {
				files = *p
			}
			if len(files) == 0 {
				if asked.Get() {
					return t.note("No photo picked.")
				}
				return t.note("Nothing picked yet.")
			}
			var tiles []core.Node
			for i, f := range files {
				tiles = append(tiles, core.View{Style: core.Style{Gap: 4, Width: core.Pt(120)}, Children: []core.Node{
					core.Image{Source: core.ImageSource{Data: f.Data}, ResizeMode: core.Cover, Label: f.Name,
						TestID: fmt.Sprintf("picked-%d", i),
						Style:  core.Style{Width: core.Pt(120), Height: core.Pt(90), CornerRadius: radius, Background: t.sunken}},
					core.Text{Content: f.Name, MaxLines: 1, Style: core.Style{FontSize: 12}},
					core.Text{Content: fmt.Sprintf("%g × %g, %d KB", f.Size.Width, f.Size.Height, (len(f.Data)+512)/1024),
						Style: t.caption()},
				}})
			}
			return flow(tiles...)
		}),
	)
}

func appearanceDemo(cx *core.Scope, t *theme) core.Node {
	env := cx.Env()
	names := map[core.Appearance]string{core.AppearanceSystem: "the system's", core.AppearanceLight: "light", core.AppearanceDark: "dark"}
	return t.demo("Appearance", "SetAppearance shows the app light or dark whatever the system's; title bars, alerts, menus "+
		"and the system's own views follow.",
		flow(
			t.plain("System", func() { cx.SetAppearance(core.AppearanceSystem) }),
			t.plain("Light", func() { cx.SetAppearance(core.AppearanceLight) }),
			t.plain("Dark", func() { cx.SetAppearance(core.AppearanceDark) }),
		),
		t.live(func() string {
			shown := "light"
			if env.Dark() {
				shown = "dark"
			}
			return fmt.Sprintf("Forced: %s; shown: %s.", names[env.Appearance()], shown)
		}),
	)
}

func toastDemo(cx *core.Scope, t *theme) core.Node {
	return t.demo("Toast", "ShowToast shows Android's toast at the bottom of the screen; other platforms have none.",
		flow(
			t.plain("Short toast", func() { cx.ShowToast("Saved", core.ToastShort) }),
			t.plain("Long toast", func() { cx.ShowToast("Your changes were saved to the device", core.ToastLong) }),
		),
	)
}
