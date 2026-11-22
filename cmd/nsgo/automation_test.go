package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/automation"
)

// startHub starts a dev server on any port with an automation hub.
func startHub(t *testing.T) *automationHub {
	t.Helper()
	srv, err := startDevServer("", 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.close)
	h, err := newAutomationHub(srv, "native")
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// ask posts a tool's request to h's bridge with token and headers, returning the response.
func ask(t *testing.T, h *automationHub, token string, header map[string]string, method string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(automation.Request{ID: 5, Method: method})
	req, _ := http.NewRequest(http.MethodPost, h.srv.url()+"/_nsgo/automation", bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// answerOf decodes a bridge response.
func answerOf(t *testing.T, resp *http.Response) automation.Answer {
	t.Helper()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the bridge answered %s", resp.Status)
	}
	var a automation.Answer
	if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
		t.Fatal(err)
	}
	return a
}

// postOut posts lines to the dev server's /_nsgo/devtools/out as the app does, with token when given.
func postOut(t *testing.T, h *automationHub, token string, lines ...string) {
	t.Helper()
	post, _ := http.NewRequest(http.MethodPost, h.srv.url()+"/_nsgo/devtools/out", strings.NewReader(strings.Join(lines, "\n")+"\n"))
	post.Header.Set("Content-Type", "application/x-ndjson")
	if token != "" {
		post.Header.Set(tokenHeader, token)
	}
	resp, err := http.DefaultClient.Do(post)
	if err != nil {
		t.Error(err)
		return
	}
	resp.Body.Close()
}

// fakeApp follows h's server's events with the run's token as an app's DevTools stream does, and answers each automation request with answer over /_nsgo/devtools/out, until ctx ends; it reports once it is connected.
func fakeApp(ctx context.Context, t *testing.T, h *automationHub, answer func(automation.Request) automation.Answer) (connected chan int) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		// The hub can receive an answer before its HTTP response reaches
		// postOut. Finish that request before startHub closes the server.
		<-done
	})
	connected = make(chan int, 1)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, h.srv.url()+"/_nsgo/events?devtools", nil)
	req.Header.Set(tokenHeader, h.srv.token)
	go func() {
		defer close(done)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			if ctx.Err() == nil {
				t.Errorf("connect the fake app: %v", err)
			}
			connected <- 0
			return
		}
		defer resp.Body.Close()
		connected <- resp.StatusCode
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			data, ok := strings.CutPrefix(sc.Text(), "data: automation ")
			if !ok || answer == nil {
				continue
			}
			var r automation.Request
			json.Unmarshal([]byte(data), &r)
			body, _ := json.Marshal(answer(r))
			postOut(t, h, h.srv.token, `{"type":"automation","surface":0,"data":`+string(body)+`}`)
		}
	}()
	return connected
}

func TestAutomationBridgeRefusesStrangers(t *testing.T) {
	h := startHub(t)
	if resp := ask(t, h, "", nil, automation.Status); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no token: %s", resp.Status)
	}
	if resp := ask(t, h, h.srv.token, nil, automation.Status); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("the run's token on the bridge: %s", resp.Status)
	}
	if resp := ask(t, h, h.bridgeToken, map[string]string{"Origin": "http://localhost"}, automation.Status); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a web page's request: %s", resp.Status)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/_nsgo/automation", strings.NewReader("{}"))
	req.RemoteAddr = "192.168.1.5:5000"
	req.Header.Set("Authorization", "Bearer "+h.bridgeToken)
	h.bridge(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("a request from the network: %d", rec.Code)
	}
	app, _ := http.NewRequest(http.MethodGet, h.srv.url()+"/_nsgo/events", nil)
	app.Header.Set(tokenHeader, h.bridgeToken)
	if resp, err := http.DefaultClient.Do(app); err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("an app with the tools' token: %v %v", resp, err)
	} else {
		resp.Body.Close()
	}
	if a := answerOf(t, ask(t, h, h.bridgeToken, nil, automation.Status)); a.Error != nil || a.ID != 5 {
		t.Errorf("status: %+v", a)
	}
}

