package android

import "github.com/nsgo-dev/nsgo/internal/driver"

func readGeometrySnapshot(payload []byte) (uint32, driver.GeometrySnapshot, bool) {
	if len(payload) < 16 {
		return 0, driver.GeometrySnapshot{}, false
	}
	r := reader{payload}
	surface, version, count := r.u32(), r.u64(), r.u32()
	if uint64(count)*60 > uint64(len(payload)-16) {
		return 0, driver.GeometrySnapshot{}, false
	}
	snapshot := driver.GeometrySnapshot{Version: version, Bounds: make(map[driver.NodeID]driver.Bounds, count), Matrices: make(map[driver.NodeID]driver.Matrix3D, count), Sizes: make(map[driver.NodeID]driver.Size, count)}
	for range count {
		id := driver.NodeID(r.u32())
		snapshot.Bounds[id] = driver.Bounds{Origin: driver.Point{X: r.points(), Y: r.points()}, Size: driver.Size{Width: r.points(), Height: r.points()}}
		var quad [4]driver.Point
		for i := range quad {
			quad[i] = driver.Point{X: r.points(), Y: r.points()}
		}
		size := driver.Size{Width: r.points(), Height: r.points()}
		if m, ok := driver.MatrixFromQuad(quad, size); ok {
			snapshot.Matrices[id], snapshot.Sizes[id] = m, size
		}
	}
	return surface, snapshot, true
}
