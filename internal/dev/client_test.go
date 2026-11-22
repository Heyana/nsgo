//go:build nsgodev

package dev

import (
	"bufio"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// nsgo.Run starts DevTools before any graph, so an app's function that shows its view with nsgo.SetMainView has its trees registered and its commits sent with the run's token, and SetMainView, however often it is called, starts nothing again.
func TestDevClientStartsDevToolsOnceBeforeTheGraph(t *testing.T) {
	var mu sync.Mutex
	commits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The server takes the run's token alone, as nsgo dev's does.
		if r.Header.Get("X-NSGo-Token") != "run-token" {
			http.Error(w, "no token", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/_nsgo/devtools/out" {
			return
		}
		sc := bufio.NewScanner(r.Body)
		for sc.Scan() {
			if strings.Contains(sc.Text(), `"type":"commit"`) {
				mu.Lock()
				commits++
				mu.Unlock()
			}
		}
	}))
	defer server.Close()
	devServer, devDevtools, devtools.ServerToken = server.URL, "1", "run-token"
	t.Cleanup(func() {
		devServer, devDevtools, devtools.ServerToken = "", "", ""
		engine.Trace, driver.Applied = nil, nil
		devtools.Enabled, driver.TraceCauses = false, false
		devtools.MountOverlay = nil
		devOverlays.Store(nil)
	})

	// As nsgo.Run does before it starts the engine.
	Start()
	if !devtools.Enabled || engine.Trace == nil {
		t.Fatalf("after Start devtools.Enabled = %v and engine.Trace is set: %v", devtools.Enabled, engine.Trace != nil)
	}
	root := core.NewRoot(core.RootOptions{Post: func(func()) {}})
	core.NewTree(root, core.ApproxTextEngine{})
	defer root.Dispose()
	if len(devtools.Surfaces()) == 0 {
		t.Fatal("a tree made after Start did not register with DevTools")
	}

	// The app's function shows a view, then another: each is wrapped, nothing starts again.
	view := func(*core.Scope) core.Node { return core.View{} }
	Wrap(view)
	Wrap(view)
	engine.Trace(&driver.MountBatch{}, time.Millisecond)
	count := func() int {
		mu.Lock()
		defer mu.Unlock()
		return commits
	}
	for deadline := time.Now().Add(5 * time.Second); count() == 0 && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
	}
	// The client posts a quarter of a second's messages at once: another wrapper's would have come with them.
	time.Sleep(500 * time.Millisecond)
	if n := count(); n != 1 {
		t.Fatalf("one commit reached the page %d times", n)
	}
}

// The overlay's title and buttons are the dev client's catalog's, in the language the app shows: nsgo dev -lang zh-Hans's, as a device's set to Chinese.
func TestDevOverlayUsesCatalogLanguage(t *testing.T) {
	appSpeaks(t, "en", "zh-Hans")
	for _, tc := range []struct {
		locale string
		kind   devFailureKind
		want   []string
	}{
		{"en-US", devAppFailed, []string{"The app failed", "Copy", "Reload"}},
		{"en-US", devBuildFailed, []string{"Build failed", "Copy", "Reload"}},
		{"zh-CN", devAppFailed, []string{"应用运行失败", "复制", "重新加载"}},
		{"zh-Hans", devBuildFailed, []string{"构建失败", "复制", "重新加载"}},
	} {
		t.Run(fmt.Sprintf("%s/%d", tc.locale, tc.kind), func(t *testing.T) {
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				return devOverlay(cx, devFailure{kind: tc.kind, Text: "details"}, func() {}, func() {})
			}, nsgotest.Environment(core.Environment{Locale: tc.locale}))
			for _, text := range append(tc.want, "details") {
				if !app.Exists("text:" + text) {
					t.Fatalf("the overlay does not show %q\n%s", text, app.Dump())
				}
			}
		})
	}
}

// The overlay follows the app's language as it changes, as the app's own words do.
func TestDevOverlayFollowsTheLanguage(t *testing.T) {
	appSpeaks(t, "en", "zh-Hans")
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return devOverlay(cx, devFailure{kind: devBuildFailed, Text: "details"}, func() {}, func() {})
	}, nsgotest.Environment(core.Environment{Locale: "en-US"}))
	if !app.Exists("text:Build failed") {
		t.Fatalf("the overlay is not in English\n%s", app.Dump())
	}
	app.SetEnvironment(core.Environment{Locale: "zh-CN"})
	if !app.Exists("text:构建失败") || !app.Exists("text:重新加载") {
		t.Fatalf("the overlay did not follow the language to Chinese\n%s", app.Dump())
	}
}

