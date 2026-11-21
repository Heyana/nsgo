//go:build windows

package windows

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"unicode/utf16"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// A window station owns a separate clipboard. The child process changes
// its station before creating any windows, without opening the user's.
func TestNativeClipboardItems(t *testing.T) {
	const child = "NSGO_TEST_PRIVATE_CLIPBOARD"
	if os.Getenv(child) != "1" {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestNativeClipboardItems$", "-test.v")
		cmd.Env = append(os.Environ(), child+"=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("private clipboard test: %v\n%s", err, out)
		}
		if bytes.Contains(out, []byte("--- SKIP:")) {
			t.Skipf("private clipboard test:\n%s", out)
		}
		t.Logf("private clipboard test:\n%s", out)
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	w := &window{hwnd: privateClipboardWindow(t)}
	defer destroyWindow.Call(w.hwnd)

	file := filepath.Join(t.TempDir(), "clipboard probe.txt")
	if err := os.WriteFile(file, []byte("clipboard fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	// DROPFILES with a wide, double-NUL-terminated file list.
	drop := make([]byte, 20)
	binary.LittleEndian.PutUint32(drop, 20)
	binary.LittleEndian.PutUint32(drop[16:], 1)
	for _, unit := range utf16.Encode([]rune(file + "\x00\x00")) {
		drop = binary.LittleEndian.AppendUint16(drop, unit)
	}
	var text []byte
	for _, unit := range utf16.Encode([]rune("bold\x00")) {
		text = binary.LittleEndian.AppendUint16(text, unit)
	}
	reps := []driver.ClipboardRepresentation{
		{Type: "CF_UNICODETEXT", Data: text},
		{Type: "Rich Text Format", Data: []byte("{\\rtf1 \\b bold}\x00")},
		{Type: "CF_HDROP", Data: drop},
		{Type: "NSGo test bytes", Data: []byte{1, 0, 2, 3}},
		{Type: "Clipboard Viewer Ignore"},
	}
	w.setClipboard("bold")
	before := clipboardChangeCount()
	if err := w.writeClipboardItems([]driver.ClipboardItem{{Representations: reps}}); err != nil {
		t.Fatal(err)
	}
	seeded := clipboardChangeCount()
	saved, err := w.readClipboardItems()
	if err != nil || len(saved) != 1 {
		t.Fatalf("saved %v items: %v", len(saved), err)
	}

	w.setClipboard("temporary")
	overwritten := clipboardChangeCount()
	if got := w.clipboard(); got != "temporary" {
		t.Fatalf("overwrote the clipboard with %q", got)
	}
	if err := w.writeClipboardItems(saved); err != nil {
		t.Fatal(err)
	}
	restored := clipboardChangeCount()
	if before < 0 || seeded <= before || overwritten <= seeded || restored <= overwritten {
		t.Errorf("clipboard change counts = %d, %d, %d, %d", before, seeded, overwritten, restored)
	}
	items, err := w.readClipboardItems()
	if err != nil || len(items) != 1 {
		t.Fatalf("restored %v items: %v", len(items), err)
	}
	formats := func(item driver.ClipboardItem) map[string][]byte {
		out := map[string][]byte{}
		for _, rep := range item.Representations {
			out[rep.Type] = rep.Data
		}
		return out
	}
	beforeData, afterData := formats(saved[0]), formats(items[0])
	for _, rep := range reps {
		if recorded, ok := beforeData[rep.Type]; !ok || !bytes.HasPrefix(recorded, rep.Data) {
			t.Errorf("the saved clipboard lacks the bytes of %q", rep.Type)
		}
	}
	for name, want := range beforeData {
		if got, ok := afterData[name]; !ok || !bytes.Equal(want, got) {
			t.Errorf("restored format %q has %d bytes, want %d", name, len(got), len(want))
		}
	}
	if got := w.clipboard(); got != "bold" {
		t.Errorf("restored text = %q", got)
	}
	if ok, _, err := openClipboard.Call(w.hwnd); ok == 0 {
		t.Fatal(err)
	}
	defer closeClipboard.Call()
	h, _, _ := getClipboardData.Call(cfHDrop)
	if got := hdropFiles(h); len(got) != 1 || got[0] != fileURL(file) {
		t.Errorf("restored file drop = %v", got)
	}
}

func privateClipboardWindow(t *testing.T) uintptr {
	t.Helper()
	const genericAll = 0x10000000
	createStation := user32.NewProc("CreateWindowStationW")
	setStation := user32.NewProc("SetProcessWindowStation")
	createDesktop := user32.NewProc("CreateDesktopW")
	setDesktop := user32.NewProc("SetThreadDesktop")

	name := wide(fmt.Sprintf("NSGoTestClipboard%d", os.Getpid()))
	station, _, err := createStation.Call(uintptr(unsafe.Pointer(name)), 0, genericAll, 0)
	if station == 0 {
		t.Skipf("cannot create a private window station: %v", err)
	}
	if ok, _, err := setStation.Call(station); ok == 0 {
		t.Skipf("cannot select the private window station: %v", err)
	}
	desktop, _, err := createDesktop.Call(uintptr(unsafe.Pointer(wide("Clipboard"))), 0, 0, 0, genericAll, 0)
	if desktop == 0 {
		t.Skipf("cannot create the private clipboard desktop: %v", err)
	}
	if ok, _, err := setDesktop.Call(desktop); ok == 0 {
		t.Skipf("cannot select the private clipboard desktop: %v", err)
	}
	// The station and desktop handles live only in this test child.
	hwnd, _, err := createWindowExW.Call(0, uintptr(unsafe.Pointer(wide("STATIC"))), 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0)
	if hwnd == 0 {
		t.Skipf("cannot create a window on the private clipboard desktop: %v", err)
	}
	return hwnd
}
