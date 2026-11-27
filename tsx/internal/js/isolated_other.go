//go:build !js

package js

// IsolatedFrameAvailable reports whether this host can create an opaque-origin
// sandboxed DOM iframe with MessageChannel.
func IsolatedFrameAvailable() bool { return false }

// IsolatedChannel is a private MessagePort channel to an opaque-origin
// sandboxed iframe on the web.
type IsolatedChannel struct{}

// OpenIsolatedFrame is only available on the web (GOOS=js).
func OpenIsolatedFrame(srcdoc, initToken string, onMessage func(map[string]any)) *IsolatedChannel {
	return nil
}

func (*IsolatedChannel) Ready() <-chan struct{} { return nil }
func (*IsolatedChannel) Post(map[string]any)    {}
func (*IsolatedChannel) Close()                 {}
