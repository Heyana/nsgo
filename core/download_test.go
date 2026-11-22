package core

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestBackgroundDownloadCommandsAndOwner(t *testing.T) {
	var commands []driver.DownloadCommand
	driver.ConnectDownloads(func(command driver.DownloadCommand) error {
		commands = append(commands, command)
		return nil
	})
	defer driver.ConnectDownloads(nil)

	var posted []func()
	root := testRoot(RootOptions{Post: func(fn func()) { posted = append(posted, fn) }})
	defer root.Dispose()
	view := root.Child()
	destination := filepath.Join(t.TempDir(), "film")
	task, err := view.Download(DownloadRequest{URL: "https://localhost/film", Headers: map[string]string{"X-Example": "a"}},
		destination, DownloadOptions{CertificatePEM: "test certificate"})
	if err != nil || task == "" {
		t.Fatalf("start = %q, %v", task, err)
	}
	if start := commands[0]; start.Op != "start" || start.ID != string(task) || start.Path != destination ||
		start.URL != "https://localhost/film" || start.CertificatePEM != "test certificate" || start.Headers["X-Example"] != "a" {
		t.Fatalf("start command = %+v", start)
	}
	var states []DownloadState
	task.OnUpdate(view, func(p DownloadProgress) { states = append(states, p.State) })
	task.Pause()
	task.Resume()
	report := driver.DownloadProgress{ID: string(task), Path: destination, State: "running", Loaded: 1 << 31, Total: 1<<31 + 9}
	driver.ReportDownload(report)
	driver.ReportDownload(report)
	driver.ReportDownload(driver.DownloadProgress{ID: "another", State: "running"})
	if len(states) != 0 {
		t.Fatal("the host ran a view callback directly")
	}
	for _, fn := range posted {
		fn()
	}
	if !slices.Equal(states, []DownloadState{DownloadRunning}) {
		t.Fatalf("task states = %v", states)
	}
	posted = nil
	view.Dispose()
	driver.ReportDownload(report)
	if len(posted) != 0 {
		t.Fatal("the closed owner still listens")
	}
	task.Cancel()
	task.Forget()

	var ops []string
	for _, c := range commands {
		ops = append(ops, c.Op)
	}
	if want := []string{"start", "list", "pause", "resume", "cancel", "forget"}; !slices.Equal(ops, want) {
		t.Fatalf("commands = %v, want %v", ops, want)
	}
}

func TestBackgroundDownloadRejectsInvalidRequests(t *testing.T) {
	root := testRoot(RootOptions{})
	defer root.Dispose()
	dest := filepath.Join(t.TempDir(), "film")
	cases := []struct {
		req  DownloadRequest
		path string
		opts DownloadOptions
	}{
		{DownloadRequest{URL: "file:///tmp/film"}, dest, DownloadOptions{}},
		{DownloadRequest{URL: "https://name:password@localhost/file"}, dest, DownloadOptions{}},
		{DownloadRequest{URL: "https://localhost/file"}, "relative", DownloadOptions{}},
		{DownloadRequest{URL: "https://localhost/file", Headers: map[string]string{"rAnGe": "bytes=5-"}}, dest, DownloadOptions{}},
		{DownloadRequest{URL: "http://localhost/file"}, dest, DownloadOptions{CertificatePEM: "test certificate"}},
	}
	for i, c := range cases {
		if _, err := root.Download(c.req, c.path, c.opts); err == nil || errors.Is(err, ErrDownloadUnavailable) {
			t.Errorf("invalid request %d = %v", i, err)
		}
	}
	if _, err := root.Download(DownloadRequest{URL: "https://localhost/file"}, dest, DownloadOptions{}); !errors.Is(err, ErrDownloadUnavailable) {
		t.Fatalf("no host = %v", err)
	}
}
