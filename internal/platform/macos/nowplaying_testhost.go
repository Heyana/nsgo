//go:build darwin && !ios && nsgotest

package macos

import "github.com/nsgo-dev/nsgo/internal/driver"

func (t *testSurface) MediaCommand(revision uint32, command driver.MediaCommand) {
	onMainSync(func() {
		if t.own.closed {
			return
		}
		t.own.applyBatches(0, true)
		if nowPlaying.owner == t.own && nowPlaying.input.revision == revision {
			nowPlaying.input.send(command)
		}
	})
}
