package android

import (
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// javaNames are the driver's names of the kinds Java names otherwise.
var javaNames = map[string]string{"EventHoverIn": "EventMouseEnter", "EventHoverOut": "EventMouseLeave", "EventHoverMove": "EventMouseMove"}

// TestJavaProtocolMatchesTheDriver holds Protocol.java's item and event kinds, roles and key uses to the driver's of the same names, which the Java runtime reads the batches and writes the events by: a kind the driver gains shifts the ones after it.
func TestJavaProtocolMatchesTheDriver(t *testing.T) {
	testhost.RequireSource(t)
	fset := token.NewFileSet()
	dir := filepath.Join("..", "..", "driver")
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, f := range pkgs["driver"].Files {
		files = append(files, f)
	}
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	pkg, err := conf.Check("github.com/nsgo-dev/nsgo/internal/driver", fset, files, nil)
	if err != nil {
		t.Fatal(err)
	}
	java, err := os.ReadFile(filepath.Join("java", "dev", "oneo", "nsgo", "Protocol.java"))
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, m := range regexp.MustCompile(`static final int (ITEM|EVENT|ROLE|KEY_USE|ORIENTATION)_(\w+) = (\d+);`).FindAllStringSubmatch(string(java), -1) {
		var name string
		for _, word := range strings.Split(m[1]+"_"+m[2], "_") {
			if word == "URL" {
				name += word
				continue
			}
			name += word[:1] + strings.ToLower(word[1:])
		}
		if n, ok := javaNames[name]; ok {
			name = n
		}
		c, ok := pkg.Scope().Lookup(name).(*types.Const)
		if !ok {
			t.Errorf("Protocol.java's %s_%s is no driver constant %s", m[1], m[2], name)
			continue
		}
		want, _ := constant.Int64Val(c.Val())
		if got, _ := strconv.Atoi(m[3]); int64(got) != want {
			t.Errorf("Protocol.java's %s_%s is %d, the driver's %s %d", m[1], m[2], got, name, want)
		}
		checked++
	}
	if checked < 50 {
		t.Fatalf("checked only %d constants", checked)
	}
}

// TestOverlayProtocolMatchesJava holds Protocol.java's overlay constants to Go's: the window commands writeWindow sends, what a SURFACE message reports, and the driver's permissions and ends; and why a video paused.
func TestOverlayProtocolMatchesJava(t *testing.T) {
	testhost.RequireSource(t)
	java, err := os.ReadFile(filepath.Join("java", "dev", "oneo", "nsgo", "Protocol.java"))
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]int{}
	for _, m := range regexp.MustCompile(`static final int (\w+) = (\d+);`).FindAllStringSubmatch(string(java), -1) {
		values[m[1]], _ = strconv.Atoi(m[2])
	}
	for name, want := range map[string]int{
		"WINDOW_NONE": windowNone, "WINDOW_FRAME": windowFrame, "WINDOW_SHOW": windowShow, "WINDOW_HIDE": windowHide,
		"WINDOW_FRONT": windowFront, "WINDOW_DESTROY": windowDestroy,
		"SURFACE_STATE": surfaceState, "SURFACE_ENDED": surfaceEnded,
		"OVERLAY_DENIED": driver.OverlayDenied, "OVERLAY_GRANTED": driver.OverlayGranted, "OVERLAY_UNAVAILABLE": driver.OverlayUnavailable,
		"OVERLAY_END_DENIED": driver.OverlayEndDenied, "OVERLAY_END_UNAVAILABLE": driver.OverlayEndUnavailable,
		"OVERLAY_END_REVOKED": driver.OverlayEndRevoked, "OVERLAY_END_REMOVED": driver.OverlayEndRemoved,
		"OVERLAY_END_FAILED":       driver.OverlayEndFailed,
		"VIDEO_PAUSE_INTERRUPTION": driver.VideoPauseInterruption, "VIDEO_PAUSE_ROUTE_LOST": driver.VideoPauseRouteLost,
		"VIDEO_PAUSE_CONTROLS": driver.VideoPauseControls,
	} {
		if got, ok := values[name]; !ok || got != want {
			t.Errorf("Protocol.java's %s is %d (found %v), Go's %d", name, got, ok, want)
		}
	}
}

