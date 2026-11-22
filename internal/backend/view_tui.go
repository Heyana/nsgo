//go:build nsgotui

package backend

import "github.com/nsgo-dev/nsgo/internal/engine"

func newView(engine.App, Options, engine.HostOptions) View {
	panic("nsgo: NewView needs a GUI backend; use Run for a terminal application")
}
