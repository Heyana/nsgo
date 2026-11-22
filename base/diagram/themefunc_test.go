package diagram

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestThemeFuncNilKeepsDefaults(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			View{TestID: "plain", Source: "flowchart LR\n A[Example]"},
			View{TestID: "dynamic", Source: "flowchart LR\n A[Example]", ThemeFunc: func() *Theme { return nil }},
		}}
	})
	if !slices.Equal(fills(app.Find("id:plain")), fills(app.Find("id:dynamic"))) {
		t.Fatal("a nil dynamic theme did not keep the appearance's defaults")
	}
}
