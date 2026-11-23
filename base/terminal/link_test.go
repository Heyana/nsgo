package terminal

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base/terminal/internal/vt"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func pathTerminal(t *testing.T) *Terminal {
	t.Helper()
	term, err := New(Options{Conn: newRecordingConn()})
	if err != nil {
		if !vt.Supported {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	return term
}

func directoryURL(path string) string {
	name := filepath.ToSlash(path)
	if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}
	return (&url.URL{Scheme: "file", Path: name}).String()
}

func TestFindPath(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("paths require a local filesystem")
	}
	dir := t.TempDir()
	for _, name := range []string{"file.go", "LICENSE", "a b.txt", "路径.txt", "last.", ".hidden"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("witness"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct{ text, at, name string }{
		{"see ./file.go", "file", "file.go"},
		{"(file.go:12:4),", "file", "file.go"},
		{"see LICENSE", "LIC", "LICENSE"},
		{"'a b.txt'", "b.txt", "a b.txt"},
		{"\"a b.txt\"", "a b", "a b.txt"},
		{"[路径.txt]", "路径", "路径.txt"},
		{"last.", "last", "last."},
		{".hidden", "hidden", ".hidden"},
		{filepath.Join(dir, "file.go"), "file.go", "file.go"},
		{directoryURL(filepath.Join(dir, "a b.txt")), "b.txt", "a b.txt"},
		{"missing.go", "missing", ""},
		{"file.go:12:4", "12", ""},
		{"(file.go)", ")", ""},
		{"https://example.com/file.go", "file.go", ""},
		{"file://remote.invalid" + filepath.ToSlash(filepath.Join(dir, "file.go")), "file.go", ""},
	} {
		start, end, got := findPath(c.text, strings.Index(c.text, c.at), dir)
		want := ""
		if c.name != "" {
			want = filepath.Join(dir, c.name)
		}
		if got != want {
			t.Errorf("findPath(%q at %q) = %q [%d:%d], want %q", c.text, c.at, got, start, end, want)
		}
	}
	if got := existingPath(".", dir); got != dir {
		t.Errorf("directory: %q", got)
	}
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if got := existingPath("~/file.go", t.TempDir()); got != filepath.Join(dir, "file.go") {
		t.Errorf("home: %q", got)
	}
}

func TestPathUsesShellDirectory(t *testing.T) {
	term := pathTerminal(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.go"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	term.Resize(20, 4)
	term.Feed([]byte("\x1b]7;" + directoryURL(dir) + "\x07\x1b[2J\x1b[H./file.go"))
	term.mu.Lock()
	got := term.linkAt(3, 0)
	term.mu.Unlock()
	if got.path != filepath.Join(dir, "file.go") || got.url != directoryURL(got.path) {
		t.Fatalf("OSC 7 path: %+v", got)
	}
	term.Feed([]byte("\x1b]7;file://remote.invalid" + filepath.ToSlash(dir) + "\x07"))
	term.mu.Lock()
	got = term.linkAt(3, 0)
	term.mu.Unlock()
	if got.url != "" {
		t.Errorf("remote directory became local: %+v", got)
	}
	term.Feed([]byte("\x1b]7;" + directoryURL(t.TempDir()) + "\x07"))
	term.mu.Lock()
	got = term.linkAt(3, 0)
	term.mu.Unlock()
	if got.url != "" {
		t.Errorf("old working directory remained in use: %+v", got)
	}
	// A local shell without OSC 7 starts in Options.Dir. A Conn without a
	// local directory report has no local fallback.
	term.Feed([]byte("\x1b]7;\x07"))
	term.mu.Lock()
	connection := term.opts.Conn
	term.opts.Dir, term.opts.Conn = dir, nil
	if got := term.linkAt(3, 0); got.path != filepath.Join(dir, "file.go") {
		t.Errorf("initial shell directory: %+v", got)
	}
	term.opts.Conn = connection
	if got := term.linkAt(3, 0); got.url != "" {
		t.Errorf("remote connection used local fallback: %+v", got)
	}
	term.mu.Unlock()
}

func TestViewOpensPaths(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("paths require a local filesystem")
	}
	term := pathTerminal(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "路径")
	if err := os.WriteFile(file, []byte("terminal path witness"), 0o600); err != nil {
		t.Fatal(err)
	}
	ref := &Ref{}
	env := core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}
	if runtime.GOOS == "windows" {
		env.OS = core.OSWindows
	} else if runtime.GOOS == "linux" {
		env.OS = core.OSLinux
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return View{Terminal: term, Ref: ref, TestID: "term", Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}}
	}, nsgotest.Size(640, 360), nsgotest.Environment(env))
	app.WaitUntil(5*time.Second, func() bool { return ref.v != nil && ref.v.cols > 20 })
	term.Feed([]byte("\x1b]7;" + directoryURL(dir) + "\x07\x1b[2J\x1b[H路径\r\nnot-a-file\r\nhttps://example.com"))
	app.Settle()
	v := ref.v
	at := func(col, row int) core.Point {
		return core.Point{X: core.Points(v.padX + (float32(col)+0.5)*v.cellW), Y: core.Points(v.padY + (float32(row)+0.5)*v.cellH)}
	}
	mods := core.Modifiers{Control: true}
	if runtime.GOOS == "darwin" {
		mods = core.Modifiers{Platform: true}
	}
	click := func(col, row int, held core.Modifiers) {
		app.PressAtWith("id:term-screen", at(col, row), held)
		app.Settle()
	}
	click(3, 0, core.Modifiers{})
	if len(app.OpenedFiles) != 0 {
		t.Fatal("plain click opened a path")
	}
	click(3, 0, mods)
	app.WaitUntil(3*time.Second, func() bool { return len(app.OpenedFiles) == 1 })
	if app.OpenedFiles[0].Path != file {
		t.Fatalf("opened file %q, want %q", app.OpenedFiles[0].Path, file)
	}
	click(3, 1, mods)
	if len(app.OpenedFiles) != 1 {
		t.Fatal("nonexistent path opened")
	}
	app.Post(func() {
		v.pressDown(core.PressEvent{Position: at(3, 0), Modifiers: mods, Clicks: 1})
		v.pressUp(core.PressEvent{Position: at(3, 1), Modifiers: mods, Clicks: 1})
	})
	app.Settle()
	if len(app.OpenedFiles) != 1 {
		t.Fatal("release outside the path opened it")
	}
	click(3, 2, mods)
	app.WaitUntil(3*time.Second, func() bool { return len(app.OpenedURLs) == 1 })
	if app.OpenedURLs[0] != "https://example.com" {
		t.Fatalf("URL changed: %v", app.OpenedURLs)
	}
	t.Logf("opened file=%s URL=%s", filepath.Base(app.OpenedFiles[0].Path), app.OpenedURLs[0])
}

