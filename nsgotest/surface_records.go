package nsgotest

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// SurfaceKind identifies what the app opened.
type SurfaceKind uint8

const (
	SurfaceWindow SurfaceKind = iota
	SurfacePanel
	SurfaceTrayIcon
	SurfaceOverlay
	SurfaceScene
)

// SurfaceConfig records a surface's opening options. ID identifies the
// surface, and Main reports whether it is the app's main window.
type SurfaceConfig struct {
	ID                             uint32
	Kind                           SurfaceKind
	Title                          string
	Width, Height                  core.Points
	Frame                          core.WindowFrame
	Hidden                         bool
	Focusable, Passive, Accessible bool
	Material                       core.WindowMaterial
	Constraints                    core.WindowConstraints
	HideOnBlur, NoActivate         bool
	PointerEvents                  core.WindowPointerEvents
	Transparent, NoShadow          bool
	SkipTaskbar                    bool
	// Tray and Scene identify an existing tray icon or system scene,
	// when the surface was opened for one.
	Tray, Scene uint32
	Workspaces  core.WindowWorkspaces
	Main        bool
}

var surfaceKinds = map[driver.SurfaceKind]SurfaceKind{
	driver.SurfaceWindow:   SurfaceWindow,
	driver.SurfacePanel:    SurfacePanel,
	driver.SurfaceTrayIcon: SurfaceTrayIcon,
	driver.SurfaceOverlay:  SurfaceOverlay,
	driver.SurfaceScene:    SurfaceScene,
}

func surfaceRecord(c *driver.SurfaceConfig) SurfaceConfig {
	return SurfaceConfig{
		ID: c.ID, Kind: surfaceKinds[c.Kind], Title: c.Title,
		Width: c.Width, Height: c.Height, Frame: c.Frame, Hidden: c.Hidden,
		Focusable: c.Focusable, Passive: c.Passive, Accessible: c.Accessible,
		Material: c.Material, Constraints: c.Constraints, HideOnBlur: c.HideOnBlur, NoActivate: c.NoActivate,
		PointerEvents: c.PointerEvents, Transparent: c.Transparent, NoShadow: c.NoShadow, SkipTaskbar: c.SkipTaskbar,
		Tray: c.Tray, Scene: c.Scene, Workspaces: c.Workspaces, Main: c.Main,
	}
}

// StatusBarConfig records the resolved status bar settings the app sent.
type StatusBarConfig struct {
	Style    core.StatusBarStyle
	Hidden   bool
	Animated bool
}
