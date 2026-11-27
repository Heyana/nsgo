//go:build (darwin && !nsgoembed && !nsgowatch && !nsgotui) || (linux && !nsgotui) || (windows && !nsgotui) || android || nsgoandroidhost

package backend

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
)

// Set by the nsgo tool with -ldflags -X: updateFeed is this target's appcast, updateKey the public key its downloads are signed with, and updateAutomatic "1" when the app checks on its own.
var updateFeed, updateKey, updateAutomatic, updateChannel = "", "", "", ""

// updateConfig is how the app updates, as the nsgo tool set it from
// nsgo.yaml's updates; macOS's Sparkle reads its own from Info.plist.
func updateConfig() updates.Config {
	info := core.App()
	return updates.Config{App: name, ID: id, Version: info.Version, Build: info.Build,
		Feed: updateFeed, Key: updateKey, Automatic: updateAutomatic == "1", Channel: updateChannel, Development: info.Development}
}
