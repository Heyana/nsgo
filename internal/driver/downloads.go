package driver

import (
	"errors"
	"maps"
	"sync"
)

// DownloadCommand is a process-wide request for the background transfer host.
type DownloadCommand struct {
	Op, ID, URL, Path, CertificatePEM string
	Headers                           map[string]string
}

// DownloadProgress describes a durable transfer or an update to one running now.
type DownloadProgress struct {
	ID, Path, State, Error string
	Loaded, Total          int64
}

var downloads struct {
	sync.Mutex
	send      func(DownloadCommand) error
	next      uint64
	listeners map[uint64]func(DownloadProgress)
}

// ConnectDownloads installs the process's platform download host.
func ConnectDownloads(send func(DownloadCommand) error) {
	downloads.Lock()
	downloads.send = send
	downloads.Unlock()
}

// ErrDownloadUnavailable is the reply on a platform without a download host.
var ErrDownloadUnavailable = errors.New("background downloads are unavailable")

// SendDownload forwards a command when the platform has a download host.
func SendDownload(command DownloadCommand) error {
	downloads.Lock()
	send := downloads.send
	downloads.Unlock()
	if send == nil {
		return ErrDownloadUnavailable
	}
	return send(command)
}

// HearDownloads registers a listener of the host's reports. The callback
// runs on the host's receiver and must return promptly.
func HearDownloads(fn func(DownloadProgress)) func() {
	downloads.Lock()
	defer downloads.Unlock()
	downloads.next++
	id := downloads.next
	if downloads.listeners == nil {
		downloads.listeners = make(map[uint64]func(DownloadProgress))
	}
	downloads.listeners[id] = fn
	return func() {
		downloads.Lock()
		delete(downloads.listeners, id)
		downloads.Unlock()
	}
}

// ReportDownload delivers a change or a snapshot requested by Go.
func ReportDownload(progress DownloadProgress) {
	downloads.Lock()
	listeners := maps.Clone(downloads.listeners)
	downloads.Unlock()
	for _, fn := range listeners {
		fn(progress)
	}
}
