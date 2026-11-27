package core

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// hostsAccess allows network and web view requests to hosts, the host
// handlers named, and nothing else.
type hostsAccess struct {
	hosts    []string
	handlers []string
}

type schemesAccess struct {
	hostsAccess
	schemes []string
}

func (a schemesAccess) WebSchemes() []string { return a.schemes }

func (a hostsAccess) Allows(req AccessRequest) bool {
	switch req.Permission {
	case AccessNetwork, AccessWebView:
		host := strings.TrimPrefix(strings.TrimPrefix(req.Resource, "https://"), "http://")
		host, _, _ = strings.Cut(host, "/")
		return req.Method != AccessInput && driver.AllowsHost(a.hosts, host)
	case AccessHost:
		return slices.Contains(a.handlers, req.Resource)
	}
	return false
}

func (a hostsAccess) WebHosts() []string { return a.hosts }

func TestAccessHoldsWhatItsOwnerRenders(t *testing.T) {
	var failed []string
	onError := func(err error) { failed = append(failed, err.Error()) }
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		held := Func(func(cx *Scope) Node {
			cx.ProvideAccess(hostsAccess{hosts: []string{"ok.com", "*.cdn.com"}})
			return View{Children: []Node{
				Image{Source: Src("https://ok.com/a.png"), TestID: "ok"},
				Image{Source: Src("https://evil.io/a.png"), OnError: onError, TestID: "evil"},
				Image{Source: Src("https://ok.com@evil.io/a.png"), TestID: "userinfo"},
				Image{Source: Src("file:///etc/passwd"), TestID: "file"},
				Image{Source: ImageSource{URI: "https://evil.io/b.png", Data: []byte{1}}, TestID: "bytes"},
				Image{Source: Src("data:image/png;base64,AA=="), TestID: "data"},
				Video{Source: "https://img.cdn.com/v.mp4", TestID: "video"},
				WebView{Source: WebSrc("https://evil.io/"), OnError: onError, TestID: "web"},
			}}
		})
		return View{Children: []Node{held, Image{Source: Src("https://evil.io/host.png"), TestID: "host"}}}
	})
	b := h.commit()
	uris := map[string]string{}
	var web *driver.WebConfig
	for _, it := range b.Items {
		switch it.Kind {
		case driver.ItemImage:
			uris[h.tree.nodes[it.ID].props.TestID] = it.Image.Source.URI
		case driver.ItemVideo:
			uris["video"] = it.Video.Source
		case driver.ItemWeb:
			web = it.Web
		}
	}
	want := map[string]string{"ok": "https://ok.com/a.png", "evil": "", "userinfo": "", "file": "",
		"bytes": "https://evil.io/b.png", "data": "data:image/png;base64,AA==", "video": "https://img.cdn.com/v.mp4",
		"host": "https://evil.io/host.png"}
	for id, uri := range want {
		if uris[id] != uri {
			t.Errorf("%s loads %q, want %q", id, uris[id], uri)
		}
	}
	if web == nil || web.Source.Src != "about:blank" || !web.Restricted || web.Hosts != "ok.com *.cdn.com" || web.Schemes != "about blob data" {
		t.Fatalf("the web view is sent %+v", web)
	}
	h.drain()
	h.commit()
	h.drain()
	if len(failed) != 2 || !strings.Contains(failed[0], "evil.io") {
		t.Fatalf("OnError heard %q, once for each refusal", failed)
	}
}

func TestAccessRefusesServices(t *testing.T) {
	var cx *Scope
	h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node {
		cx = c.Child()
		cx.Run(func() { cx.ProvideAccess(hostsAccess{}) })
		return View{}
	})
	h.commit()
	var opened []bool
	cx.OpenURL("https://example.com", func(ok bool) { opened = append(opened, ok) })
	cx.SetStatusBar(StatusBarSetting{Hidden: ToggleOn})
	err := cx.CheckAccess(AccessRequest{Permission: AccessClipboard})
	var coded *Error
	if !errors.As(err, &coded) || coded.Code != CodeAccessDenied || coded.Permission != AccessClipboard {
		t.Fatalf("CheckAccess = %v", err)
	}
	b := h.commit()
	for _, it := range b.Items {
		if it.Kind == driver.ItemOpenURL {
			t.Fatal("a refused URL reached the platform")
		}
	}
	if !slices.Equal(opened, []bool{false}) || h.tree.status.base.Hidden != ToggleInherit {
		t.Fatalf("opened %v, status bar %+v", opened, h.tree.status.base)
	}
	if h.root.CheckAccess(AccessRequest{Permission: AccessClipboard}) != nil {
		t.Fatal("the app's own code was refused")
	}
}

