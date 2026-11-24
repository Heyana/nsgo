// Package shareinbox stages iOS share-extension items in an App Group until the containing app next activates. Committed batches are atomic directories; unfinished imports are never visible to the reader.
package shareinbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const Directory = "NSGoShares"
const maxManifest = 1 << 20
const MaxTextBytes = maxManifest / 2
const maxItems = 128

// MaxBytes bounds one incoming batch without loading its files into memory.
const MaxBytes int64 = 512 << 20

type Batch struct {
	dir   string
	items []driver.SharedItem
	bytes int64
}

func Begin(group string) (*Batch, error) {
	root := filepath.Join(group, Directory)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(root, fmt.Sprintf(".incoming-%020d-", time.Now().UnixNano()))
	if err != nil {
		return nil, err
	}
	return &Batch{dir: dir}, nil
}

func (b *Batch) Abort() { os.RemoveAll(b.dir) }

func (b *Batch) Text(text, mime string) error {
	if text == "" {
		return nil
	}
	if len(b.items) >= maxItems || len(text) > MaxTextBytes {
		return errors.New("share exceeds the item limit")
	}
	b.items = append(b.items, driver.SharedItem{Text: text, MIME: mime})
	return nil
}

func (b *Batch) File(path, name, mime string) error {
	if len(b.items) >= maxItems {
		return errors.New("share exceeds the item limit")
	}
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("shared item is not a regular file")
	}
	if info.Size() > MaxBytes-b.bytes {
		return errors.New("share exceeds 512 MiB")
	}
	rel := fmt.Sprintf("file-%03d", len(b.items))
	n, err := copyFile(filepath.Join(b.dir, rel), io.LimitReader(in, MaxBytes-b.bytes+1))
	if err != nil {
		return err
	}
	b.bytes += n
	if b.bytes > MaxBytes {
		return errors.New("share exceeds 512 MiB")
	}
	if name == "" {
		name = filepath.Base(path)
	}
	b.items = append(b.items, driver.SharedItem{Path: rel, Name: name, MIME: mime})
	return nil
}

func (b *Batch) Commit() error {
	if len(b.items) == 0 {
		return errors.New("share contains no supported items")
	}
	data, err := json.Marshal(b.items)
	if err != nil {
		return err
	}
	if len(data) > maxManifest {
		return errors.New("share manifest is too large")
	}
	if err := os.WriteFile(filepath.Join(b.dir, "items.json"), data, 0o600); err != nil {
		return err
	}
	return os.Rename(b.dir, filepath.Join(filepath.Dir(b.dir), strings.TrimPrefix(filepath.Base(b.dir), ".incoming-")))
}

// Reader tracks batches handed to this engine, retaining their durable copies until an app callback acknowledges them. A new process reads unacknowledged batches again; waking an existing reader never duplicates a pending batch.
type Reader struct {
	group, cache string
	mu           sync.Mutex
	pending      map[string]bool
}

func NewReader(group, cache string) *Reader {
	return &Reader{group: group, cache: cache, pending: map[string]bool{}}
}

// Drain copies committed batches into private cache storage. Acknowledge may run concurrently, and removes only its batch after the app callback returns.
func (r *Reader) Drain(deliver func([]driver.SharedItem, func() error)) error {
	root := filepath.Join(r.group, Directory)
	dirs, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var failures []error
	for _, entry := range dirs {
		if !entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			// A killed extension can leave a partial import, and a batch that cannot be read is set aside (rejected): remove either after a day, which outlasts any writer iOS keeps alive.
			if strings.HasPrefix(entry.Name(), ".incoming-") || strings.HasPrefix(entry.Name(), rejected) {
				if info, err := entry.Info(); err == nil && time.Since(info.ModTime()) > 24*time.Hour {
					if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
						failures = append(failures, err)
					}
				}
			}
			continue
		}
		dir := filepath.Join(root, entry.Name())
		r.mu.Lock()
		if r.pending[dir] {
			r.mu.Unlock()
			continue
		}
		r.pending[dir] = true
		r.mu.Unlock()
		forget := func() { r.mu.Lock(); delete(r.pending, dir); r.mu.Unlock() }
		items, err := readBatch(dir, r.cache)
		if err != nil {
			// A batch whose contents are invalid would fail again on every activation: set it aside. Other errors, such as a full cache, leave it for the next activation.
			var bad invalidBatch
			if errors.As(err, &bad) {
				if moveErr := os.Rename(dir, filepath.Join(root, rejected+entry.Name())); moveErr != nil {
					failures = append(failures, moveErr)
				}
			}
			forget()
			failures = append(failures, fmt.Errorf("share %s: %w", entry.Name(), err))
			continue
		}
		acknowledge := sync.OnceValue(func() error { err := os.RemoveAll(dir); forget(); return err })
		deliver(items, acknowledge)
	}
	return errors.Join(failures...)
}

// rejected prefixes a committed batch the reader could not read, which it no longer delivers.
const rejected = ".rejected-"

// invalidBatch is a readBatch error caused by the batch's own contents, which reading again cannot fix.
type invalidBatch struct{ error }

func (e invalidBatch) Unwrap() error { return e.error }

func invalid(err error) error { return invalidBatch{err} }

func readBatch(dir, cache string) ([]driver.SharedItem, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, invalid(err)
	}
	defer root.Close()
	manifest, err := root.Open("items.json")
	if err != nil {
		return nil, invalid(err)
	}
	defer manifest.Close()
	var items []driver.SharedItem
	data, err := io.ReadAll(io.LimitReader(manifest, maxManifest+1))
	if err != nil {
		return nil, invalid(err)
	}
	if len(data) > maxManifest {
		return nil, invalid(errors.New("share manifest is too large"))
	}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, invalid(err)
	}
	if len(items) == 0 || len(items) > maxItems {
		return nil, invalid(errors.New("invalid share item count"))
	}
	if err := os.MkdirAll(cache, 0o700); err != nil {
		return nil, err
	}
	out, err := os.MkdirTemp(cache, "share-")
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			os.RemoveAll(out)
		}
	}()
	var total int64
	for i := range items {
		item := &items[i]
		if (item.Text == "") == (item.Path == "") {
			return nil, invalid(errors.New("invalid shared item"))
		}
		if item.Path == "" {
			continue
		}
		// The writer's private file names are never paths chosen by a sender.
		if item.Path != fmt.Sprintf("file-%03d", i) {
			return nil, invalid(errors.New("invalid shared file name"))
		}
		info, err := root.Lstat(item.Path)
		if err != nil {
			return nil, invalid(err)
		}
		if !info.Mode().IsRegular() {
			return nil, invalid(errors.New("invalid shared file"))
		}
		in, err := root.Open(item.Path)
		if err != nil {
			return nil, invalid(err)
		}
		path := filepath.Join(out, item.Path)
		n, err := copyFile(path, io.LimitReader(in, MaxBytes-total+1))
		in.Close()
		if err != nil {
			return nil, err
		}
		total += n
		if total > MaxBytes {
			return nil, invalid(errors.New("share exceeds 512 MiB"))
		}
		item.Path = path
	}
	success = true
	return items, nil
}

func copyFile(path string, src io.Reader) (int64, error) {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(out, src)
	if err == nil {
		err = out.Sync()
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	return n, err
}