// An app whose catalog lacks Chinese shows the overlay in English on a device set to Chinese, as it shows NSGo's other words.
func TestDevOverlayKeepsTheLanguageTheAppShows(t *testing.T) {
	appSpeaks(t, "en")
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return devOverlay(cx, devFailure{kind: devAppFailed, Text: "details"}, func() {}, func() {})
	}, nsgotest.Environment(core.Environment{Locale: "zh-CN"}))
	if !app.Exists("text:The app failed") || !app.Exists("text:Copy") {
		t.Fatalf("the overlay is not in English\n%s", app.Dump())
	}
}

// appSpeaks gives the test's app a catalog of langs, as the code nsgo writes for an app's catalogs does.
func appSpeaks(t *testing.T, langs ...string) {
	i18n.SetApp(i18n.NewCatalog(langs[0], langs[1:]...))
	t.Cleanup(func() { i18n.SetApp(nil) })
}

// Command or Control+R (mod+r) posts the reload the overlay's Reload button does, over the overlay too, and an app that binds the key itself keeps it.
func TestDevReloadKey(t *testing.T) {
	var reloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_nsgo/reload" && r.Method == http.MethodPost {
			reloads.Add(1)
		}
	}))
	defer server.Close()
	devServer = server.URL
	t.Setenv("NSGO_DEV_SERVER", "")
	t.Cleanup(func() { devServer = "" })

	// waitFor waits for want reloads, then a while longer for any more, as each is posted off the engine's goroutine.
	waitFor := func(what string, want int32) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); reloads.Load() < want && time.Now().Before(deadline); {
			time.Sleep(5 * time.Millisecond)
		}
		time.Sleep(200 * time.Millisecond)
		if got := reloads.Load(); got != want {
			t.Fatalf("%s: %d reloads posted, want %d", what, got, want)
		}
	}

	app := nsgotest.Start(t, Wrap(func(*core.Scope) core.Node { return core.View{} }))
	app.Key("mod+r")
	waitFor("mod+r", 1)

	// An app that failed to render shows the overlay, a modal over it: the key still builds again.
	app = nsgotest.Start(t, Wrap(func(*core.Scope) core.Node { panic("render failed") }))
	app.Find("nsgo-dev-overlay")
	app.Key("mod+r")
	waitFor("mod+r over the overlay", 2)

	var own atomic.Int32
	app = nsgotest.Start(t, Wrap(func(cx *core.Scope) core.Node {
		cx.OnKey("mod+r", func() { own.Add(1) })
		return core.View{}
	}))
	app.Key("mod+r")
	waitFor("mod+r an app binds", 2)
	if own.Load() != 1 {
		t.Fatalf("the app's own mod+r ran %d times, want once", own.Load())
	}

	// An app whose application menu has an enabled item with the key keeps it too, as desktops match the keymap before the menu: the binding leaves the keymap while the item is enabled.
	var disabled *core.Signal[bool]
	chosen := 0
	app = nsgotest.Start(t, Wrap(func(cx *core.Scope) core.Node {
		disabled = core.NewSignal(cx, false)
		cx.Effect(func() {
			cx.SetApplicationMenu([]core.MenuItem{{Label: "File", Items: []core.MenuItem{
				{Label: "Run", Shortcut: "mod+r", Disabled: disabled.Get(), OnSelect: func() { chosen++ }},
			}}})
		})
		return core.View{}
	}), nsgotest.Environment(driver.Environment{OS: driver.OSMacOS, Idiom: driver.IdiomDesktop}))
	app.Key("mod+r")
	waitFor("mod+r the app's menu has", 2)
	if chosen != 1 {
		t.Fatalf("the menu's Run ran %d times, want once", chosen)
	}
	app.Post(func() { disabled.Set(true) })
	app.Settle()
	app.Key("mod+r")
	waitFor("mod+r the app's menu has disabled", 3)
}