func TestWrappedPathAndFileHyperlink(t *testing.T) {
	term := pathTerminal(t)
	dir := t.TempDir()
	name := strings.Repeat("x", 25) + ".txt"
	file := filepath.Join(dir, name)
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	term.Resize(20, 4)
	term.Feed([]byte("\x1b]7;" + directoryURL(dir) + "\x07\x1b[2J\x1b[H" + name + "\r\n\x1b]8;;" + directoryURL(file) + "\x1b\\named\x1b]8;;\x1b\\"))
	term.mu.Lock()
	defer term.mu.Unlock()
	for _, at := range [][2]int{{3, 0}, {3, 1}, {2, 2}} {
		got := term.linkAt(at[0], at[1])
		if got.path != file {
			t.Errorf("path at %v: %+v", at, got)
		}
		if at[1] < 2 && len(got.spans) != 2 {
			t.Errorf("wrapped path spans: %+v", got.spans)
		}
	}
}

func TestPathWideCells(t *testing.T) {
	term := pathTerminal(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "路径")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	term.Resize(20, 4)
	term.Feed([]byte("\x1b]7;" + directoryURL(dir) + "\x07\x1b[2J\x1b[H路径"))
	term.mu.Lock()
	defer term.mu.Unlock()
	for col := range 4 {
		got := term.linkAt(col, 0)
		if got.path != file || len(got.spans) != 1 || got.spans[0] != (linkSpan{0, 0, 3}) {
			t.Errorf("wide path at %d: %+v", col, got)
		}
	}
}
