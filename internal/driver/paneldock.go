package driver

// PanelDock attaches a panel to an output's usable edge. A default Edge leaves the panel's placement unchanged; Display is an optional display ID.
type PanelDock struct {
	Edge    WindowPlacement
	Display string
	Margin  Points
}

// DockWindow places a fixed-size panel along the selected display's usable edge, centered on the other axis. An absent explicit display is unavailable.
func DockWindow(size Size, displays []DisplayInfo, dock PanelDock) (WindowFrame, bool) {
	if dock.Edge == WindowPlaceDefault || dock.Edge > WindowPlaceRight || len(displays) == 0 || size.Width <= 0 || size.Height <= 0 {
		return WindowFrame{}, false
	}
	on := displays[0]
	if dock.Display != "" {
		found := false
		for _, candidate := range displays {
			if candidate.ID == dock.Display {
				on, found = candidate, true
				break
			}
		}
		if !found {
			return WindowFrame{}, false
		}
	} else {
		for _, candidate := range displays {
			if candidate.Main {
				on = candidate
				break
			}
		}
	}
	area := on.VisibleFrame
	if area.Size.Width <= 0 || area.Size.Height <= 0 {
		area = on.Frame
	}
	if area.Size.Width <= 0 || area.Size.Height <= 0 {
		return WindowFrame{}, false
	}
	margin := max(dock.Margin, 0)
	x := area.Origin.X + (area.Size.Width-size.Width)/2
	y := area.Origin.Y + (area.Size.Height-size.Height)/2
	switch dock.Edge {
	case WindowPlaceTop:
		y = area.Origin.Y + margin
	case WindowPlaceBottom:
		y = area.Origin.Y + area.Size.Height - size.Height - margin
	case WindowPlaceLeft:
		x = area.Origin.X + margin
	case WindowPlaceRight:
		x = area.Origin.X + area.Size.Width - size.Width - margin
	}
	x = max(area.Origin.X, min(x, area.Origin.X+area.Size.Width-size.Width))
	y = max(area.Origin.Y, min(y, area.Origin.Y+area.Size.Height-size.Height))
	return WindowFrame{X: x, Y: y, Width: size.Width, Height: size.Height, Move: true}, true
}
