package core

import (
	"slices"
	"testing"

	"github.com/oneo-me/nsgo/internal/driver"
)

// command returns the last item of kind in b.
func command(b *driver.MountBatch, kind driver.ItemKind) *driver.MountItem {
	var found *driver.MountItem
	if b == nil {
		return nil
	}
	for i := range b.Items {
		if b.Items[i].Kind == kind {
			found = &b.Items[i]
		}
	}
	return found
}

func TestClipboardReads(t *testing.T) {
	var cx *Scope
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node { cx = c; return View{} })
	h.commit()
	var text string
	var has, hasImage bool
	var img ClipboardImage
	var ok bool
	var files []string
	cx.ReadClipboard(func(s string) { text = s })
	cx.HasClipboardText(func(b bool) { has = b })
	cx.ReadClipboardImage(func(i ClipboardImage, o bool) { img, ok = i, o })
	cx.HasClipboardImage(func(b bool) { hasImage = b })
	cx.ReadClipboardFiles(func(u []string) { files = u })
	cx.SetClipboardImage([]byte{1, 2, 3})
	b := h.commit()
	var reads []driver.MountItem
	for _, it := range b.Items {
		if it.Kind == driver.ItemReadClipboard {
			reads = append(reads, it)
		}
	}
	want := []driver.ClipboardRead{driver.ClipboardReadText, driver.ClipboardReadHasText, driver.ClipboardReadImage,
		driver.ClipboardReadHasImage, driver.ClipboardReadFiles}
	if len(reads) != len(want) {
		t.Fatalf("reads = %+v", reads)
	}
	for i, r := range reads {
		if r.Clipboard != want[i] {
			t.Fatalf("read %d = %v, want %v", i, r.Clipboard, want[i])
		}
	}
	if it := command(b, driver.ItemSetClipboardImage); it == nil || !slices.Equal(it.Data.([]byte), []byte{1, 2, 3}) {
		t.Fatalf("set image = %+v", it)
	}
	answers := []driver.Event{
		{Text: "hi"}, {Finished: true}, {Data: &driver.ClipboardImage{PNG: []byte{9}, Size: Size{Width: 2, Height: 3}}},
		{Finished: true}, {URIs: []string{"file:///a.txt"}},
	}
	for i, ev := range answers {
		ev.Kind, ev.Count = driver.EventClipboard, reads[i].Count
		h.tree.Dispatch(ev)
		h.tree.Dispatch(ev) // answered once
	}
	if text != "hi" || !has || !ok || img.Size.Width != 2 || !hasImage || !slices.Equal(files, []string{"file:///a.txt"}) {
		t.Fatalf("text %q has %v image %+v %v hasImage %v files %v", text, has, img, ok, hasImage, files)
	}
	cx.ReadClipboardImage(func(i ClipboardImage, o bool) { ok = o })
	h.tree.Dispatch(driver.Event{Kind: driver.EventClipboard, Count: command(h.commit(), driver.ItemReadClipboard).Count})
	if ok {
		t.Fatal("a clipboard without an image reads as one")
	}
}

func TestCheckURL(t *testing.T) {
	for _, u := range []string{"https://example.com/a?b=c#d", "http://localhost:8080", "mailto:ada@example.com?subject=Hi%20there",
		"tel:+1-555-0100", "sms:+15550100", "maps://?q=Paris", "lb://router/stock/700.HK"} {
		if err := CheckURL(u); err != nil {
			t.Errorf("%s: %v", u, err)
		}
	}
	long := "https://example.com/" + string(make([]byte, MaxURLLength))
	for _, u := range []string{"", "example.com", "/relative/path", "javascript:alert(1)", "JavaScript:alert(1)",
		"data:text/html,<b>x</b>", "file:///etc/passwd", "blob:https://example.com/1", "about:blank", "asset:/logo.png",
		"https://exa mple.com", "https://example.com/\n", "https:///path", long} {
		if CheckURL(u) == nil {
			t.Errorf("%.40q is opened", u)
		}
	}
}

func TestOpenURL(t *testing.T) {
	var cx *Scope
	var hosted []string
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node { cx = c; return View{} })
	h.commit()
	var got []bool
	cx.OpenURL("https://example.com", func(ok bool) { got = append(got, ok) })
	it := command(h.commit(), driver.ItemOpenURL)
	if it == nil || it.Value != "https://example.com" {
		t.Fatalf("item = %+v", it)
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventOpenURL, Count: it.Count, Finished: true})
	cx.OpenURL("javascript:alert(1)", func(ok bool) { got = append(got, ok) })
	if command(h.commit(), driver.ItemOpenURL) != nil {
		t.Fatal("a refused URL was sent")
	}
	cx.OpenURL("tel:1", nil)
	h.tree.Dispatch(driver.Event{Kind: driver.EventOpenURL, Count: command(h.commit(), driver.ItemOpenURL).Count})
	if !slices.Equal(got, []bool{true, false}) {
		t.Fatalf("answers = %v", got)
	}

	// An embedding host's handler takes URLs first.
	root := NewRoot(RootOptions{OpenURL: func(u string) bool { hosted = append(hosted, u); return u == "lb://a" }})
	tree := NewTree(root, ApproxTextEngine{})
	tree.Mount(func(c *Scope) Node { cx = c; return View{} })
	tree.Commit()
	got = nil
	cx.OpenURL("lb://a", func(ok bool) { got = append(got, ok) })
	cx.OpenURL("https://b.example", func(ok bool) { got = append(got, ok) })
	b := tree.Commit()
	if !slices.Equal(hosted, []string{"lb://a", "https://b.example"}) || !slices.Equal(got, []bool{true}) ||
		command(b, driver.ItemOpenURL).Value != "https://b.example" {
		t.Fatalf("hosted %v answers %v", hosted, got)
	}
}

