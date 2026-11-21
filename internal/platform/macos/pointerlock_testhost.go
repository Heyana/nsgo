//go:build darwin && !ios && nsgotest

package macos

func (t *testSurface) PointerDelta(dx, dy float64) {
	t.follow()
	onMainSync(func() { t.s.pointerMotion(dx, dy) })
}
