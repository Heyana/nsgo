package android

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type fakeFolderDoc struct {
	name  string
	id    string
	dir   bool
	data  []byte
	sized bool
}

type fakeFolderBridge struct {
	handles bool

	mu         sync.Mutex
	children   map[string][]fakeFolderDoc
	docs       map[string][]byte
	nextHandle uint32
	open       map[uint32]string
	opCounts   map[byte]int
	persisted  map[string]struct {
		name   string
		rootID string
	}
}

func newFakeFolderBridge(handles bool) *fakeFolderBridge {
	return &fakeFolderBridge{
		handles:    handles,
		children:   map[string][]fakeFolderDoc{},
		docs:       map[string][]byte{},
		nextHandle: 1,
		open:       map[uint32]string{},
		opCounts:   map[byte]int{},
		persisted: map[string]struct {
			name   string
			rootID string
		}{},
	}
}

func (b *fakeFolderBridge) supportsFolderHandles() bool { return b.handles }

func (b *fakeFolderBridge) askFolder(op byte, write func(w *writer)) (*reader, error) {
	var req writer
	if write != nil {
		write(&req)
	}
	in := reader{req.buf}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.opCounts[op]++

	var out writer
	switch op {
	case folderList:
		_ = in.str() // tree URI
		docID := in.str()
		entries := b.children[docID]
		out.u32(uint32(len(entries)))
		for _, e := range entries {
			out.str(e.name)
			out.str(e.id)
			if e.dir {
				out.byte(1)
			} else {
				out.byte(0)
			}
			if e.sized {
				out.u64(uint64(len(e.data)))
			} else {
				var unknown int64 = -1
				out.u64(uint64(unknown))
			}
			out.u64(0)
		}
	case folderOpen:
		_ = in.str() // tree URI
		docID := in.str()
		if _, ok := b.docs[docID]; !ok {
			return nil, fmt.Errorf("missing document %s", docID)
		}
		h := b.nextHandle
		b.nextHandle++
		b.open[h] = docID
		out.u32(h)
	case folderReadHandle:
		h := in.u32()
		off := int64(in.u64())
		n := int(in.u32())
		docID, ok := b.open[h]
		if !ok {
			return nil, fmt.Errorf("document handle %d is closed", h)
		}
		out.str(string(sliceBytes(b.docs[docID], off, n)))
	case folderClose:
		h := in.u32()
		delete(b.open, h)
	case folderReopen:
		token := in.str()
		entry, ok := b.persisted[token]
		if !ok {
			return nil, errors.New("directory grant is no longer held")
		}
		out.str(entry.name)
		out.str(token)
		out.str(entry.rootID)
	case folderRelease:
		token := in.str()
		delete(b.persisted, token)
	case folderRead:
		_ = in.str() // tree URI
		docID := in.str()
		off := int64(in.u64())
		n := int(in.u32())
		data, ok := b.docs[docID]
		if !ok {
			return nil, fmt.Errorf("missing document %s", docID)
		}
		out.str(string(sliceBytes(data, off, n)))
	default:
		return nil, fmt.Errorf("unexpected op %d", op)
	}
	return &reader{out.buf}, nil
}

func sliceBytes(data []byte, off int64, n int) []byte {
	if off < 0 || off >= int64(len(data)) || n <= 0 {
		return nil
	}
	end := min(off+int64(n), int64(len(data)))
	return data[off:end]
}

