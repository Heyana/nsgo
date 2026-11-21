package driver

import "sync"

// cloudBridge is how package cloud reaches the Android runtime's Google Play Games saved games bridge outside any surface, mirroring Recordings and LiveUpdates: JSON commands wait until the backend connects, and reports wait for package cloud's handler.
var cloudBridge struct {
	sync.Mutex
	send    func([]byte)
	queue   [][]byte
	hear    func([]byte)
	reports [][]byte
}

// SendCloudStorage sends package cloud's msg to the connected Android runtime.
func SendCloudStorage(msg []byte) {
	cloudBridge.Lock()
	send := cloudBridge.send
	if send == nil {
		cloudBridge.queue = append(cloudBridge.queue, msg)
	}
	cloudBridge.Unlock()
	if send != nil {
		send(msg)
	}
}

// ConnectCloudStorage registers the Android backend's sender for package cloud messages, draining any queued messages first. A nil send disconnects it.
func ConnectCloudStorage(send func([]byte)) {
	cloudBridge.Lock()
	var queue [][]byte
	if send != nil {
		queue, cloudBridge.queue = cloudBridge.queue, nil
	}
	cloudBridge.send = send
	cloudBridge.Unlock()
	for _, msg := range queue {
		send(msg)
	}
}

// CloudStorageConnected reports whether a cloud storage runtime bridge is connected.
func CloudStorageConnected() bool {
	cloudBridge.Lock()
	defer cloudBridge.Unlock()
	return cloudBridge.send != nil
}

// HearCloudStorage registers package cloud's handler for runtime reports, delivering any queued reports first.
func HearCloudStorage(fn func([]byte)) {
	cloudBridge.Lock()
	reports := cloudBridge.reports
	cloudBridge.hear, cloudBridge.reports = fn, nil
	cloudBridge.Unlock()
	for _, r := range reports {
		fn(r)
	}
}

// ReportCloudStorage hands a runtime report to package cloud.
func ReportCloudStorage(report []byte) {
	cloudBridge.Lock()
	hear := cloudBridge.hear
	if hear == nil {
		cloudBridge.reports = append(cloudBridge.reports, report)
	}
	cloudBridge.Unlock()
	if hear != nil {
		hear(report)
	}
}
