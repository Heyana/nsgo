package updates

import "github.com/nsgo-dev/nsgo/internal/driver"

// State resolves build and installation facts without network I/O.
func (c Config) State(appStore bool) driver.UpdaterState {
	managed := driver.UpdateManagedSelf
	switch {
	case c.Store != "":
		managed = driver.UpdateManagedStore
	case c.Development:
		managed = driver.UpdateManagedDevelopment
	case c.Channel == "none" || c.Channel == "steam":
		managed = driver.UpdateManagedChannel
	case appStore:
		managed = driver.UpdateManagedPage
	case c.Channel == "store":
		managed = driver.UpdateManagedChannel
	case c.Feed == "":
		managed = driver.UpdateManagedChannel
	case c.PageOnly:
		managed = driver.UpdateManagedPage
	}
	return driver.UpdaterState{Managed: managed, Store: c.Store, AutomaticChecks: c.Automatic && (managed == driver.UpdateManagedSelf || managed == driver.UpdateManagedPage)}
}

func (c Config) answer(appStore bool) driver.Update {
	s := c.State(appStore)
	return driver.Update{App: c.App, Current: c.Version, Managed: s.Managed, Store: s.Store}
}