func TestAutomationRelaysToTheApp(t *testing.T) {
	h := startHub(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app := fakeApp(ctx, t, h, func(r automation.Request) automation.Answer {
		return automation.Reply(r.ID, automation.WindowsResult{Windows: []automation.Window{{ID: 1, Title: r.Method}}}, nil)
	})
	if code := <-app; code != http.StatusOK {
		t.Fatalf("the app connected with %d", code)
	}
	a := answerOf(t, ask(t, h, h.bridgeToken, nil, automation.Windows))
	var r automation.WindowsResult
	if a.Error != nil || a.ID != 5 || json.Unmarshal(a.Result, &r) != nil || r.Windows[0].Title != "windows" {
		t.Fatalf("relayed %+v", a)
	}
	if second := fakeApp(ctx, t, h, nil); <-second != http.StatusConflict {
		t.Error("a second app was let in while one is connected")
	}
	// Events but the DevTools stream, as a page or the app's other listeners follow them, carry no request.
	plain, _ := http.NewRequestWithContext(ctx, http.MethodGet, h.srv.url()+"/_nsgo/events", nil)
	plain.Header.Set(tokenHeader, h.srv.token)
	if resp, err := http.DefaultClient.Do(plain); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("other events: %v %v", resp, err)
	} else {
		resp.Body.Close()
	}
	var s automation.StatusResult
	json.Unmarshal(answerOf(t, ask(t, h, h.bridgeToken, nil, automation.Status)).Result, &s)
	if !s.App || s.Run != 1 || s.Target != "native" {
		t.Errorf("status %+v", s)
	}
}

// TestAutomationAnswersReachTheAutomationAlone posts answers beside DevTools' messages: DevTools pages get theirs and never an answer, and an answer counts only from a post with the run's token.
func TestAutomationAnswersReachTheAutomationAlone(t *testing.T) {
	h := startHub(t)
	page := make(chan string, 16)
	h.srv.mu.Lock()
	h.srv.pages[page] = true
	h.srv.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := make(chan automation.Request, 1)
	app := fakeApp(ctx, t, h, func(r automation.Request) automation.Answer {
		requests <- r
		return automation.Answer{}
	})
	<-app
	done := make(chan automation.Answer, 1)
	go func() { done <- h.call(automation.Request{ID: 7, Method: automation.Windows}, time.Second) }()
	asked := <-requests
	forged, _ := json.Marshal(automation.Reply(asked.ID, automation.WindowsResult{Windows: []automation.Window{{Title: "forged"}}}, nil))
	postOut(t, h, "", `{"type":"automation","data":`+string(forged)+`}`)
	real, _ := json.Marshal(automation.Reply(asked.ID, automation.WindowsResult{Windows: []automation.Window{{Title: "real"}}}, nil))
	postOut(t, h, h.srv.token, `{"type":"automation","data":`+string(real)+`}`, `{"type":"elements","surface":0}`)
	a := <-done
	var r automation.WindowsResult
	if a.ID != 7 || json.Unmarshal(a.Result, &r) != nil || len(r.Windows) != 1 || r.Windows[0].Title != "real" {
		t.Fatalf("answered %+v %s", a, a.Result)
	}
	h.srv.mu.Lock()
	delete(h.srv.pages, page)
	h.srv.mu.Unlock()
	close(page)
	var got []string
	for m := range page {
		got = append(got, m)
	}
	if len(got) != 1 || !strings.Contains(got[0], `"elements"`) {
		t.Errorf("a DevTools page got %q", got)
	}
}

