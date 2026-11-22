package ios

import (
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/field"
)

// This file builds on every system, so tests check iOS paste clipping and routing rules on any host.

// shouldPasteSelf reports whether an editor's paste: must insert through insertText: itself instead of calling UIKit's super paste:. On iOS 26 UIKit's paste: registers its undo step before asking the delegate, so refusing or replacing the paste in shouldChange leaves the undo manager corrupt and crashes on the next undo.
func shouldPasteSelf(c driver.InputConfig) bool {
	return c.MaxLength > 0 || !c.Multiline || c.Submit() != driver.SubmitBehaviorNewline
}

// pasteRoom returns the clamped UTF-16 length of sel within current, the remaining UTF-16 room under maxLength, and whether maxLength limits the input.
func pasteRoom(current string, sel driver.TextRange, maxLength int) (replaced, room int, limited bool) {
	curLen := driver.UTF16Len(current)
	replaced = min(max(sel.End-sel.Start, 0), curLen)
	if maxLength <= 0 {
		return replaced, 0, false
	}
	return replaced, max(maxLength-(curLen-replaced), 0), true
}

// preparePasteText normalizes line breaks for single-line fields and clips text to the UTF-16 room left by maxLength when replacing sel in current. clip trims to at most maxUnits UTF-16 units (on iOS, via NSString's composed character sequences); nil falls back to driver.ClipUTF16. ok is false when nothing should be inserted.
func preparePasteText(text string, multiline bool, current string, sel driver.TextRange, maxLength int, clip func(s string, maxUnits int) string) (out string, replaced int, ok bool) {
	if text == "" {
		return "", 0, false
	}
	if !multiline {
		text = field.SingleLine(text)
	}
	var room int
	var limited bool
	replaced, room, limited = pasteRoom(current, sel, maxLength)
	if limited {
		if room <= 0 {
			return "", replaced, false
		}
		if driver.UTF16Len(text) > room {
			if clip != nil {
				text = clip(text, room)
			} else {
				text = driver.ClipUTF16(text, room)
			}
		}
	}
	if text == "" {
		return "", replaced, false
	}
	return text, replaced, true
}
