//go:build nsgominiprogram

package tsx_test

import (
	"errors"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

func TestMiniProgramTSXUnsupported(t *testing.T) {
	p := &pkg.Package{App: pkg.App{ID: "dev.nsgo.unsupported"}}

	var loadErr error
	nsgotest.Start(t, func(cx *core.Scope) core.Node {
		var plugin *tsx.Plugin
		plugin, loadErr = tsx.Load(cx, p, tsx.Sandbox{})
		if plugin != nil {
			t.Errorf("tsx.Load returned non-nil plugin %v on nsgominiprogram", plugin)
		}
		return core.View{}
	})
	if !errors.Is(loadErr, tsx.ErrUnsupported) || !errors.Is(loadErr, errors.ErrUnsupported) {
		t.Fatalf("tsx.Load err = %v, want tsx.ErrUnsupported / errors.ErrUnsupported", loadErr)
	}

	expectCaught := func(name string, try func(*core.Scope) core.Node) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			var got error
			nsgotest.Start(t, func(*core.Scope) core.Node {
				return core.Catch{
					Try: try,
					Fallback: func(_ *core.Scope, err error, _ func()) core.Node {
						got = err
						return core.Text{Content: "caught"}
					},
				}
			})
			if !errors.Is(got, tsx.ErrUnsupported) || !errors.Is(got, errors.ErrUnsupported) {
				t.Fatalf("%s error = %v, want tsx.ErrUnsupported / errors.ErrUnsupported", name, got)
			}
		})
	}

	expectCaught("View", tsx.View{Package: p}.Render)
	expectCaught("Sandboxed", tsx.Sandboxed("app.js", "", tsx.Sandbox{}))
	expectCaught("App", tsx.App("app.js", ""))
	expectCaught("SplitApp", tsx.SplitApp("app.js", "", nil))
	expectCaught("Hot", func(cx *core.Scope) core.Node {
		app, _ := tsx.Hot("app.js", "")
		return app(cx)
	})
}
