package core

import (
	"errors"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// ReadClipboardItems calls done with a snapshot of the clipboard's native
// format names and bytes. The combined bytes are limited to 48 MiB.
// A platform without representation access reports CodeUnavailable.
func (cx *Scope) ReadClipboardItems(done func([]ClipboardItem, error)) {
	t, err := cx.clipboardItemsTree()
	if err != nil {
		done(nil, err)
		return
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemReadClipboard, Clipboard: driver.ClipboardReadItems}, func(ev driver.Event) {
		result, _ := ev.Data.(driver.ClipboardResult)
		done(result.Items, clipboardItemsError(result.Err))
	})
}

// WriteClipboardItems replaces the clipboard with items, or clears it when
// items is empty. An optional done callback receives the result when the
// platform finishes. The bytes are copied before the call returns.
func (cx *Scope) WriteClipboardItems(items []ClipboardItem, done ...func(error)) {
	var complete func(error)
	if len(done) > 0 {
		complete = done[0]
	}
	t, err := cx.clipboardItemsTree()
	if err == nil {
		err = clipboardItemsError(driver.ValidateClipboardItems(items))
	}
	if err != nil {
		if complete != nil {
			complete(err)
		}
		return
	}
	item := driver.MountItem{Kind: driver.ItemSetClipboardItems, Data: driver.CloneClipboardItems(items)}
	if complete != nil {
		t.request(cx, item, func(ev driver.Event) {
			result, _ := ev.Data.(driver.ClipboardResult)
			complete(clipboardItemsError(result.Err))
		})
		return
	}
	t.commands = append(t.commands, item)
	t.changed = true
}

// ClipboardChangeCount calls done with the platform's clipboard sequence
// number, or -1 if unavailable. Compare counts for equality because a native
// sequence can wrap.
func (cx *Scope) ClipboardChangeCount(done func(int64)) {
	t, err := cx.clipboardItemsTree()
	if err != nil {
		done(-1)
		return
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemReadClipboard, Clipboard: driver.ClipboardReadChangeCount}, func(ev driver.Event) {
		result, ok := ev.Data.(driver.ClipboardResult)
		if !ok || result.Err != nil {
			done(-1)
			return
		}
		done(result.ChangeCount)
	})
}

// ClipboardFormats calls done with native format names currently offered by the clipboard, without copying their payloads. It answers an empty slice when the clipboard is empty or the platform cannot name its formats.
func (cx *Scope) ClipboardFormats(done func([]string)) {
	t, err := cx.clipboardItemsTree()
	if err != nil {
		done(nil)
		return
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemReadClipboard, Clipboard: driver.ClipboardReadFormats}, func(ev driver.Event) {
		result, ok := ev.Data.(driver.ClipboardResult)
		if !ok || result.Err != nil {
			done(nil)
			return
		}
		done(result.Formats)
	})
}

func (cx *Scope) clipboardItemsTree() (*Tree, error) {
	if err := cx.gate(serviceClipboard, ""); err != nil {
		return nil, err
	}
	if t := cx.tree(); t != nil && cx.Env().clipboardItems.Get() {
		return t, nil
	}
	return nil, Unavailable("clipboard representations are unavailable on this platform")
}

func clipboardItemsError(err error) error {
	if errors.Is(err, driver.ErrClipboardQuota) {
		return &Error{Code: CodeQuotaExceeded, Message: err.Error(), Err: err}
	}
	return err
}
