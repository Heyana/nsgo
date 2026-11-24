package tsx

import (
	"errors"
	"testing"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// TestIsolation holds the page's engine to refusing what it cannot isolate, on every target.
func TestIsolation(t *testing.T) {
	caps := &pkg.Capabilities{}
	for _, c := range []struct {
		name    string
		s       Sandbox
		browser bool
		refused bool
	}{
		{"capabilities on the web", Sandbox{Capabilities: caps}, true, true},
		{"a grant on the web", Sandbox{Grant: caps}, true, true},
		{"a package's view on the web", Sandbox{}.of(&pkg.Package{}), true, true},
		{"opted in on the web", Sandbox{Capabilities: caps, Grant: caps, WebUnisolated: true}, true, false},
		{"no capabilities on the web", Sandbox{Storage: "com.example.plugin", Modules: []string{"*"}}, true, false},
		{"disabled storage on the web", Sandbox{DisableStorage: true}, true, true},
		{"disabled storage opted in on the web", Sandbox{DisableStorage: true, WebUnisolated: true}, true, false},
		{"disabled storage on goja", Sandbox{DisableStorage: true}, false, false},
		{"capabilities on goja", Sandbox{Capabilities: caps, Grant: caps}, false, false},
	} {
		err := c.s.isolation(c.browser)
		if c.refused != errors.Is(err, ErrUnisolated) || !c.refused && err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}
