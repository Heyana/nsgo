//go:build darwin

package apple

import (
	"encoding/base64"
	"errors"
	"os"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// NSURLBookmarkCreationOptions / NSURLBookmarkResolutionOptions for macOS.
// On iOS, security-scoped bookmarks are created and resolved with options 0
// while inside startAccessingSecurityScopedResource.
const (
	bookmarkCreationWithSecurityScope              uint = 1 << 11
	bookmarkCreationSecurityScopeAllowOnlyReadOnly uint = 1 << 12
	bookmarkResolutionWithSecurityScope            uint = 1 << 10
)

type scopedDirectory struct {
	url    objc.ID
	scoped bool
}

var (
	scopedDirsMu sync.Mutex
	scopedDirs   = map[string]scopedDirectory{}
)

// CreateDirectoryBookmark encodes a persistent bookmark token for an NSURL
// directory. On macOS it requests a read-only app-scoped security-scoped
// bookmark first and falls back to a standard bookmark in unsandboxed builds;
// on iOS it creates a standard security-scoped bookmark (options 0).
func CreateDirectoryBookmark(url objc.ID, macOS bool) (string, bool) {
	if url == 0 {
		return "", false
	}
	create := func(opts uint) objc.ID {
		var nsErr objc.ID
		return url.Send(sel("bookmarkDataWithOptions:includingResourceValuesForKeys:relativeToURL:error:"),
			opts, objc.ID(0), objc.ID(0), unsafe.Pointer(&nsErr))
	}
	var data objc.ID
	if macOS {
		data = create(bookmarkCreationWithSecurityScope | bookmarkCreationSecurityScopeAllowOnlyReadOnly)
		if data == 0 {
			data = create(0)
		}
	} else {
		data = create(0)
	}
	if data == 0 {
		return "", false
	}
	raw := Bytes(data)
	if len(raw) == 0 {
		return "", false
	}
	return base64.StdEncoding.EncodeToString(raw), true
}

// TrackScopedDirectory retains url under token so ReleaseDirectoryBookmark can
// stop accessing its security-scoped resource and release it.
func TrackScopedDirectory(token string, url objc.ID, scoped bool) {
	if token == "" || url == 0 {
		return
	}
	url.Send(sel("retain"))
	scopedDirsMu.Lock()
	prev, hadPrev := scopedDirs[token]
	scopedDirs[token] = scopedDirectory{url: url, scoped: scoped}
	scopedDirsMu.Unlock()
	if hadPrev && prev.url != 0 {
		if prev.scoped {
			prev.url.Send(sel("stopAccessingSecurityScopedResource"))
		}
		prev.url.Send(sel("release"))
	}
}

// OpenDirectoryBookmark resolves a bookmark token created by
// CreateDirectoryBookmark, activates its security-scoped resource, verifies
// the directory is still accessible and refreshes stale bookmarks.
func OpenDirectoryBookmark(token string, macOS bool) (driver.Directory, error) {
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil || len(raw) == 0 {
		return driver.Directory{}, errors.New("apple: invalid directory bookmark token")
	}
	nsData := NSData(raw)
	resolve := func(opts uint) (objc.ID, bool, objc.ID) {
		var stale byte
		var nsErr objc.ID
		u := objc.ID(Class("NSURL")).Send(sel("URLByResolvingBookmarkData:options:relativeToURL:bookmarkDataIsStale:error:"),
			nsData, opts, objc.ID(0), unsafe.Pointer(&stale), unsafe.Pointer(&nsErr))
		return u, stale != 0, nsErr
	}
	var url, nsErr objc.ID
	var stale bool
	if macOS {
		url, stale, nsErr = resolve(bookmarkResolutionWithSecurityScope)
		if url == 0 {
			url, stale, nsErr = resolve(0)
		}
	} else {
		url, stale, nsErr = resolve(0)
	}
	if url == 0 {
		if msg := GoString(nsErr.Send(sel("localizedDescription"))); msg != "" {
			return driver.Directory{}, errors.New(msg)
		}
		return driver.Directory{}, errors.New("apple: directory bookmark could not be resolved")
	}
	scoped := objc.Send[bool](url, sel("startAccessingSecurityScopedResource"))
	path := GoString(url.Send(sel("path")))
	name := GoString(url.Send(sel("lastPathComponent")))
	info, statErr := os.Stat(path)
	if statErr != nil || !info.IsDir() {
		if scoped {
			url.Send(sel("stopAccessingSecurityScopedResource"))
		}
		if statErr != nil {
			return driver.Directory{}, statErr
		}
		return driver.Directory{}, errors.New("apple: bookmarked path is not a directory")
	}
	if stale {
		if fresh, ok := CreateDirectoryBookmark(url, macOS); ok {
			token = fresh
		}
	}
	TrackScopedDirectory(token, url, scoped)
	return driver.Directory{Name: name, Path: path, Token: token}, nil
}

// ReleaseDirectoryBookmark stops accessing the security-scoped resource for
// token if tracked and releases the retained NSURL.
func ReleaseDirectoryBookmark(token string) {
	if token == "" {
		return
	}
	scopedDirsMu.Lock()
	entry, ok := scopedDirs[token]
	if ok {
		delete(scopedDirs, token)
	}
	scopedDirsMu.Unlock()
	if ok && entry.url != 0 {
		if entry.scoped {
			entry.url.Send(sel("stopAccessingSecurityScopedResource"))
		}
		entry.url.Send(sel("release"))
	}
}

// HandleOpenDirectory resolves a persisted bookmark token and dispatches the
// EventFiles reply to dispatch.
func HandleOpenDirectory(token string, id uint32, macOS bool, dispatch func(driver.Event)) {
	dir, err := OpenDirectoryBookmark(token, macOS)
	ev := driver.Event{Kind: driver.EventFiles, Count: id}
	if err != nil {
		ev.Text = err.Error()
	} else {
		ev.Data = dir
	}
	dispatch(ev)
}

// HandleReleaseDirectory releases a tracked security-scoped bookmark and
// dispatches the EventFiles acknowledgment to dispatch.
func HandleReleaseDirectory(token string, id uint32, dispatch func(driver.Event)) {
	ReleaseDirectoryBookmark(token)
	dispatch(driver.Event{Kind: driver.EventFiles, Count: id})
}
