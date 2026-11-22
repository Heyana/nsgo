package terminal

import (
	"io"
	"runtime"
	"strings"
	"testing"

	baseTerminal "github.com/nsgo-dev/nsgo/base/terminal"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx"
)

type actionConn struct {
	*io.PipeReader
	writer *io.PipeWriter
}

func (c *actionConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *actionConn) Close() error                { return c.writer.Close() }

func TestActionPropsKeepSession(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("desktop terminal session")
	}
	r, w := io.Pipe()
	conn := &actionConn{PipeReader: r, writer: w}
	t.Cleanup(func() { conn.Close() })
	ref := &Ref{}
	var step *core.Signal[core.Points]
	var policy *core.Signal[baseTerminal.RightClickAction]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		step = core.NewSignal(cx, core.Points(2))
		policy = core.NewSignal(cx, baseTerminal.RightClickIgnore)
		return Terminal{Conn: conn, Ref: ref, AutoFocus: true, TestID: "term",
			Keybinds: func() []baseTerminal.Keybind {
				return []baseTerminal.Keybind{{Key: "f2", Action: baseTerminal.IncreaseFontSize, Amount: step.Get()}}
			}, RightClickAction: policy.Get,
			Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}}
	}, nsgotest.Size(480, 300), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	if ref.t == nil {
		t.Fatal("terminal runtime unavailable")
	}
	session := ref.t
	session.Feed([]byte("keep this session"))
	size := func() core.Points {
		var result core.Points
		app.Post(func() { result = ref.FontSize() })
		app.Settle()
		return result
	}
	app.Key("f2")
	if got := size(); got != 15 {
		t.Fatalf("initial binding size=%v", got)
	}
	app.Post(func() { step.Set(4); policy.Set(baseTerminal.RightClickContextMenu) })
	app.Settle()
	if ref.t != session || !strings.Contains(session.Text(), "keep this session") {
		t.Fatal("changing action props restarted the session")
	}
	app.Key("f2")
	if got := size(); got != 17 {
		t.Fatalf("updated binding size=%v", got)
	}
	app.RightClick("id:term-screen")
	if got := len(app.Query("role:menuitem")); got != 4 {
		t.Fatalf("updated right-click policy opened %d items, want 4", got)
	}
	decl := tsx.Declarations()["types/nsgo/base.d.ts"]
	for _, field := range []string{"keybinds?:", "rightClickAction?:", "copy_to_clipboard", "increase_font_size", "copy-or-paste"} {
		if !strings.Contains(decl, field) {
			t.Errorf("Terminal declarations lack %q", field)
		}
	}
}
