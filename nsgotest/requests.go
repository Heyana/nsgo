package nsgotest

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Share records what the app asked to send through the system share sheet.
type Share struct {
	Files []core.SharedFile
	Text  string
	// From is the sheet's anchor in surface points, zero for the surface's center.
	From core.Bounds
}

// WindowAction identifies a window request recorded by App.Window.
type WindowAction string

const (
	WindowSetTitleBar      WindowAction = "setTitleBar"
	WindowMinimize         WindowAction = "minimize"
	WindowToggleMaximize   WindowAction = "toggleMaximize"
	WindowToggleFullScreen WindowAction = "toggleFullScreen"
	WindowClose            WindowAction = "close"
	WindowSetFrame         WindowAction = "setFrame"
	WindowSetLevel         WindowAction = "setLevel"
	WindowOrderFront       WindowAction = "orderFront"
	WindowSetMenuBar       WindowAction = "setMenuBar"
	WindowSetControls      WindowAction = "setControls"
	WindowSetMaterial      WindowAction = "setMaterial"
	WindowSetConstraints   WindowAction = "setConstraints"
	WindowShow             WindowAction = "show"
	WindowHide             WindowAction = "hide"
	WindowDestroy          WindowAction = "destroy"
	WindowSetAnchor        WindowAction = "setAnchor"
	WindowSetWorkspaces    WindowAction = "setWorkspaces"
	WindowSetPointerEvents WindowAction = "setPointerEvents"
	WindowSetTransparent   WindowAction = "setTransparent"
	WindowSetShadow        WindowAction = "setShadow"
	WindowSetSkipTaskbar   WindowAction = "setSkipTaskbar"
	WindowFitSize          WindowAction = "fitSize"
)

// WindowRequest records a window action and the options it was given.
type WindowRequest struct {
	Action   WindowAction
	TitleBar core.TitleBarStyle
	// Height is the custom title bar's height, zero for the platform's own.
	Height      core.Points
	Frame       core.WindowFrame
	Level       core.WindowLevel
	MenuBar     core.MenuBarMode
	Controls    core.WindowControls
	Material    core.WindowMaterial
	Constraints core.WindowConstraints
	// Anchor is the tray icon below which the panel opens, if set.
	Anchor uint32
	// AnchorBounds and Placement locate the panel beside a rectangle in screen points.
	AnchorBounds  core.Bounds
	Placement     core.WindowPlacement
	Workspaces    core.WindowWorkspaces
	PointerEvents core.WindowPointerEvents
	// On enables transparency, the window shadow or skipping the taskbar,
	// according to Action.
	On bool
}

var windowActions = map[driver.WindowAction]WindowAction{
	driver.WindowSetTitleBar:      WindowSetTitleBar,
	driver.WindowMinimize:         WindowMinimize,
	driver.WindowToggleMaximize:   WindowToggleMaximize,
	driver.WindowToggleFullScreen: WindowToggleFullScreen,
	driver.WindowClose:            WindowClose,
	driver.WindowSetFrame:         WindowSetFrame,
	driver.WindowSetLevel:         WindowSetLevel,
	driver.WindowOrderFront:       WindowOrderFront,
	driver.WindowSetMenuBar:       WindowSetMenuBar,
	driver.WindowSetControls:      WindowSetControls,
	driver.WindowSetMaterial:      WindowSetMaterial,
	driver.WindowSetConstraints:   WindowSetConstraints,
	driver.WindowShow:             WindowShow,
	driver.WindowHide:             WindowHide,
	driver.WindowDestroy:          WindowDestroy,
	driver.WindowSetAnchor:        WindowSetAnchor,
	driver.WindowSetWorkspaces:    WindowSetWorkspaces,
	driver.WindowSetPointerEvents: WindowSetPointerEvents,
	driver.WindowSetTransparent:   WindowSetTransparent,
	driver.WindowSetShadow:        WindowSetShadow,
	driver.WindowSetSkipTaskbar:   WindowSetSkipTaskbar,
	driver.WindowFitSize:          WindowFitSize,
}

func windowRequest(c *driver.WindowCommand) WindowRequest {
	return WindowRequest{
		Action:        windowActions[c.Action],
		TitleBar:      c.TitleBar,
		Height:        c.Height,
		Frame:         c.Frame,
		Level:         c.Level,
		MenuBar:       c.MenuBar,
		Controls:      c.Controls,
		Material:      c.Material,
		Constraints:   c.Constraints,
		Anchor:        c.Anchor,
		AnchorBounds:  c.AnchorBounds,
		Placement:     c.Placement,
		Workspaces:    c.Workspaces,
		PointerEvents: c.PointerEvents,
		On:            c.On,
	}
}
