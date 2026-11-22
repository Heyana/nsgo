//go:build linux && !android

package linux

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// GtkRecentData mirrors GTK's public metadata structure. AppName is the stable app ID rather than a window title, so another app never shares its history.
type gtkRecentData struct {
	DisplayName, Description   *byte
	MimeType, AppName, AppExec *byte
	Groups                     uintptr
	Private                    int32
}

var (
	gtkRecentManagerGetDefault     func() uintptr
	gtkRecentManagerAddFull        func(manager uintptr, uri *byte, data *gtkRecentData) int32
	gContentTypeGuess              func(filename *byte, data uintptr, size uintptr, uncertain *int32) uintptr
	gContentTypeGetMimeType        func(contentType uintptr) uintptr
	gBookmarkFileNew               func() uintptr
	gBookmarkFileFree              func(bookmark uintptr)
	gBookmarkFileLoadFromFile      func(bookmark uintptr, filename *byte, err *uintptr) int32
	gBookmarkFileGetURIs           func(bookmark uintptr, n *uintptr) uintptr
	gBookmarkFileHasApplication    func(bookmark uintptr, uri, app *byte, err *uintptr) int32
	gBookmarkFileRemoveApplication func(bookmark uintptr, uri, app *byte, err *uintptr) int32
	gBookmarkFileGetApplications   func(bookmark uintptr, uri *byte, n *uintptr, err *uintptr) uintptr
	gBookmarkFileRemoveItem        func(bookmark uintptr, uri *byte, err *uintptr) int32
	gBookmarkFileToData            func(bookmark uintptr, length *uintptr, err *uintptr) uintptr
	bindRecentFiles                = sync.OnceFunc(func() {
		purego.RegisterLibFunc(&gtkRecentManagerGetDefault, gtkLib, "gtk_recent_manager_get_default")
		purego.RegisterLibFunc(&gtkRecentManagerAddFull, gtkLib, "gtk_recent_manager_add_full")
		purego.RegisterLibFunc(&gContentTypeGuess, gioLib, "g_content_type_guess")
		purego.RegisterLibFunc(&gContentTypeGetMimeType, gioLib, "g_content_type_get_mime_type")
		purego.RegisterLibFunc(&gBookmarkFileNew, glibLib, "g_bookmark_file_new")
		purego.RegisterLibFunc(&gBookmarkFileFree, glibLib, "g_bookmark_file_free")
		purego.RegisterLibFunc(&gBookmarkFileLoadFromFile, glibLib, "g_bookmark_file_load_from_file")
		purego.RegisterLibFunc(&gBookmarkFileGetURIs, glibLib, "g_bookmark_file_get_uris")
		purego.RegisterLibFunc(&gBookmarkFileHasApplication, glibLib, "g_bookmark_file_has_application")
		purego.RegisterLibFunc(&gBookmarkFileRemoveApplication, glibLib, "g_bookmark_file_remove_application")
		purego.RegisterLibFunc(&gBookmarkFileGetApplications, glibLib, "g_bookmark_file_get_applications")
		purego.RegisterLibFunc(&gBookmarkFileRemoveItem, glibLib, "g_bookmark_file_remove_item")
		purego.RegisterLibFunc(&gBookmarkFileToData, glibLib, "g_bookmark_file_to_data")
	})
)

// recentFile runs in mount order on GTK's thread, including a clear between two adds. Emitting changed flushes GTK's pending write before accessing its XBEL, and reloads it after a selective clear; no private GtkRecentManager ABI is used.
func (w *window) recentFile(path string, clear bool) {
	if w.opts.ID == "" {
		return
	}
	bindRecentFiles()
	manager := gtkRecentManagerGetDefault()
	var err error
	// A file monitor may still be coalescing another application's update.
	gSignalEmitByName(manager, cstr("changed"))
	if clear {
		err = clearRecentApplications(settingString(manager, "filename"), w.opts.ID)
		gSignalEmitByName(manager, cstr("changed"))
	} else {
		var executable string
		executable, err = loginExecutable()
		if err == nil {
			err = addRecentFile(manager, path, w.opts.ID, executable)
			// Persist even when the app quits before GTK's 250 ms timer fires.
			gSignalEmitByName(manager, cstr("changed"))
		}
	}
	if err != nil {
		log.Printf("nsgo: recent files: %v", err)
	}
}

func addRecentFile(manager uintptr, path, appID, executable string) error {
	if !filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
		return fmt.Errorf("document path must be absolute")
	}
	uri := linuxFileURI(path)
	contentType := gContentTypeGuess(cstr(path), 0, 0, nil)
	defer gFree(contentType)
	mime := gContentTypeGetMimeType(contentType)
	defer gFree(mime)
	// XBEL's exec uses shell quoting, then expands %u to the URI. Escape executable percent signs too; %f would insert an unquoted path with spaces.
	exec := desktopExecArg(executable) + " %u"
	data := gtkRecentData{MimeType: cstr(goString(mime)), AppName: cstr(appID), AppExec: cstr(exec)}
	if gtkRecentManagerAddFull(manager, cstr(uri), &data) == 0 {
		return fmt.Errorf("GTK could not record document")
	}
	return nil
}

// clearRecentApplications removes only appID's registration. GTK's public remove and purge calls remove other apps' registrations as well, so use GLib's public bookmark API on the same file. Unshared documents disappear altogether.
func clearRecentApplications(filename, appID string) error {
	bookmark := gBookmarkFileNew()
	defer gBookmarkFileFree(bookmark)
	var nativeError uintptr
	if gBookmarkFileLoadFromFile(bookmark, cstr(filename), &nativeError) == 0 {
		if nativeError != 0 {
			gErrorFree(nativeError)
		}
		if _, err := os.Stat(filename); os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("could not read recent documents")
	}
	var count uintptr
	uris := gBookmarkFileGetURIs(bookmark, &count)
	defer gStrfreev(uris)
	changed := false
	for _, uri := range unsafe.Slice((*uintptr)(cPointer(uris)), count) {
		name := cstr(goString(uri))
		if gBookmarkFileHasApplication(bookmark, name, cstr(appID), nil) == 0 {
			continue
		}
		gBookmarkFileRemoveApplication(bookmark, name, cstr(appID), nil)
		var applications uintptr
		apps := gBookmarkFileGetApplications(bookmark, name, &applications, nil)
		gStrfreev(apps)
		if applications == 0 {
			gBookmarkFileRemoveItem(bookmark, name, nil)
		}
		changed = true
	}
	if !changed {
		return nil
	}
	var length uintptr
	data := gBookmarkFileToData(bookmark, &length, &nativeError)
	if nativeError != 0 {
		gErrorFree(nativeError)
		return fmt.Errorf("could not encode recent documents")
	}
	defer gFree(data)
	tmp, err := os.CreateTemp(filepath.Dir(filename), ".nsgo-recent-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(unsafe.Slice((*byte)(cPointer(data)), length))
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filename)
}