// A screen's or modal's status bar function that returns no setting, as TSX's Dialog and Screen pass without a statusBar prop, asks Access nothing, so it neither logs a refusal nor uses up the runtime's one statusBar refusal line, which a real setting and SetStatusBar still get (#218).
func TestStatusBarAsksAccessOnlyForASetting(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	refusals := func() int { return strings.Count(output.String(), `"`+AccessStatusBar+`" capability`) }

	for _, set := range []string{"setting", "SetStatusBar"} {
		output.Reset()
		var enabled *Signal[bool]
		var held *Scope
		h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
			cx.ProvideAccess(hostsAccess{})
			held = cx
			enabled = NewSignal(cx, false)
			setting := func() StatusBarSetting {
				if set == "setting" && enabled.Get() {
					return StatusBarSetting{Hidden: ToggleOn}
				}
				return StatusBarSetting{}
			}
			return View{Style: Style{FlexGrow: 1}, Children: []Node{
				Stack{Style: Style{FlexGrow: 1}, Children: []Node{Screen{TestID: "home", StatusBarFunc: setting}}},
				Modal{StatusBarFunc: setting},
			}}
		})
		if got := statusItems(h.commit()); refusals() != 0 || len(got) != 0 {
			t.Fatalf("%s: empty settings logged %q and sent %v", set, output.String(), got)
		}
		if set == "setting" {
			enabled.Set(true)
		} else {
			held.SetStatusBar(StatusBarSetting{Hidden: ToggleOn})
		}
		if got := statusItems(h.commit()); refusals() != 1 || len(got) != 0 {
			t.Fatalf("%s: logged %d refusals (%q) and sent %v, want one refusal and nothing sent", set, refusals(), output.String(), got)
		}
	}
}

func TestAccessAllowsRegisteredWebScheme(t *testing.T) {
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		cx.ProvideAccess(hostsAccess{})
		if err := cx.ServeScheme("app", func(string) []byte { return []byte("ok") }); err != nil {
			t.Fatal(err)
		}
		return WebView{Source: WebSrc("app://index.html"), TestID: "web"}
	})
	b := h.commit()
	for _, item := range b.Items {
		if item.Kind == driver.ItemWeb {
			if item.Web.Source.Src != "app://index.html" || !item.Web.Restricted || item.Web.SchemeHandlers["app"] == nil {
				t.Fatalf("web config %+v", item.Web)
			}
			return
		}
	}
	t.Fatal("no web item")
}

func TestAccessWebSchemesRestrictPageOwnedNavigations(t *testing.T) {
	web := hostsAccess{hosts: []string{"*"}}
	for _, tc := range []struct {
		name         string
		outer, inner Access
		want         string
	}{
		{"without WebSchemes", web, nil, "about blob data"},
		{"nil", schemesAccess{web, nil}, nil, ""},
		{"normalized", schemesAccess{web, []string{"DATA", "javascript", "file", "Blob"}}, nil, "blob data"},
		{"nested", schemesAccess{web, []string{"about", "blob"}}, schemesAccess{web, []string{"blob", "data"}}, "blob"},
		{"nested under one without", web, schemesAccess{web, []string{"data"}}, "data"},
		{"nested over one without", schemesAccess{web, []string{"about"}}, web, "about"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
				cx.ProvideAccess(tc.outer)
				return Func(func(cx *Scope) Node {
					if tc.inner != nil {
						cx.ProvideAccess(tc.inner)
					}
					return WebView{Source: WebSrc("https://example.com"), TestID: "web"}
				})
			})
			for _, item := range h.commit().Items {
				if item.Kind == driver.ItemWeb {
					if !item.Web.Restricted || item.Web.Schemes != tc.want {
						t.Fatalf("web view restricted %v, schemes %q, want %q", item.Web.Restricted, item.Web.Schemes, tc.want)
					}
					return
				}
			}
			t.Fatal("no web item")
		})
	}
}

