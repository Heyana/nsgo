//go:build !darwin && !windows && !(linux && !android)

package microphone

const hasLocalRecorder = false

func startLocalWAV(string, RecordOptions) (*Recording, error) { return nil, ErrUnsupported }

func localEarlier() []*Recording { return nil }
