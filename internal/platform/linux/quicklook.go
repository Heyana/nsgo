//go:build linux && !android

package linux

import (
	"errors"
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Sushi owns the preview window. Calls run off GTK's thread, with only the latest request pending while the service starts or loads a file.
type filePreview struct {
	owner    *window
	parent   string
	paths    []string
	index    int
	protocol previewProtocol
}

var previews struct {
	sync.Mutex
	active              *filePreview
	closingParent       string
	closeDone           chan struct{}
	revision, attempted uint64
	parent              string
	parentKnown         bool
	parentRevision      uint64
	visible             bool
	visibility          uint64
}

var (
	previewBool   func(uintptr) int32
	previewUint16 func(uintptr) uint16
	bindPreviews  = sync.OnceFunc(func() {
		bindNotifications()
		purego.RegisterLibFunc(&previewBool, glibLib, "g_variant_get_boolean")
		purego.RegisterLibFunc(&previewUint16, glibLib, "g_variant_get_uint16")
	})
	previewSignals = sync.OnceFunc(func() {
		bindPreviews()
		if bus := sessionBus(); bus != 0 {
			gDBusConnectionSignalSubscribe(bus, cstr("org.freedesktop.DBus"), cstr("org.freedesktop.DBus"), cstr("NameOwnerChanged"),
				cstr("/org/freedesktop/DBus"), cstr(previewName), 0, cbPreviewOwner, 0, 0)
			gDBusConnectionSignalSubscribe(bus, cstr(previewName), cstr(previewV2), cstr("SelectionEvent"),
				cstr(previewPath), nil, 0, cbPreviewSelection, 0, 0)
			gDBusConnectionSignalSubscribe(bus, cstr(previewName), cstr("org.freedesktop.DBus.Properties"), cstr("PropertiesChanged"),
				cstr(previewPath), nil, 0, cbPreviewProperties, 0, 0)
		}
	})
	previewAvailable = sync.OnceValue(func() bool {
		bindPreviews()
		bus := sessionBus()
		if bus == 0 {
			return false
		}
		// Listing names never starts the previewer just to read Env.
		for _, method := range []string{"ListNames", "ListActivatableNames"} {
			reply := dbusCall(bus, "org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", method)
			if reply == 0 {
				continue
			}
			names := gVariantGetChildValue(reply, 0)
			found := false
			for i := uintptr(0); i < gVariantNChildren(names); i++ {
				name := gVariantGetChildValue(names, i)
				found = found || goString(gVariantGetString(name, 0)) == previewName
				gVariantUnref(name)
			}
			gVariantUnref(names)
			gVariantUnref(reply)
			if found {
				return true
			}
		}
		return false
	})
	previewWork = sync.OnceValue(func() chan struct{} {
		ch := make(chan struct{}, 1)
		go func() {
			for range ch {
				applyPreview()
			}
		}()
		return ch
	})
)

func wakePreview() {
	select {
	case previewWork() <- struct{}{}:
	default:
	}
}

func (w *window) canQuickLook() bool {
	return w.ownWindow() && previewAvailable()
}

func (w *window) quickLook(r *driver.QuickLookRequest) {
	if r == nil || len(r.Paths) == 0 || !w.canQuickLook() {
		return
	}
	previewSignals()
	p := &filePreview{owner: w, parent: w.portalParentWindow(), paths: slices.Clone(r.Paths),
		index: min(max(r.Index, 0), len(r.Paths)-1)}
	previews.Lock()
	previews.active = p
	finishPreviewClose()
	previews.revision++
	previews.Unlock()
	wakePreview()
}

// finishPreviewClose releases the pending close. The caller holds previews.
func finishPreviewClose() {
	previews.closingParent = ""
	if previews.closeDone != nil {
		close(previews.closeDone)
		previews.closeDone = nil
	}
}

func (w *window) closeQuickLook() {
	previews.Lock()
	p := previews.active
	if p == nil || p.owner != w {
		previews.Unlock()
		return
	}
	var done chan struct{}
	finishPreviewClose()
	// An old protocol, or an unparented window, supplies no way to tell our preview from one another app has since opened.
	if p.protocol.selection && p.parent != "" {
		previews.closingParent = p.parent
		done = make(chan struct{})
		previews.closeDone = done
	}
	previews.active = nil
	previews.revision++
	previews.Unlock()
	wakePreview()
	if done != nil {
		// Flush the external panel's close before the last GTK loop and process end; the IPC worker needs no callbacks from this thread.
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			// Keep the queued close: a slow ShowFile may still return after this window is gone while other app windows remain.
		}
	}
}

