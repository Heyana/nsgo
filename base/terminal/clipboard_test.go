package terminal

import (
	"bytes"
	"encoding/base64"
	"image"
	imagecolor "image/color"
	"image/png"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base/terminal/internal/vt"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func clipboardTerminal(t *testing.T) (*Terminal, *recordingConn) {
	t.Helper()
	conn := newRecordingConn()
	term, err := New(Options{Conn: conn})
	if err != nil {
		if !vt.Supported {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	return term, conn
}

func clipboardReply(t *testing.T, app *nsgotest.App, conn *recordingConn, end string) string {
	t.Helper()
	var reply string
	app.WaitUntil(3*time.Second, func() bool { reply += conn.take(); return strings.Contains(reply, end) })
	return reply
}

func TestProgramClipboardPermissions(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		t.Run(map[bool]string{false: "denied", true: "allowed"}[allowed], func(t *testing.T) {
			term, conn := clipboardTerminal(t)
			var scope *core.Scope
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				scope = cx
				return View{Terminal: term, AllowClipboardRead: allowed, AllowClipboardWrite: allowed,
					Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}}
			}, nsgotest.Size(480, 300))
			app.Post(func() { scope.SetClipboard("before") })
			app.Settle()
			term.Feed([]byte("\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte("program-copy")) + "\x1b\\"))
			app.Settle()
			want := "before"
			if allowed {
				want = "program-copy"
			}
			if app.Clipboard != want {
				t.Fatalf("program copy: %q want %q", app.Clipboard, want)
			}
			term.Feed([]byte("\x1b]52;c;?\x1b\\"))
			got := clipboardReply(t, app, conn, "\x1b]52;c;")
			payload := ""
			if allowed {
				payload = base64.StdEncoding.EncodeToString([]byte(want))
			}
			if got != "\x1b]52;c;"+payload+"\x1b\\" {
				t.Errorf("OSC52 read=%q", got)
			}
			term.Feed([]byte("\x1b]5522;type=read:id=read1;dGV4dC9wbGFpbg==\x1b\\"))
			ending := "status=EPERM"
			if allowed {
				ending = "status=DONE"
			}
			got = clipboardReply(t, app, conn, ending)
			if allowed && !strings.Contains(got, base64.StdEncoding.EncodeToString([]byte(want))) {
				t.Errorf("Kitty read=%q", got)
			}
			if !allowed && strings.Contains(got, base64.StdEncoding.EncodeToString([]byte("before"))) {
				t.Fatal("denied read leaked contents")
			}
		})
	}
}

func TestClipboardReadWithoutView(t *testing.T) {
	term, conn := clipboardTerminal(t)
	term.Feed([]byte("\x1b]5522;type=read;dGV4dC9wbGFpbg==\x1b\\"))
	var got string
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(got, "status=EPERM") && time.Now().Before(deadline) {
		got += conn.take()
		time.Sleep(time.Millisecond)
	}
	if !strings.Contains(got, "status=EPERM") {
		t.Fatalf("no-view denial=%q", got)
	}
}

