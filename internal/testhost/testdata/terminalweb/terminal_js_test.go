//go:build js

package terminalweb

import (
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/base/terminal"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestRemoteWebSocketSession(t *testing.T) {
	url := os.Getenv("NSGO_TERMINAL_WS")
	if url == "" {
		t.Skip("the host test supplies a real local WebSocket PTY")
	}
	conn, err := dialSocket(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	term, err := terminal.New(terminal.Options{Conn: conn, ResizeByConn: true})
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return terminal.View{Terminal: term, Label: "Remote terminal", AutoFocus: true, Font: core.Font{Size: 14},
			Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}}
	}, nsgotest.Size(760, 520))
	app.WaitUntil(10*time.Second, func() bool {
		return strings.Contains(term.Text(), "REMOTE_READY") && term.Title() == "WebSocket remote"
	})
	pixels := func(color string) {
		t.Helper()
		img := app.Capture()
		if img == nil {
			t.Fatal("the browser did not capture the terminal canvas")
		}
		count := 0
		for y := range img.Bounds().Dy() {
			for x := range img.Bounds().Dx() {
				r, g, b, _ := img.At(x, y).RGBA()
				if color == "green" && g > 20000 && g > r*3/2 && g > b*5/4 || color == "red" && r > 20000 && r > g*3/2 && r > b*3/2 {
					count++
				}
			}
		}
		if count < 10 {
			t.Fatalf("only %d %s pixels: the browser did not draw the remote ANSI text", count, color)
		}
	}
	pixels("green")
	app.Type("label:Remote terminal", "printf '\\nREMOTE_%s\\n' REPLY")
	app.Key("enter")
	app.WaitUntil(10*time.Second, func() bool { return strings.Contains(term.Text(), "\nREMOTE_REPLY") })
	app.Screenshot("terminal-web-session")
	app.Type("label:Remote terminal", "printf '\\033[?1049h\\033[H\\033[31mALT_%s\\033[0m' REMOTE")
	app.Key("enter")
	app.WaitUntil(10*time.Second, func() bool { return strings.Contains(term.Text(), "ALT_REMOTE") })
	pixels("red")
	app.Screenshot("terminal-web-alternate")
	app.Type("label:Remote terminal", "printf '\\033[?1049l'")
	app.Key("enter")
	app.WaitUntil(10*time.Second, func() bool { return strings.Contains(term.Text(), "REMOTE_REPLY") })
	term.Close()
	select {
	case <-term.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("closing the socket did not finish the remote session")
	}
}
