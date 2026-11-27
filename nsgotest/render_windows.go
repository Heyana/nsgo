//go:build windows

package nsgotest

import (
	"image"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/windows"
)

// PlatformTextEngine returns the host's native text engine (DirectWrite on
// Windows), or nil where no windowless engine is available. Pass it to
// TextEngine for native metrics.
func PlatformTextEngine() core.TextEngine { return windows.PlatformTextEngine() }

// RegisterFont registers font data with the process's native text engine.
// Register fonts before preparing text or starting tests that use them.
func RegisterFont(data []byte) error { return windows.RegisterFont(data) }

// Render plays d into a transparent bitmap without opening a window. Size is
// in points and scale is pixels per point; both must be finite and positive,
// with at most 16 Mi output pixels. It waits for images for up to 30 seconds,
// uses animated images' first frames, and reports load and decode failures.
// The drawing must have resolved its view paints and animation slots.
// macOS, Windows and Linux are supported; other targets return ErrNoPlayer.
func Render(d *core.Drawing, size core.Size, scale float64) (*image.RGBA, error) {
	return windows.RenderDrawing(d, size, scale)
}

func canRenderDrawings() bool { return true }