// mediaAccess allows web views every page and the devices named.
type mediaAccess []string

func (a mediaAccess) Allows(req AccessRequest) bool {
	return req.Permission == AccessWebView && (req.Method != AccessMedia || slices.Contains(a, req.Resource))
}

func (a mediaAccess) WebHosts() []string { return []string{"*"} }

func TestAccessHoldsWhatWebPagesCapture(t *testing.T) {
	both := WebMicrophone | WebCamera
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		held := Func(func(cx *Scope) Node {
			cx.ProvideAccess(mediaAccess{"microphone"})
			return WebView{Source: WebSrc("https://ok.com/"), Media: both, TestID: "held"}
		})
		return View{Children: []Node{held,
			WebView{Source: WebSrc("https://ok.com/"), Media: both, TestID: "own"},
			WebView{Source: WebSrc("https://ok.com/"), TestID: "none"}}}
	})
	got := map[string]WebMedia{}
	for _, it := range h.commit().Items {
		if it.Kind == driver.ItemWeb {
			got[h.tree.nodes[it.ID].props.TestID] = it.Web.Media
		}
	}
	if got["held"] != WebMicrophone || got["own"] != both || got["none"] != 0 {
		t.Fatalf("the web views may capture %v", got)
	}
	if !both.Allows(WebCamera) || WebMicrophone.Allows(both) || both.Allows(0) {
		t.Fatal("a page asking for both was given them with one allowed")
	}
}

func TestProvideHostGivesASubtreeItsOwnChannel(t *testing.T) {
	app, plugin := NewHost(), NewHost()
	app.Handle("secret", func(c *HostCall) { c.Resolve("the app's") })
	plugin.Handle("greet", func(c *HostCall) { c.Resolve("hi " + c.Data.(string)) })
	plugin.Handle("other", func(c *HostCall) { c.Resolve("refused") })
	plugin.SetParams(map[string]any{"who": "plugin"})
	var inner, bare *Scope
	h := newEmbedded(t, app, func(cx *Scope) Node {
		inner, bare = cx.Child(), cx.Child()
		inner.Run(func() {
			inner.ProvideHost(plugin)
			inner.ProvideAccess(hostsAccess{handlers: []string{"greet"}})
		})
		bare.Run(func() { bare.ProvideHost(nil) })
		return View{}
	})
	var got []string
	answer := func(v any, err error) {
		if err != nil {
			got = append(got, "error "+ErrorCode(err))
		} else {
			got = append(got, v.(string))
		}
	}
	inner.CallHost("greet", "Ada", answer)
	inner.CallHost("other", nil, answer)
	inner.CallHost("secret", nil, answer)
	bare.CallHost("secret", nil, answer)
	h.root.CallHost("secret", nil, answer)
	h.drain()
	want := []string{"hi Ada", "error " + CodeAccessDenied, "error " + CodeAccessDenied, "error " + CodeUnavailable, "the app's"}
	if !slices.Equal(got, want) {
		t.Fatalf("answers %q, want %q", got, want)
	}
	if inner.HostParams()["who"] != "plugin" || bare.HasHost() || len(h.root.HostParams()) != 0 {
		t.Fatal("the parameters are not the subtree's host's")
	}
	var heard []any
	inner.Run(func() { inner.OnHostEvent("ping", func(d any) { heard = append(heard, d) }) })
	plugin.Send("ping", 1)
	app.Send("ping", 2)
	h.drain()
	if !slices.Equal(heard, []any{float64(1)}) {
		t.Fatalf("the subtree heard %v", heard)
	}
	inner.Dispose()
	if plugin.app != nil {
		t.Fatal("the host stayed attached to a disposed subtree")
	}
}