// TestFolderHandlesCapabilityBit keeps handle-based folder ops gated behind a
// free fourth-byte capability bit that Protocol.java and NsgoView.java advertise,
// and verifies the FOLDER_* op numbers match Protocol.java.
func TestFolderHandlesCapabilityBit(t *testing.T) {
	if capFolderHandles == 0 || capFolderHandles&(capFolderHandles-1) != 0 ||
		capFolderHandles&(capExtendedSensors|capAuthSession|capVideoPiP|capCapture) != 0 {
		t.Fatalf("capFolderHandles %d is not a free single bit of the fourth byte", capFolderHandles)
	}
	protocol, err := os.ReadFile("java/dev/oneo/nsgo/Protocol.java")
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name string
		want int
	}{
		{"CAP_FOLDER_HANDLES", capFolderHandles},
		{"CAP_FOLDER_PERSIST", capFolderPersist},
		{"FOLDER_LIST", folderList},
		{"FOLDER_READ", folderRead},
		{"FOLDER_OPEN", folderOpen},
		{"FOLDER_READ_HANDLE", folderReadHandle},
		{"FOLDER_CLOSE", folderClose},
		{"FOLDER_REOPEN", folderReopen},
		{"FOLDER_RELEASE", folderRelease},
	}
	for _, c := range checks {
		re := regexp.MustCompile(`static final int ` + c.name + ` = (\d+);`)
		if m := re.FindSubmatch(protocol); m == nil || string(m[1]) != strconv.Itoa(c.want) {
			t.Fatalf("Protocol.java's %s is %q, Go has %d", c.name, m, c.want)
		}
	}
	if !strings.Contains(string(protocol), "static final int PICK_DIR_PERSIST = 1 << 31;") {
		t.Fatal("Protocol.java does not define PICK_DIR_PERSIST = 1 << 31")
	}
	view, err := os.ReadFile("java/dev/oneo/nsgo/NsgoView.java")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(view), "Protocol.CAP_FOLDER_HANDLES") {
		t.Fatal("NsgoView.java does not advertise Protocol.CAP_FOLDER_HANDLES")
	}
	if !strings.Contains(string(view), "Protocol.CAP_FOLDER_PERSIST") {
		t.Fatal("NsgoView.java does not advertise Protocol.CAP_FOLDER_PERSIST")
	}

	for _, c := range []struct {
		fourth []byte
		want   bool
	}{{nil, false}, {[]byte{0}, false}, {[]byte{capCapture}, false}, {[]byte{capFolderHandles}, true}, {[]byte{capExtendedSensors | capFolderHandles}, true}} {
		var w writer
		writeBaseEnvironment(&w, "en-US")
		w.byte(1)
		for range 4 {
			w.str("")
		}
		w.byte(capExtendedMessages)
		w.flags(false)
		w.str("")
		w.buf = append(w.buf, c.fourth...)
		r := reader{w.buf}
		if _, _, fourth := readEnvironmentCapabilities(&r); (fourth&capFolderHandles != 0) != c.want {
			t.Errorf("fourth %v: folderHandles = %v, want %v", c.fourth, fourth&capFolderHandles != 0, c.want)
		}
	}
}

