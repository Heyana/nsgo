package android

import "github.com/nsgo-dev/nsgo/internal/driver"

// writeBatchFooter ends a batch with what follows its items, which earlier runtimes stop reading where their footer ends: the geometry version, the texts' copies, then the restyled views' backdrop filters.
func writeBatchFooter(w *writer, b *driver.MountBatch) {
	writeTextCopyFooter(w, b, hasViewStyles(b))
	writeBackdropFooter(w, b)
}

// hasViewStyles reports whether the batch restyles any view, so its footer carries their backdrop filters.
func hasViewStyles(b *driver.MountBatch) bool {
	for i := range b.Items {
		if b.Items[i].Kind == driver.ItemViewStyle {
			return true
		}
	}
	return false
}

// writeBackdropFooter follows the text copies with the backdrop filters of every view the batch restyles, none included, as NsgoView reads them after the items: a count, then each view's ID and writeBackdrop's filters. It is the footer's last section, which runtimes from before backdrop filters stop short of, as ItemViewStyle itself is not framed and cannot grow.
func writeBackdropFooter(w *writer, b *driver.MountBatch) {
	if !hasViewStyles(b) {
		return
	}
	count := 0
	for i := range b.Items {
		if b.Items[i].Kind == driver.ItemViewStyle {
			count++
		}
	}
	w.u32(uint32(count))
	for i := range b.Items {
		it := &b.Items[i]
		if it.Kind != driver.ItemViewStyle {
			continue
		}
		var filters []driver.Filter
		if it.ViewStyle != nil && it.ViewStyle.Effects != nil {
			filters = it.ViewStyle.Effects.BackdropFilters
		}
		w.u32(uint32(it.ID))
		writeBackdrop(w, filters)
	}
}

// writeBackdrop writes a view's backdrop filters as BackdropMath.read reads them: whether there is a color matrix and its 20 floats, rows with offsets from 0 to 1, then the widest blur in points. Drop shadows are left out, as CSS's backdrop-filter draws none.
func writeBackdrop(w *writer, filters []driver.Filter) {
	m, color, blur, _ := driver.SplitFilters(filters)
	w.flags(color)
	if color {
		w.floats(m[:]...)
	}
	w.points(blur)
}
