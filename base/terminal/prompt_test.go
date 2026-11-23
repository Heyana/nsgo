package terminal

import (
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func protocolTerminal(t *testing.T, opts Options) *Terminal {
	t.Helper()
	if opts.Conn == nil {
		opts.Conn = newPipe()
	}
	term, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	return term
}

func TestPromptDistance(t *testing.T) {
	for _, tc := range []struct {
		name, input          string
		cols, col, row, want int
	}{
		{"ascii", "abc", 20, 2, 0, -3},
		{"prompt prefix", "abc", 20, 0, 0, -3},
		{"past end", "abc", 20, 18, 0, 0},
		{"wide tail", "a界b", 20, 4, 0, -2},
		{"graphemes", "e\u0301👍🏽z", 20, 3, 0, -2},
		{"wrapped", "abcdefghijkl", 10, 3, 0, -11},
		{"wrapped second row", "abcdefghijkl", 10, 2, 1, -2},
		{"space", "a b ", 20, 3, 0, -3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term := protocolTerminal(t, Options{})
			term.Resize(tc.cols, 6)
			term.Feed([]byte("\x1b]133;A\a$ \x1b]133;B\a" + tc.input))
			term.mu.Lock()
			got, ok := term.promptDistance(tc.col, tc.row)
			term.mu.Unlock()
			if !ok || got != tc.want {
				t.Fatalf("distance %d,%v, want %d,true", got, ok, tc.want)
			}
		})
	}
}

func TestPromptRejectsOutputAndOtherScreens(t *testing.T) {
	for _, tc := range []struct {
		name, prefix, suffix string
		scroll               bool
	}{
		{name: "unmarked", prefix: "$ abc"},
		{name: "output starts", prefix: "\x1b]133;A\a$ \x1b]133;B\aabc", suffix: "\x1b]133;C\a"},
		{name: "command ends", prefix: "\x1b]133;A\a$ \x1b]133;B\aabc", suffix: "\x1b]133;D;0\a"},
		{name: "new prompt before input", prefix: "\x1b]133;A\a$ \x1b]133;B\aabc", suffix: "\x1b]133;A\a$ "},
		{name: "alternate screen", prefix: "\x1b]133;A\a$ \x1b]133;B\aabc", suffix: "\x1b[?1049h"},
		{name: "scrollback", prefix: strings.Repeat("out\r\n", 20) + "\x1b]133;A\a$ \x1b]133;B\aabc", scroll: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term := protocolTerminal(t, Options{})
			term.Resize(20, 5)
			term.Feed([]byte(tc.prefix + tc.suffix))
			term.mu.Lock()
			if tc.scroll {
				term.term.ScrollToRow(0)
			}
			_, ok := term.promptDistance(2, 0)
			term.mu.Unlock()
			if ok {
				t.Fatal("accepted a click outside active command input")
			}
		})
	}
}

func TestOptionClickPrompt(t *testing.T) {
	conn := newRecordingConn()
	term := protocolTerminal(t, Options{Conn: conn})
	ref := &Ref{}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return View{Terminal: term, Ref: ref, TestID: "term", AutoFocus: true, NoKeyBar: true, Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}}
	}, nsgotest.Size(480, 240))
	term.Feed([]byte("\x1b]133;A\a$ \x1b]133;B\aa界b"))
	app.Settle()
	v := ref.v
	at := func(x, y int) core.Point {
		return core.Point{X: core.Points(v.padX + (float32(x)+0.5)*v.cellW), Y: core.Points(v.padY + (float32(y)+0.5)*v.cellH)}
	}
	sent := func(want string) {
		t.Helper()
		var got string
		app.WaitUntil(2*time.Second, func() bool { got += conn.take(); return got == want })
	}
	app.PressAtWith("id:term-screen", at(4, 0), core.Modifiers{Alt: true})
	sent(strings.Repeat("\x1b[D", 2))
	// Application cursor mode is respected, and a cursor moved back by the shell can move right again without counting wide spacers as editable characters.
	term.Feed([]byte("\x1b[?1h\x1b[1;3H"))
	app.Settle()
	app.PressAtWith("id:term-screen", at(5, 0), core.Modifiers{Alt: true})
	sent(strings.Repeat("\x1bOC", 2))
	// A drag, an outside release, and output between press and release send none.
	for _, mode := range []string{"drag", "outside", "output"} {
		app.Post(func() {
			e := core.PressEvent{Position: at(3, 0), Modifiers: core.Modifiers{Alt: true}, Clicks: 1}
			v.pressDown(e)
			switch mode {
			case "drag":
				v.mouseMove(core.MouseEvent{Position: at(5, 0)})
				v.mouseMove(core.MouseEvent{Position: at(3, 0)})
			case "outside":
				e.Position.X = -10
			case "output":
				term.Feed([]byte("\x1b]133;C\a"))
			}
			v.pressUp(e)
		})
		app.Settle()
		if got := conn.take(); got != "" {
			t.Errorf("%s sent %q", mode, got)
		}
	}
	app.Screenshot("terminal-option-click")
}