func TestProvideStoragePartitions(t *testing.T) {
	b := &recorder{kept: map[string]string{"app": "1"}}
	g := newGraph(NewStore(b))
	app := g.root.Storage()
	one, two := g.root.Child(), g.root.Child()
	one.ProvideStorage("one", 10)
	two.ProvideStorage("two", 0)
	s1, s2 := one.Storage(), two.Storage()
	if err := s1.SetItem("k", "v1"); err != nil {
		t.Fatal(err)
	}
	s2.SetItem("k", "v2")
	if v, _ := s1.GetItem("k"); v != "v1" {
		t.Fatalf("one reads %q", v)
	}
	if _, ok := s1.GetItem("app"); ok {
		t.Fatal("a partition read the app's entry")
	}
	if k := app.Keys(); !slices.Equal(k, []string{"app"}) {
		t.Fatalf("the app lists %v", k)
	}
	if k := s2.Keys(); !slices.Equal(k, []string{"k"}) {
		t.Fatalf("two lists %v", k)
	}
	var full *StorageFullError
	if err := s1.SetItem("big", "0123456789"); !errors.As(err, &full) || full.Quota != 10 {
		t.Fatalf("over its quota: %v", err)
	}
	app.Clear()
	if v, _ := s2.GetItem("k"); v != "v2" {
		t.Fatal("the app's Clear cleared a partition")
	}
	s1.Clear()
	if _, ok := s1.GetItem("k"); ok || len(s2.Keys()) != 1 {
		t.Fatal("a partition's Clear did not clear only its own")
	}
	for _, c := range b.changes {
		if c.Op == StorageClear {
			t.Fatal("the backend was cleared whole")
		}
	}
}

func TestWebConfigNavigable(t *testing.T) {
	c := driver.WebConfig{Restricted: true, Hosts: "ok.com *.cdn.com", Schemes: "about blob data", SchemeHandlers: map[string]driver.WebSchemeHandler{"app": nil}}
	for url, want := range map[string]bool{
		"https://ok.com/a": true, "http://OK.com": true, "https://a.cdn.com/x": true, "https://cdn.com/": false,
		"https://evil.io": false, "https://ok.com@evil.io/": false, "about:blank": true, "data:text/html,x": true,
		"file:///etc/hosts": false, "mailto:a@ok.com": false, "app://settings/index.html": true,
	} {
		if got := c.Navigable(url); got != want {
			t.Errorf("Navigable(%q) = %v", url, got)
		}
	}
	if got := driver.IntersectHosts([]string{"*.example.com", "a.com"}, []string{"api.example.com", "*"}); !slices.Equal(got, []string{"api.example.com", "*.example.com", "a.com"}) {
		t.Fatalf("IntersectHosts = %v", got)
	}
}

func TestHostHandlersRunAsTheHost(t *testing.T) {
	plugin := NewHost()
	var app, inner *Scope
	// The host's handlers call services with the app's own scope, which the plugin's Access does not hold, answering at once or later.
	plugin.Handle("now", func(c *HostCall) {
		app.OpenURL("https://help.example.com/now", nil)
		// The plugin's own scope stays held to its Access, whoever calls on it.
		inner.OpenURL("https://help.example.com/inner", nil)
		c.Resolve(nil)
	})
	plugin.Handle("later", func(c *HostCall) {
		app.Post(func() {
			app.OpenURL("https://help.example.com/later", nil)
			c.Resolve(nil)
		})
	})
	plugin.HandleOpenURL(func(u string, answer func(bool)) {
		app.SetStatusBar(StatusBarSetting{Hidden: ToggleOn})
		answer(false)
	})
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		app, inner = cx, cx.Child()
		inner.Run(func() {
			inner.ProvideHost(plugin)
			inner.ProvideAccess(allows(func(req AccessRequest) bool {
				return req.Permission == AccessHost || req.Resource == "https://plugin.example.com"
			}))
		})
		return View{}
	})
	h.commit()
	var errs []error
	answer := func(_ any, err error) { errs = append(errs, err) }
	// The plugin's code calls as it runs, under its own owner.
	inner.Run(func() {
		inner.CallHost("now", nil, answer)
		inner.PostHost("later", nil)
		inner.CallHost("later", nil, answer)
		inner.OpenURL("https://plugin.example.com", nil)
	})
	h.drain()
	if len(errs) != 2 || errs[0] != nil || errs[1] != nil {
		t.Fatalf("the calls answered %v", errs)
	}
	var urls []string
	for _, it := range h.commit().Items {
		if it.Kind == driver.ItemOpenURL {
			urls = append(urls, it.Value)
		}
	}
	want := []string{"https://help.example.com/now", "https://help.example.com/later", "https://help.example.com/later", "https://plugin.example.com"}
	if !slices.Equal(urls, want) {
		t.Fatalf("the platform opened %q, want %q", urls, want)
	}
	if h.tree.status.base.Hidden != ToggleOn {
		t.Fatal("the host's URL handler was held to the plugin's Access")
	}
}