func TestKittyPasteImageSnapshot(t *testing.T) {
	term, conn := clipboardTerminal(t)
	env := core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}
	pasteKey := "meta+v"
	if runtime.GOOS == "windows" {
		env.OS = core.OSWindows
		pasteKey = "ctrl+shift+v"
	} else if runtime.GOOS == "linux" {
		env.OS = core.OSLinux
		pasteKey = "ctrl+shift+v"
	}
	ref := &Ref{}
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		return View{Terminal: term, Ref: ref, TestID: "term", Label: "Terminal", AutoFocus: true, Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}}
	}, nsgotest.Size(480, 300), nsgotest.Environment(env))
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.SetRGBA(0, 0, imagecolor.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	original := buf.Bytes()
	app.Post(func() { scope.SetClipboardImage(original) })
	app.Settle()
	term.Feed([]byte("\x1b[?5522h"))
	app.WaitUntil(3*time.Second, func() bool { return ref.v.focused.GetUntracked() })
	app.Key(pasteKey)
	offer := clipboardReply(t, app, conn, "status=DONE")
	_, tail, ok := strings.Cut(offer, ":pw=")
	if !ok {
		t.Fatalf("paste offer=%q", offer)
	}
	password := strings.SplitN(tail, "\x1b", 2)[0]
	if !strings.Contains(offer, base64.StdEncoding.EncodeToString([]byte("image/png\n"))) {
		t.Fatalf("no image offer=%q", offer)
	}
	// The system clipboard changes after the explicit paste. The token can read only its offered snapshot, never this later clipboard value.
	app.Post(func() { scope.SetClipboard("later-secret") })
	app.Settle()
	mime := base64.StdEncoding.EncodeToString([]byte("image/png"))
	name := base64.StdEncoding.EncodeToString([]byte("Paste event"))
	query := "\x1b]5522;type=read:pw=" + password + ":name=" + name + ";" + base64.StdEncoding.EncodeToString([]byte("text/plain image/png .")) + "\x1b\\"
	term.Feed([]byte(query))
	response := clipboardReply(t, app, conn, "status=DONE")
	if strings.Contains(response, base64.StdEncoding.EncodeToString([]byte("later-secret"))) {
		t.Fatal("paste token read clipboard contents outside its offer")
	}
	var got []byte
	for _, packet := range strings.Split(response, "\x1b\\") {
		if _, chunk, found := strings.Cut(packet, "status=DATA:mime="+mime+";"); found {
			p, err := base64.StdEncoding.DecodeString(chunk)
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, p...)
		}
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("snapshot image=%d bytes, want %d", len(got), len(original))
	}
	term.Feed([]byte(query))
	retry := clipboardReply(t, app, conn, "status=EPERM")
	if strings.Contains(retry, "status=DATA") {
		t.Fatalf("one-use token served data again: %q", retry)
	}
	term.Feed([]byte("\x1b]52;c;?\x1b\\"))
	denial := clipboardReply(t, app, conn, "\x1b]52;c;")
	if denial != "\x1b]52;c;\x1b\\" {
		t.Fatalf("paste enabled unsolicited reads: %q", denial)
	}
	term.Feed([]byte("\r\nKitty PNG snapshot verified\r\nUnsolicited clipboard reads denied"))
	app.Settle()
	if os.Getenv("NSGO_TERMINAL_CLIPBOARD_SCREENSHOT") == "1" {
		app.Screenshot("terminal-clipboard")
	}
	t.Logf("Kitty paste offered image/png; one-use token returned exact %d-byte PNG snapshot after clipboard changed", len(got))
}

func TestPendingClipboardReadRevoked(t *testing.T) {
	term, conn := clipboardTerminal(t)
	var scope *core.Scope
	var visible *core.Signal[bool]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		visible = core.NewSignal(cx, true)
		return core.Show{When: visible.Get, Then: func(cx *core.Scope) core.Node {
			return View{Terminal: term, AllowClipboardRead: true}
		}}
	}, nsgotest.Size(480, 300))
	app.Post(func() { scope.SetClipboard("must-not-escape") })
	app.Settle()
	app.Post(func() { term.Feed([]byte("\x1b]52;c;?\x1b\\")); visible.Set(false) })
	got := clipboardReply(t, app, conn, "\x1b]52;c;")
	if got != "\x1b]52;c;\x1b\\" {
		t.Fatalf("disposed view answered read: %q", got)
	}
}

type clipboardAccessDenied struct{}

func (clipboardAccessDenied) Allows(req core.AccessRequest) bool {
	return req.Permission != core.AccessClipboard
}
func (clipboardAccessDenied) WebHosts() []string { return nil }

