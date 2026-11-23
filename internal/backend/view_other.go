//go:build !nsgotui && (nsgoandroidhost || (!(darwin && !ios) && !windows && !(linux && !android)))

package backend

import "github.com/nsgo-dev/nsgo/internal/engine"

func newView(engine.App, Options, engine.HostOptions) View {
	panic("nsgo: NewView embeds apps in desktop hosts; iOS and Android apps embed them with the frameworks nsgo build -embed makes")
}
