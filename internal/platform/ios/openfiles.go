//go:build ios

package ios

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

// Imported documents have the same lifetime as document-picker copies:
// app-owned temporary files, independent of a provider's security scope.
// Keep each URL until its coordinated import finishes, off UIKit's thread.
var fileOpens struct {
	sync.Mutex
	pending [][]objc.ID
	running bool
}

// launchFiles belongs to UIKit's thread, as launchLinks does. The core
// queues these files for the first OnOpenFiles handler after startup.
var launchFiles []string

func importContextFiles(contexts objc.ID) {
	var urls []objc.ID
	if contexts != 0 {
		apple.ForEach(contexts, func(context objc.ID) {
			u := context.Send(sel("URL"))
			if u != 0 && objc.Send[bool](u, sel("isFileURL")) {
				urls = append(urls, u.Send(sel("retain")))
			}
		})
	}
	if len(urls) == 0 {
		return
	}
	fileOpens.Lock()
	fileOpens.pending = append(fileOpens.pending, urls)
	if !fileOpens.running {
		fileOpens.running = true
		go importOpenedFiles()
	}
	fileOpens.Unlock()
}

func importOpenedFiles() {
	for {
		fileOpens.Lock()
		if len(fileOpens.pending) == 0 {
			fileOpens.running = false
			fileOpens.Unlock()
			return
		}
		urls := fileOpens.pending[0]
		fileOpens.pending[0] = nil
		fileOpens.pending = fileOpens.pending[1:]
		fileOpens.Unlock()
		var paths []string
		for _, u := range urls {
			path, err := importOpenedFile(u)
			u.Send(selRelease)
			if err != nil {
				log.Printf("nsgo: import an opened document: %v", err)
			} else {
				paths = append(paths, path)
			}
		}
		if len(paths) > 0 {
			apple.OnMain(func() {
				if runner == nil || runner.engine == nil {
					launchFiles = append(launchFiles, paths...)
				} else {
					runner.engine.Dispatch(driver.Event{Kind: driver.EventOpenFiles, Data: paths})
				}
			})
		}
	}
}

func (s *surface) sendLaunchFiles() {
	if len(launchFiles) > 0 {
		s.engine.Dispatch(driver.Event{Kind: driver.EventOpenFiles, Data: launchFiles})
		launchFiles = nil
	}
}

func importOpenedFile(url objc.ID) (path string, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
	defer pool.Send(sel("drain"))
	if scoped := objc.Send[bool](url, sel("startAccessingSecurityScopedResource")); scoped {
		defer url.Send(sel("stopAccessingSecurityScopedResource"))
	}
	name := apple.GoString(url.Send(sel("lastPathComponent")))
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return "", errors.New("the opened document has no file name")
	}
	coordinator := objc.ID(apple.Class("NSFileCoordinator")).Send(selAlloc).
		Send(sel("initWithFilePresenter:"), objc.ID(0))
	if coordinator == 0 {
		return "", errors.New("document coordination is unavailable")
	}
	defer coordinator.Send(selRelease)
	accessor := objcblock.New(func(_ objc.Block, resolved objc.ID) {
		path, err = copyOpenedFile(apple.GoString(resolved.Send(sel("path"))), name)
	})
	defer accessor.Release()
	var nativeError objc.ID
	coordinator.Send(sel("coordinateReadingItemAtURL:options:error:byAccessor:"),
		url, uintptr(0), unsafe.Pointer(&nativeError), accessor)
	if nativeError != 0 {
		if path != "" {
			os.RemoveAll(filepath.Dir(path))
		}
		return "", fmt.Errorf("coordinate the document: %s", apple.GoString(nativeError.Send(sel("localizedDescription"))))
	}
	if path == "" && err == nil {
		err = errors.New("the document coordinator provided no file")
	}
	return path, err
}

func copyOpenedFile(source, name string) (path string, err error) {
	file, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("the opened document is not a regular file")
	}
	dir, err := os.MkdirTemp("", "nsgo-open-file-")
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(dir)
		}
	}()
	path = filepath.Join(dir, name)
	copy, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(copy, file)
	if closeErr := copy.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	return path, nil
}
