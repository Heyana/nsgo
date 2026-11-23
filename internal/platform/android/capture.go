package android

import (
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// filterCaptures keeps ItemCapture away from a runtime that does not set capCapture, answering each capture unavailable instead: mount items carry no length, so a runtime built before SurfaceCapture.java would misread the rest of the batch. It copies b rather than change it, since a test's observer may hold the batch.
func filterCaptures(capable bool, b *driver.MountBatch, dispatch func(driver.Event)) *driver.MountBatch {
	capture := func(it driver.MountItem) bool { return it.Kind == driver.ItemCapture }
	if capable || !slices.ContainsFunc(b.Items, capture) {
		return b
	}
	for _, it := range b.Items {
		if capture(it) {
			dispatch(driver.Event{Kind: driver.EventFiles, Count: it.Count, Text: driver.NoCapture})
		}
	}
	out := *b
	out.Items = slices.DeleteFunc(slices.Clone(b.Items), capture)
	return &out
}
