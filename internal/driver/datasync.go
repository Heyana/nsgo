package driver

import (
	"errors"
	"sync"
)

// ErrDataSyncUnavailable is returned when the platform runtime has no dataSync
// foreground service host or the app does not declare it.
var ErrDataSyncUnavailable = errors.New("data sync: foreground service is unavailable")

// dataSync connects mobile.StartDataSync (implemented in core) to the Android
// runtime's DataSyncService outside any surface, as the foreground service keeps
// the entire Go process alive across Activity destruction.
var dataSync struct {
	sync.Mutex
	send    func([]byte) error
	hear    func([]byte)
	reports [][]byte
}

// ConnectDataSync installs the process's Android dataSync host. A nil send
// disconnects the host and notifies the listener with a nil report so any
// pending start fails cleanly instead of waiting forever.
func ConnectDataSync(send func([]byte) error) {
	dataSync.Lock()
	dataSync.send = send
	hear := dataSync.hear
	if send == nil {
		dataSync.reports = nil
	}
	dataSync.Unlock()
	if send == nil && hear != nil {
		hear(nil)
	}
}

// DataSyncConnected reports whether a dataSync host is connected.
func DataSyncConnected() bool {
	dataSync.Lock()
	defer dataSync.Unlock()
	return dataSync.send != nil
}

// SendDataSync forwards a JSON command to the connected dataSync host.
func SendDataSync(msg []byte) error {
	dataSync.Lock()
	send := dataSync.send
	dataSync.Unlock()
	if send == nil {
		return ErrDataSyncUnavailable
	}
	return send(msg)
}

// HearDataSync registers the listener for the host's JSON reports, delivering
// any reports that arrived before registration. The callback runs on the host's
// reader goroutine and must not block.
func HearDataSync(fn func([]byte)) {
	dataSync.Lock()
	reports := dataSync.reports
	dataSync.hear, dataSync.reports = fn, nil
	dataSync.Unlock()
	for _, r := range reports {
		fn(r)
	}
}

// ReportDataSync delivers a JSON report from the dataSync host to Go.
func ReportDataSync(report []byte) {
	if report == nil {
		return
	}
	dataSync.Lock()
	hear := dataSync.hear
	if hear == nil {
		dataSync.reports = append(dataSync.reports, report)
	}
	dataSync.Unlock()
	if hear != nil {
		hear(report)
	}
}
