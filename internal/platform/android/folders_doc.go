package android

import (
	"errors"
	"io"
	"io/fs"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// The requests a msgFolder makes, kept in sync with Protocol.FOLDER_* in Protocol.java.
const (
	folderList       = 0
	folderRead       = 1
	folderOpen       = 2
	folderReadHandle = 3
	folderClose      = 4
	folderReopen     = 5
	folderRelease    = 6
)

// folderChunk is the most bytes one read asks for.
const folderChunk = 1 << 20

// folderBridge sends a msgFolder request of op and waits for Java's answer, its error already read.
type folderBridge interface {
	askFolder(op byte, write func(w *writer)) (*reader, error)
	supportsFolderHandles() bool
}

// tree reads a document tree through Java, each entry's handle its documentRef.
type tree struct {
	bridge folderBridge
	uri    string
	rootID string
}

var _ driver.FolderMediaSource = tree{}

func (t tree) MediaURI(_ string, e driver.FolderEntry) (string, error) {
	ref, ok := e.Handle.(documentRef)
	if !ok || ref.id == "" {
		return "", driver.ErrNoMediaURI
	}
	return androidDocumentURI(t.uri, t.rootID, ref.id)
}

// androidDocumentURI builds the exact SAF document URI that Android's
// DocumentsContract.buildDocumentUriUsingTree(Uri.parse(treeURI), docID)
// produces: content://<authority>/tree/<encodedRoot>/document/<encodedDocID>.
// Preserves multi-user authorities such as 10@com.android.externalstorage.documents.
func androidDocumentURI(treeURI, rootID, docID string) (string, error) {
	u, err := url.Parse(treeURI)
	if err != nil || u.Scheme != "content" || u.Host == "" {
		return "", errors.New("android: invalid document tree URI")
	}
	authority := u.Host
	if u.User != nil {
		authority = u.User.String() + "@" + authority
	}
	var encodedRoot string
	if rootID != "" {
		encodedRoot = androidURIEncode(rootID)
	} else {
		p := strings.TrimPrefix(u.EscapedPath(), "/")
		if rest, ok := strings.CutPrefix(p, "tree/"); ok {
			encodedRoot, _, _ = strings.Cut(rest, "/")
		}
	}
	if encodedRoot == "" || docID == "" {
		return "", errors.New("android: document tree URI has no tree root")
	}
	return "content://" + authority + "/tree/" + encodedRoot + "/document/" + androidURIEncode(docID), nil
}

// androidURIEncode encodes s identically to Android's android.net.Uri.encode(s, null):
// unencoded bytes are A-Z, a-z, 0-9 and _-!.~'()*, everything else is %XX (uppercase).
func androidURIEncode(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	b.Grow(len(s))
	for i := range len(s) {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			strings.IndexByte("_-!.~'()*", c) >= 0 {
			b.WriteByte(c)
		} else {
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0x0f])
		}
	}
	return b.String()
}

// documentRef is a document of a tree: its ID, and whether its provider said its size.
type documentRef struct {
	id    string
	sized bool
}

func (t tree) List(_ string, e driver.FolderEntry) ([]driver.FolderEntry, error) {
	r, err := t.bridge.askFolder(folderList, func(w *writer) {
		w.str(t.uri)
		w.str(e.Handle.(documentRef).id)
	})
	if err != nil {
		return nil, err
	}
	entries := make([]driver.FolderEntry, r.u32())
	for i := range entries {
		e := &entries[i]
		e.Name = r.str()
		ref := documentRef{id: r.str()}
		e.Dir = r.byte() != 0
		// -1 for a size the provider does not say.
		e.Size = int64(r.u64())
		ref.sized = e.Size >= 0
		e.Size = max(e.Size, 0)
		e.Handle = ref
		if ms := int64(r.u64()); ms > 0 {
			e.Modified = time.UnixMilli(ms)
		}
	}
	return entries, nil
}

func (t tree) Open(_ string, e driver.FolderEntry) (io.ReadSeekCloser, error) {
	ref := e.Handle.(documentRef)
	d := &document{tree: t, id: ref.id, size: -1}
	if ref.sized {
		d.size = e.Size
	}
	if t.bridge.supportsFolderHandles() {
		r, err := t.bridge.askFolder(folderOpen, func(w *writer) {
			w.str(t.uri)
			w.str(ref.id)
		})
		if err != nil {
			return nil, err
		}
		d.handle = r.u32()
	}
	return d, nil
}

// document reads a file of a tree a chunk at a time, keeping an opened provider
// descriptor on runtimes with capFolderHandles; it seeks anywhere, supports
// concurrent ReadAt, and seeks from its end once its size is known.
type document struct {
	tree
	id string

	mu     sync.Mutex
	handle uint32
	off    int64
	// size is -1 until known.
	size   int64
	closed bool
}

