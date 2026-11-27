package driver

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"testing"
	"testing/fstest"
	"time"
)

// bytesFile is a file's bytes as a platform's reader gives them.
type bytesFile struct{ *bytes.Reader }

func (bytesFile) Close() error { return nil }

func openBytes(data string) func() (io.ReadSeekCloser, error) {
	return func() (io.ReadSeekCloser, error) { return bytesFile{bytes.NewReader([]byte(data))}, nil }
}

func TestTreeFS(t *testing.T) {
	when := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	fsys := TreeFS([]TreeFile{
		{Path: "Bach/Goldberg/01 Aria.flac", Size: 4, Modified: when, Open: openBytes("aria")},
		{Path: "Bach/cover.jpg", Size: 3, Open: openBytes("jpg")},
		{Path: "notes.txt", Size: 5, Open: openBytes("notes")},
		{Path: "../outside", Open: openBytes("no")},
	})
	if err := fstest.TestFS(fsys, "Bach/Goldberg/01 Aria.flac", "Bach/cover.jpg", "notes.txt"); err != nil {
		t.Fatal(err)
	}
	got, err := fs.ReadFile(fsys, "Bach/Goldberg/01 Aria.flac")
	if err != nil || string(got) != "aria" {
		t.Fatalf("ReadFile = %q, %v", got, err)
	}
	if info, _ := fs.Stat(fsys, "Bach/Goldberg/01 Aria.flac"); !info.ModTime().Equal(when) {
		t.Errorf("ModTime = %v, want %v", info.ModTime(), when)
	}
}

// listing is a FolderSource over a map, counting its listings.
type listing struct {
	dirs  map[string][]FolderEntry
	lists int
}

func (l *listing) List(name string, e FolderEntry) ([]FolderEntry, error) {
	l.lists++
	if e.Handle != name {
		return nil, errors.New("listed by the wrong handle")
	}
	return l.dirs[name], nil
}

func (l *listing) Open(name string, e FolderEntry) (io.ReadSeekCloser, error) {
	return bytesFile{bytes.NewReader([]byte(e.Handle.(string)))}, nil
}

func TestFolderFS(t *testing.T) {
	src := &listing{dirs: map[string][]FolderEntry{
		".":   {{Name: "b.txt", Size: 1, Handle: "b"}, {Name: "a", Dir: true, Handle: "a"}, {Name: "..", Dir: true}},
		"a":   {{Name: "c.txt", Size: 1, Handle: "c"}, {Name: "d", Dir: true, Handle: "a/d"}},
		"a/d": nil,
	}}
	fsys := FolderFS(src, FolderEntry{Handle: "."})
	if err := fstest.TestFS(fsys, "a/c.txt", "b.txt", "a/d"); err != nil {
		t.Fatal(err)
	}
	if got, err := fs.ReadFile(fsys, "a/c.txt"); err != nil || string(got) != "c" {
		t.Fatalf("ReadFile = %q, %v", got, err)
	}
	if _, err := fsys.Open("a/c.txt/e"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a file's child: %v, want ErrNotExist", err)
	}

	// Opening what a listing named lists nothing again; reading a folder
	// lists it afresh.
	src.lists = 0
	src.dirs["."] = append(src.dirs["."], FolderEntry{Name: "new.txt", Handle: "new"})
	if _, err := fsys.Open("a/c.txt"); err != nil || src.lists != 0 {
		t.Fatalf("Open listed %d times: %v", src.lists, err)
	}
	if _, err := fsys.Open("new.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a file added since the listing: %v", err)
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if want := []string{"a", "b.txt", "new.txt"}; !slices.Equal(names, want) {
		t.Errorf("ReadDir = %q, want %q", names, want)
	}
	if _, err := fsys.Open("new.txt"); err != nil {
		t.Errorf("a file the new listing names: %v", err)
	}
}

