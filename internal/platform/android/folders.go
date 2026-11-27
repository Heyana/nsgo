//go:build android || nsgoandroidhost

package android

import (
	"errors"
	"sync"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// A folder picked on Android is a document tree the runtime holds the
// grant to, so the Go process reads it by asking Java: msgFolder lists a
// folder, opens, reads or closes a file, and msgFolderReply answers. A
// document is its tree's URI and its ID.

// folders are the folder reads waiting for Java's answers, by request.
type folders struct {
	mu      sync.Mutex
	last    uint32
	waiting map[uint32]chan *reader
	// picks are the folder picks Java has yet to answer, by their answerKey, whose EventFiles carry folders rather than files.
	picks map[uint64]bool
}

// askFolder sends a msgFolder of op with what write adds and waits for Java's answer, its error read.
func (s *surface) askFolder(op byte, write func(w *writer)) (*reader, error) {
	f := &s.folders
	ch := make(chan *reader, 1)
	f.mu.Lock()
	f.last++
	n := f.last
	if f.waiting == nil {
		f.waiting = map[uint32]chan *reader{}
	}
	f.waiting[n] = ch
	f.mu.Unlock()
	s.send(msgFolder, func(w *writer) {
		w.u32(n)
		w.byte(op)
		if write != nil {
			write(w)
		}
	})
	r := <-ch
	if msg := r.str(); msg != "" {
		return nil, errors.New(msg)
	}
	return r, nil
}

func (s *surface) supportsFolderHandles() bool { return s.folderHandles.Load() }

// folderReply hands Java's answer to the read waiting for it.
func (s *surface) folderReply(r *reader) {
	f := &s.folders
	n := r.u32()
	f.mu.Lock()
	ch := f.waiting[n]
	delete(f.waiting, n)
	f.mu.Unlock()
	if ch != nil {
		ch <- &reader{buf: append([]byte(nil), r.buf...)}
	}
}

// pickFolders records a folder pick, by its answerKey, for its answer.
func (s *surface) pickFolders(id uint64) {
	f := &s.folders
	f.mu.Lock()
	if f.picks == nil {
		f.picks = map[uint64]bool{}
	}
	f.picks[id] = true
	f.mu.Unlock()
}

// pickedFolders reports whether EventFiles id answers a folder pick, which
// it then forgets.
func (s *surface) pickedFolders(id uint64) bool {
	f := &s.folders
	f.mu.Lock()
	defer f.mu.Unlock()
	ok := f.picks[id]
	delete(f.picks, id)
	return ok
}

// readFolders reads the folders an EventFiles answering a folder pick
// carries, one per line: the name, the tree's URI, its root's ID and
// optionally its persisted token.
func (s *surface) readFolders(data string) []driver.Directory {
	return parseFolders(s, data)
}

// openDirectory reopens a persisted SAF tree grant by token on a worker goroutine.
func (s *surface) openDirectory(surfaceID, count uint32, token string) {
	if !s.folderPersist.Load() {
		s.dispatchTo(surfaceID, driver.Event{Kind: driver.EventFiles, Count: count, Text: driver.NoDirectoryPersistence})
		return
	}
	go func() {
		dir, err := reopenFolder(s, token)
		ev := driver.Event{Kind: driver.EventFiles, Count: count}
		if err != nil {
			ev.Text = err.Error()
		} else {
			ev.Data = dir
		}
		s.dispatchTo(surfaceID, ev)
	}()
}

// releaseDirectory drops a persisted SAF tree grant by token on a worker goroutine.
func (s *surface) releaseDirectory(surfaceID, count uint32, token string) {
	if !s.folderPersist.Load() {
		s.dispatchTo(surfaceID, driver.Event{Kind: driver.EventFiles, Count: count, Text: driver.NoDirectoryPersistence})
		return
	}
	go func() {
		ev := driver.Event{Kind: driver.EventFiles, Count: count}
		if err := releaseFolder(s, token); err != nil {
			ev.Text = err.Error()
		}
		s.dispatchTo(surfaceID, ev)
	}()
}