// TestOverlayItemsEncode checks the fields ItemOpenSurface and ItemWindow send, in the order Overlays and NsgoView.apply read them.
func TestOverlayItemsEncode(t *testing.T) {
	var w writer
	writeSurface(&w, &driver.SurfaceConfig{ID: 3, Kind: driver.SurfaceOverlay, Width: 200, Height: 100,
		Frame: driver.WindowFrame{X: 10, Y: 20, Move: true}, Focusable: true, Accessible: true})
	r := reader{w.buf}
	if id, width, height, x, y, flags := r.u32(), r.points(), r.points(), r.points(), r.points(), r.byte(); id != 3 ||
		width != 200 || height != 100 || x != 10 || y != 20 || flags != 1|2|8|32 || len(r.buf) != 0 {
		t.Errorf("surface %d %v %v %v %v %b, %d left", id, width, height, x, y, flags, len(r.buf))
	}
	for _, c := range []struct {
		cmd   driver.WindowCommand
		op    byte
		flags byte
	}{
		{driver.WindowCommand{Action: driver.WindowSetFrame, Frame: driver.WindowFrame{X: 5, Y: 6, Move: true}}, windowFrame, 1},
		{driver.WindowCommand{Action: driver.WindowSetFrame, Frame: driver.WindowFrame{Width: 50, Height: 60}}, windowFrame, 2},
		{driver.WindowCommand{Action: driver.WindowShow}, windowShow, 0},
		{driver.WindowCommand{Action: driver.WindowHide}, windowHide, 0},
		{driver.WindowCommand{Action: driver.WindowOrderFront}, windowFront, 0},
		{driver.WindowCommand{Action: driver.WindowDestroy}, windowDestroy, 0},
		{driver.WindowCommand{Action: driver.WindowMinimize}, windowNone, 0},
	} {
		var w writer
		writeWindow(&w, &c.cmd)
		r := reader{w.buf}
		op := r.byte()
		x, y, width, height := r.points(), r.points(), r.points(), r.points()
		flags := r.byte()
		f := c.cmd.Frame
		if op != c.op || flags != c.flags || x != f.X || y != f.Y || width != f.Width || height != f.Height || len(r.buf) != 0 {
			t.Errorf("%v: op %d flags %b frame %v %v %v %v", c.cmd.Action, op, flags, x, y, width, height)
		}
	}
}

// TestJavaMessagesMatchTheBackend holds Protocol.java's message types for package microphone's recordings and the web views' cookies to android.go's, which carry them both ways.
func TestJavaMessagesMatchTheBackend(t *testing.T) {
	testhost.RequireSource(t)
	read := func(path ...string) string {
		data, err := os.ReadFile(filepath.Join(path...))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	goSource, java := read("android.go"), read("java", "dev", "oneo", "nsgo", "Protocol.java")
	for goName, javaName := range map[string]string{"msgRecording": "RECORDING", "msgRecordingReport": "RECORDING_REPORT", "msgCookies": "COOKIES", "msgCookiesSet": "COOKIES_SET", "msgCanvasImage": "CANVAS_IMAGE", "msgCanvasRequest": "CANVAS_REQUEST", "msgPixelFrame": "PIXEL_FRAME", "msgDiskSpace": "DISK_SPACE", "msgDiskSpaceReply": "DISK_SPACE_REPLY"} {
		g := regexp.MustCompile(`\b` + goName + ` += (\d+)`).FindStringSubmatch(goSource)
		j := regexp.MustCompile(`static final int ` + javaName + ` = (\d+);`).FindStringSubmatch(java)
		if g == nil || j == nil || g[1] != j[1] {
			t.Errorf("android.go's %s is %v, Protocol.java's %s %v", goName, g, javaName, j)
		}
	}
}

// TestWebCommandsMatchJava holds Protocol.java's web commands to the driver's, which WebNode.command switches on: WebSendInput, which Android does without, sits between them.
func TestWebCommandsMatchJava(t *testing.T) {
	testhost.RequireSource(t)
	java, err := os.ReadFile(filepath.Join("java", "dev", "oneo", "nsgo", "Protocol.java"))
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]int{}
	for _, m := range regexp.MustCompile(`static final int (WEB_\w+) = (\d+);`).FindAllStringSubmatch(string(java), -1) {
		values[m[1]], _ = strconv.Atoi(m[2])
	}
	for name, want := range map[string]driver.WebCommand{
		"WEB_RELOAD": driver.WebReload, "WEB_GO_BACK": driver.WebGoBack, "WEB_GO_FORWARD": driver.WebGoForward,
		"WEB_STOP_LOADING": driver.WebStopLoading, "WEB_POST_MESSAGE": driver.WebPostMessage,
		"WEB_INJECT_JAVASCRIPT": driver.WebInjectJavaScript, "WEB_LOAD": driver.WebLoad,
	} {
		if got, ok := values[name]; !ok || got != int(want) {
			t.Errorf("Protocol.java's %s is %d (found %v), the driver's %d", name, got, ok, want)
		}
	}
}

