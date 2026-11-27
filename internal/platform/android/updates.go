//go:build android || nsgoandroidhost

package android

import (
	"context"
	"os"
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
)

// updateConfig is how the app Run runs updates. A check of an APK the
// user installed reads the app's appcast through the runtime's HTTP, and
// package updater hands a verified APK to Android's package installer; one
// of a Google Play app enters its native update flow, and other stores
// answer as managed without reading an appcast.
var updateConfig updates.Config

// InstalledBy names the store that installed the app, which updates it,
// from the installing package the runtime passes in NSGO_INSTALLER
// (updates.AndroidStore).
func InstalledBy() string { return updates.AndroidStore(os.Getenv("NSGO_INSTALLER")) }

// withoutUpdateChecks answers the checks for updates of surface id's batch b, which Go carries out, and returns b without them, as the runtime knows no such item.
func (s *surface) withoutUpdateChecks(id uint32, b *driver.MountBatch) *driver.MountBatch {
	check := func(it driver.MountItem) bool { return it.Kind == driver.ItemCheckForUpdates }
	if !slices.ContainsFunc(b.Items, check) {
		return b
	}
	c := updateConfig
	for _, it := range b.Items {
		if check(it) {
			if c.PlayUpdates() && s.playUpdates.Load() {
				err := s.sendExtended(extPlayUpdate, func(w *writer) {
					w.u32(id)
					w.u32(it.Count)
				})
				if err != nil {
					s.answerPlayUpdate(id, it.Count, false, err.Error())
				}
				continue
			}
			if c.Key != "" && !c.PageOnly {
				updates.EnableDownload()
			}
			updates.Answer(it.Count, func(ctx context.Context) driver.Update { return updates.Find(ctx, c) },
				func(ev driver.Event) { s.dispatchTo(id, ev) })
		}
	}
	out := *b
	out.Items = slices.DeleteFunc(slices.Clone(b.Items), check)
	return &out
}

// withoutDesktopItems removes the desktop-only items Android has nothing for: task progress, which launchers have no portable surface for, the app icon's menu and recent documents. The Java protocol need not carry a command that has no effect.
func withoutDesktopItems(b *driver.MountBatch) *driver.MountBatch {
	desktop := func(it driver.MountItem) bool {
		return it.Kind == driver.ItemTaskProgress || it.Kind == driver.ItemDockMenu || it.Kind == driver.ItemRecentFile
	}
	if !slices.ContainsFunc(b.Items, desktop) {
		return b
	}
	out := *b
	out.Items = slices.DeleteFunc(slices.Clone(b.Items), desktop)
	return &out
}

func (s *surface) playUpdateReply(r *reader) {
	from, count, shown, failure := r.u32(), r.u32(), r.byte() != 0, r.str()
	s.answerPlayUpdate(from, count, shown, failure)
}

func (s *surface) answerPlayUpdate(from, count uint32, shown bool, failure string) {
	c := updateConfig
	state := c.State(false)
	u := driver.Update{App: c.App, Current: c.Version, Managed: state.Managed, Store: state.Store, Err: failure}
	s.dispatchTo(from, driver.Event{Kind: driver.EventUpdate, Count: count, Finished: shown, Data: &u})
}
