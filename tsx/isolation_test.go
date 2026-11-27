package tsx

import (
	"errors"
	"testing"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// TestIsolation holds a page-authority engine to requiring either an opaque-origin
// isolated frame or Sandbox.WebUnisolated whenever capabilities, grants or
// DisableStorage restrict the package.
func TestIsolation(t *testing.T) {
	caps := &pkg.Capabilities{}
	for _, c := range []struct {
		name             string
		s                Sandbox
		browser          bool
		hasIsolatedFrame bool
		refused          bool
	}{
		{"capabilities on the web without frame", Sandbox{Capabilities: caps}, true, false, true},
		{"capabilities on the web with opaque frame", Sandbox{Capabilities: caps}, true, true, false},
		{"a grant on the web without frame", Sandbox{Grant: caps}, true, false, true},
		{"a grant on the web with opaque frame", Sandbox{Grant: caps}, true, true, false},
		{"a package's view on the web without frame", Sandbox{}.of(&pkg.Package{}), true, false, true},
		{"a package's view on the web with opaque frame", Sandbox{}.of(&pkg.Package{}), true, true, false},
		{"opted in on the web", Sandbox{Capabilities: caps, Grant: caps, WebUnisolated: true}, true, false, false},
		{"no capabilities on the web", Sandbox{Storage: "com.example.plugin", Modules: []string{"*"}}, true, false, false},
		{"disabled storage on the web without frame", Sandbox{DisableStorage: true}, true, false, true},
		{"disabled storage on the web with opaque frame", Sandbox{DisableStorage: true}, true, true, false},
		{"disabled storage opted in on the web", Sandbox{DisableStorage: true, WebUnisolated: true}, true, false, false},
		{"disabled storage on goja", Sandbox{DisableStorage: true}, false, false, false},
		{"capabilities on goja", Sandbox{Capabilities: caps, Grant: caps}, false, false, false},
	} {
		err := c.s.isolationWith(c.browser, c.hasIsolatedFrame)
		if c.refused != errors.Is(err, ErrUnisolated) || !c.refused && err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}