func TestPickImages(t *testing.T) {
	var cx *Scope
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node { cx = c; return View{} })
	h.commit()
	var got []PickedFile
	cx.PickImages(PickOptions{Multiple: true, Limit: 2, MaxDimension: 1000, Quality: 3}, func(f []PickedFile) { got = f })
	it := command(h.commit(), driver.ItemPickImages)
	if it == nil || *it.Pick != (driver.PickRequest{Limit: 2, MaxDimension: 1000, Quality: 1}) {
		t.Fatalf("pick = %+v", it)
	}
	png := []byte("\x89PNG\r\n\x1a\n....")
	h.tree.Dispatch(driver.Event{Kind: driver.EventPick, Count: it.Count, Data: []driver.PickedFile{
		{Name: "IMG_1.HEIC", MIME: "image/jpeg"}, {Name: "/tmp/shot", Data: png}, {Name: "third.png"}}})
	if len(got) != 2 || got[0].Name != "IMG_1.jpg" || got[1].Name != "shot.png" || got[1].MIME != "image/png" {
		t.Fatalf("files = %+v", got)
	}
	cx.PickImages(PickOptions{Limit: 5}, nil)
	if r := command(h.commit(), driver.ItemPickImages).Pick; r.Limit != 1 {
		t.Fatalf("a single pick asks for %d", r.Limit)
	}
	cx.PickImages(PickOptions{Multiple: true}, nil)
	if r := command(h.commit(), driver.ItemPickImages).Pick; r.Limit != MaxPicked {
		t.Fatalf("a pick without a limit asks for %d", r.Limit)
	}
}

func TestPickRequestEncodings(t *testing.T) {
	px := func(w, h Points) Size { return Size{Width: w, Height: h} }
	var r driver.PickRequest
	for _, c := range []struct {
		mime string
		size Size
		want driver.PickEncoding
	}{{"image/jpeg", px(4000, 3000), driver.PickKeep}, {"image/heic", px(400, 300), driver.PickJPEG},
		{"image/gif", px(400, 300), driver.PickKeep}} {
		if got := r.Encoding(c.mime, c.size); got != c.want {
			t.Errorf("%s: %v, want %v", c.mime, got, c.want)
		}
	}
	r.Format = driver.PickCurrent
	if r.Encoding("image/heic", px(400, 300)) != driver.PickKeep {
		t.Error("PickCurrent converts HEIC")
	}
	r.MaxDimension = 1000
	if r.Encoding("image/heic", px(4000, 3000)) != driver.PickJPEG || r.Encoding("image/png", px(4000, 3000)) != driver.PickPNG ||
		r.Encoding("image/png", px(400, 300)) != driver.PickKeep {
		t.Error("scaling encodings")
	}
	r.Quality = 0.5
	if r.Encoding("image/jpeg", px(400, 300)) != driver.PickJPEG {
		t.Error("a quality keeps JPEGs")
	}
	if r.ScaledSize(px(4000, 3000)) != px(1000, 750) || r.ScaledSize(px(300, 4000)) != px(75, 1000) ||
		r.ScaledSize(px(400, 300)) != px(400, 300) {
		t.Error("scaled sizes")
	}
	for _, c := range [][3]string{{"IMG_0042.HEIC", "image/jpeg", "IMG_0042.jpg"}, {"a.jpeg", "image/jpeg", "a.jpeg"},
		{"", "image/png", "image.png"}, {"photo", "image/webp", "photo.webp"}, {"notes.txt", "", "notes.txt"},
		{`C:\pics\b.PNG`, "image/png", "b.PNG"}} {
		if got := driver.PickedName(c[0], c[1]); got != c[2] {
			t.Errorf("PickedName(%q, %q) = %q, want %q", c[0], c[1], got, c[2])
		}
	}
}

