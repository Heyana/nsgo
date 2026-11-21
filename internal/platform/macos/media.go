//go:build darwin && !ios

package macos

import (
	"log"
	"net/url"
	"strings"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// configureNative makes a native view's component on its first item and
// passes it the props that changed.
func (s *surface) configureNative(v *view, c *driver.NativeConfig) {
	if c.Bind {
		s.animator.BindNative(v.id, c.Bindings)
	}
	if v.component == nil && v.componentView == 0 {
		factory := s.components[c.Component]
		if factory == nil && c.Component == driver.SwitchComponent {
			factory = newSwitch
		}
		if factory == nil {
			if s.unknown == nil {
				s.unknown = map[string]bool{}
			}
			if !s.unknown[c.Component] {
				s.unknown[c.Component] = true
				log.Printf("nsgo: no native component %q is registered", c.Component)
			}
			v.componentView = ^objc.ID(0) // tried
			return
		}
		e := s.engine
		id := v.id
		v.component = factory(driver.NewNativeContext(
			func(name string, data any) {
				e.Dispatch(driver.Event{Kind: driver.EventNative, ID: id, Text: name, Data: data})
			},
			func(size driver.Size) { e.Dispatch(driver.Event{Kind: driver.EventNativeSize, ID: id, Size: size}) },
		))
		if v.component == nil {
			return
		}
		if native := nativeView(v.component.View()); native != 0 {
			const widthSizable, heightSizable = 2, 16
			native.Send(sel("setAutoresizingMask:"), uint(widthSizable|heightSizable))
			native.Send(selSetFrame, objc.Send[apple.CGRect](v.native, selBounds))
			v.native.Send(selAddSubview, native)
			v.componentView = native
			v.surface.directTree(native)
		}
	}
	if v.component == nil {
		return
	}
	for name, value := range c.Props {
		v.component.SetProp(name, value)
	}
	if _, ok := v.component.(*nsSwitch); ok {
		v.updateAccessibility()
	}
}

// nativeView returns the NSView a component's View gives.
func nativeView(view any) objc.ID {
	switch h := view.(type) {
	case objc.ID:
		return h
	case uintptr:
		return objc.ID(h)
	}
	return 0
}

// File drops: the surface takes file URLs dragged from other apps and
// hands them to the innermost view under the pointer that listens for
// them, deciding at once from the mounted views.

var pasteboardFileURL = "public.file-url"

func (s *surface) acceptFiles() {
	types := objc.ID(apple.Class("NSArray")).Send(sel("arrayWithObject:"), apple.NSString(pasteboardFileURL))
	s.native.Send(sel("registerForDraggedTypes:"), types)
}

// dropTarget returns the view a drag at info's location would drop on.
func (s *surface) dropTarget(info objc.ID) *view {
	loc := objc.Send[apple.CGPoint](info, sel("draggingLocation"))
	up := s.native.Send(selSuperview)
	if up == 0 {
		return nil
	}
	p := objc.Send[apple.CGPoint](up, selConvertPoint, loc, objc.ID(0))
	rawHit = true
	hit := s.native.Send(sel("hitTest:"), p)
	rawHit = false
	var modal objc.ID
	if n := len(s.modals); n > 0 {
		modal = s.modals[n-1].native
	}
	for n := hit; n != 0 && n != up; n = n.Send(selSuperview) {
		v := views[n]
		if v == nil || v.native != n || s.views == nil || int(v.id) >= len(s.views) || s.views[v.id] != v {
			continue
		}
		if v.events.Has(driver.EventFileDrop) {
			if v.disabled {
				return nil
			}
			if modal != 0 && !objc.Send[bool](v.native, sel("isDescendantOf:"), modal) {
				return nil
			}
			return v
		}
		if v.kind == driver.KindModal {
			return nil // a modal bounds the search
		}
	}
	return nil
}

// enter makes v the view files are dragged over, telling the views that
// listen.
func (s *surface) enter(v *view) {
	if s.dragTarget == v {
		return
	}
	if old := s.dragTarget; old != nil {
		old.dispatch(driver.EventFileDragLeave)
	}
	s.dragTarget = v
	if v != nil {
		v.dispatch(driver.EventFileDragEnter)
	}
}

const dragCopy = 1

func dragAllowsCopy(info objc.ID) bool {
	return info != 0 && objc.Send[uint](info, sel("draggingSourceOperationMask"))&dragCopy != 0
}

// draggingEntered reads whether the drag carries files once, so moves
// only hit test.
func draggingEntered(self objc.ID, cmd objc.SEL, info objc.ID) uint {
	sv := views[self]
	if sv == nil || sv.surface == nil {
		return 0
	}
	s := sv.surface
	s.dragFiles = len(droppedFiles(info)) > 0
	return draggingUpdated(self, cmd, info)
}

func draggingUpdated(self objc.ID, _ objc.SEL, info objc.ID) uint {
	sv := views[self]
	if sv == nil || sv.surface == nil {
		return 0
	}
	s := sv.surface
	defer s.measuring(false)()
	if !s.dragFiles || !dragAllowsCopy(info) {
		s.enter(nil)
		return 0
	}
	v := s.dropTarget(info)
	s.enter(v)
	if v == nil {
		return 0
	}
	return dragCopy
}

func draggingExited(self objc.ID, _ objc.SEL, _ objc.ID) {
	sv := views[self]
	if sv == nil || sv.surface == nil {
		return
	}
	s := sv.surface
	defer s.measuring(false)()
	s.dragFiles = false
	s.enter(nil)
}

func prepareForDrag(self objc.ID, _ objc.SEL, info objc.ID) bool {
	sv := views[self]
	if sv == nil || sv.surface == nil {
		return false
	}
	s := sv.surface
	defer s.measuring(false)()
	return s.dragFiles && dragAllowsCopy(info) && s.dropTarget(info) != nil
}

func performDrag(self objc.ID, _ objc.SEL, info objc.ID) bool {
	sv := views[self]
	if sv == nil || sv.surface == nil {
		return false
	}
	s := sv.surface
	defer s.measuring(false)()
	s.dragFiles = false
	if !dragAllowsCopy(info) {
		s.enter(nil)
		return false
	}
	v := s.dropTarget(info)
	uris := droppedFiles(info)
	if v == nil || len(uris) == 0 {
		s.enter(nil)
		return false
	}
	s.enter(v)
	v.report(driver.Event{Kind: driver.EventFileDrop, URIs: uris})
	s.enter(nil)
	return true
}

// droppedFiles returns the file: URLs of the local files a drag carries.
func droppedFiles(info objc.ID) []string {
	if info == 0 {
		return nil
	}
	return fileURLs(info.Send(sel("draggingPasteboard")))
}

// fileURLs returns the file: URLs of the local files pasteboard pb holds.
func fileURLs(pb objc.ID) []string {
	if pb == 0 {
		return nil
	}
	classes := objc.ID(apple.Class("NSArray")).Send(sel("arrayWithObject:"), objc.ID(apple.Class("NSURL")))
	key := apple.NSString("NSPasteboardURLReadingFileURLsOnlyKey")
	yes := objc.ID(apple.Class("NSNumber")).Send(sel("numberWithBool:"), true)
	opts := objc.ID(apple.Class("NSDictionary")).Send(sel("dictionaryWithObject:forKey:"), yes, key)
	list := pb.Send(sel("readObjectsForClasses:options:"), classes, opts)
	n := objc.Send[uint](list, sel("count"))
	var uris []string
	for i := range n {
		u := list.Send(sel("objectAtIndex:"), i)
		path := apple.GoString(u.Send(sel("path")))
		if !strings.HasPrefix(path, "/") {
			continue
		}
		uris = append(uris, (&url.URL{Scheme: "file", Path: path}).String())
	}
	return uris
}