// allows is an Access of a function.
type allows func(AccessRequest) bool

func (a allows) Allows(req AccessRequest) bool { return a(req) }

func (a allows) WebHosts() []string { return nil }

func TestVideoSeparateAudioUsesNetworkGrant(t *testing.T) {
	var failure error
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		cx.ProvideAccess(hostsAccess{hosts: []string{"ok.com"}})
		return Video{
			Source:      "https://ok.com/picture.mp4",
			AudioSource: "https://other.com/sound.m4a",
			OnError:     func(err error) { failure = err },
		}
	})
	var config *driver.VideoConfig
	for _, it := range h.commit().Items {
		if it.Kind == driver.ItemVideo {
			config = it.Video
		}
	}
	h.drain()
	if config == nil || config.Source != "" || config.AudioSource != "" {
		t.Fatalf("refused separate audio should stop both sources, got %+v", config)
	}
	if failure == nil || !strings.Contains(failure.Error(), "other.com") {
		t.Fatalf("audio source refusal = %v", failure)
	}
}

type mediaFSAndHostAccess struct {
	hosts []string
	roots []string
}

func (a mediaFSAndHostAccess) Allows(req AccessRequest) bool {
	switch req.Permission {
	case AccessNetwork:
		host := strings.TrimPrefix(strings.TrimPrefix(req.Resource, "https://"), "http://")
		host, _, _ = strings.Cut(host, "/")
		host, _, _ = strings.Cut(host, ":")
		return driver.AllowsHost(a.hosts, host)
	case AccessFS:
		if req.Method != AccessRead {
			return false
		}
		for _, root := range a.roots {
			resolvedRoot, err := filepath.EvalSymlinks(root)
			if err != nil {
				continue
			}
			rel, err := filepath.Rel(resolvedRoot, req.Resource)
			if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
				return true
			}
		}
	}
	return false
}

func (a mediaFSAndHostAccess) WebHosts() []string { return nil }