// TestWebCookieOpsMatchJava holds Protocol.java's COOKIES_* ops to the driver's WebCookiesOp, which writeWebCookies sends and NsgoView.cookies switches on.
func TestWebCookieOpsMatchJava(t *testing.T) {
	testhost.RequireSource(t)
	java, err := os.ReadFile(filepath.Join("java", "dev", "oneo", "nsgo", "Protocol.java"))
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]int{}
	for _, m := range regexp.MustCompile(`static final int (COOKIES_(?:ADD|LIST|CLEAR)) = (\d+);`).FindAllStringSubmatch(string(java), -1) {
		values[m[1]], _ = strconv.Atoi(m[2])
	}
	for name, want := range map[string]driver.WebCookiesOp{"COOKIES_ADD": driver.WebCookiesSet, "COOKIES_LIST": driver.WebCookiesList, "COOKIES_CLEAR": driver.WebCookiesClear} {
		if got, ok := values[name]; !ok || got != int(want) {
			t.Errorf("Protocol.java's %s is %d (found %v), the driver's %d", name, got, ok, want)
		}
	}
}

// TestWebCookiesMessages writes a msgCookies of each op as Java reads it, and reads msgCookiesSet answers as Java writes them: a list's Cookie header becomes the cookies of the URL's host, names and values alone.
func TestWebCookiesMessages(t *testing.T) {
	var w writer
	writeWebCookies(&w, 3, 9, &driver.WebCookiesRequest{Op: driver.WebCookiesList, Filter: driver.WebCookieFilter{URL: "https://www.example.com/watch"}})
	r := reader{w.buf}
	if surface, count, op, u := r.u32(), r.u32(), r.byte(), r.str(); surface != 3 || count != 9 || op != byte(driver.WebCookiesList) || u != "https://www.example.com/watch" || len(r.buf) != 0 {
		t.Errorf("list: surface %d, count %d, op %d, URL %q, %d bytes left", surface, count, op, u, len(r.buf))
	}
	w = writer{}
	writeWebCookies(&w, 3, 10, &driver.WebCookiesRequest{Op: driver.WebCookiesClear})
	r = reader{w.buf}
	if surface, count, op := r.u32(), r.u32(), r.byte(); surface != 3 || count != 10 || op != byte(driver.WebCookiesClear) || len(r.buf) != 0 {
		t.Errorf("clear: surface %d, count %d, op %d, %d bytes left", surface, count, op, len(r.buf))
	}
	w = writer{}
	c := driver.WebCookie{Name: "SID", Value: "a", Domain: ".example.com", Secure: true}
	writeWebCookies(&w, 3, 11, &driver.WebCookiesRequest{Cookies: []driver.WebCookie{c}})
	r = reader{w.buf}
	if surface, count, op, n, u, value := r.u32(), r.u32(), r.byte(), r.u32(), r.str(), r.str(); surface != 3 || count != 11 || op != byte(driver.WebCookiesSet) || n != 1 || u != c.URL() || value != c.SetCookie() {
		t.Errorf("set: surface %d, count %d, op %d, %d cookies, %q %q", surface, count, op, n, u, value)
	}

	answer := func(text string, op driver.WebCookiesOp, list ...string) driver.Event {
		var w writer
		w.u32(3)
		w.u32(9)
		w.str(text)
		w.byte(byte(op))
		for _, s := range list {
			w.str(s)
		}
		surface, ev := readWebCookiesSet(&reader{w.buf})
		if surface != 3 || ev.Kind != driver.EventWebCookies || ev.Count != 9 || ev.Text != text {
			t.Errorf("answer to surface %d: %+v", surface, ev)
		}
		return ev
	}
	ev := answer("", driver.WebCookiesList, "https://WWW.example.com/watch", "SID=a=b; pref=1;  bare")
	want := []driver.WebCookie{{Name: "SID", Value: "a=b", Domain: "www.example.com"}, {Name: "pref", Value: "1", Domain: "www.example.com"}, {Value: "bare", Domain: "www.example.com"}}
	if got, _ := ev.Data.([]driver.WebCookie); !slices.Equal(got, want) {
		t.Errorf("list answered %+v, want %+v", ev.Data, want)
	}
	if got, ok := answer("", driver.WebCookiesList, "https://example.com/", "").Data.([]driver.WebCookie); !ok || len(got) != 0 {
		t.Errorf("an empty header answered %#v, want no cookies", got)
	}
	if ev := answer("Android has no WebView", driver.WebCookiesList, "https://example.com/", ""); ev.Data != nil {
		t.Errorf("a failed list answered %+v", ev.Data)
	}
	if ev := answer("", driver.WebCookiesClear); ev.Data != nil {
		t.Errorf("a clear answered %+v", ev.Data)
	}
}

