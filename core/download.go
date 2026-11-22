package core

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// DownloadRequest describes a GET request with optional HTTP headers.
type DownloadRequest struct {
	URL     string
	Headers map[string]string
}

// DownloadOptions configures a background download.
type DownloadOptions struct {
	CertificatePEM string
}

// DownloadTask identifies a download, including after the app restarts.
type DownloadTask string

// DownloadState is the latest state of a background download.
type DownloadState string

const (
	DownloadQueued      DownloadState = "queued"
	DownloadRunning     DownloadState = "running"
	DownloadPaused      DownloadState = "paused"
	DownloadCompleted   DownloadState = "completed"
	DownloadFailed      DownloadState = "failed"
	DownloadCanceled    DownloadState = "canceled"
	DownloadInterrupted DownloadState = "interrupted"
)

// DownloadProgress reports a task, destination, state and byte counts.
type DownloadProgress struct {
	Task          DownloadTask
	Path          string
	State         DownloadState
	Loaded, Total int64
	Error         string
}

// ErrDownloadUnavailable is returned where the platform has no transfer host.
var ErrDownloadUnavailable = driver.ErrDownloadUnavailable

// Download starts writing req to an absolute, app-accessible path. The task belongs to the app and keeps running when cx is disposed. It stages a partial file beside destination, then atomically publishes the result.
func (cx *Scope) Download(req DownloadRequest, destination string, opts DownloadOptions) (DownloadTask, error) {
	if accessOf(cx.current()) != nil {
		return "", AccessDenied("backgroundDownloads", "")
	}
	u, err := url.Parse(req.URL)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return "", errors.New("download needs an HTTP or HTTPS URL without user information")
	}
	if opts.CertificatePEM != "" && u.Scheme != "https" {
		return "", errors.New("a download's pinned certificate needs HTTPS")
	}
	if !filepath.IsAbs(destination) || strings.ContainsRune(destination, 0) || filepath.Clean(destination) == string(filepath.Separator) {
		return "", errors.New("download destination needs an absolute file path")
	}
	for name, value := range req.Headers {
		switch strings.ToLower(name) {
		case "range", "if-range", "accept-encoding":
			return "", fmt.Errorf("download manages the %q header", name)
		}
		if strings.TrimSpace(name) != name || name == "" || strings.ContainsAny(name+value, "\r\n\x00") {
			return "", errors.New("invalid download header")
		}
	}
	task := DownloadTask(rand.Text())
	if err := driver.SendDownload(driver.DownloadCommand{
		Op: "start", ID: string(task), URL: u.String(), Path: filepath.Clean(destination),
		CertificatePEM: opts.CertificatePEM, Headers: req.Headers,
	}); err != nil {
		return "", err
	}
	return task, nil
}

// OnDownloadUpdate reports saved and live transfers while the current owner lives.
func (cx *Scope) OnDownloadUpdate(fn func(DownloadProgress)) {
	owner := cx.current()
	if accessOf(owner) != nil {
		return
	}
	last := make(map[DownloadTask]DownloadProgress)
	stop := driver.HearDownloads(func(report driver.DownloadProgress) {
		owner.Post(func() {
			progress := DownloadProgress{Task: DownloadTask(report.ID), Path: report.Path, State: DownloadState(report.State),
				Loaded: report.Loaded, Total: report.Total, Error: report.Error}
			if previous, ok := last[progress.Task]; ok && previous == progress {
				return
			}
			last[progress.Task] = progress
			fn(progress)
		})
	})
	owner.addCleanup(stop)
	if err := driver.SendDownload(driver.DownloadCommand{Op: "list"}); err != nil {
		stop()
	}
}

// Pause asks the host to pause the task and retain its partial file.
func (t DownloadTask) Pause() { t.command("pause") }

// Resume asks the host to resume a paused task.
func (t DownloadTask) Resume() { t.command("resume") }

// Cancel asks the host to stop the task and remove its partial file.
func (t DownloadTask) Cancel() { t.command("cancel") }

// Forget removes the stored report of a finished task.
func (t DownloadTask) Forget() { t.command("forget") }

func (t DownloadTask) command(op string) {
	if t != "" {
		driver.SendDownload(driver.DownloadCommand{Op: op, ID: string(t)})
	}
}

// OnUpdate reports changes to this task on cx's engine.
func (t DownloadTask) OnUpdate(cx *Scope, fn func(DownloadProgress)) {
	cx.OnDownloadUpdate(func(p DownloadProgress) {
		if p.Task == t {
			fn(p)
		}
	})
}
