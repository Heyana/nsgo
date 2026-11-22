//go:build !darwin && !windows && !(linux && !android)

package microphone

// Supported says this target can record: on Android, that the app declares the runtime's recording service (nsgo.yaml's android.recording), once the runtime has said so; elsewhere only while nsgotest's fake microphone is connected.
func Supported() bool { return reachable() && recorderSupported() }

// Allowed says the app may record without asking: on Android, that it holds RECORD_AUDIO.
func Allowed() bool { return reachable() && recorderAllowed() }

// Authorize asks for the microphone the first time, waiting for the answer, so call it off the UI. It answers nil once the app may record and ErrDenied when it may not. On Android it asks for RECORD_AUDIO through the visible activity, and answers ErrUnavailable when none is visible; it reports ErrUnsupported on targets that cannot record.
func Authorize() error {
	if !reachable() {
		return ErrUnsupported
	}
	return recorderAuthorize()
}

// Capture reports ErrUnsupported. Android records only to a file, with RecordWAV, rather than streaming PCM through the runtime's pipe.
func Capture(int, func([]int16)) (*Stream, error) { return nil, ErrUnsupported }
