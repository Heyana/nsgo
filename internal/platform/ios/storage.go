//go:build ios

package ios

import (
	"path/filepath"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/appstartup"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// appDirs resolves storage before starting the engine or reconnecting
// downloads in a background launch. A host's explicit directory wins.
func appDirs(id, hostData string) (data, cache, logs string) {
	data, cache, logs = engine.AppDirs(id)
	if hostData != "" {
		return hostData, cache, filepath.Join(hostData, "logs")
	}
	bundle := objc.ID(apple.Class("NSBundle")).Send(sel("mainBundle"))
	group := apple.GoString(bundle.Send(sel("objectForInfoDictionaryKey:"), apple.NSString(appstartup.StorageAppGroupKey)))
	if group == "" {
		return data, cache, logs
	}
	container, err := apple.AppGroupContainer(group)
	if err != nil {
		// Falling back would silently fork the clip's and app's data.
		panic(err)
	}
	return engine.AppGroupDirs(id, container)
}