// TestAutomationAnswerOverTheLimitFails has the app answer with a line longer than the server takes: the request fails at once rather than when it times out.
func TestAutomationAnswerOverTheLimitFails(t *testing.T) {
	limit := maxAnswer
	maxAnswer = 1 << 10
	t.Cleanup(func() { maxAnswer = limit })
	h := startHub(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app := fakeApp(ctx, t, h, func(r automation.Request) automation.Answer {
		return automation.Reply(r.ID, automation.SnapshotResult{Text: strings.Repeat("x", 2<<10)}, nil)
	})
	<-app
	start := time.Now()
	a := h.call(automation.Request{ID: 3, Method: automation.Snapshot}, time.Second)
	if a.ID != 3 || a.Error == nil || a.Error.Code != automation.Failed || !strings.Contains(a.Error.Message, "lost") {
		t.Fatalf("answered %d bytes, error %v", len(a.Result), a.Error)
	}
	if took := time.Since(start); took > answerFor/2 {
		t.Errorf("the request failed after %v", took)
	}
}

func TestAutomationWithoutAnApp(t *testing.T) {
	h := startHub(t)
	if a := h.call(automation.Request{ID: 2, Method: automation.Windows}, 50*time.Millisecond); a.Error == nil || a.Error.Code != automation.NoApp || a.ID != 2 {
		t.Errorf("no app: %+v", a)
	}
	ctx, cancel := context.WithCancel(context.Background())
	app := fakeApp(ctx, t, h, nil)
	<-app
	done := make(chan automation.Answer, 1)
	go func() { done <- h.call(automation.Request{ID: 3, Method: automation.Windows}, time.Second) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case a := <-done:
		if a.Error == nil || a.Error.Code != automation.NoApp {
			t.Errorf("an app gone while asked: %+v", a)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a request waited on an app gone")
	}
}

func TestAutomationStatusAndLogs(t *testing.T) {
	h := startHub(t)
	h.srv.send("error", "main.go:3: undefined: x")
	h.setBuilding(true)
	var s automation.StatusResult
	json.Unmarshal(answerOf(t, ask(t, h, h.bridgeToken, nil, automation.Status)).Result, &s)
	if !s.Building || s.BuildError != "main.go:3: undefined: x" || s.App {
		t.Errorf("status %+v", s)
	}
	for i := range 2500 {
		h.log("line " + strconv.Itoa(i))
	}
	logs := func(n int) []string {
		params, _ := json.Marshal(automation.LogsParams{Lines: n})
		var r automation.LogsResult
		json.Unmarshal(h.answer(automation.Request{Method: automation.Logs, Params: params}).Result, &r)
		return r.Lines
	}
	if got := logs(0); len(got) != 200 || got[199] != "line 2499" {
		t.Errorf("the default tail: %d lines, last %q", len(got), got[len(got)-1])
	}
	if got := logs(5000); len(got) != logLimit || got[0] != "line 500" {
		t.Errorf("the whole ring: %d lines from %q", len(got), got[0])
	}
}

func TestAutomationSessionFile(t *testing.T) {
	h := startHub(t)
	dir := t.TempDir()
	if err := h.writeSession(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(sessionPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	// Windows reports writable files as 0666; chmod only controls read-only.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("the session file: %v %v", info, err)
	}
	s, err := readSession(dir)
	if err != nil || s.Token != h.bridgeToken || s.URL != h.srv.url() || s.PID != os.Getpid() {
		t.Fatalf("read %+v %v", s, err)
	}
	// Another live nsgo dev -automation keeps its session.
	other := startHub(t)
	pid, _ := idleProcess(t)
	data, _ := os.ReadFile(sessionPath(dir))
	theirs := bytes.Replace(data, []byte("pid: "+strconv.Itoa(os.Getpid())), []byte("pid: "+strconv.Itoa(pid)), 1)
	os.WriteFile(sessionPath(dir), theirs, 0o600)
	if err := other.writeSession(dir); !errors.Is(err, errSessionTaken) {
		t.Errorf("a second session over a live one: %v", err)
	}
	if got, err := os.ReadFile(sessionPath(dir)); err != nil || !bytes.Equal(got, theirs) {
		t.Fatalf("a second session replaced the live session: %v", err)
	}
	other.removeSession()
	if _, err := os.Stat(sessionPath(dir)); err != nil {
		t.Error("a hub removed a session it did not write")
	}
	// A session whose process ID another process took, after its nsgo dev ended unannounced, is stale: its server answers no more.
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	os.WriteFile(sessionPath(dir), bytes.Replace(theirs, []byte(h.srv.url()), []byte(gone.URL), 1), 0o600)
	if s, _ := readSession(dir); s == nil || s.URL != gone.URL || sessionLive(s) {
		t.Fatalf("a session no server answers is live: %+v", s)
	}
	if err := other.writeSession(dir); err != nil {
		t.Errorf("a session over a stale one: %v", err)
	}
	os.WriteFile(sessionPath(dir), data, 0o600)
	h.removeSession()
	if _, err := os.Stat(sessionPath(dir)); !os.IsNotExist(err) {
		t.Errorf("the session file outlived its hub: %v", err)
	}
}

func TestDevAutomationRefusesWhatItCannotDrive(t *testing.T) {
	p := &project{}
	p.Dev.Target = "native"
	if err := dev(context.Background(), p, devOptions{automation: true, release: true}); err == nil || !strings.Contains(err.Error(), "-release") {
		t.Errorf("-automation -release: %v", err)
	}
	p.Dev.Target = "android"
	if err := dev(context.Background(), p, devOptions{automation: true, release: true}); err == nil || !strings.Contains(err.Error(), "-release") {
		t.Errorf("-target android -automation -release: %v", err)
	}
	p.Dev.Target = "web"
	if err := dev(context.Background(), p, devOptions{automation: true}); err == nil || !strings.Contains(err.Error(), "macOS, Windows, Linux, Android and Apple devices and simulators so far") {
		t.Errorf("-automation -target web: %v", err)
	}
	for _, target := range []string{"native", "ios", "tvos", "watchos", "ios_simulator", "tvos_simulator", "watchos_simulator"} {
		if err := automationTargetError(target, "darwin", false); err != nil {
			t.Errorf("darwin %s: %v", target, err)
		}
		if err := automationTargetError(target, "darwin", true); err == nil || !strings.Contains(err.Error(), "-release") {
			t.Errorf("darwin %s -release: %v", target, err)
		}
		for _, goos := range []string{"linux", "windows"} {
			if goos == "windows" && target == "native" {
				if err := automationTargetError(target, goos, false); err != nil {
					t.Errorf("%s %s: %v", goos, target, err)
				}
				continue
			}
			if goos == "linux" && target == "native" {
				continue
			}
			if err := automationTargetError(target, goos, false); err == nil {
				t.Errorf("%s %s unexpectedly allowed", goos, target)
			}
		}
	}
	if err := automationTargetError("native", "linux", false); err != nil {
		t.Errorf("linux native: %v", err)
	}
	if err := automationTargetError("native", "linux", true); err == nil || !strings.Contains(err.Error(), "-release") {
		t.Errorf("linux native -release: %v", err)
	}
	for _, goos := range []string{"darwin", "linux", "windows"} {
		if err := automationTargetError("android", goos, false); err != nil {
			t.Errorf("%s android: %v", goos, err)
		}
		if err := automationTargetError("android", goos, true); err == nil || !strings.Contains(err.Error(), "-release") {
			t.Errorf("%s android -release: %v", goos, err)
		}
	}
	for _, target := range []string{"web", "miniprogram_wechat"} {
		if err := automationTargetError(target, "darwin", false); err == nil {
			t.Errorf("darwin %s unexpectedly allowed", target)
		}
	}
}

func TestSimulatorAutomationEnvAndLaunch(t *testing.T) {
	h := startHub(t)
	opts := devOptions{server: h.srv, automation: true, device: "test-sim"}
	gotEnv := simulatorEnv(opts)
	wantServer := "SIMCTL_CHILD_NSGO_DEV_SERVER=" + h.srv.url()
	wantToken := "SIMCTL_CHILD_NSGO_DEV_TOKEN=" + h.srv.token
	if !slices.Contains(gotEnv, wantServer) || !slices.Contains(gotEnv, wantToken) {
		t.Fatalf("simulatorEnv = %v, want %q and %q", gotEnv, wantServer, wantToken)
	}
	if runtime.GOOS == "windows" {
		t.Skip("the simulator launcher fixture uses a shell script")
	}
	for _, tc := range []struct {
		sys     *appleOS
		runtime string
	}{
		{iOS, "com.apple.CoreSimulator.SimRuntime.iOS-27-0"},
		{tvOS, "com.apple.CoreSimulator.SimRuntime.tvOS-27-0"},
		{watchOS, "com.apple.CoreSimulator.SimRuntime.watchOS-26-0"},
	} {
		t.Run(tc.sys.target+"_simulator", func(t *testing.T) {
			recorded := filepath.Join(t.TempDir(), "launch.env")
			t.Setenv("NSGO_RECORD_ENV", recorded)
			t.Setenv("NSGO_SIM_RUNTIME", tc.runtime)
			checkXcrun(t, `case "$2" in
list) printf '{"devices":{"%s":[{"udid":"test-sim","name":"Sim","state":"Booted"}]}}\n' "$NSGO_SIM_RUNTIME" ;;
bootstatus|install|terminate) exit 0 ;;
launch)
	printf 'ARGS=%s\nSERVER=%s\nTOKEN=%s\n' "$*" "$SIMCTL_CHILD_NSGO_DEV_SERVER" "$SIMCTL_CHILD_NSGO_DEV_TOKEN" > "$NSGO_RECORD_ENV"
	exit 0
	;;
*) exit 1 ;;
esac
`)
			stubOpen(t, `exit 0`)
			p := &project{Dir: t.TempDir(), Name: "Sample", ID: "dev.example.sample"}
			p.prepareDev(opts)
			app, err := iosSession(tc.sys, opts).start(t.Context(), p)
			if err != nil {
				t.Fatal(err)
			}
			if err := app.wait(t.Context()); err != nil {
				t.Fatal(err)
			}
			app.stop()
			data, err := os.ReadFile(recorded)
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			for _, want := range []string{
				"SERVER=" + h.srv.url(),
				"TOKEN=" + h.srv.token,
				"dev.example.sample",
			} {
				if !strings.Contains(text, want) {
					t.Errorf("launch output missing %q:\n%s", want, text)
				}
			}
		})
	}
}

func TestSimulatorAutomationBuildTags(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":  "module simauto\n\ngo 1.26\n",
		"main.go": "package main\nfunc main() {}\n",
	})
	for _, tc := range []struct {
		name       string
		automation bool
		extra      []string
		wantAuto   bool
		wantWatch  bool
	}{
		{"ios_dev", false, nil, false, false},
		{"ios_automation", true, nil, true, false},
		{"watchos_dev", false, []string{"nsgowatch"}, false, true},
		{"watchos_automation", true, []string{"nsgowatch"}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &project{Dir: dir, Name: "Sample", ID: "dev.example.sample"}
			p.prepareDev(devOptions{automation: tc.automation})
			cmd, done, err := goBuildCommand(t.Context(), p, filepath.Join(t.TempDir(), "out"), nil, buildExtra{tags: tc.extra})
			if err != nil {
				t.Fatal(err)
			}
			defer done()
			var tags string
			for i, a := range cmd.Args {
				if a == "-tags" && i+1 < len(cmd.Args) {
					tags = cmd.Args[i+1]
				}
			}
			parts := strings.Split(tags, ",")
			if !slices.Contains(parts, "nsgodev") {
				t.Errorf("tags %q missing nsgodev", tags)
			}
			if got := slices.Contains(parts, "nsgoautomation"); got != tc.wantAuto {
				t.Errorf("tags %q nsgoautomation=%v, want %v", tags, got, tc.wantAuto)
			}
			if got := slices.Contains(parts, "nsgowatch"); got != tc.wantWatch {
				t.Errorf("tags %q nsgowatch=%v, want %v", tags, got, tc.wantWatch)
			}
		})
	}
}

