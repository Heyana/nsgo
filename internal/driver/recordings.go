package driver

import "sync"

// Recordings are how package microphone records where the system's audio API is not the Go process's own, on Android, whose Java runtime owns AudioRecord and the foreground service a recording runs in. The package reaches the recorder outside any surface, as a recording belongs to the process rather than to a view: its messages, JSON, wait here until the recorder connects, and the recorder's reports wait for the package's handler. nsgotest's fake microphone connects as the recorder too, one app's at a time, as the channel is one per process.
var recordings struct {
	sync.Mutex
	send    func([]byte)
	queue   [][]byte
	hear    func([]byte)
	reports [][]byte
	dir     string
}

// SetRecordingsDir sets the app's recordings directory (DataDir/recordings),
// which package microphone uses for local WAV recordings on macOS, iOS,
// Windows and Linux.
func SetRecordingsDir(dir string) {
	recordings.Lock()
	recordings.dir = dir
	recordings.Unlock()
}

// RecordingsDir returns the app's recordings directory, or "" if no engine
// has configured one.
func RecordingsDir() string {
	recordings.Lock()
	defer recordings.Unlock()
	return recordings.dir
}

// SendRecording sends package microphone's msg to the recorder, once one has connected.
func SendRecording(msg []byte) {
	recordings.Lock()
	send := recordings.send
	if send == nil {
		recordings.queue = append(recordings.queue, msg)
	}
	recordings.Unlock()
	if send != nil {
		send(msg)
	}
}

// ConnectRecordings is the recorder's, the Android backend's or nsgotest's fake microphone's: send carries package microphone's messages, those that waited first. A nil send disconnects the recorder, and messages wait for the next.
func ConnectRecordings(send func([]byte)) {
	recordings.Lock()
	var queue [][]byte
	if send != nil {
		queue, recordings.queue = recordings.queue, nil
	}
	recordings.send = send
	recordings.Unlock()
	for _, msg := range queue {
		send(msg)
	}
}

// RecordingsConnected says whether a recorder is connected.
func RecordingsConnected() bool {
	recordings.Lock()
	defer recordings.Unlock()
	return recordings.send != nil
}

// HearRecordings is package microphone's: fn hears the recorder's reports, those that waited first. It runs on the goroutine that reports, which it must not hold up.
func HearRecordings(fn func([]byte)) {
	recordings.Lock()
	reports := recordings.reports
	recordings.hear, recordings.reports = fn, nil
	recordings.Unlock()
	for _, r := range reports {
		fn(r)
	}
}

// ReportRecording is the recorder's: it hands package microphone a report.
func ReportRecording(report []byte) {
	recordings.Lock()
	hear := recordings.hear
	if hear == nil {
		recordings.reports = append(recordings.reports, report)
	}
	recordings.Unlock()
	if hear != nil {
		hear(report)
	}
}