func TestClipboardProgramsRespectViewAccess(t *testing.T) {
	term, conn := clipboardTerminal(t)
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		cx.ProvideAccess(clipboardAccessDenied{})
		return View{Terminal: term, AllowClipboardRead: true, AllowClipboardWrite: true}
	}, nsgotest.Size(480, 300))
	app.Post(func() { scope.SetClipboard("scope-secret") })
	app.Settle()
	term.Feed([]byte("\x1b]52;c;cHJvZ3JhbS1jb3B5\x1b\\\x1b]52;c;?\x1b\\"))
	got := clipboardReply(t, app, conn, "\x1b]52;c;")
	if got != "\x1b]52;c;\x1b\\" || app.Clipboard != "scope-secret" {
		t.Fatalf("access gate failed: reply=%q clipboard=%q", got, app.Clipboard)
	}
}

func TestClipboardPlainPasteAndPrimary(t *testing.T) {
	term, conn := clipboardTerminal(t)
	var scope *core.Scope
	ref := &Ref{}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		return View{Terminal: term, Ref: ref, AllowClipboardRead: true, AllowClipboardWrite: true}
	}, nsgotest.Size(480, 300))
	app.Post(func() { scope.SetClipboard("hello\nworld") })
	app.Settle()
	term.Feed([]byte("\x1b[?2004h"))
	app.Post(ref.Paste)
	got := clipboardReply(t, app, conn, "\x1b[201~")
	if got != "\x1b[200~hello\nworld\x1b[201~" {
		t.Fatalf("ordinary paste=%q", got)
	}
	term.Feed([]byte("\x1b]52;p;cHJpbWFyeQ==\x1b\\\x1b]52;p;?\x1b\\"))
	got = clipboardReply(t, app, conn, "\x1b]52;p;")
	if got != "\x1b]52;p;\x1b\\" || app.Clipboard != "hello\nworld" {
		t.Fatalf("primary affected native clipboard: %q %q", got, app.Clipboard)
	}
}

func TestClipboardOwnerReplacementRevokesRead(t *testing.T) {
	term, conn := clipboardTerminal(t)
	var scope *core.Scope
	var allowed *core.Signal[bool]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		allowed = core.NewSignal(cx, true)
		return core.Dynamic(func(cx *core.Scope) core.Node { return View{Terminal: term, AllowClipboardRead: allowed.Get()} })
	}, nsgotest.Size(480, 300))
	app.Post(func() { scope.SetClipboard("old-owner-secret") })
	app.Settle()
	app.Post(func() { term.Feed([]byte("\x1b]52;c;?\x1b\\")); allowed.Set(false) })
	got := clipboardReply(t, app, conn, "\x1b]52;c;")
	if got != "\x1b]52;c;\x1b\\" {
		t.Fatalf("old owner served pending request: %q", got)
	}
}

func TestKittyInputPasteEventSnapshot(t *testing.T) {
	term, conn := clipboardTerminal(t)
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		return View{Terminal: term, Label: "Terminal", Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}}
	}, nsgotest.Size(480, 300))
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	app.Post(func() { scope.SetClipboard("unrelated-clipboard") })
	app.Settle()
	term.Feed([]byte("\x1b[?5522h"))
	app.Paste("label:Terminal", core.PasteEvent{Files: []core.PastedFile{{MIME: "image/png", Data: data.Bytes()}}})
	offer := clipboardReply(t, app, conn, "status=DONE")
	_, tail, ok := strings.Cut(offer, ":pw=")
	if !ok {
		t.Fatalf("input paste offer=%q", offer)
	}
	password := strings.SplitN(tail, "\x1b", 2)[0]
	term.Feed([]byte("\x1b]5522;type=read:pw=" + password + ":name=UGFzdGUgZXZlbnQ=;aW1hZ2UvcG5n\x1b\\"))
	got := clipboardReply(t, app, conn, "status=DONE")
	if !strings.Contains(got, base64.StdEncoding.EncodeToString(data.Bytes())) || strings.Contains(got, base64.StdEncoding.EncodeToString([]byte("unrelated-clipboard"))) {
		t.Fatalf("input snapshot reply=%q", got)
	}
}