func TestAppleDeviceAutomationBuildAndLaunch(t *testing.T) {
	h := startHub(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":  "module devauto\n\ngo 1.26\n",
		"main.go": "package main\nfunc main() {}\n",
	})

	// Check ldflags (-X devServer), build tags and overlay token delivery (kept off command lines)
	// for iOS, tvOS and watchOS physical device automation builds.
	for _, tc := range []struct {
		sys       *appleOS
		extraTags []string
		wantWatch bool
	}{
		{iOS, nil, false},
		{tvOS, nil, false},
		{watchOS, []string{"nsgowatch"}, true},
	} {
		t.Run(tc.sys.target+"_build", func(t *testing.T) {
			p := &project{Dir: dir, Name: "Sample", ID: "dev.example.sample", Version: "1.0.0"}
			opts := devOptions{server: h.srv, automation: true, team: "TEAM123"}
			p.prepareDev(opts)
			// Use a synthetic LAN URL so the build assertion is independent of host network interfaces.
			p.devURL, p.devToken = "http://192.168.1.42:8080", h.srv.token
			cmd, done, err := goBuildCommand(t.Context(), p, filepath.Join(t.TempDir(), "out"), nil, buildExtra{tags: tc.extraTags})
			if err != nil {
				t.Fatal(err)
			}
			defer done()

			var tags, ldflags string
			for i, a := range cmd.Args {
				if strings.Contains(a, h.srv.token) {
					t.Errorf("go build argument %q leaked run token", a)
				}
				if a == "-tags" && i+1 < len(cmd.Args) {
					tags = cmd.Args[i+1]
				}
				if a == "-ldflags" && i+1 < len(cmd.Args) {
					ldflags = cmd.Args[i+1]
				}
			}
			parts := strings.Split(tags, ",")
			if !slices.Contains(parts, "nsgodev") || !slices.Contains(parts, "nsgoautomation") {
				t.Errorf("tags = %q, want nsgodev and nsgoautomation", tags)
			}
			if got := slices.Contains(parts, "nsgowatch"); got != tc.wantWatch {
				t.Errorf("tags = %q, nsgowatch=%v, want %v", tags, got, tc.wantWatch)
			}
			wantDevServer := "-X 'github.com/nsgo-dev/nsgo/internal/dev.devServer=http://192.168.1.42:8080'"
			if !strings.Contains(ldflags, wantDevServer) {
				t.Errorf("ldflags = %q, missing %q", ldflags, wantDevServer)
			}
			tokenFiles, _ := filepath.Glob(filepath.Join(p.cache(), "devtoken-*.go"))
			if len(tokenFiles) != 1 {
				t.Fatalf("devtoken overlay files = %v, want 1", tokenFiles)
			}
			data, err := os.ReadFile(tokenFiles[0])
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), h.srv.token) || !strings.Contains(string(data), "NSGO_DEV_TOKEN") {
				t.Errorf("devtoken overlay = %q, want NSGO_DEV_TOKEN with run token", data)
			}
		})
	}

	if got := appleDeviceLaunchArgs("UDID-1", "dev.example.sample", []string{"--nsgo-fps"}); !slices.Equal(got, []string{
		"devicectl", "device", "process", "launch", "--console", "--terminate-existing", "--device", "UDID-1", "dev.example.sample", "--nsgo-fps",
	}) {
		t.Fatalf("appleDeviceLaunchArgs = %v", got)
	}

	if runtime.GOOS == "windows" {
		t.Skip("the devicectl launcher fixture uses shell scripts")
	}

	// Verify iosDeviceSession.start runs Xcode signing, devicectl install and devicectl launch
	// for iPhone, Apple TV and Apple Watch.
	for _, tc := range []struct {
		sys      *appleOS
		platform string
		udid     string
		name     string
	}{
		{iOS, "iOS", "IPHONE-UDID", "Test iPhone"},
		{tvOS, "tvOS", "APPLETV-UDID", "Living Room"},
		{watchOS, "watchOS", "WATCH-UDID", "Test Watch"},
	} {
		t.Run(tc.sys.target+"_launch", func(t *testing.T) {
			projDir := t.TempDir()
			appDir := filepath.Join(projDir, ".nsgo", "dev", tc.sys.target, "app", "Sample.app")
			if err := os.MkdirAll(appDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(appDir, "Info.plist"), []byte("<plist/>"), 0o644); err != nil {
				t.Fatal(err)
			}
			commands := filepath.Join(t.TempDir(), "devicectl.log")
			t.Setenv("NSGO_DEVICECTL_LOG", commands)
			t.Setenv("NSGO_DEVICE_JSON", `{"result":{"devices":[{"deviceProperties":{"name":"`+tc.name+`","developerModeStatus":"enabled"},"hardwareProperties":{"reality":"physical","platform":"`+tc.platform+`","udid":"`+tc.udid+`"},"connectionProperties":{"pairingState":"paired","tunnelState":"connected"}}]}}`)
			bin := t.TempDir()
			writeFiles(t, bin, map[string]string{
				"xcrun": `#!/bin/sh
case "$1:$2:$3" in
devicectl:list:devices)
	printf '%s\n' "$NSGO_DEVICE_JSON"
	exit 0
	;;
devicectl:device:install|devicectl:device:process)
	printf '%s\n' "$*" >> "$NSGO_DEVICECTL_LOG"
	exit 0
	;;
*)
	exit 1
	;;
esac
`,
				"xcodebuild": `#!/bin/sh
for arg in "$@"; do
	case "$arg" in
	CONFIGURATION_BUILD_DIR=*)
		mkdir -p "${arg#CONFIGURATION_BUILD_DIR=}/Sample.app"
		;;
	esac
done
exit 0
`,
			})
			for _, tool := range []string{"xcrun", "xcodebuild"} {
				if err := os.Chmod(filepath.Join(bin, tool), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

			opts := devOptions{server: h.srv, automation: true, team: "TEAM123", fps: true}
			p := &project{Dir: projDir, Name: "Sample", ID: "dev.example.sample"}
			p.prepareDev(opts)
			running, err := iosDeviceSession(tc.sys, opts).start(t.Context(), p)
			if err != nil {
				t.Fatal(err)
			}
			if err := running.wait(t.Context()); err != nil {
				t.Fatal(err)
			}
			running.stop()

			logBytes, err := os.ReadFile(commands)
			if err != nil {
				t.Fatal(err)
			}
			logText := string(logBytes)
			for _, want := range []string{
				"devicectl device install app --device " + tc.udid,
				"devicectl device process launch --console --terminate-existing --device " + tc.udid + " dev.example.sample --nsgo-fps",
			} {
				if !strings.Contains(logText, want) {
					t.Errorf("devicectl log missing %q:\n%s", want, logText)
				}
			}
		})
	}
}

func TestAutomationSupportedTargets(t *testing.T) {
	cases := []struct {
		target, hostOS string
		want           bool
	}{
		{"native", "darwin", true},
		{"native", "linux", true},
		{"native", "windows", true},
		{"android", "darwin", true},
		{"android", "linux", true},
		{"android", "windows", true},
		{"web", "darwin", false},
		{"ios_simulator", "darwin", true},
		{"miniprogram_wechat", "darwin", false},
	}
	for _, tc := range cases {
		if got := automationSupported(tc.target, tc.hostOS); got != tc.want {
			t.Errorf("automationSupported(%q, %q) = %v, want %v", tc.target, tc.hostOS, got, tc.want)
		}
	}
}

func TestCheckDevAutomation(t *testing.T) {
	for _, tc := range []struct {
		target, goos string
		release      bool
		want         string
	}{
		{"native", "darwin", false, ""},
		{"native", "windows", false, ""},
		{"native", "linux", false, ""},
		{"native", "linux", true, "-release"},
		{"web", "darwin", false, "macOS, Windows, Linux, Android and Apple devices and simulators so far"},
		{"web", "windows", false, "macOS, Windows, Linux, Android and Apple devices and simulators so far"},
		{"native", "darwin", true, "-release"},
		{"native", "windows", true, "-release"},
	} {
		err := checkDevAutomation(tc.target, tc.goos, tc.release)
		if tc.want == "" {
			if err != nil {
				t.Errorf("checkDevAutomation(%q, %q, %v) = %v, want nil", tc.target, tc.goos, tc.release, err)
			}
		} else if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("checkDevAutomation(%q, %q, %v) = %v, want %q", tc.target, tc.goos, tc.release, err, tc.want)
		}
	}
}

func TestWindowsAutomationCrossBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-builds a Windows automation binary")
	}
	p, err := load(filepath.Join("..", "..", "testdata", "automation"))
	if err != nil {
		t.Fatal(err)
	}
	p.prepareDev(devOptions{automation: true})
	dir := t.TempDir()
	if err := buildWindows(context.Background(), p, dir, "arm64"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, p.Name+".exe"))
	if err != nil || info.Size() == 0 {
		t.Fatalf("Windows automation binary missing or empty: %v, %v", info, err)
	}
}
