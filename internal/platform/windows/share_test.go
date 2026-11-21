//go:build windows

package windows

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWindowsShareHelpers(t *testing.T) {
	if got := windowsShareTitle("My App", driver.ShareRequest{Text: "hello"}); got != "My App" {
		t.Fatalf("title = %q, want My App", got)
	}
	if got := windowsShareTitle("  ", driver.ShareRequest{Files: []driver.SharedFile{{Name: "Notes.txt"}}}); got != "Notes.txt" {
		t.Fatalf("title = %q, want Notes.txt", got)
	}
	if got := windowsShareTitle("", driver.ShareRequest{Text: "hello"}); got != "Share" {
		t.Fatalf("title = %q, want Share", got)
	}

	for _, tc := range []struct {
		in, want string
	}{
		{"https://example.com/a?b=1", "https://example.com/a?b=1"},
		{"  http://example.com  ", "http://example.com"},
		{"HTTPS://Example.com/x", "HTTPS://Example.com/x"},
		{"hello https://example.com", ""},
		{"https://example.com/a b", ""},
		{"javascript:alert(1)", ""},
		{"file:///C:/tmp/a.txt", ""},
		{"plain text", ""},
		{"", ""},
	} {
		if got := shareWebLink(tc.in); got != tc.want {
			t.Errorf("shareWebLink(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	for _, hr := range []uintptr{0, eNotImpl, eNoInterface, classENotAvailable, regdbEClassNotReg, hrNotSupported, hrNotFound} {
		ev := windowsShareEvent(7, "ShowShareUIForWindow", hr)
		if ev.Kind != driver.EventFiles || ev.Count != 7 || ev.Finished || ev.Text != driver.NoShareSheet {
			t.Errorf("hr 0x%x -> %+v, want NoShareSheet", hr, ev)
		}
	}
	ev := windowsShareEvent(8, "ShowShareUIForWindow", 0x80004005)
	if ev.Kind != driver.EventFiles || ev.Count != 8 || ev.Finished || !strings.Contains(ev.Text, "80004005") {
		t.Fatalf("E_FAIL -> %+v", ev)
	}
}

func TestStageWindowsShareFiles(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(src, []byte("%PDF"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths, err := stageWindowsShareFiles([]driver.SharedFile{
		{Path: src, Name: "report.pdf", MIME: "application/pdf"},
		{Path: src, Name: "Q3.pdf", MIME: "application/pdf"},
		{Data: []byte("bytes"), Name: "note.txt", MIME: "text/plain"},
	})
	if err != nil {
		t.Fatalf("stageWindowsShareFiles: %v", err)
	}
	if len(paths) != 3 {
		t.Fatalf("got %d paths, want 3", len(paths))
	}
	if paths[0] != src {
		t.Errorf("paths[0] = %q, want %q", paths[0], src)
	}
	if filepath.Base(paths[1]) != "Q3.pdf" {
		t.Errorf("paths[1] = %q", paths[1])
	}
	if b, err := os.ReadFile(paths[2]); err != nil || string(b) != "bytes" || filepath.Base(paths[2]) != "note.txt" {
		t.Errorf("paths[2] = %q (%q, %v)", paths[2], b, err)
	}
}

type fakeStorageItem struct {
	vtbl *[128]uintptr
	refs int32
}

var fakeStorageItemVtbl = [128]uintptr{
	comQueryInterface: syscall.NewCallback(func(f *fakeStorageItem, iid *guid, out *unsafe.Pointer) uintptr {
		if *iid == iidUnknown || *iid == iidStorageItem {
			*out = unsafe.Pointer(f)
			f.refs++
			return sOK
		}
		*out = nil
		return eNoInterface
	}),
	comAddRef: syscall.NewCallback(func(f *fakeStorageItem) uintptr {
		f.refs++
		return uintptr(f.refs)
	}),
	comRelease: syscall.NewCallback(func(f *fakeStorageItem) uintptr {
		f.refs--
		return uintptr(f.refs)
	}),
}

func TestStorageItemIterableCOM(t *testing.T) {
	a := &fakeStorageItem{vtbl: &fakeStorageItemVtbl, refs: 1}
	b := &fakeStorageItem{vtbl: &fakeStorageItemVtbl, refs: 1}
	ca := (*comObject)(unsafe.Pointer(a))
	cb := (*comObject)(unsafe.Pointer(b))

	iterable := newStorageItemIterable([]*comObject{ca, cb})
	if a.refs != 2 || b.refs != 2 {
		t.Fatalf("refs after create = %d, %d", a.refs, b.refs)
	}

	var qi unsafe.Pointer
	if hr := storageIterableQueryInterface(iterable, &iidStorageItemIterable, &qi); hr != sOK || qi != unsafe.Pointer(iterable) {
		t.Fatalf("QI IIterable = 0x%x, %p", hr, qi)
	}
	storageIterableRelease(iterable)
	if hr := storageIterableQueryInterface(iterable, &iidAgileObject, &qi); hr != sOK {
		t.Fatalf("QI IAgileObject = 0x%x", hr)
	}
	storageIterableRelease(iterable)
	if hr := storageIterableQueryInterface(iterable, &iidStorageItem, &qi); hr != eNoInterface || qi != nil {
		t.Fatalf("QI unrelated = 0x%x, %p", hr, qi)
	}

	var iterPtr unsafe.Pointer
	if hr := storageIterableFirst(iterable, &iterPtr); hr != sOK || iterPtr == nil {
		t.Fatalf("First = 0x%x, %p", hr, iterPtr)
	}
	it := (*storageItemIterator)(iterPtr)
	if a.refs != 3 || b.refs != 3 {
		t.Fatalf("refs after First = %d, %d", a.refs, b.refs)
	}

	var has byte
	storageIteratorHasCurrent(it, &has)
	if has != 1 {
		t.Fatalf("HasCurrent[0] = %d, want 1", has)
	}
	var cur unsafe.Pointer
	if hr := storageIteratorCurrent(it, &cur); hr != sOK || cur != unsafe.Pointer(a) {
		t.Fatalf("Current[0] = 0x%x, %p", hr, cur)
	}
	ca.release()

	storageIteratorMoveNext(it, &has)
	if has != 1 {
		t.Fatalf("MoveNext[0->1] = %d, want 1", has)
	}
	var many [2]uintptr
	var actual uint32
	if hr := storageIteratorGetMany(it, 2, unsafe.Pointer(&many[0]), &actual); hr != sOK || actual != 1 || many[0] != cb.ptr() {
		t.Fatalf("GetMany = 0x%x, actual=%d, got=%x", hr, actual, many[0])
	}
	cb.release()

	storageIteratorHasCurrent(it, &has)
	if has != 0 {
		t.Fatalf("HasCurrent at end = %d, want 0", has)
	}
	if hr := storageIteratorCurrent(it, &cur); hr != eBounds || cur != nil {
		t.Fatalf("Current at end = 0x%x, %p", hr, cur)
	}

	storageIteratorRelease(it)
	iterable.release()
	if a.refs != 1 || b.refs != 1 {
		t.Fatalf("refs after release = %d, %d, want 1, 1", a.refs, b.refs)
	}
}