func applyPreview() {
	previews.Lock()
	p, revision := previews.active, previews.revision
	closing := previews.closingParent
	owned := closing != "" && previews.parentKnown && previews.parent == closing && previews.visible
	if p == nil {
		done := previews.closeDone
		if !owned && previews.parentKnown && (previews.parent != closing || !previews.visible) {
			finishPreviewClose()
		}
		previews.Unlock()
		if owned {
			gVariantUnrefNonZero(dbusCall(sessionBus(), previewName, previewPath, previewV2, "Close"))
			previews.Lock()
			if previews.closeDone == done {
				finishPreviewClose()
			}
			previews.Unlock()
		}
		return
	}
	if revision == previews.attempted {
		previews.Unlock()
		return
	}
	path, parent := p.paths[p.index], p.parent
	previews.attempted = revision
	previews.Unlock()

	bus := sessionBus()
	reply, err := previewCall(bus, "org.freedesktop.DBus.Introspectable", "Introspect")
	if err != nil {
		log.Printf("nsgo: file preview: %v", err)
		return
	}
	protocol, ok := previewProtocolOf(variantString(reply, 0))
	gVariantUnref(reply)
	if !ok {
		log.Print("nsgo: the file previewer has no supported ShowFile method")
		return
	}
	previews.Lock()
	if previews.active != p || previews.revision != revision {
		previews.Unlock()
		return
	}
	p.protocol = protocol
	visibility, parentRevision := previews.visibility, previews.parentRevision
	previews.Unlock()
	str := func(s string) uintptr { return gVariantNewString(cstr(s)) }
	args := []uintptr{str(previewFileURI(path))}
	if protocol.iface == previewName {
		xid, _ := strconv.ParseUint(strings.TrimPrefix(parent, "x11:"), 16, 32)
		args = append(args, gVariantNewUint32(uint32(xid)))
	} else {
		args = append(args, str(parent))
	}
	args = append(args, gVariantNewBoolean(0)) // Refresh an already shown file.
	if protocol.signature == "ssbs" {
		args = append(args, str(""))
	}
	reply, err = previewCall(bus, protocol.iface, "ShowFile", args...)
	gVariantUnrefNonZero(reply)
	if err != nil {
		log.Printf("nsgo: file preview: %v", err)
		return
	}
	previews.Lock()
	// ShowFile may reuse a window that was already visible when this client subscribed. Respect any explicit visibility signal in flight.
	if previews.visibility == visibility {
		previews.visible = true
	}
	if previews.parentRevision == parentRevision {
		previews.parent, previews.parentKnown = parent, true
	}
	previews.Unlock()
}

func previewSelectionCallback(conn, sender, path, iface, signal, params, data uintptr) {
	if gVariantNChildren(params) != 1 {
		return
	}
	v := gVariantGetChildValue(params, 0)
	var direction uint32
	switch goString(gVariantGetTypeString(v)) {
	case "q":
		direction = uint32(previewUint16(v))
	case "u":
		direction = gVariantGetUint32(v)
	default:
		gVariantUnref(v)
		return
	}
	gVariantUnref(v)
	previews.Lock()
	p := previews.active
	if p != nil && p.protocol.selection && p.parent != "" && previews.parentKnown && previews.parent == p.parent && previews.visible {
		if next, changed := previewSelection(p.index, len(p.paths), direction); changed {
			p.index = next
			previews.revision++
		}
	}
	previews.Unlock()
	wakePreview()
}

// Use the service's signals: Sushi 50 advertises readable properties but answers Get with an error. Its PropertiesChanged supplies both values.
func previewPropertiesCallback(conn, sender, path, iface, signal, params, data uintptr) {
	if goString(gVariantGetTypeString(params)) != "(sa{sv}as)" || variantString(params, 0) != previewV2 {
		return
	}
	changed := gVariantGetChildValue(params, 1)
	defer gVariantUnref(changed)
	previews.Lock()
	for i := uintptr(0); i < gVariantNChildren(changed); i++ {
		entry := gVariantGetChildValue(changed, i)
		name := variantString(entry, 0)
		box := gVariantGetChildValue(entry, 1)
		value := gVariantGetVariant(box)
		switch {
		case name == "ParentHandle" && goString(gVariantGetTypeString(value)) == "s":
			previews.parent, previews.parentKnown = goString(gVariantGetString(value, 0)), true
			previews.parentRevision++
		case name == "Visible" && goString(gVariantGetTypeString(value)) == "b":
			previews.visible = previewBool(value) != 0
			previews.visibility++
		}
		gVariantUnref(value)
		gVariantUnref(box)
		gVariantUnref(entry)
	}
	invalidated := gVariantGetChildValue(params, 2)
	for i := uintptr(0); i < gVariantNChildren(invalidated); i++ {
		switch variantString(invalidated, int(i)) {
		case "ParentHandle":
			previews.parentKnown = false
			previews.parentRevision++
		case "Visible":
			previews.visible = false
			previews.visibility++
		}
	}
	gVariantUnref(invalidated)
	previews.Unlock()
	wakePreview()
}

// A replacement service has no known parent or visible panel. Never attribute its keys or Close to a window from the old process.
func previewOwnerCallback(conn, sender, path, iface, signal, params, data uintptr) {
	if goString(gVariantGetTypeString(params)) != "(sss)" || variantString(params, 0) != previewName {
		return
	}
	previews.Lock()
	previews.parent, previews.parentKnown, previews.visible = "", false, false
	previews.parentRevision++
	previews.visibility++
	finishPreviewClose()
	previews.Unlock()
}

// Preview startup and renderer discovery can take longer than the short property queries dbusTry serves. This timeout runs only on the worker.
func previewCall(bus uintptr, iface, method string, args ...uintptr) (uintptr, error) {
	var params uintptr
	if len(args) == 0 {
		params = gVariantNewTuple(nil, 0)
	} else {
		params = gVariantNewTuple(&args[0], uintptr(len(args)))
	}
	var gerr uintptr
	reply := gDBusConnectionCallSync(bus, cstr(previewName), cstr(previewPath), cstr(iface), cstr(method),
		params, 0, 0, 30000, 0, &gerr)
	if gerr != 0 {
		defer gErrorFree(gerr)
		return 0, errors.New(goString((*gError)(cPointer(gerr)).message))
	}
	return reply, nil
}
