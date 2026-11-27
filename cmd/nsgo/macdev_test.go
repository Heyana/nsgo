package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMacDevUpdatesBinaryInPlaceOnGoOnlyChange(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS .app bundle build needs darwin codesign")
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":             "module macdevprobe\n\ngo 1.26\n",
		"nsgo.yaml":          "name: Probe\nid: dev.nsgo.macdevprobe\nversion: 1.0.0\n",
		"main.go":            "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"v1\") }\n",
		"resources/note.txt": "r1",
	})
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	p.prepareDev(devOptions{})
	outDir := filepath.Join(t.TempDir(), "macos")
	app := filepath.Join(outDir, p.Name+".app")
	bin := filepath.Join(app, "Contents", "MacOS", p.Name)
	plist := filepath.Join(app, "Contents", "Info.plist")
	res := filepath.Join(app, "Contents", "Resources", "note.txt")

	var bundle macDevBundle
	inPlace, err := bundle.build(t.Context(), p, outDir, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	if inPlace {
		t.Fatal("first build reported inPlace = true")
	}
	plistBefore, err := os.Stat(plist)
	if err != nil {
		t.Fatal(err)
	}
	resBefore, err := os.Stat(res)
	if err != nil {
		t.Fatal(err)
	}
	binBefore, err := os.Stat(bin)
	if err != nil {
		t.Fatal(err)
	}
	if err := run(t.Context(), "codesign", "--verify", "--strict", app); err != nil {
		t.Fatalf("first build signature: %v", err)
	}

	// Go-only edit: must update Contents/MacOS/<name> in place with a new inode
	// while preserving Info.plist and Resources untouched.
	writeFiles(t, dir, map[string]string{
		"main.go": "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"v2\") }\n",
	})
	inPlace, err = bundle.build(t.Context(), p, outDir, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	if !inPlace {
		t.Fatal("Go-only edit did not update the .app bundle in place")
	}
	plistAfter, err := os.Stat(plist)
	if err != nil {
		t.Fatal(err)
	}
	resAfter, err := os.Stat(res)
	if err != nil {
		t.Fatal(err)
	}
	binAfter, err := os.Stat(bin)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(plistBefore, plistAfter) {
		t.Error("Go-only edit rewrote Info.plist instead of updating in place")
	}
	if !os.SameFile(resBefore, resAfter) {
		t.Error("Go-only edit recopied Resources instead of updating in place")
	}
	if os.SameFile(binBefore, binAfter) {
		t.Error("Go-only edit overwrote the running binary's inode instead of swapping atomically")
	}
	if err := run(t.Context(), "codesign", "--verify", "--strict", app); err != nil {
		t.Fatalf("in-place build signature: %v", err)
	}
	out, err := exec.Command(bin).Output()
	if err != nil || strings.TrimSpace(string(out)) != "v2" {
		t.Fatalf("updated binary output = %q, err = %v; want v2", out, err)
	}

	// A Go compile error must keep the previous working .app bundle intact.
	writeFiles(t, dir, map[string]string{
		"main.go": "package main\nfunc main() { invalid }\n",
	})
	if _, err := bundle.build(t.Context(), p, outDir, runtime.GOARCH); err == nil {
		t.Fatal("invalid Go code unexpectedly built")
	}
	out, err = exec.Command(bin).Output()
	if err != nil || strings.TrimSpace(string(out)) != "v2" {
		t.Fatalf("failed Go build broke previous bundle: output = %q, err = %v", out, err)
	}

	// Editing a bundled resource must fall back to a full bundle rebuild.
	writeFiles(t, dir, map[string]string{
		"main.go":            "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"v3\") }\n",
		"resources/note.txt": "r2-updated",
	})
	inPlace, err = bundle.build(t.Context(), p, outDir, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	if inPlace {
		t.Fatal("resource edit did not trigger a full .app rebuild")
	}
	gotRes, err := os.ReadFile(res)
	if err != nil || string(gotRes) != "r2-updated" {
		t.Fatalf("rebuilt resource = %q, err = %v; want r2-updated", gotRes, err)
	}

	// Editing nsgo.yaml must also fall back to a full bundle rebuild.
	writeFiles(t, dir, map[string]string{
		"nsgo.yaml": "name: Probe\nid: dev.nsgo.macdevprobe\nversion: 1.0.1\n",
	})
	p, err = load(dir)
	if err != nil {
		t.Fatal(err)
	}
	p.prepareDev(devOptions{})
	inPlace, err = bundle.build(t.Context(), p, outDir, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	if inPlace {
		t.Fatal("nsgo.yaml edit did not trigger a full .app rebuild")
	}
	plistBytes, err := os.ReadFile(plist)
	if err != nil || !strings.Contains(string(plistBytes), "1.0.1") {
		t.Fatalf("rebuilt Info.plist missing new version 1.0.1: %v", err)
	}
}

func TestDevServerStateHandoffAndOverlapReady(t *testing.T) {
	srv, err := startDevServer("127.0.0.1", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.close()

	// Unauthenticated state access must be refused.
	resp, err := http.Get(srv.url() + "/_nsgo/state")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /_nsgo/state = %d, want 401", resp.StatusCode)
	}

	// Authenticated POST merges keys and GET returns them.
	postState := func(payload string) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, srv.url()+"/_nsgo/state", strings.NewReader(payload))
		req.Header.Set(tokenHeader, srv.token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("POST /_nsgo/state = %d, want 204", resp.StatusCode)
		}
	}
	postState(`{"a":1,"b":"two"}`)
	postState(`{"b":"updated","c":[true]}`)

	req, _ := http.NewRequest(http.MethodGet, srv.url()+"/_nsgo/state", nil)
	req.Header.Set(tokenHeader, srv.token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		resp.Body.Close()
		t.Fatal(err)
	}
	resp.Body.Close()
	if string(got["a"]) != "1" || string(got["b"]) != `"updated"` || !bytes.Equal(got["c"], []byte("[true]")) {
		t.Fatalf("state = %v, want merged keys a=1, b=updated, c=[true]", got)
	}

	// Handoff handshake over SSE: prepareHandoff sends "handoff <id>" and returns once acked.
	sseReq, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.url()+"/_nsgo/events", nil)
	sseReq.Header.Set(tokenHeader, srv.token)
	sseResp, err := http.DefaultClient.Do(sseReq)
	if err != nil {
		t.Fatal(err)
	}
	defer sseResp.Body.Close()
	sc := bufio.NewScanner(sseResp.Body)
	// Consume initial "data: hello".
	for sc.Scan() {
		if sc.Text() == "data: hello" {
			break
		}
	}
	handoffDone := make(chan struct{})
	go func() {
		srv.prepareHandoff(t.Context(), 2*time.Second)
		close(handoffDone)
	}()
	var handoffID string
	for sc.Scan() {
		if id, ok := strings.CutPrefix(sc.Text(), "data: handoff "); ok {
			handoffID = id
			break
		}
	}
	if handoffID == "" {
		t.Fatal("did not receive handoff event on /_nsgo/events")
	}
	ackReq, _ := http.NewRequest(http.MethodPost, srv.url()+"/_nsgo/handoff?id="+handoffID, nil)
	ackReq.Header.Set(tokenHeader, srv.token)
	ackResp, err := http.DefaultClient.Do(ackReq)
	if err != nil {
		t.Fatal(err)
	}
	ackResp.Body.Close()
	select {
	case <-handoffDone:
	case <-time.After(2 * time.Second):
		t.Fatal("prepareHandoff did not unblock after ack")
	}

	// Overlap readiness: beginReady sets NSGO_DEV_READY_ID in devEnv and unblocks on /_nsgo/ready.
	id, readyCh, endReady := srv.beginReady()
	env := devEnv(devOptions{server: srv})
	wantEnv := "NSGO_DEV_READY_ID=" + id
	found := false
	for _, e := range env {
		if e == wantEnv {
			found = true
		}
	}
	if !found {
		t.Fatalf("devEnv = %v, missing %q", env, wantEnv)
	}
	go func() {
		r, _ := http.NewRequest(http.MethodPost, srv.url()+"/_nsgo/ready?id="+id, nil)
		r.Header.Set(tokenHeader, srv.token)
		if resp, err := http.DefaultClient.Do(r); err == nil {
			resp.Body.Close()
		}
	}()
	waitDone := make(chan struct{})
	go func() {
		waitOverlapReady(t.Context(), readyCh, make(chan error, 1), 2*time.Second)
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("waitOverlapReady did not unblock on /_nsgo/ready")
	}
	endReady()
	if gotID := srv.currentReadyID(); gotID != "" {
		t.Fatalf("readyID after endReady = %q, want empty", gotID)
	}

	// Early exit during overlap must unblock immediately and preserve the error on exited.
	exited := make(chan error, 1)
	wantErr := errors.New("boom")
	exited <- wantErr
	_, readyCh2, endReady2 := srv.beginReady()
	defer endReady2()
	start := time.Now()
	waitOverlapReady(t.Context(), readyCh2, exited, 2*time.Second)
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("waitOverlapReady waited for timeout instead of unblocking on early exit")
	}
	if err := <-exited; !errors.Is(err, wantErr) {
		t.Fatalf("preserved exit error = %v, want %v", err, wantErr)
	}
}
