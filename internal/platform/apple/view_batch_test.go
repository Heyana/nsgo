//go:build darwin

package apple

import "testing"

func TestImageBatchSkipsReplacedViewsAndReleasesItsQueue(t *testing.T) {
	first, replacement, stale := new(int), new(int), new(int)
	views := []*int{first, replacement, stale}
	storage := views[:]
	var started []*int
	views = StartImages(views, 2.5, func(v *int) bool { return v != stale }, func(v *int, scale float64) {
		if scale != 2.5 {
			t.Error("image received the wrong backing scale", scale)
		}
		started = append(started, v)
	})
	if len(started) != 2 || started[0] != first || started[1] != replacement || len(views) != 0 || cap(views) != cap(storage) {
		t.Fatal("the image queue lost its order, selected a stale view or discarded reusable storage")
	}
	for _, v := range storage {
		if v != nil {
			t.Fatal("the image queue retained a view after the batch")
		}
	}
}