// TestSpokenWordsMatchJava holds Protocol.java's SPOKEN_* indexes to the order spoken writes the words in: each English word is its constant's name.
func TestSpokenWordsMatchJava(t *testing.T) {
	testhost.RequireSource(t)
	java, err := os.ReadFile(filepath.Join("java", "dev", "oneo", "nsgo", "Protocol.java"))
	if err != nil {
		t.Fatal(err)
	}
	words := spoken("en")
	found := 0
	for _, m := range regexp.MustCompile(`static final int SPOKEN_(\w+) = (\d+);`).FindAllStringSubmatch(string(java), -1) {
		i, _ := strconv.Atoi(m[2])
		want := strings.ToLower(strings.ReplaceAll(m[1], "_", " "))
		if m[1] == "SEPARATOR" {
			want = ", "
		}
		if i >= len(words) || words[i] != want {
			t.Errorf("Protocol.java's SPOKEN_%s is %d, which spoken says as %q", m[1], i, words[min(i, len(words)-1)])
		}
		found++
	}
	if found != len(words) {
		t.Errorf("Protocol.java has %d SPOKEN_ indexes for spoken's %d words", found, len(words))
	}
}

// TestDirectionSaysTheWordsInTheAppsLanguage checks that ItemDirection carries TalkBack's words in the language the app shows, as NsgoView.apply reads them after the direction.
func TestDirectionSaysTheWordsInTheAppsLanguage(t *testing.T) {
	english := spoken("en")
	for _, c := range []struct {
		lang      string
		rtl       bool
		searchBox string
		partly    string
		separator string
	}{
		{"en", false, "search box", "partially checked", ", "},
		{"zh-Hans", false, "搜索框", "部分选中", "，"},
		{"zh-CN", false, "搜索框", "部分选中", "，"},
		{"de", true, "search box", "partially checked", ", "},
	} {
		var w writer
		writeDirection(&w, &driver.MountItem{Kind: driver.ItemDirection, RTL: c.rtl, Language: c.lang})
		r := reader{w.buf}
		if rtl := r.byte(); rtl != map[bool]byte{false: 0, true: 1}[c.rtl] {
			t.Errorf("%s: direction %d", c.lang, rtl)
		}
		words := make([]string, r.u32())
		for i := range words {
			words[i] = r.str()
		}
		if len(r.buf) != 0 || len(words) != len(english) {
			t.Fatalf("%s: %d words, %d bytes left", c.lang, len(words), len(r.buf))
		}
		if words[2] != c.searchBox || words[13] != c.partly || words[len(words)-1] != c.separator {
			t.Errorf("%s: a search box is %q, partially checked %q and separator %q", c.lang, words[2], words[13], words[len(words)-1])
		}
		for i, s := range words {
			if s == "" || c.searchBox != english[2] && s == english[i] {
				t.Errorf("%s: word %d is %q", c.lang, i, s)
			}
		}
	}
}

func TestReadEnvironmentSetsCanRequestReview(t *testing.T) {
	for _, more := range []byte{0, 4} {
		var w writer
		for range 7 {
			w.float(1)
		}
		w.byte(0) // flags
		w.byte(more)
		w.byte(byte(driver.IdiomPhone))
		w.str("en-US")
		w.str("Pixel")
		w.str("Google Pixel 9")
		w.float(56) // HeaderHeight
		w.float(96) // LargeHeaderHeight
		w.u32(35)   // API level

		env := readEnvironment(&reader{w.buf})
		if want := more != 0; env.CanRequestReview != want {
			t.Errorf("readEnvironment with capability byte %d: CanRequestReview = %v, want %v", more, env.CanRequestReview, want)
		}
	}
}