func TestAndroidFolderHandles(t *testing.T) {
	large := bytes.Repeat([]byte("0123456789abcdef"), (folderChunk+4096)/16)
	unsized := []byte("unsized-audio-header-and-payload")

	b := newFakeFolderBridge(true)
	b.children["root"] = []fakeFolderDoc{
		{name: "song.flac", id: "doc-song", data: large, sized: true},
		{name: "stream.ogg", id: "doc-stream", data: unsized, sized: false},
	}
	b.docs["doc-song"] = large
	b.docs["doc-stream"] = unsized

	fsys := driver.FolderFS(tree{bridge: b, uri: "content://tree"}, driver.FolderEntry{Handle: documentRef{id: "root"}})
	if err := fstest.TestFS(fsys, "song.flac", "stream.ogg"); err != nil {
		t.Fatal(err)
	}
	if len(b.open) != 0 {
		t.Fatalf("leaked %d handles after fstest.TestFS", len(b.open))
	}

	b.opCounts = map[byte]int{}
	f, err := fsys.Open("song.flac")
	if err != nil {
		t.Fatal(err)
	}
	if b.opCounts[folderOpen] != 1 || len(b.open) != 1 {
		t.Fatalf("after Open: opens=%d active=%d", b.opCounts[folderOpen], len(b.open))
	}

	ra, ok := f.(io.ReaderAt)
	if !ok {
		t.Fatal("opened Android folder file does not implement io.ReaderAt")
	}
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		t.Fatal("opened Android folder file does not implement io.ReadSeeker")
	}

	// Sequential Read + Seek + multi-chunk ReadAt reuse the single open handle.
	head := make([]byte, 16)
	if n, err := io.ReadFull(f, head); err != nil || n != 16 || !bytes.Equal(head, large[:16]) {
		t.Fatalf("Read head = %d, %v", n, err)
	}
	multi := make([]byte, folderChunk+2048)
	if n, err := ra.ReadAt(multi, 1024); err != nil || n != len(multi) || !bytes.Equal(multi, large[1024:1024+len(multi)]) {
		t.Fatalf("multi-chunk ReadAt = %d, %v", n, err)
	}
	// ReadAt must not move the sequential cursor.
	next := make([]byte, 16)
	if n, err := io.ReadFull(f, next); err != nil || !bytes.Equal(next, large[16:32]) {
		t.Fatalf("Read after ReadAt = %d, %v", n, err)
	}
	// Partial ReadAt reaching EOF must return io.EOF.
	tail := make([]byte, 32)
	if n, err := ra.ReadAt(tail, int64(len(large)-10)); n != 10 || !errors.Is(err, io.EOF) || !bytes.Equal(tail[:10], large[len(large)-10:]) {
		t.Fatalf("tail ReadAt = %d, %v", n, err)
	}
	if _, err := ra.ReadAt(tail[:1], -1); err == nil {
		t.Fatal("ReadAt with negative offset succeeded")
	}
	if n, err := ra.ReadAt(nil, 0); n != 0 || err != nil {
		t.Fatalf("empty ReadAt = %d, %v", n, err)
	}

	// Concurrent ReadAt calls run safely on one handle.
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			off := int64(i * 4096)
			buf := make([]byte, 256)
			n, err := ra.ReadAt(buf, off)
			if err != nil || n != len(buf) || !bytes.Equal(buf, large[off:off+256]) {
				errs <- fmt.Errorf("goroutine %d: n=%d err=%v", i, n, err)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	if b.opCounts[folderOpen] != 1 || b.opCounts[folderRead] != 0 || b.opCounts[folderReadHandle] == 0 {
		t.Fatalf("op counts = %+v, want 1 open, 0 stateless reads, >0 handle reads", b.opCounts)
	}

	// Close releases the handle once and refuses subsequent operations.
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if b.opCounts[folderClose] != 1 || len(b.open) != 0 {
		t.Fatalf("after Close: closes=%d active=%d", b.opCounts[folderClose], len(b.open))
	}
	if _, err := f.Read(head); !errors.Is(err, fs.ErrClosed) {
		t.Errorf("Read after Close = %v, want fs.ErrClosed", err)
	}
	if _, err := ra.ReadAt(head, 0); !errors.Is(err, fs.ErrClosed) {
		t.Errorf("ReadAt after Close = %v, want fs.ErrClosed", err)
	}
	if _, err := rs.Seek(0, io.SeekStart); !errors.Is(err, fs.ErrClosed) {
		t.Errorf("Seek after Close = %v, want fs.ErrClosed", err)
	}

	// Unsized documents: ReadAt past EOF must not record a bogus size for SeekEnd.
	uf, err := fsys.Open("stream.ogg")
	if err != nil {
		t.Fatal(err)
	}
	defer uf.Close()
	ura := uf.(io.ReaderAt)
	urs := uf.(io.ReadSeeker)
	if n, err := ura.ReadAt(make([]byte, 8), 5000); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("ReadAt past unsized EOF = %d, %v", n, err)
	}
	if _, err := urs.Seek(0, io.SeekEnd); err == nil {
		t.Fatal("SeekEnd succeeded after ReadAt past EOF on unsized document")
	}
	// A ReadAt that starts before EOF and crosses it learns the exact size.
	buf := make([]byte, 16)
	if n, err := ura.ReadAt(buf, int64(len(unsized)-4)); n != 4 || !errors.Is(err, io.EOF) {
		t.Fatalf("ReadAt across unsized EOF = %d, %v", n, err)
	}
	if end, err := urs.Seek(0, io.SeekEnd); err != nil || end != int64(len(unsized)) {
		t.Fatalf("SeekEnd after crossing EOF = %d, %v, want %d", end, err, len(unsized))
	}
}

