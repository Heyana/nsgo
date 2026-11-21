package driver

// NoCapture is how a platform without runtime surface capture reports it.
const NoCapture = "this platform has no surface capture"

// CaptureResult answers a capture request with the PNG or an error.
func CaptureResult(count uint32, png []byte, err error) Event {
	if err != nil {
		return Event{Kind: EventFiles, Count: count, Text: err.Error()}
	}
	return Event{Kind: EventFiles, Count: count, Data: png, Finished: true}
}