var _ io.ReaderAt = (*document)(nil)

func (d *document) readChunk(off int64, p []byte) (int, error) {
	d.mu.Lock()
	closed := d.closed
	handle := d.handle
	size := d.size
	d.mu.Unlock()
	if closed {
		return 0, fs.ErrClosed
	}
	if size >= 0 && off >= size {
		return 0, io.EOF
	}
	n := min(len(p), folderChunk)
	var r *reader
	var err error
	if handle != 0 {
		r, err = d.bridge.askFolder(folderReadHandle, func(w *writer) {
			w.u32(handle)
			w.u64(uint64(off))
			w.u32(uint32(n))
		})
	} else {
		r, err = d.bridge.askFolder(folderRead, func(w *writer) {
			w.str(d.uri)
			w.str(d.id)
			w.u64(uint64(off))
			w.u32(uint32(n))
		})
	}
	if err != nil {
		return 0, err
	}
	got := copy(p, r.bytes())
	if got == 0 {
		return 0, io.EOF
	}
	return got, nil
}

func (d *document) Read(p []byte) (int, error) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return 0, fs.ErrClosed
	}
	off := d.off
	d.mu.Unlock()
	if len(p) == 0 {
		return 0, nil
	}
	got, err := d.readChunk(off, p)
	if got > 0 {
		d.mu.Lock()
		d.off = off + int64(got)
		d.mu.Unlock()
		return got, nil
	}
	if err == io.EOF {
		// The end, which the listing may not have said.
		d.mu.Lock()
		if d.size < 0 {
			d.size = off
		}
		d.mu.Unlock()
	}
	return 0, err
}

func (d *document) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("negative offset")
	}
	d.mu.Lock()
	closed := d.closed
	d.mu.Unlock()
	if closed {
		return 0, fs.ErrClosed
	}
	if len(p) == 0 {
		return 0, nil
	}
	var total int
	for len(p) > 0 {
		got, err := d.readChunk(off, p)
		total += got
		off += int64(got)
		p = p[got:]
		if err != nil {
			if err == io.EOF && total > 0 {
				d.mu.Lock()
				if d.size < 0 {
					d.size = off
				}
				d.mu.Unlock()
			}
			return total, err
		}
	}
	return total, nil
}

func (d *document) Seek(offset int64, whence int) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return 0, fs.ErrClosed
	}
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += d.off
	case io.SeekEnd:
		if d.size < 0 {
			return 0, errors.New("the document's size is unknown")
		}
		offset += d.size
	default:
		return 0, errors.New("invalid whence")
	}
	if offset < 0 {
		return 0, errors.New("seek before the start")
	}
	d.off = offset
	return offset, nil
}

func (d *document) Close() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	handle := d.handle
	d.handle = 0
	d.mu.Unlock()
	if handle != 0 {
		_, err := d.bridge.askFolder(folderClose, func(w *writer) {
			w.u32(handle)
		})
		return err
	}
	return nil
}

// parseFolders decodes the tab-separated lines Java's Services.pickedTree writes:
// name, tree URI, root document ID, and optionally the persisted grant token.
func parseFolders(bridge folderBridge, data string) []driver.Directory {
	var dirs []driver.Directory
	for line := range strings.SplitSeq(data, "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 3 {
			continue
		}
		var token string
		if len(f) >= 4 {
			token = f[3]
		}
		src := tree{bridge: bridge, uri: f[1], rootID: f[2]}
		root := driver.FolderEntry{Handle: documentRef{id: f[2]}}
		dirs = append(dirs, driver.Directory{Name: f[0], Token: token, FS: driver.FolderFS(src, root)})
	}
	return dirs
}

// reopenFolder asks Java's Services.reopenTree to verify and reopen a persisted
// SAF tree grant, returning a live Directory backed by FolderFS.
func reopenFolder(bridge folderBridge, token string) (driver.Directory, error) {
	r, err := bridge.askFolder(folderReopen, func(w *writer) {
		w.str(token)
	})
	if err != nil {
		return driver.Directory{}, err
	}
	name := r.str()
	treeURI := r.str()
	rootID := r.str()
	if treeURI == "" || rootID == "" {
		return driver.Directory{}, errors.New("android: incomplete reopened directory reply")
	}
	src := tree{bridge: bridge, uri: treeURI, rootID: rootID}
	root := driver.FolderEntry{Handle: documentRef{id: rootID}}
	return driver.Directory{Name: name, Token: treeURI, FS: driver.FolderFS(src, root)}, nil
}

// releaseFolder asks Java's Services.releaseTree to drop a persisted SAF grant.
func releaseFolder(bridge folderBridge, token string) error {
	_, err := bridge.askFolder(folderRelease, func(w *writer) {
		w.str(token)
	})
	return err
}