// Command or Control+R builds again from every window of an app that opens windows, as from its main window: one opened before the main view and one after, and a window that binds the key itself keeps it, opened before or after (#446).
func TestDevReloadKeyInEveryWindow(t *testing.T) {
	var reloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_nsgo/reload" && r.Method == http.MethodPost {
			reloads.Add(1)
		}
	}))
	defer server.Close()
	devServer = server.URL
	t.Setenv("NSGO_DEV_SERVER", "")
	t.Cleanup(func() { devServer = "" })

	root := core.NewRoot(core.RootOptions{Post: func(func()) {}, Windows: &core.WindowOptions{Width: 400, Height: 300}})
	app := core.NewTree(root, core.ApproxTextEngine{})
	own := 0
	windows := map[string]*core.Window{}
	app.Mount(core.AppRoot(func(cx *core.Scope) {
		windows["Settings"] = cx.OpenWindow(core.WindowOptions{Title: "Settings"}, func(*core.Scope) core.Node { return core.View{} })
		windows["Preview"] = cx.OpenWindow(core.WindowOptions{Title: "Preview"}, func(cx *core.Scope) core.Node {
			cx.OnKey("mod+r", func() { own++ })
			return core.View{}
		})
		cx.SetMainView(Wrap(func(*core.Scope) core.Node { return core.View{} }))
		windows["Editor"] = cx.OpenWindow(core.WindowOptions{Title: "Editor"}, func(cx *core.Scope) core.Node {
			cx.OnKey("mod+r", func() { own++ })
			return core.View{}
		})
	}))
	app.Commit()
	// keymaps are the keymaps the windows were sent, by their titles.
	keymaps := map[string][]driver.KeyBinding{}
	trees := map[string]*core.Tree{}
	for _, s := range app.Surfaces() {
		for title, w := range windows {
			if s.Window() != w {
				continue
			}
			trees[title] = s
			if b := s.Commit(); b != nil {
				for _, it := range b.Items {
					if it.Kind == driver.ItemKeymap {
						keymaps[title] = it.Keys
					}
				}
			}
		}
	}
	press := func(title string) {
		t.Helper()
		id, _ := driver.FindKey(keymaps[title], []string{"mod+r"}, 0, false)
		if id == 0 {
			t.Fatalf("the %q window's keymap %+v has no mod+r", title, keymaps[title])
		}
		trees[title].Dispatch(driver.Event{Kind: driver.EventKey, Count: id})
	}
	waitFor := func(what string, want int32) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); reloads.Load() < want && time.Now().Before(deadline); {
			time.Sleep(5 * time.Millisecond)
		}
		time.Sleep(200 * time.Millisecond)
		if got := reloads.Load(); got != want {
			t.Fatalf("%s: %d reloads posted, want %d", what, got, want)
		}
	}
	press("Settings")
	waitFor("mod+r in a window opened before the main view", 1)
	press("Preview")
	press("Editor")
	waitFor("mod+r in windows that bind it", 1)
	if own != 2 {
		t.Fatalf("the windows' own mod+r ran %d times, want twice", own)
	}
}

// The app scope owns dev's reload binding, so it survives disposal of the main view while another app window keeps the process alive (#713).
func TestDevReloadKeySurvivesMainViewClose(t *testing.T) {
	var reloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_nsgo/reload" && r.Method == http.MethodPost {
			reloads.Add(1)
		}
	}))
	defer server.Close()
	devServer = server.URL
	t.Setenv("NSGO_DEV_SERVER", "")
	t.Cleanup(func() { devServer = "" })

	root := core.NewRoot(core.RootOptions{Post: func(func()) {}, Windows: &core.WindowOptions{Width: 400, Height: 300}})
	tree := core.NewTree(root, core.ApproxTextEngine{})
	defer root.Dispose()
	var settings, main *core.Window
	tree.Mount(core.AppRoot(WrapApp(func(cx *core.Scope) {
		cx.SetMainView(Wrap(func(*core.Scope) core.Node { return core.View{} }))
		settings = cx.OpenWindow(core.WindowOptions{Title: "Settings"}, func(*core.Scope) core.Node { return core.View{} })
	})))
	tree.Commit()
	main = root.MainWindow()
	if main == nil || settings == nil {
		t.Fatal("the app did not open its main and settings windows")
	}
	var settingsTree *core.Tree
	for _, s := range tree.Surfaces() {
		if s.Window() == settings {
			settingsTree = s
		}
	}
	if settingsTree == nil {
		t.Fatal("the app has no settings tree")
	}
	var keymap []driver.KeyBinding
	press := func(when string, want int32) {
		t.Helper()
		for _, it := range settingsTree.Commit().Items {
			if it.Kind == driver.ItemKeymap {
				keymap = it.Keys
			}
		}
		id, _ := driver.FindKey(keymap, []string{"mod+r"}, 0, false)
		if id == 0 {
			t.Fatalf("%s, the settings keymap %+v has no reload key", when, keymap)
		}
		settingsTree.Dispatch(driver.Event{Kind: driver.EventKey, Count: id})
		for deadline := time.Now().Add(5 * time.Second); reloads.Load() < want && time.Now().Before(deadline); {
			time.Sleep(5 * time.Millisecond)
		}
		// A second binding firing too would reload twice.
		time.Sleep(50 * time.Millisecond)
		if got := reloads.Load(); got != want {
			t.Fatalf("%s, reloads = %d, want %d", when, got, want)
		}
	}
	// The main view's own binding and the app scope's are both bound while the main view shows; one press reloads once.
	press("with the main view open", 1)
	main.Close()
	tree.Commit()
	if !slices.Contains(tree.Surfaces(), settingsTree) {
		t.Fatal("settings tree disappeared with the main window")
	}
	press("after the main view closed", 2)
}

