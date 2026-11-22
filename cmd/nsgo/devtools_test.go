package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// events follows an event stream at url, handing each data line to a channel.
func events(t *testing.T, url string) <-chan string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	out := make(chan string, 16)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				out <- data
			}
		}
	}()
	return out
}

func next(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case s := <-ch:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("no event")
		return ""
	}
}

// request sends s a request for path, with body as contentType when given and the token in tokenHeader when given.
func request(t *testing.T, s *devServer, method, path, contentType, body, token string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, s.url()+path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set(tokenHeader, token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestDevToolsServerPassesMessagesBothWays(t *testing.T) {
	s, err := startDevServer("", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	base := s.url()

	resp, err := http.Get(base + "/_nsgo/devtools/")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(page), "NSGo DevTools") || !strings.Contains(string(page), "devtools.js") {
		t.Fatalf("the page is %q", page)
	}

	// The app's messages reach every page, one event a line.
	pages := events(t, base+"/_nsgo/devtools/stream?nsgo-token="+s.token)
	if got := next(t, pages); got != `{"type":"connected"}` {
		t.Fatalf("the page's first event = %q", got)
	}
	resp = request(t, s, "POST", "/_nsgo/devtools/out", "application/x-ndjson", "{\"type\":\"commit\"}\n\n{\"type\":\"hot\"}\n", s.token)
	resp.Body.Close()
	if a, b := next(t, pages), next(t, pages); a != `{"type":"commit"}` || b != `{"type":"hot"}` {
		t.Fatalf("pages heard %q and %q", a, b)
	}

	// A page's command reaches the app as one devtools event.
	app := events(t, base+"/_nsgo/events?nsgo-token="+s.token)
	// The server greets an app once it follows its events.
	if got := next(t, app); got != "hello" {
		t.Fatalf("the app's first event = %q", got)
	}
	resp = request(t, s, "POST", "/_nsgo/devtools/command", "application/json", "{\n  \"op\": \"elements\",\n  \"surface\": 0\n}", s.token)
	resp.Body.Close()
	if got := next(t, app); got != `devtools {"op":"elements","surface":0}` {
		t.Fatalf("the app heard %q", got)
	}

	// Another site's page cannot drive the app or speak for it: it sends neither JSON without asking first, which the server never allows, nor its own origin.
	for _, c := range []struct {
		method, path, body, contentType, origin string
		want                                    int
	}{
		{"GET", "/_nsgo/devtools/command", "", "", "", http.StatusMethodNotAllowed},
		{"POST", "/_nsgo/devtools/command", "not json", "application/json", "", http.StatusBadRequest},
		{"GET", "/_nsgo/devtools/out", "", "", "", http.StatusMethodNotAllowed},
		{"POST", "/_nsgo/devtools/command", `{"op":"pick","on":true}`, "text/plain", "", http.StatusUnsupportedMediaType},
		{"POST", "/_nsgo/devtools/command", `{"op":"pick","on":true}`, "application/json", "https://evil.example", http.StatusForbidden},
		{"POST", "/_nsgo/devtools/command", `{"op":"surfaces"}`, "application/json; charset=utf-8", base, http.StatusOK},
		{"POST", "/_nsgo/devtools/out", `{"type":"step"}`, "text/plain", "", http.StatusUnsupportedMediaType},
		{"POST", "/_nsgo/devtools/out", `{"type":"step"}`, "application/x-ndjson", "null", http.StatusForbidden},
	} {
		req, _ := http.NewRequest(c.method, base+c.path, strings.NewReader(c.body))
		req.Header.Set(tokenHeader, s.token)
		if c.contentType != "" {
			req.Header.Set("Content-Type", c.contentType)
		}
		if c.origin != "" {
			req.Header.Set("Origin", c.origin)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != c.want {
			t.Errorf("%s %s %q from %q = %d, want %d", c.method, c.path, c.body, c.origin, resp.StatusCode, c.want)
		}
	}
}

// A phone is told the server's first address, which is the one the machine reaches other hosts through, not a virtual network's that comes first among the interfaces.
func TestDevServerTellsPhonesTheOutboundAddressFirst(t *testing.T) {
	out := outboundIP()
	if out == nil || !out.IsPrivate() {
		t.Skip("no private outbound address here")
	}
	s := &devServer{addr: &net.TCPAddr{IP: net.IPv4zero, Port: 8080}}
	urls := s.lanURLs()
	if len(urls) == 0 || urls[0] != "http://"+out.String()+":8080" {
		t.Fatalf("lanURLs = %v, want %s first", urls, out)
	}
}

// Whatever reads the app's state, the build's or the project's, or changes them, answers only the run's token, from this machine as from the network, and no other site's page reads it.
func TestDevServerTakesItsTokenOnly(t *testing.T) {
	s, err := startDevServer("", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	s.setBundles(map[string]string{"App.tsx": "export default 1"})
	// Another token of the same length, differing in its first digit.
	other := "1" + s.token[1:]
	if s.token[0] == '1' {
		other = "0" + s.token[1:]
	}
	for _, c := range []struct {
		method, path, contentType, body string
		want                            int
		stream                          bool
	}{
		{"GET", "/_nsgo/events", "", "", http.StatusOK, true},
		{"GET", "/_nsgo/bundle?entry=App.tsx", "", "", http.StatusOK, false},
		{"POST", "/_nsgo/reload", "", "", http.StatusOK, false},
		{"POST", "/_nsgo/trace", "text/plain", "", http.StatusOK, false},
		{"GET", "/_nsgo/devtools/stream", "", "", http.StatusOK, true},
		{"POST", "/_nsgo/devtools/out", "application/x-ndjson", "{\"type\":\"commit\"}\n", http.StatusOK, false},
		{"GET", "/_nsgo/devtools/project", "", "", http.StatusOK, false},
		{"POST", "/_nsgo/devtools/command", "application/json", `{"op":"surfaces"}`, http.StatusOK, false},
	} {
		for _, token := range []string{"", other, s.token[:len(s.token)-1], s.token + "0"} {
			resp := request(t, s, c.method, c.path, c.contentType, c.body, token)
			resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("%s %s with token %q = %d, want 401", c.method, c.path, token, resp.StatusCode)
			}
		}
		resp := request(t, s, c.method, c.path, c.contentType, c.body, s.token)
		if resp.StatusCode != c.want {
			t.Errorf("%s %s with the token = %d, want %d", c.method, c.path, resp.StatusCode, c.want)
		}
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("%s %s lets %q read it", c.method, c.path, got)
		}
		resp.Body.Close()
		// A page's EventSource sends no header: its streams take the token in the query.
		if c.stream {
			sep := "?"
			if strings.Contains(c.path, "?") {
				sep = "&"
			}
			if got := next(t, events(t, s.url()+c.path+sep+"nsgo-token="+s.token)); got == "" {
				t.Errorf("%s with the token in the query sent nothing", c.path)
			}
		}
	}
	// The pages' own files need no token: they carry none of the app's.
	if resp := request(t, s, "GET", "/_nsgo/devtools/", "", "", ""); resp.StatusCode != http.StatusOK {
		t.Errorf("the DevTools page without a token = %d", resp.StatusCode)
	} else {
		resp.Body.Close()
	}
}

// A request naming the server by another name, as a site whose name was pointed at this machine sends from its pages, is refused, token or not; the machine's addresses and names are its own.
func TestDevServerRefusesForeignHosts(t *testing.T) {
	s, err := startDevServer("", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	port := strconv.Itoa(s.addr.Port)
	name, _ := os.Hostname()
	for host, want := range map[string]int{
		"evil.example:" + port:     http.StatusForbidden,
		"evil.example":             http.StatusForbidden,
		"localhost.evil.example":   http.StatusForbidden,
		"127.0.0.1.nip.io:" + port: http.StatusForbidden,
		"127.0.0.1:" + port:        http.StatusOK,
		"localhost:" + port:        http.StatusOK,
		"LOCALHOST.:" + port:       http.StatusOK,
		"[::1]:" + port:            http.StatusOK,
		"[fe80::1%25en0]:" + port:  http.StatusOK,
		"192.168.1.20:" + port:     http.StatusOK,
		name + ":" + port:          http.StatusOK,
	} {
		for _, path := range []string{"/_nsgo/devtools/project", "/_nsgo/devtools/"} {
			req, _ := http.NewRequest("GET", s.url()+path, nil)
			req.Host = host
			req.Header.Set(tokenHeader, s.token)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != want {
				t.Errorf("%s for Host %q = %d, want %d", path, host, resp.StatusCode, want)
			}
		}
	}
}

// TestDevTokenStaysOffCommandLines: no command nsgo dev runs carries the run's token, as every user of the machine may read commands' arguments. A target that passes the app no environment gets it from a file the build adds to the app's main package rather than from go build's -ldflags, which the linker's arguments repeat, and a page opens in the browser without its address on a command line.
func TestDevTokenStaysOffCommandLines(t *testing.T) {
	srv, err := startDevServer("", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.close()
	dir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":  "module tokentest\n\ngo 1.26\n",
		"main.go": "package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n)\n\nfunc main() { fmt.Print(os.Getenv(\"NSGO_DEV_TOKEN\")) }\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := &project{Dir: dir, Name: "Token", ID: "com.example.token", Version: "1.0.0", devURL: srv.url(), devToken: srv.token}
	bin := filepath.Join(dir, "app")
	cmd, done, err := goBuildCommand(context.Background(), p, bin, nil, buildExtra{})
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range cmd.Args {
		if strings.Contains(arg, srv.token) {
			t.Errorf("go build's argument %q carries the run's token", arg)
		}
	}
	files, _ := filepath.Glob(filepath.Join(p.cache(), "devtoken-*.go"))
	if len(files) != 1 {
		t.Fatalf("the build's token files: %q", files)
	}
	if info, err := os.Stat(files[0]); err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("the token file's mode: %v, %v", info.Mode(), err)
	}
	// -x prints the commands go build runs, the compiler's and the linker's among them.
	cmd.Args = slices.Insert(cmd.Args, 2, "-x")
	var output strings.Builder
	cmd.Stdout, cmd.Stderr = &output, &output
	err = cmd.Run()
	done()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, output.String())
	}
	if strings.Contains(output.String(), srv.token) {
		t.Errorf("a command go build ran carries the run's token:\n%s", output.String())
	}
	if left, _ := filepath.Glob(filepath.Join(p.cache(), "devtoken-*.go")); len(left) != 0 {
		t.Errorf("the build left %q", left)
	}
	got, err := exec.Command(bin).Output()
	if err != nil || string(got) != srv.token {
		t.Errorf("the app's NSGO_DEV_TOKEN = %q, %v; want the run's token", got, err)
	}

	url := srv.pageURL(srv.url(), "/_nsgo/devtools/")
	open, file, err := openPage(url, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range open.Args {
		if strings.Contains(arg, srv.token) {
			t.Errorf("opening the page, argument %q carries the run's token", arg)
		}
	}
	var gives string
	if file == "" && open.Stdin != nil {
		b, _ := io.ReadAll(open.Stdin)
		gives = string(b)
	} else if file != "" {
		b, _ := os.ReadFile(file)
		gives = string(b)
		if info, err := os.Stat(file); err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Errorf("the opening page's mode: %v, %v", info.Mode(), err)
		}
	}
	if !strings.Contains(gives, url) {
		t.Errorf("opening the page gives the browser %q, not %s", gives, url)
	}
}

// TestDevPagesTakeTheTokenOutOfTheirAddress opens the DevTools page and the web target's page at the addresses nsgo dev prints, in a headless browser: each takes the run's token out of its address at once, and keeps it for the tab, so that after a reload it still follows the server and the DevTools page still sends the app its commands.
func TestDevPagesTakeTheTokenOutOfTheirAddress(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a browser")
	}
	if _, err := findBrowser(); err != nil {
		t.Skip(err)
	}
	s, err := startDevServer("", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	// The web target's page, with nothing to run but nsgo dev's script.
	dir := t.TempDir()
	for _, name := range []string{"exec.js", "page.css", "mount.js"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.Create(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	err = indexHTML.Execute(f, map[string]any{"Name": "Token", "Exec": "exec.js", "Style": "page.css", "Module": "mount.js", "Reload": true})
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	s.serve(dir)
	// A build failed, which the page shows once it follows the server.
	s.send("error", "the build failed")
	app := events(t, s.url()+"/_nsgo/events?nsgo-token="+s.token)
	if got := next(t, app); got != "hello" {
		t.Fatalf("the app's first event = %q", got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	b, err := startBrowser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.close()
	open := func(url string) *webPage {
		raw, err := b.call("", "Target.createTarget", map[string]any{"url": url})
		if err != nil {
			t.Fatal(err)
		}
		var target struct{ TargetID string }
		json.Unmarshal(raw, &target)
		raw, err = b.call("", "Target.attachToTarget", map[string]any{"targetId": target.TargetID, "flatten": true})
		if err != nil {
			t.Fatal(err)
		}
		var attached struct{ SessionID string }
		json.Unmarshal(raw, &attached)
		return &webPage{b: b, session: attached.SessionID}
	}
	eval := func(pg *webPage, expr string) any {
		t.Helper()
		raw, err := pg.call("Runtime.evaluate", map[string]any{"expression": expr, "returnByValue": true})
		if err != nil {
			t.Fatal(err)
		}
		var res struct{ Result struct{ Value any } }
		json.Unmarshal(raw, &res)
		return res.Result.Value
	}
	wait := func(pg *webPage, expr string) {
		t.Helper()
		for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
			if eval(pg, expr) == true {
				return
			}
		}
		t.Fatalf("the page at %v never had %s", eval(pg, "location.href"), expr)
	}
	// heard waits for the app to hear the DevTools page's first command, which it sends once it follows the app's messages.
	heard := func() {
		t.Helper()
		for {
			if got := next(t, app); strings.HasPrefix(got, "devtools ") && strings.Contains(got, `"surfaces"`) {
				return
			}
		}
	}
	tokenGone := fmt.Sprintf("!location.href.includes(%q) && sessionStorage.getItem(\"nsgo-token\") === %q", s.token, s.token)
	reload := func(pg *webPage) {
		t.Helper()
		eval(pg, "window.before = true")
		if _, err := pg.call("Page.reload", map[string]any{}); err != nil {
			t.Fatal(err)
		}
		wait(pg, "!window.before && document.readyState === 'complete'")
	}

	devtools := open(s.pageURL(s.url(), "/_nsgo/devtools/"))
	heard()
	wait(devtools, tokenGone)
	reload(devtools)
	heard()
	wait(devtools, tokenGone)

	failed := `document.body.innerText.includes("Build failed")`
	web := open(s.pageURL(s.url(), "/?nsgo-fps"))
	wait(web, failed)
	wait(web, tokenGone+` && location.search === "?nsgo-fps"`)
	reload(web)
	wait(web, failed)
	wait(web, tokenGone)

	// The overlay says what the app's Go gives it in the language the app shows.
	eval(web, `globalThis.nsgoDevMessages = {buildFailed: "构建失败", copy: "复制", reload: "重新加载"}`)
	s.send("error", "the build failed again")
	wait(web, `["构建失败", "复制", "重新加载", "the build failed again"].every((s) => document.body.innerText.includes(s)) && !document.body.innerText.includes("Build failed")`)
}