func TestDirectoryFS(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(path.Join(dir, "song.mp3"), []byte("mp3"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A folder on disk reads through its path, one without through its FS.
	for _, d := range []Directory{{Path: dir}, {FS: fstest.MapFS{"song.mp3": {Data: []byte("mp3")}}}} {
		if err := fstest.TestFS(d, "song.mp3"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := (Directory{}).Open("."); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a Directory without a path: %v", err)
	}
	if got := FinishDirectories(DirectoriesRequest{Limit: 1}, []Directory{{FS: fstest.MapFS{}}}); got[0].Name != "" {
		t.Errorf("a folder without a path is named %q", got[0].Name)
	}
}

// seekOnlyFile implements io.ReadSeekCloser without io.ReaderAt.
type seekOnlyFile struct{ r *bytes.Reader }

func (f seekOnlyFile) Read(p []byte) (int, error)                { return f.r.Read(p) }
func (f seekOnlyFile) Seek(off int64, whence int) (int64, error) { return f.r.Seek(off, whence) }
func (seekOnlyFile) Close() error                                { return nil }

func TestFolderFSReaderAt(t *testing.T) {
	// A source whose opened files implement io.ReaderAt exposes it through FolderFS and TreeFS.
	fsys := TreeFS([]TreeFile{{Path: "track.flac", Size: 10, Open: openBytes("0123456789")}})
	f, err := fsys.Open("track.flac")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ra, ok := f.(io.ReaderAt)
	if !ok {
		t.Fatal("opened TreeFS file does not implement io.ReaderAt")
	}
	var head [2]byte
	if n, err := f.Read(head[:]); err != nil || n != 2 || string(head[:]) != "01" {
		t.Fatalf("Read = %d, %q, %v", n, head, err)
	}
	var mid [4]byte
	if n, err := ra.ReadAt(mid[:], 4); err != nil || n != 4 || string(mid[:]) != "4567" {
		t.Fatalf("ReadAt = %d, %q, %v", n, mid, err)
	}
	// ReadAt must not disturb the sequential read cursor.
	var next [2]byte
	if n, err := f.Read(next[:]); err != nil || n != 2 || string(next[:]) != "23" {
		t.Fatalf("Read after ReadAt = %d, %q, %v", n, next, err)
	}

	// A source whose opened files do not implement io.ReaderAt stays without it.
	plain := TreeFS([]TreeFile{{Path: "plain.txt", Size: 4, Open: func() (io.ReadSeekCloser, error) {
		return seekOnlyFile{bytes.NewReader([]byte("abcd"))}, nil
	}}})
	pf, err := plain.Open("plain.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	if _, ok := pf.(io.ReaderAt); ok {
		t.Fatal("opened file without io.ReaderAt unexpectedly implements io.ReaderAt")
	}
}

type mediaListing struct {
	listing
	uris map[string]string
}

func (m *mediaListing) MediaURI(_ string, e FolderEntry) (string, error) {
	if u, ok := m.uris[e.Handle.(string)]; ok {
		return u, nil
	}
	return "", ErrNoMediaURI
}

func TestDirectoryMediaURI(t *testing.T) {
	// Disk-backed Directory returns a file:// URL for regular files and rejects dirs/missing/escapes.
	dir := t.TempDir()
	sub := path.Join(dir, "Album #1")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path.Join(sub, "01 Aria.flac"), []byte("flac"), 0o644); err != nil {
		t.Fatal(err)
	}
	diskDir := Directory{Path: dir}
	uri, err := diskDir.MediaURI("Album #1/01 Aria.flac")
	if err != nil || uri != FilePathURL(path.Join(dir, "Album #1", "01 Aria.flac")) {
		t.Fatalf("disk MediaURI = %q, %v", uri, err)
	}
	if _, err := diskDir.MediaURI("Album #1"); err == nil {
		t.Error("MediaURI on directory succeeded")
	}
	if _, err := diskDir.MediaURI("missing.flac"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("MediaURI on missing file = %v, want ErrNotExist", err)
	}
	if _, err := diskDir.MediaURI("../escape.flac"); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("MediaURI on invalid path = %v, want ErrInvalid", err)
	}

	// FolderFS with FolderMediaSource resolves URIs, listing uncached folders on demand.
	src := &mediaListing{
		listing: listing{dirs: map[string][]FolderEntry{
			".": {{Name: "disc", Dir: true, Handle: "disc"}},
			"disc": {
				{Name: "song.flac", Size: 4, Handle: "doc-1"},
			},
		}},
		uris: map[string]string{
			"doc-1": "content://com.android.externalstorage.documents/tree/primary%3AMusic/document/primary%3AMusic%2Fdisc%2Fsong.flac",
		},
	}
	safDir := Directory{Name: "Music", FS: FolderFS(src, FolderEntry{Handle: "."})}
	got, err := safDir.MediaURI("disc/song.flac")
	if err != nil || got != src.uris["doc-1"] {
		t.Fatalf("FolderFS MediaURI = %q, %v", got, err)
	}
	if _, err := safDir.MediaURI("disc"); err == nil {
		t.Error("FolderFS MediaURI on directory succeeded")
	}
	if _, err := safDir.MediaURI("disc/nope.flac"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("FolderFS MediaURI on missing = %v, want ErrNotExist", err)
	}

	// FolderFS without FolderMediaSource and generic fs.FS report ErrNoMediaURI.
	plainDir := Directory{FS: TreeFS([]TreeFile{{Path: "song.mp3", Size: 3, Open: openBytes("mp3")}})}
	if _, err := plainDir.MediaURI("song.mp3"); !errors.Is(err, ErrNoMediaURI) {
		t.Errorf("plain FolderFS MediaURI = %v, want ErrNoMediaURI", err)
	}
	mapDir := Directory{FS: fstest.MapFS{"song.mp3": {Data: []byte("mp3")}}}
	if _, err := mapDir.MediaURI("song.mp3"); !errors.Is(err, ErrNoMediaURI) {
		t.Errorf("MapFS MediaURI = %v, want ErrNoMediaURI", err)
	}
	if _, err := (Directory{}).MediaURI("song.mp3"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("empty Directory MediaURI = %v, want ErrNotExist", err)
	}
}