// Wrap with -devtools, -highlight and -layout mounts DevTools' overlay and layout audit and answers DevTools commands (elements, layout) sent over the dev server's event stream.
func TestDevWrapOverlayAndDevToolsCommands(t *testing.T) {
	var (
		mu         sync.Mutex
		gotTypes   []string
		cmdCh      = make(chan string, 8)
		stopServer = make(chan struct{})
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-NSGo-Token") != "mp-token" {
			http.Error(w, "no token", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/_nsgo/events":
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			fmt.Fprint(w, "data: hello\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			if !r.URL.Query().Has("devtools") {
				select {
				case <-stopServer:
				case <-r.Context().Done():
				}
				return
			}
			for {
				select {
				case cmd := <-cmdCh:
					fmt.Fprintf(w, "data: devtools %s\n\n", cmd)
					if flusher != nil {
						flusher.Flush()
					}
				case <-stopServer:
					return
				case <-r.Context().Done():
					return
				}
			}
		case "/_nsgo/devtools/out":
			sc := bufio.NewScanner(r.Body)
			for sc.Scan() {
				line := sc.Text()
				for _, typ := range []string{`"type":"elements"`, `"type":"layout"`, `"type":"surfaces"`} {
					if strings.Contains(line, typ) {
						mu.Lock()
						gotTypes = append(gotTypes, typ)
						mu.Unlock()
					}
				}
			}
		}
	}))
	defer func() {
		close(stopServer)
		server.Close()
	}()

	prevServer, prevDevtools, prevHighlight, prevLayout, prevToken := devServer, devDevtools, devHighlight, devLayout, devtools.ServerToken
	prevStart, prevChannel := start, devChannel
	t.Setenv("NSGO_DEV_SERVER", "")
	devServer, devDevtools, devHighlight, devLayout, devtools.ServerToken = server.URL, "1", "1", "bounds,grid=8", "mp-token"
	devChannel = nil
	start = sync.OnceFunc(func() {
		startDevTools(server.URL)
		startDevOverlay(devHighlight == "1", parseDevLayout(devLayout))
	})
	t.Cleanup(func() {
		devServer, devDevtools, devHighlight, devLayout, devtools.ServerToken = prevServer, prevDevtools, prevHighlight, prevLayout, prevToken
		start, devChannel = prevStart, prevChannel
		engine.Trace, driver.Applied = nil, nil
		devtools.Enabled, driver.TraceCauses = false, false
		devtools.MountOverlay = nil
		devOverlays.Store(nil)
	})

	Start()
	app := nsgotest.Start(t, Wrap(func(*core.Scope) core.Node {
		return core.View{TestID: "content", Children: []core.Node{core.Text{Content: "Hello"}}}
	}))
	if !app.Exists(devtools.OverlayTestID) || !app.Exists("nsgo-layout-audit") || !app.Exists("nsgo-layout-readout") {
		t.Fatalf("DevTools overlay or layout audit missing\n%s", app.Dump())
	}

	surfaces := devtools.Surfaces()
	if len(surfaces) == 0 {
		t.Fatal("surface did not register with DevTools")
	}
	surface := surfaces[len(surfaces)-1]
	t.Cleanup(func() { devtools.Unregister(surface) })
	sid := surface.ID()
	cmdCh <- `{"op":"surfaces"}`
	cmdCh <- fmt.Sprintf(`{"op":"elements","surface":%d}`, sid)
	cmdCh <- fmt.Sprintf(`{"op":"watch-layout","surface":%d}`, sid)

	has := func(want string) bool {
		mu.Lock()
		defer mu.Unlock()
		return slices.Contains(gotTypes, want)
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		app.Settle()
		if has(`"type":"surfaces"`) && has(`"type":"elements"`) && has(`"type":"layout"`) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("DevTools messages received = %v, want surfaces, elements and layout", gotTypes)
}
