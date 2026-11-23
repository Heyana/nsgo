//go:build darwin && !ios

package macos

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Closing a surface frees its views as removing them does: a shader view's Metal layer and the thread that renders it go with the window.
func TestClosingASurfaceReleasesItsShaders(t *testing.T) {
	onMain(func() {
		loadAppKit()
		s := newSurface(Options{Width: 200, Height: 100}, false)
		defer func() {
			delete(views, s.native)
			s.native.Send(selRelease)
		}()
		v := s.create(driver.KindShader, 1)
		s.views = []*view{nil, v}
		s.native.Send(selAddSubview, v.native)
		sl := v.shader
		if sl == nil {
			t.Skip("no Metal device")
		}
		s.close()
		if sl.Layer != 0 {
			t.Error("a closed surface's shader keeps its layer")
		}
		if _, ok := views[v.native]; ok {
			t.Error("a closed surface's view stays among the native views")
		}
		if s.views[1] != nil {
			t.Error("a closed surface keeps its views")
		}
	})
}