func TestPopupMenu(t *testing.T) {
	var cx *Scope
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node { cx = c; return View{} })
	h.tree.SetEnvironment(Environment{OS: OSMacOS, Idiom: IdiomDesktop})
	h.commit()
	var log []string
	cx.PopupMenu([]MenuItem{{Label: "Copy", OnSelect: func() { log = append(log, "copy") }},
		{Label: "More", Items: []MenuItem{{Label: "Deep", OnSelect: func() { log = append(log, "deep") }}}}},
		Point{X: 10, Y: 20}, func() { log = append(log, "closed") })
	it := command(h.commit(), driver.ItemPopupMenu)
	if it == nil || it.Point != (Point{X: 10, Y: 20}) || len(it.Menu) != 2 || it.Menu[1].Items[0].ID == 0 {
		t.Fatalf("popup = %+v", it)
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventMenu, Count: it.Menu[1].Items[0].ID})
	h.tree.Dispatch(driver.Event{Kind: driver.EventMenuClosed, Count: it.Count})
	h.tree.Dispatch(driver.Event{Kind: driver.EventMenuClosed, Count: it.Count})

	// A platform may report the item chosen after the menu closed.
	cx.PopupMenu([]MenuItem{{Label: "Late", OnSelect: func() { log = append(log, "late") }}}, Point{}, nil)
	late := command(h.commit(), driver.ItemPopupMenu)
	h.tree.Dispatch(driver.Event{Kind: driver.EventMenuClosed, Count: late.Count})
	h.tree.Dispatch(driver.Event{Kind: driver.EventMenu, Count: late.Menu[0].ID})
	if log[len(log)-1] != "late" {
		t.Fatalf("log = %q", log)
	}
	log = log[:len(log)-1]

	// A second menu closes the first.
	cx.PopupMenu([]MenuItem{{Label: "A"}}, Point{}, func() { log = append(log, "first") })
	cx.PopupMenu([]MenuItem{{Label: "B"}}, Point{}, nil)
	if !slices.Equal(log, []string{"deep", "closed", "first"}) {
		t.Fatalf("log = %q", log)
	}

	// Phones have no context menus: closed runs at once.
	h.tree.SetEnvironment(Environment{OS: OSIOS, Idiom: IdiomPhone})
	h.commit()
	cx.PopupMenu([]MenuItem{{Label: "A"}}, Point{}, func() { log = append(log, "phone") })
	if command(h.commit(), driver.ItemPopupMenu) != nil || log[len(log)-1] != "phone" {
		t.Fatalf("log = %q", log)
	}
}

func TestWindowFrameAndLevel(t *testing.T) {
	var cx *Scope
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node { cx = c; return View{} })
	h.commit()
	cx.SetWindowFrame(WindowFrame{Height: 300, Width: -5, Animated: true})
	cx.SetWindowLevel(WindowLevelFloating)
	cx.OrderFront()
	cx.SetMenuBar(MenuBarAutoHide)
	var got []WindowCommand
	for _, it := range h.commit().Items {
		if it.Kind == driver.ItemWindow {
			got = append(got, *it.Window)
		}
	}
	want := []WindowCommand{{Action: WindowSetFrame, Frame: WindowFrame{Height: 300, Animated: true}},
		{Action: WindowSetLevel, Level: WindowLevelFloating}, {Action: WindowOrderFront},
		{Action: WindowSetMenuBar, MenuBar: MenuBarAutoHide}}
	if !slices.Equal(got, want) {
		t.Fatalf("commands = %+v", got)
	}
}

func TestAppearanceForcesDark(t *testing.T) {
	var cx *Scope
	var seen []bool
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node {
		cx = c
		c.Effect(func() { seen = append(seen, c.Env().Dark()) })
		return View{}
	})
	h.tree.SetEnvironment(Environment{Dark: false, OSVersion: OSVersion{Major: 17, Minor: 4}})
	h.commit()
	cx.SetAppearance(AppearanceDark)
	if it := command(h.commit(), driver.ItemAppearance); it == nil || it.Appearance != AppearanceDark {
		t.Fatalf("appearance item = %+v", it)
	}
	// The system turning light or dark changes nothing while forced.
	h.tree.SetEnvironment(Environment{Dark: true})
	h.tree.SetEnvironment(Environment{Dark: false})
	cx.SetAppearance(AppearanceDark)
	if command(h.commit(), driver.ItemAppearance) != nil {
		t.Fatal("the same appearance was sent again")
	}
	cx.SetAppearance(AppearanceSystem)
	h.tree.SetEnvironment(Environment{Dark: true})
	h.commit()
	if !slices.Equal(seen, []bool{false, true, false, true}) || cx.Env().Appearance() != AppearanceSystem {
		t.Fatalf("dark = %v", seen)
	}
	h.tree.SetEnvironment(Environment{OSVersion: OSVersion{Major: 17, Minor: 4}})
	if v := cx.Env().OSVersion(); !v.AtLeast(17, 0) || v.AtLeast(17, 5) || v.AtLeast(18, 0) || v.String() != "17.4" {
		t.Fatalf("version = %v", v)
	}
}

func TestToastAndQuit(t *testing.T) {
	var cx *Scope
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node { cx = c; return View{} })
	h.commit()
	cx.ShowToast("Saved", ToastLong)
	cx.Quit()
	b := h.commit()
	if it := command(b, driver.ItemToast); it == nil || it.Value != "Saved" || it.Count != 1 || command(b, driver.ItemQuit) == nil {
		t.Fatalf("items = %+v", b.Items)
	}
}