func TestAndroidFolderLegacyFallback(t *testing.T) {
	data := []byte("legacy-runtime-bytes")
	b := newFakeFolderBridge(false)
	b.children["root"] = []fakeFolderDoc{{name: "a.bin", id: "doc-a", data: data, sized: true}}
	b.docs["doc-a"] = data

	fsys := driver.FolderFS(tree{bridge: b, uri: "content://tree"}, driver.FolderEntry{Handle: documentRef{id: "root"}})
	f, err := fsys.Open("a.bin")
	if err != nil {
		t.Fatal(err)
	}
	ra := f.(io.ReaderAt)
	got := make([]byte, 6)
	if n, err := ra.ReadAt(got, 7); err != nil || n != 6 || string(got) != "runtim" {
		t.Fatalf("legacy ReadAt = %d, %q, %v", n, got, err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if b.opCounts[folderOpen] != 0 || b.opCounts[folderReadHandle] != 0 || b.opCounts[folderClose] != 0 || b.opCounts[folderRead] != 1 {
		t.Fatalf("legacy op counts = %+v, want only folderRead=1", b.opCounts)
	}
}

func TestFolderHandlesJava(t *testing.T) {
	javac, java := androidJDK(t)
	classes := t.TempDir()
	args := []string{
		"-d", classes,
		filepath.Join("java", "dev", "oneo", "nsgo", "FolderHandles.java"),
		filepath.Join("testdata", "FolderHandlesCheck.java"),
	}
	if out, err := exec.CommandContext(t.Context(), javac, args...).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(t.Context(), java, "-cp", classes, "dev.oneo.nsgo.FolderHandlesCheck").CombinedOutput(); err != nil {
		t.Fatalf("FolderHandlesCheck: %v\n%s", err, out)
	}
}

func TestAndroidFolderPersistAndMediaURI(t *testing.T) {
	if capFolderPersist == 0 || capFolderPersist&(capFolderPersist-1) != 0 ||
		capFolderPersist&(capExtendedSensors|capAuthSession|capVideoPiP|capCapture|capFolderHandles) != 0 {
		t.Fatalf("capFolderPersist %d is not a free single bit of the fourth byte", capFolderPersist)
	}

	const treeURI = "content://com.android.externalstorage.documents/tree/primary%3AMusic"
	const rootID = "primary:Music"
	const songID = "primary:Music/Bach/01 Aria #1 (Live).flac"
	const wantURI = "content://com.android.externalstorage.documents/tree/primary%3AMusic/document/primary%3AMusic%2FBach%2F01%20Aria%20%231%20(Live).flac"

	b := newFakeFolderBridge(true)
	b.children[rootID] = []fakeFolderDoc{
		{name: "Bach", id: "primary:Music/Bach", dir: true},
	}
	b.children["primary:Music/Bach"] = []fakeFolderDoc{
		{name: "01 Aria #1 (Live).flac", id: songID, data: []byte("fLaC-audio"), sized: true},
	}
	b.docs[songID] = []byte("fLaC-audio")
	b.persisted[treeURI] = struct {
		name   string
		rootID string
	}{name: "Music", rootID: rootID}

	// Legacy 3-field line (without Persist) has empty Token; 4-field persisted line carries Token.
	dirs := parseFolders(b, "Legacy\t"+treeURI+"\t"+rootID+"\nMusic\t"+treeURI+"\t"+rootID+"\t"+treeURI+"\n")
	if len(dirs) != 2 || dirs[0].Name != "Legacy" || dirs[0].Token != "" || dirs[1].Name != "Music" || dirs[1].Token != treeURI {
		t.Fatalf("parseFolders = %+v", dirs)
	}

	// MediaURI produces the exact DocumentsContract.buildDocumentUriUsingTree content:// URI.
	gotURI, err := dirs[1].MediaURI("Bach/01 Aria #1 (Live).flac")
	if err != nil || gotURI != wantURI {
		t.Fatalf("MediaURI = %q, %v; want %q", gotURI, err, wantURI)
	}
	if _, err := dirs[1].MediaURI("Bach"); err == nil {
		t.Fatal("MediaURI on subdirectory succeeded")
	}

	// Multi-user authority (10@...) and empty rootID fallback both preserve authority and tree root.
	multi, err := androidDocumentURI("content://10@com.android.externalstorage.documents/tree/primary%3AMusic", "", "primary:Music/a b.mp3")
	if err != nil || multi != "content://10@com.android.externalstorage.documents/tree/primary%3AMusic/document/primary%3AMusic%2Fa%20b.mp3" {
		t.Fatalf("multi-user androidDocumentURI = %q, %v", multi, err)
	}

	// Reopen persisted directory via FOLDER_REOPEN, read via persistent handles + ReaderAt, and get MediaURI.
	reopened, err := reopenFolder(b, treeURI)
	if err != nil {
		t.Fatalf("reopenFolder: %v", err)
	}
	if reopened.Name != "Music" || reopened.Token != treeURI {
		t.Fatalf("reopened = %+v", reopened)
	}
	gotURI, err = reopened.MediaURI("Bach/01 Aria #1 (Live).flac")
	if err != nil || gotURI != wantURI {
		t.Fatalf("reopened MediaURI = %q, %v", gotURI, err)
	}
	f, err := reopened.Open("Bach/01 Aria #1 (Live).flac")
	if err != nil {
		t.Fatalf("reopened Open: %v", err)
	}
	buf := make([]byte, 5)
	if n, err := f.(io.ReaderAt).ReadAt(buf, 5); err != nil || n != 5 || string(buf) != "audio" {
		t.Fatalf("reopened ReadAt = %d, %q, %v", n, buf, err)
	}
	_ = f.Close()

	// Release grant via FOLDER_RELEASE; subsequent reopen fails.
	if err := releaseFolder(b, treeURI); err != nil {
		t.Fatalf("releaseFolder: %v", err)
	}
	if _, err := reopenFolder(b, treeURI); err == nil {
		t.Fatal("reopenFolder succeeded after releaseFolder")
	}
	// Release is idempotent.
	if err := releaseFolder(b, treeURI); err != nil {
		t.Fatalf("second releaseFolder: %v", err)
	}
}
