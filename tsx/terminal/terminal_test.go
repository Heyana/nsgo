package terminal

import (
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx"
)

func TestRenderingProps(t *testing.T) {
	declaration := tsx.Declarations()["types/nsgo/base.d.ts"]
	for _, want := range []string{"cursorBlink?: boolean", "minimumContrast?: number", "paddingColor?:", "\"background\"", "\"extend\"", "\"extend-always\""} {
		if !strings.Contains(declaration, want) {
			t.Fatalf("terminal declaration is missing %s", want)
		}
	}
}

type clipboardConn struct {
	*io.PipeReader
	writer *io.PipeWriter
	input  chan string
}

func (c *clipboardConn) Write(p []byte) (int, error) { c.input <- string(p); return len(p), nil }
func (c *clipboardConn) Close() error                { return c.writer.Close() }

func TestClipboardPermissionChangesKeepSession(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("desktop terminal session")
	}
	r, w := io.Pipe()
	conn := &clipboardConn{PipeReader: r, writer: w, input: make(chan string, 16)}
	t.Cleanup(func() { conn.Close() })
	ref := &Ref{}
	var scope *core.Scope
	var allow *core.Signal[bool]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		allow = core.NewSignal(cx, false)
		return Terminal{Conn: conn, Ref: ref, AllowClipboardRead: allow.Get, AllowClipboardWrite: allow.Get,
			Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}}
	}, nsgotest.Size(480, 300))
	if ref.t == nil {
		t.Fatal("terminal unavailable")
	}
	session := ref.t
	app.Post(func() { scope.SetClipboard("tsx-clipboard") })
	app.Settle()
	query := "\x1b]52;c;?\x1b\\"
	read := func() string {
		t.Helper()
		session.Feed([]byte(query))
		var got string
		app.WaitUntil(3*time.Second, func() bool {
			select {
			case p := <-conn.input:
				got += p
			default:
			}
			return strings.Contains(got, "\x1b]52;c;")
		})
		return got
	}
	if got := read(); got != "\x1b]52;c;\x1b\\" {
		t.Fatalf("default permission=%q", got)
	}
	app.Post(func() { allow.Set(true) })
	app.Settle()
	if ref.t != session {
		t.Fatal("permission change restarted session")
	}
	if got := read(); got != "\x1b]52;c;dHN4LWNsaXBib2FyZA==\x1b\\" {
		t.Fatalf("allowed permission=%q", got)
	}
	app.Post(func() { allow.Set(false) })
	app.Settle()
	if ref.t != session {
		t.Fatal("permission revocation restarted session")
	}
	if got := read(); got != "\x1b]52;c;\x1b\\" {
		t.Fatalf("revoked permission=%q", got)
	}
}
