//go:build darwin && !ios

package macos

import (
	"errors"
	"fmt"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func pasteboardChangeCount(pb objc.ID) int64 {
	return objc.Send[int64](pb, sel("changeCount"))
}

func readClipboardItems(pb objc.ID) ([]driver.ClipboardItem, error) {
	generation := pasteboardChangeCount(pb)
	native := pb.Send(sel("pasteboardItems"))
	var items []driver.ClipboardItem
	remaining := uint(driver.MaxClipboardImage)
	for i := range objc.Send[uint](native, sel("count")) {
		object := native.Send(sel("objectAtIndex:"), i)
		types := object.Send(sel("types"))
		var reps []driver.ClipboardRepresentation
		for j := range objc.Send[uint](types, sel("count")) {
			kind := types.Send(sel("objectAtIndex:"), j)
			data := object.Send(sel("dataForType:"), kind)
			name := apple.GoString(kind)
			if data == 0 {
				reps = append(reps, driver.ClipboardRepresentation{Type: name})
				continue // retain presence-only and deferred format markers
			}
			n := objc.Send[uint](data, sel("length"))
			if n > remaining {
				return nil, driver.ErrClipboardQuota
			}
			remaining -= n
			reps = append(reps, driver.ClipboardRepresentation{Type: name, Data: apple.Bytes(data)})
		}
		if len(reps) > 0 {
			items = append(items, driver.ClipboardItem{Representations: reps})
		}
	}
	if pasteboardChangeCount(pb) != generation {
		return nil, errors.New("the clipboard changed while it was being read")
	}
	return items, nil
}

func clipboardFormats(pb objc.ID) ([]string, error) {
	native := pb.Send(sel("pasteboardItems"))
	var formats []string
	seen := make(map[string]bool)
	for i := range objc.Send[uint](native, sel("count")) {
		object := native.Send(sel("objectAtIndex:"), i)
		types := object.Send(sel("types"))
		for j := range objc.Send[uint](types, sel("count")) {
			name := apple.GoString(types.Send(sel("objectAtIndex:"), j))
			if name != "" && !seen[name] {
				seen[name] = true
				formats = append(formats, name)
			}
		}
	}
	return formats, nil
}

func writeClipboardItems(pb objc.ID, items []driver.ClipboardItem) error {
	list := objc.ID(apple.Class("NSMutableArray")).Send(sel("array"))
	for _, item := range items {
		object := objc.ID(apple.Class("NSPasteboardItem")).Send(selAlloc).Send(sel("init"))
		for _, rep := range item.Representations {
			if !objc.Send[bool](object, sel("setData:forType:"), apple.NSData(rep.Data), apple.NSString(rep.Type)) {
				object.Send(selRelease)
				return fmt.Errorf("the pasteboard refused format %q", rep.Type)
			}
		}
		list.Send(sel("addObject:"), object)
		object.Send(selRelease)
	}
	pb.Send(sel("clearContents"))
	if len(items) > 0 && !objc.Send[bool](pb, sel("writeObjects:"), list) {
		return errors.New("the pasteboard refused the clipboard items")
	}
	return nil
}

func setClipboardText(pb objc.ID, text string) {
	pb.Send(sel("clearContents"))
	pb.Send(sel("setString:forType:"), apple.NSString(text), apple.NSString(pasteboardText))
}
