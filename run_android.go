//go:build android

package nsgo

import "github.com/oneo-me/nsgo/platform/android"

func run(app App) { android.Run(app) }
