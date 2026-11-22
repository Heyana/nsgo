package android

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestReadNativeGeometrySnapshot(t *testing.T) {
	var w writer
	w.u32(7)
	w.u64(42)
	w.u32(1)
	w.u32(3)
	w.bounds(driver.Bounds{Origin: driver.Point{X: 12.5, Y: 30}, Size: driver.Size{Width: 80, Height: 40}})
	for _, p := range [4]driver.Point{{X: 12.5, Y: 30}, {X: 92.5, Y: 30}, {X: 92.5, Y: 70}, {X: 12.5, Y: 70}} {
		w.points(p.X)
		w.points(p.Y)
	}
	w.points(80)
	w.points(40)
	surface, snapshot, ok := readGeometrySnapshot(w.buf)
	if !ok || surface != 7 || snapshot.Version != 42 || snapshot.Bounds[3] != (driver.Bounds{Origin: driver.Point{X: 12.5, Y: 30}, Size: driver.Size{Width: 80, Height: 40}}) {
		t.Fatalf("native geometry = %d, %+v, %v", surface, snapshot, ok)
	}
	for _, data := range [][]byte{nil, w.buf[:15], w.buf[:len(w.buf)-1]} {
		if _, _, ok := readGeometrySnapshot(data); ok {
			t.Fatalf("accepted truncated snapshot of %d bytes", len(data))
		}
	}
}
