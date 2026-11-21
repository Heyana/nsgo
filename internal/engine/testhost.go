package engine

import (
	"os"
	"sync"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// TestPin is the environment a test pins over the platform's, which it
// may change while the app runs.
type TestPin struct {
	mu  sync.Mutex
	env *driver.Environment
}

// Set pins env from now on.
func (p *TestPin) Set(env *driver.Environment) {
	p.mu.Lock()
	p.env = env
	p.mu.Unlock()
}

func (p *TestPin) apply(env driver.Environment) driver.Environment {
	p.mu.Lock()
	defer p.mu.Unlock()
	return testhost.Pin(env, p.env)
}

// UseTest sets what a test host opens an app with over o: the test's size,
// storage and observers, pin's environment over the platform's, and data
// and cache directories of its own, so a test never sees the app's.
func (o *Options) UseTest(t testhost.Options, pin *TestPin) {
	o.CheckCommits = t.CheckCommits
	if t.Size.Width > 0 && t.Size.Height > 0 {
		// A geometry measured at the surface's size is not the test's.
		o.Size, o.Geometry = t.Size, driver.Geometry{}
	}
	o.Observe, o.Event = t.Observe, t.Event
	o.TakePhoto, o.ScanCode = t.TakePhoto, t.ScanCode
	o.DiskSpace = &t.DiskSpace
	o.LogPerf = t.FPS
	// The machine's clipboard is never the test's, and its clock is.
	o.Clipboard = &driver.MemoryClipboard{}
	if t.Clock != nil {
		o.Clock = t.Clock
	}
	pin.Set(t.Environment)
	o.Pin = pin.apply
	o.Storage = t.Storage
	// Nor is its keychain.
	o.Secrets = t.Secrets
	if o.Secrets == nil {
		o.Secrets = &driver.MemorySecrets{}
	}
	o.Host.Channel = t.Host
	dir, err := os.MkdirTemp("", "nsgo-test-")
	if err == nil {
		o.DataDir, o.CacheDir, o.LogsDir = dir+"/data", dir+"/cache", dir+"/logs"
	} else {
		o.DataDir, o.CacheDir, o.LogsDir = "", "", ""
	}
}