// TestReadEnvironmentMapsPowerState: NsgoView writes PowerManager's thermal status plus one, 0 before API 29, and the power-save mode as 0 unknown, 1 off or 2 on.
func TestReadEnvironmentMapsPowerState(t *testing.T) {
	for _, c := range []struct {
		thermal, saver byte
		state          driver.ThermalState
		mode           driver.LowPowerMode
	}{
		{0, 0, driver.ThermalUnknown, driver.LowPowerUnknown},
		{1, 1, driver.ThermalNominal, driver.LowPowerOff},
		{2, 2, driver.ThermalFair, driver.LowPowerOn},
		{3, 1, driver.ThermalFair, driver.LowPowerOff},
		{4, 1, driver.ThermalSerious, driver.LowPowerOff},
		{5, 1, driver.ThermalCritical, driver.LowPowerOff},
		{7, 1, driver.ThermalCritical, driver.LowPowerOff},
		{8, 3, driver.ThermalUnknown, driver.LowPowerUnknown},
	} {
		var w writer
		for range 7 {
			w.float(1)
		}
		w.byte(0) // flags
		w.byte(0) // capabilities
		w.byte(byte(driver.IdiomPhone))
		w.str("en-US")
		w.str("Pixel")
		w.str("Google Pixel 9")
		w.float(56) // HeaderHeight
		w.float(96) // LargeHeaderHeight
		w.u32(35)   // API level
		w.byte(c.thermal)
		w.byte(c.saver)

		env := readEnvironment(&reader{w.buf})
		if env.ThermalState != c.state || env.LowPowerMode != c.mode {
			t.Errorf("readEnvironment with power bytes %d, %d = %v, %v; want %v, %v", c.thermal, c.saver, env.ThermalState, env.LowPowerMode, c.state, c.mode)
		}
	}
}

// TestReadEventLongPressIndex verifies that msgEvent decodes EventLongPress (and EventPress) character byte offsets, including -1 for misses and touchless long presses, and that NsgoView reports pressedIndex for every EVENT_LONG_PRESS emission.
func TestReadEventLongPressIndex(t *testing.T) {
	encodeEvent := func(surface uint32, kind driver.EventKind, id driver.NodeID, count uint32, pt, page driver.Point, index int32, mods byte) []byte {
		var w writer
		w.u32(surface)
		w.byte(byte(kind))
		w.u32(uint32(id))
		w.u32(count)
		w.points(pt.X)
		w.points(pt.Y)
		w.str("") // text
		w.points(0)
		w.points(0)
		w.u32(uint32(index))
		w.byte(0) // phase
		w.points(0)
		w.points(0)
		w.float(0) // value
		w.byte(0)  // no navigation
		w.u64(0)   // time
		w.u64(0)   // duration
		w.byte(0)  // buffering
		w.str("")  // data
		w.byte(0)  // finished
		w.points(page.X)
		w.points(page.Y)
		w.points(0)
		w.points(0)
		w.byte(mods)
		return w.buf
	}
	for _, want := range []int{-1, 0, 5, 14} {
		raw := encodeEvent(0, driver.EventLongPress, 42, 1, driver.Point{X: 18, Y: 9}, driver.Point{X: 30, Y: 40}, int32(want), 4)
		r := reader{raw}
		from, ev, data := readEvent(&r)
		if len(r.buf) != 0 || from != 0 || data != "" {
			t.Fatalf("index %d: from=%d data=%q remaining=%d", want, from, data, len(r.buf))
		}
		if ev.Kind != driver.EventLongPress || ev.ID != 42 || ev.Count != 1 || ev.Index != want ||
			ev.Point != (driver.Point{X: 18, Y: 9}) || ev.Page != (driver.Point{X: 30, Y: 40}) || !ev.Modifiers.Shift {
			t.Errorf("index %d: decoded %+v", want, ev)
		}
	}

	testhost.RequireSource(t)
	java, err := os.ReadFile(filepath.Join("java", "dev", "oneo", "nsgo", "NsgoView.java"))
	if err != nil {
		t.Fatal(err)
	}
	calls := regexp.MustCompile(`pressed\(n,\s*Protocol\.EVENT_LONG_PRESS,\s*([^;]+)\);`).FindAllStringSubmatch(string(java), -1)
	if len(calls) < 2 {
		t.Fatalf("expected at least 2 EVENT_LONG_PRESS emissions in NsgoView.java (system and custom delay), found %d", len(calls))
	}
	for _, m := range calls {
		if strings.TrimSpace(m[1]) != "pressedIndex(v)" {
			t.Errorf("NsgoView.java emits EVENT_LONG_PRESS with %q, want pressedIndex(v)", m[1])
		}
	}
}