func TestAccessMediaFSReadMenuIconsAndCanonicalURL(t *testing.T) {
	base := t.TempDir()
	allowedDir := filepath.Join(base, "allowed")
	outsideDir := filepath.Join(base, "outside")
	if err := os.MkdirAll(allowedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	imgFile := filepath.Join(allowedDir, "pic.png")
	vidFile := filepath.Join(allowedDir, "clip.mp4")
	outsideFile := filepath.Join(outsideDir, "secret.png")
	for _, f := range []string{imgFile, vidFile, outsideFile} {
		if err := os.WriteFile(f, []byte("bytes"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	escapeLink := filepath.Join(allowedDir, "escape.png")
	_ = os.Symlink(outsideFile, escapeLink)

	win := goruntime.GOOS == "windows"
	realImg, err := filepath.EvalSymlinks(imgFile)
	if err != nil {
		t.Fatal(err)
	}
	realVid, err := filepath.EvalSymlinks(vidFile)
	if err != nil {
		t.Fatal(err)
	}
	wantImgURL := driver.CanonicalFileURL(realImg, win)
	wantVidURL := driver.CanonicalFileURL(realVid, win)

	var errs []error
	onErr := func(err error) { errs = append(errs, err) }

	var held *Scope
	h := newHost(t, Size{Width: 200, Height: 200}, func(cx *Scope) Node {
		cx.Env().ratio.Set(2)
		return Func(func(cx *Scope) Node {
			held = cx
			cx.ProvideAccess(mediaFSAndHostAccess{hosts: []string{"ok.com"}, roots: []string{allowedDir}})
			return View{
				Style: Style{CursorImage: &CustomCursor{
					Image:   Src(outsideFile),
					Image2x: Src(imgFile),
				}},
				Children: []Node{
					Image{Source: Src(imgFile), TestID: "img-path"},
					Image{Source: Src(driver.CanonicalFileURL(imgFile, win)), TestID: "img-fileurl"},
					Image{Source: Src(outsideFile), OnError: onErr, TestID: "img-outside"},
					Image{Source: Src(escapeLink), OnError: onErr, TestID: "img-symlink"},
					Image{Source: Src("https://OK.COM:443/a/../b.png"), TestID: "img-canon"},
					Video{Source: vidFile, TestID: "vid-ok"},
					Video{Source: outsideFile, OnError: onErr, TestID: "vid-outside"},
					WebView{Source: WebSrc(driver.CanonicalFileURL(imgFile, win)), TestID: "web-file"},
				},
			}
		})
	})

	b := h.commit()
	imgURIs := map[string]string{}
	vidURIs := map[string]string{}
	var imgCheck func(string) error
	var cursor *CustomCursor
	var webSrc string
	for _, it := range b.Items {
		switch it.Kind {
		case driver.ItemImage:
			id := h.tree.nodes[it.ID].props.TestID
			imgURIs[id] = it.Image.Source.URI
			if id == "img-canon" {
				imgCheck = driver.ImageSourceCheck(it.Image.Source)
			}
		case driver.ItemVideo:
			vidURIs[h.tree.nodes[it.ID].props.TestID] = it.Video.Source
		case driver.ItemWeb:
			webSrc = it.Web.Source.Src
		case driver.ItemViewStyle:
			if it.ViewStyle != nil && it.ViewStyle.CursorImage != nil {
				cursor = it.ViewStyle.CursorImage
			}
		}
	}

	if imgURIs["img-path"] != wantImgURL || imgURIs["img-fileurl"] != wantImgURL {
		t.Fatalf("allowed local images = %v, want %q", imgURIs, wantImgURL)
	}
	if imgURIs["img-outside"] != "" || imgURIs["img-symlink"] != "" {
		t.Fatalf("refused local images leaked: %v", imgURIs)
	}
	if imgURIs["img-canon"] != "https://ok.com/b.png" {
		t.Fatalf("canonical HTTP image URI = %q, want https://ok.com/b.png", imgURIs["img-canon"])
	}
	if imgCheck == nil || imgCheck("https://ok.com/next.png") != nil || ErrorCode(imgCheck("https://evil.io/x.png")) != CodeAccessDenied {
		t.Fatalf("image source redirect check failed: hasCheck=%v", imgCheck != nil)
	}
	if vidURIs["vid-ok"] != wantVidURL || vidURIs["vid-outside"] != "" {
		t.Fatalf("video URIs = %v, want ok=%q outside=\"\"", vidURIs, wantVidURL)
	}
	// WebView must NEVER allow fs.read local files.
	if webSrc != "about:blank" {
		t.Fatalf("webview local file was not refused: %q", webSrc)
	}
	// CursorImage refuses outside file and canonicalizes allowed local file.
	if cursor == nil || !cursor.Image.Empty() || cursor.Image2x.URI != wantImgURL {
		t.Fatalf("cursor image grants = %+v", cursor)
	}

	// Menu item icons: checked across application menu, dock menu and popup menu.
	h.tree.reported.OS = OSMacOS
	held.Run(func() {
		items := []MenuItem{
			{Label: "Allowed Local", Icon: MenuIcon{Source: Src(imgFile)}},
			{Label: "Outside Local", Icon: MenuIcon{Source: Src(outsideFile)}},
			{Label: "Symlink Escape", Icon: MenuIcon{Source: Src(escapeLink)}},
			{Label: "Canonical HTTP", Icon: MenuIcon{Source: Src("https://OK.COM:443/dir/../icon.png")}},
			{Label: "Denied HTTP", Icon: MenuIcon{Source: Src("https://evil.io/icon.png")}},
			{Label: "Sub", Items: []MenuItem{
				{Label: "Nested Denied", Icon: MenuIcon{Source: Src(outsideFile)}},
				{Label: "Nested Allowed", Icon: MenuIcon{Source: Src(imgFile)}},
			}},
		}
		held.SetApplicationMenu(items)
		held.SetDockMenu(items)
	})
	menuBatch := h.commit()
	checkedMenus := 0
	for _, it := range menuBatch.Items {
		if it.Kind != driver.ItemMenu && it.Kind != driver.ItemDockMenu {
			continue
		}
		checkedMenus++
		if len(it.Menu) != 6 {
			t.Fatalf("menu entries = %d, want 6", len(it.Menu))
		}
		if it.Menu[0].Icon.Source.URI != wantImgURL {
			t.Errorf("menu[0] icon URI = %q, want %q", it.Menu[0].Icon.Source.URI, wantImgURL)
		}
		if !it.Menu[1].Icon.Empty() || !it.Menu[2].Icon.Empty() || !it.Menu[4].Icon.Empty() {
			t.Errorf("ungranted menu icons were not stripped: %+v %+v %+v", it.Menu[1].Icon, it.Menu[2].Icon, it.Menu[4].Icon)
		}
		if it.Menu[3].Icon.Source.URI != "https://ok.com/icon.png" {
			t.Errorf("menu[3] canonical icon URI = %q, want https://ok.com/icon.png", it.Menu[3].Icon.Source.URI)
		}
		if !it.Menu[5].Items[0].Icon.Empty() || it.Menu[5].Items[1].Icon.Source.URI != wantImgURL {
			t.Errorf("submenu icons = %+v", it.Menu[5].Items)
		}
	}
	if checkedMenus != 2 {
		t.Fatalf("checked %d menus, want 2 (app + dock)", checkedMenus)
	}

	// ImageSize & KeepImage must check the scale-picked variant, not the 1x URI.
	var sizeErr error
	variantBypass := ImageSource{
		URI:      "https://ok.com/1x.png",
		Scales:   ScalesOf(1, 2),
		Variants: map[float32]string{2: "https://evil.io/2x.png"},
	}
	held.Run(func() {
		held.ImageSize(variantBypass, func(_ Size, err error) { sizeErr = err })
		held.KeepImage(variantBypass)()
	})
	if ErrorCode(sizeErr) != CodeAccessDenied {
		t.Fatalf("ImageSize variant bypass error = %v, want ERR_ACCESS_DENIED", sizeErr)
	}
	bypassBatch := h.commit()
	for _, it := range bypassBatch.Items {
		if it.Kind == driver.ItemImageInfo || it.Kind == driver.ItemKeepImage {
			t.Fatalf("variant bypass emitted %v", it.Kind)
		}
	}

	// ImageSize with an allowed canonical URL attaches sourceCheck.
	held.Run(func() {
		held.ImageSize(Src("https://OK.COM:443/x/../y.png"), func(Size, error) {})
	})
	infoBatch := h.commit()
	var gotInfo *driver.ImageSource
	for _, it := range infoBatch.Items {
		if it.Kind == driver.ItemImageInfo {
			gotInfo = &it.Image.Source
		}
	}
	if gotInfo == nil || gotInfo.URI != "https://ok.com/y.png" || driver.ImageSourceCheck(*gotInfo) == nil {
		t.Fatalf("ImageSize item = %+v, want canonical URI and redirect check", gotInfo)
	}

	h.drain()
	if len(errs) != 3 {
		t.Fatalf("expected 3 OnError callbacks (img-outside, img-symlink, vid-outside), got %d: %v", len(errs), errs)
	}
}
