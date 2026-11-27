//go:build nsgo_steam

package updates

// EnableDownload is a no-op in Steam builds, where Steam owns updates.
func EnableDownload() {}
