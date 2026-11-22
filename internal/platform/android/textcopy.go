package android

import "github.com/nsgo-dev/nsgo/internal/driver"

// writeTextCopyFooter follows the optional geometry version with selection metadata for each mounted text. Earlier runtimes ignore the footer; copies follow the latest spans even when their text layout was reused. With more, a later section of the footer follows, so both parts are written even when empty.
func writeTextCopyFooter(w *writer, b *driver.MountBatch, more bool) {
	count := 0
	for _, item := range b.Items {
		if item.Kind == driver.ItemText {
			count++
		}
	}
	if count == 0 && b.GeometryVersion == 0 && !more {
		return
	}
	w.u64(b.GeometryVersion)
	if count == 0 && !more {
		return
	}
	w.u32(uint32(count))
	for _, item := range b.Items {
		if item.Kind != driver.ItemText {
			continue
		}
		w.u32(uint32(item.ID))
		writeTextCopies(w, item.Text)
	}
}

func writeTextCopies(w *writer, text *driver.TextContent) {
	count := 0
	for _, span := range text.Spans {
		if span.Hidden || span.Copy != "" {
			count++
		}
	}
	w.u32(uint32(count))
	for _, span := range text.Spans {
		if !span.Hidden && span.Copy == "" {
			continue
		}
		w.u32(uint32(driver.UTF16Units(text.Text, span.Start)))
		w.u32(uint32(driver.UTF16Units(text.Text, span.End)))
		w.flags(span.Hidden)
		w.str(span.Copy)
	}
}
