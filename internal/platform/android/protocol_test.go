package android

import (
	"fmt"
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

func TestNetworkEnvironmentFlags(t *testing.T) {
	for _, tc := range []struct {
		name   string
		bits   byte
		want   *driver.Network
		review bool
	}{
		{"unknown", 0, nil, false},
		{"review only", 4, nil, true},
		{"offline", 8, &driver.Network{}, false},
		{"online", 8 | 16, &driver.Network{Online: true}, false},
		{"metered and review", 4 | 8 | 16 | 32, &driver.Network{Online: true, Expensive: true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var w writer
			for range 7 {
				w.points(0)
			}
			w.byte(0)
			w.byte(tc.bits)
			env := readEnvironment(&reader{buf: w.buf})
			if env.CanRequestReview != tc.review {
				t.Errorf("review support = %v, want %v", env.CanRequestReview, tc.review)
			}
			got := env.Network
			if (got == nil) != (tc.want == nil) || got != nil && *got != *tc.want {
				t.Fatalf("network flags %08b decoded as %v, want %v", tc.bits, got, tc.want)
			}
		})
	}
}

// TestJavaProtocolMatchesTheDriver holds Protocol.java's item and event kinds, roles and key uses to the driver's of the same names, which the Java runtime reads the batches and writes the events by: a kind the driver gains shifts the ones after it.
func TestJavaProtocolMatchesTheDriver(t *testing.T) {
	for _, problem := range javaProtocolKindErrors(readJavaProtocol(t), driverProtocolConstants(t)) {
		t.Error(problem)
	}
}

func driverProtocolConstants(t *testing.T) *types.Package {
	t.Helper()
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
	return pkg
}

// Match only Java's declarations: Android need not implement every driver kind.
func javaProtocolKindErrors(source string, pkg *types.Package) []string {
	var problems []string
	declarations := regexp.MustCompile(`\bstatic\s+final\s+int\s+((?:ITEM|EVENT|ROLE|KEY_USE|ORIENTATION)_[A-Z0-9_]+)\s*=\s*([^;]+);`).FindAllStringSubmatch(source, -1)
	if len(declarations) == 0 {
		return []string{"Protocol.java has no driver constant declarations"}
	}
	for _, m := range declarations {
		var name string
		for _, word := range strings.Split(m[1], "_") {
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
			problems = append(problems, fmt.Sprintf("Protocol.java's %s is no driver constant %s", m[1], name))
			continue
		}
		want, _ := constant.Int64Val(c.Val())
		got, err := strconv.ParseInt(strings.TrimSpace(m[2]), 10, 64)
		if err != nil {
			problems = append(problems, fmt.Sprintf("Protocol.java's %s: invalid kind number %q: %v", m[1], m[2], err))
		} else if got != want {
			problems = append(problems, fmt.Sprintf("Protocol.java's %s is %d, the driver's %s %d", m[1], got, name, want))
		}
	}
	return problems
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
		"BORDER_DASHED":        int(driver.BorderDashed), "BORDER_DOTTED": int(driver.BorderDotted),
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

// TestJavaMessagesMatchTheBackend holds Protocol.java's message types to android.go's.
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
	for goName, javaName := range map[string]string{"msgRecording": "RECORDING", "msgAssetReply": "ASSET_REPLY", "msgRecordingReport": "RECORDING_REPORT", "msgCookies": "COOKIES", "msgCookiesSet": "COOKIES_SET", "msgCanvasImage": "CANVAS_IMAGE", "msgCanvasRequest": "CANVAS_REQUEST", "msgPixelFrame": "PIXEL_FRAME", "msgDiskSpace": "DISK_SPACE", "msgDiskSpaceReply": "DISK_SPACE_REPLY", "msgPerfPanel": "PERF_PANEL", "msgPerfFrame": "PERF_FRAME", "msgDiscoveryReply": "DISCOVERY_REPLY", "msgShaderRelease": "SHADER_RELEASE"} {
		g := regexp.MustCompile(`\b` + goName + ` += (\d+)`).FindStringSubmatch(goSource)
		j := regexp.MustCompile(`static final int ` + javaName + ` = (\d+);`).FindStringSubmatch(java)
		if g == nil || j == nil || g[1] != j[1] {
			t.Errorf("android.go's %s is %v, Protocol.java's %s %v", goName, g, javaName, j)
		}
	}
	for name, op := range map[string]driver.DiscoveryOp{
		"DISCOVERY_ADVERTISE": driver.DiscoveryAdvertise, "DISCOVERY_BROWSE": driver.DiscoveryBrowse,
		"DISCOVERY_STOP": driver.DiscoveryStop,
	} {
		match := regexp.MustCompile(`static final int ` + name + ` = (\d+);`).FindStringSubmatch(java)
		if match == nil || match[1] != strconv.Itoa(int(op)) {
			t.Errorf("Protocol.java's %s is %v, the driver's %d", name, match, op)
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
		switch m[1] {
		case "SEPARATOR":
			want = ", "
		case "HEADING_LEVEL":
			want = "heading level {level}"
		case "LEVEL":
			want = "level {level}"
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

// TestDirectionSaysTheWordsInTheAppsLanguage checks that ItemDirection carries TalkBack's words in the language the app shows and the forced language tag, as NsgoView.apply reads them after the direction.
func TestDirectionSaysTheWordsInTheAppsLanguage(t *testing.T) {
	english := spoken("en")
	for _, c := range []struct {
		lang       string
		forced     bool
		wantTag    string
		rtl        bool
		searchBox  string
		partly     string
		separator  string
		spinButton string
		headingLvl string
		level      string
	}{
		{"en", false, "", false, "search box", "partially checked", ", ", "spin button", "heading level {level}", "level {level}"},
		{"zh-Hans", true, "zh-Hans", false, "搜索框", "部分选中", "，", "微调按钮", "{level} 级标题", "第 {level} 级"},
		{"zh-CN", true, "zh-CN", false, "搜索框", "部分选中", "，", "微调按钮", "{level} 级标题", "第 {level} 级"},
		{"de", false, "", true, "search box", "partially checked", ", ", "spin button", "heading level {level}", "level {level}"},
	} {
		var w writer
		writeDirection(&w, &driver.MountItem{Kind: driver.ItemDirection, RTL: c.rtl, Language: c.lang, Forced: c.forced})
		r := reader{w.buf}
		if rtl := r.byte(); rtl != map[bool]byte{false: 0, true: 1}[c.rtl] {
			t.Errorf("%s: direction %d", c.lang, rtl)
		}
		words := make([]string, r.u32())
		for i := range words {
			words[i] = r.str()
		}
		gotForced := r.str()
		if len(r.buf) != 0 || len(words) != len(english) {
			t.Fatalf("%s: %d words, %d bytes left", c.lang, len(words), len(r.buf))
		}
		if gotForced != c.wantTag {
			t.Errorf("%s (forced=%v): forced tag = %q, want %q", c.lang, c.forced, gotForced, c.wantTag)
		}
		if words[2] != c.searchBox || words[13] != c.partly || words[16] != c.separator || words[17] != c.spinButton || words[18] != c.headingLvl || words[19] != c.level {
			t.Errorf("%s: searchBox=%q partly=%q sep=%q spinButton=%q headingLevel=%q level=%q", c.lang, words[2], words[13], words[16], words[17], words[18], words[19])
		}
		for i, s := range words {
			if s == "" || c.searchBox != english[2] && s == english[i] {
				t.Errorf("%s: word %d is %q", c.lang, i, s)
			}
		}
	}
}

func TestForcedLanguageEncoding(t *testing.T) {
	for _, tc := range []struct {
		lang   string
		forced bool
		want   string
	}{
		{"de", false, ""},
		{"", true, ""},
		{"   ", true, ""},
		{"de", true, "de"},
		{"zh_Hans", true, "zh-Hans"},
		{"  pt_BR ", true, "pt-BR"},
		{"en-XA", true, ""},
		{"ar-XB", true, ""},
		{"en_xa", true, ""},
		{"AR-xb", true, ""},
	} {
		got := forcedLanguage(&driver.MountItem{Language: tc.lang, Forced: tc.forced})
		if got != tc.want {
			t.Errorf("forcedLanguage(%q, forced=%v) = %q, want %q", tc.lang, tc.forced, got, tc.want)
		}
	}
}

// TestViewPropsEncodesLevelAndSpinButton checks that writeViewProps writes RoleSpinButton and Level (clamping negatives to 0) in the exact order NsgoView.props reads them (#1594).
func TestViewPropsEncodesLevelAndSpinButton(t *testing.T) {
	labels := []driver.NodeID{4, 9}
	actions := []driver.AccessibilityAction{{Name: driver.ActionIncrement}, {Name: driver.ActionDecrement}}
	for _, c := range []struct {
		name      string
		props     driver.ViewProps
		wantRole  driver.Role
		wantLevel uint32
	}{
		{"spin button", driver.ViewProps{Role: driver.RoleSpinButton, Label: "Count", Actions: &actions, Value: driver.AccessibilityValue{Min: 0, Max: 10, Now: 3, Text: "3"}, LabelledBy: &labels}, driver.RoleSpinButton, 0},
		{"heading level", driver.ViewProps{Role: driver.RoleHeader, Label: "Section", Level: 3}, driver.RoleHeader, 3},
		{"tree item level", driver.ViewProps{Role: driver.RoleTreeItem, Label: "Child", Level: 2}, driver.RoleTreeItem, 2},
		{"negative level clamped", driver.ViewProps{Role: driver.RoleHeader, Level: -4}, driver.RoleHeader, 0},
	} {
		var w writer
		writeViewProps(&w, &c.props)
		r := reader{w.buf}
		role := driver.Role(r.byte())
		_ = r.byte() // flags
		_, _ = r.str(), r.str()
		_, _, _, _ = r.points(), r.points(), r.points(), r.points()
		_ = r.str() // help
		_ = r.byte()
		_, _ = r.byte(), r.byte()
		_, _, _ = r.float(), r.float(), r.float()
		_ = r.str() // valueText
		for range r.u32() {
			_, _ = r.str(), r.str()
		}
		_ = r.byte()
		_, _, _, _ = r.points(), r.points(), r.points(), r.points()
		_ = r.u32()
		if r.byte() != 0 {
			_, _ = r.u32(), r.points()
		}
		_, _, _ = r.byte(), r.byte(), r.u32()
		_, _ = r.byte(), r.u32()
		for range r.u32() {
			_ = r.u32()
		}
		level := r.u32()
		if role != c.wantRole || level != c.wantLevel || len(r.buf) != 0 {
			t.Errorf("%s: role=%d (want %d), level=%d (want %d), %d bytes left", c.name, role, c.wantRole, level, c.wantLevel, len(r.buf))
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
		if want := more&4 != 0; env.CanRequestReview != want {
			t.Errorf("readEnvironment with capability byte %d: CanRequestReview = %v, want %v", more, env.CanRequestReview, want)
		}
		if want := more&8 != 0; env.DiscoverServices != want {
			t.Errorf("readEnvironment with capability byte %d: DiscoverServices = %v, want %v", more, env.DiscoverServices, want)
		}
	}
}

// TestReadEnvironmentSeparatesSpeechFromAudio: NsgoView writes text to speech as bit 1 of the second capability byte and audio playback as capAudio in the third, so a device without a speech engine still plays sounds, a speaking one does not claim audio by itself, the online bit 16 of the second byte says nothing of audio, and a runtime built before the third byte reads as having no audio.
func TestReadEnvironmentSeparatesSpeechFromAudio(t *testing.T) {
	for _, c := range []struct {
		more         byte
		third        []byte
		speak, audio bool
	}{
		{0, nil, false, false},
		{1, nil, true, false},
		{8 | 16, []byte{capExtendedMessages}, false, false},
		{0, []byte{capAudio}, false, true},
		{1, []byte{capExtendedMessages | capAudio}, true, true},
	} {
		var w writer
		w.floats(0, 0, 0, 0, 0, 1, 2.5)
		w.byte(0) // flags
		w.byte(c.more)
		w.byte(byte(driver.IdiomPhone))
		w.str("en-US")
		w.str("Pixel")
		w.str("Google Pixel 9")
		w.floats(56, 96) // HeaderHeight, LargeHeaderHeight
		w.u32(35)        // API level
		w.byte(0)        // thermal state
		w.byte(0)        // low-power mode
		if c.third != nil {
			w.byte(1)
			w.str("h23")
			w.str("metric")
			w.str("celsius")
			w.str("US")
			w.buf = append(w.buf, c.third...)
		}

		env := readEnvironment(&reader{w.buf})
		if env.Speak != c.speak || env.Audio != c.audio || env.AudioPlaybackControls != c.audio {
			t.Errorf("readEnvironment with capability bytes %d and %v: Speak %v, Audio %v, AudioPlaybackControls %v; want %v, %v, %v", c.more, c.third, env.Speak, env.Audio, env.AudioPlaybackControls, c.speak, c.audio, c.audio)
		}
	}

	testhost.RequireSource(t)
	java, err := os.ReadFile(filepath.Join("java", "dev", "oneo", "nsgo", "Protocol.java"))
	if err != nil {
		t.Fatal(err)
	}
	view, err := os.ReadFile(filepath.Join("java", "dev", "oneo", "nsgo", "NsgoView.java"))
	if err != nil {
		t.Fatal(err)
	}
	if m := regexp.MustCompile(`static final int CAP_AUDIO = (\d+);`).FindSubmatch(java); m == nil || string(m[1]) != strconv.Itoa(capAudio) {
		t.Errorf("Protocol.java's CAP_AUDIO is %q, android.go's capAudio %d", m, capAudio)
	}
	if m := regexp.MustCompile(`int third = ([^;]*);`).FindSubmatch(view); m == nil || !strings.Contains(string(m[1]), "Protocol.CAP_AUDIO") {
		t.Errorf("NsgoView.environment's third capability byte is %q, without Protocol.CAP_AUDIO", m)
	}
}

func TestReadEnvironmentHapticPatternsUseIndependentFlag(t *testing.T) {
	// Timed haptics take the third capability byte's capHapticPatterns bit, apart from the second byte's network bits and the runtime's other capabilities.
	for _, third := range []byte{0, capExtendedMessages, capHapticPatterns, capHapticPatterns | capDiscoverServices, 255 &^ capHapticPatterns} {
		var w writer
		writeBaseEnvironment(&w, "en-US")
		w.buf[7*4+1] = 8 | 16 | 32
		w.byte(1)
		w.str("h23")
		w.str("metric")
		w.str("celsius")
		w.str("US")
		w.byte(third)
		env, _ := readEnvironmentCapabilities(&reader{w.buf})
		if want := third&capHapticPatterns != 0; env.HapticPatterns != want {
			t.Errorf("third capability byte %d: HapticPatterns = %v, want %v", third, env.HapticPatterns, want)
		}
		if env.Network == nil || !env.Network.Online || !env.Network.Expensive {
			t.Errorf("third capability byte %d: Network = %+v, want online and expensive", third, env.Network)
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

// TestWriteEditingEncodesPadding checks that writeEditing appends InputConfig.Padding
// (top, right, bottom, left) in the order InputField.configure reads it (#1598).
func TestWriteEditingEncodesPadding(t *testing.T) {
	pad := driver.Edges{Top: 18, Right: 20, Bottom: 22, Left: 12}
	in := &driver.InputConfig{
		Multiline:        true,
		MaxLength:        250,
		SubmitBehavior:   driver.SubmitBehaviorNewline,
		ReturnKey:        driver.ReturnKeySend,
		AutoCapitalize:   driver.AutoCapitalizeWords,
		NoAutoCorrect:    true,
		HideCaret:        false,
		AutoComplete:     driver.AutoCompleteEmail,
		PlaceholderColor: driver.RGB(0x667788),
		SelectionColor:   driver.RGB(0x112233),
		PasteBehavior:    driver.PasteReport,
		Padding:          pad,
	}
	var w writer
	writeEditing(&w, in)
	r := reader{w.buf}
	if flags := r.byte(); flags != 1 {
		t.Fatalf("multiline flags = %d, want 1", flags)
	}
	if maxLen := r.u32(); maxLen != 250 {
		t.Fatalf("maxLength = %d, want 250", maxLen)
	}
	if submit, ret, capMode := r.byte(), r.byte(), r.byte(); submit != byte(in.Submit()) || ret != byte(driver.ReturnKeySend) || capMode != byte(driver.AutoCapitalizeWords) {
		t.Fatalf("submit=%d returnKey=%d cap=%d", submit, ret, capMode)
	}
	if flags := r.byte(); flags != 1 {
		t.Fatalf("editing flags = %d, want 1", flags)
	}
	if hint := r.str(); hint != "emailAddress" {
		t.Fatalf("autofill hint = %q, want emailAddress", hint)
	}
	if ph, sel := r.u32(), r.u32(); ph != uint32(in.PlaceholderColor) || sel != uint32(in.Caret()) {
		t.Fatalf("colors ph=%#x sel=%#x", ph, sel)
	}
	if paste := r.byte(); paste != byte(driver.PasteReport) {
		t.Fatalf("paste = %d, want %d", paste, driver.PasteReport)
	}
	got := driver.Edges{Top: r.points(), Right: r.points(), Bottom: r.points(), Left: r.points()}
	if got != pad || len(r.buf) != 0 {
		t.Fatalf("padding = %+v (%d bytes left), want %+v", got, len(r.buf), pad)
	}
}

// TestWriteScrollConfigEncodesEdges checks that writeScrollConfig appends the
// four scroll edge effects (top, right, bottom, left) after HeaderInset in the
// order Scrolling.Config.read decodes them (#1608).
func TestWriteScrollConfigEncodesEdges(t *testing.T) {
	topTint, topLine := driver.RGB(0xfafafa), driver.RGBA(0x000000, 0.1)
	botTint, botLine := driver.RGB(0x111111), driver.RGBA(0xffffff, 0.07)
	cfg := &driver.ScrollConfig{
		Behavior: driver.ScrollBehavior{
			SnapToOffsets: []driver.Points{100, 200},
			ContentInset:  driver.Edges{Top: 95, Bottom: 34},
		},
		MinZoomScale: 1,
		MaxZoomScale: 1,
		ZoomScale:    1,
		HeaderInset:  95,
		Edges: [4]driver.ScrollEdge{
			// Top: Automatic with positive extent -> drawn soft (1).
			{Effect: driver.EdgeEffectAutomatic, Extent: 71, Tint: topTint, Line: topLine},
			// Right: Hidden even with extent -> drawn none (0).
			{Effect: driver.EdgeEffectHidden, Extent: 40, Tint: topTint, Line: topLine},
			// Bottom: Hard with positive extent -> drawn hard (2).
			{Effect: driver.EdgeEffectHard, Extent: 56, Tint: botTint, Line: botLine},
			// Left: Soft with 0 extent -> drawn none (0).
			{Effect: driver.EdgeEffectSoft, Extent: 0, Tint: topTint, Line: topLine},
		},
	}
	var w writer
	writeScrollConfig(&w, cfg)
	r := reader{w.buf}
	// Skip header fields up to HeaderInset to verify full stream alignment.
	_, _, _ = r.byte(), r.byte(), r.byte() // flags
	_ = r.points()                         // SnapToInterval
	_ = r.byte()                           // SnapToAlignment
	if n := r.u32(); n != 2 {
		t.Fatalf("snap offsets = %d, want 2", n)
	}
	_, _ = r.points(), r.points()
	_ = r.float() // Deceleration
	inTop, _, inBot, _ := r.points(), r.points(), r.points(), r.points()
	if inTop != 95 || inBot != 34 {
		t.Fatalf("insets = (%v, %v), want (95, 34)", inTop, inBot)
	}
	_, _ = r.byte(), r.byte() // keyboard dismiss, persist taps
	_, _, _ = r.float(), r.float(), r.float()
	_ = r.points() // RefreshIndicator
	if hi := r.points(); hi != 95 {
		t.Fatalf("headerInset = %v, want 95", hi)
	}
	// Top: soft (1)
	if mode, ext, tint, line := r.byte(), r.points(), driver.Color(r.u32()), driver.Color(r.u32()); mode != 1 || ext != 71 || tint != topTint || line != topLine {
		t.Fatalf("top edge = (mode=%d ext=%v tint=%#x line=%#x)", mode, ext, tint, line)
	}
	// Right: none (0)
	if mode := r.byte(); mode != 0 {
		t.Fatalf("right edge mode = %d, want 0", mode)
	}
	// Bottom: hard (2)
	if mode, ext, tint, line := r.byte(), r.points(), driver.Color(r.u32()), driver.Color(r.u32()); mode != 2 || ext != 56 || tint != botTint || line != botLine {
		t.Fatalf("bottom edge = (mode=%d ext=%v tint=%#x line=%#x)", mode, ext, tint, line)
	}
	// Left: none (0)
	if mode := r.byte(); mode != 0 {
		t.Fatalf("left edge mode = %d, want 0", mode)
	}
	if len(r.buf) != 0 {
		t.Fatalf("%d unexpected trailing bytes", len(r.buf))
	}
}

// TestReadEventSelectionChange checks that EventSelectionChange from an Android selection area
// decodes its text and area-relative box (and zero size/empty text when cleared) and that
// NsgoView gates emission on Protocol.EVENT_SELECTION_CHANGE and scales by density (#1555).
func TestReadEventSelectionChange(t *testing.T) {
	encode := func(surface uint32, id driver.NodeID, text string, box driver.Bounds) []byte {
		var w writer
		w.u32(surface)
		w.byte(byte(driver.EventSelectionChange))
		w.u32(uint32(id))
		w.u32(0) // count
		w.points(box.Origin.X)
		w.points(box.Origin.Y)
		w.str(text)
		w.points(box.Size.Width)
		w.points(box.Size.Height)
		w.u32(0)  // index
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
		w.points(0)
		w.points(0)
		w.points(0)
		w.points(0)
		w.byte(0) // mods
		return w.buf
	}
	cases := []struct {
		name    string
		surface uint32
		id      driver.NodeID
		text    string
		box     driver.Bounds
	}{
		{
			name: "selected", surface: 0, id: 17, text: "alpha\nbeta",
			box: driver.Bounds{Origin: driver.Point{X: 12.5, Y: 24}, Size: driver.Size{Width: 160, Height: 48}},
		},
		{
			name: "cleared", surface: 2, id: 17, text: "",
			box: driver.Bounds{},
		},
	}
	for _, tc := range cases {
		r := reader{encode(tc.surface, tc.id, tc.text, tc.box)}
		from, ev, data := readEvent(&r)
		if len(r.buf) != 0 || from != tc.surface || data != "" {
			t.Fatalf("%s: from=%d data=%q remaining=%d", tc.name, from, data, len(r.buf))
		}
		if ev.Kind != driver.EventSelectionChange || ev.ID != tc.id || ev.Text != tc.text ||
			ev.Point != tc.box.Origin || ev.Size != tc.box.Size || ev.Count != 0 || ev.Index != 0 || ev.Ranges != nil {
			t.Errorf("%s: decoded %+v, want text=%q box=%+v", tc.name, ev, tc.text, tc.box)
		}
	}

	testhost.RequireSource(t)
	java, err := os.ReadFile(filepath.Join("java", "dev", "oneo", "nsgo", "NsgoView.java"))
	if err != nil {
		t.Fatal(err)
	}
	want := `n.has(Protocol.EVENT_SELECTION_CHANGE)`
	if !strings.Contains(string(java), want) {
		t.Errorf("NsgoView.java missing %q gate for selection reports", want)
	}
	wantCall := `event(Protocol.EVENT_SELECTION_CHANGE, n.id, 0, x / density, y / density, text, width / density, height / density, 0)`
	if !strings.Contains(string(java), wantCall) {
		t.Errorf("NsgoView.java missing %q", wantCall)
	}
}
