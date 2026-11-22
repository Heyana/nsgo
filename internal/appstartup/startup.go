// Package appstartup holds startup identities shared by native backends and
// packaging without linking the runtime into the build tool.
package appstartup

// WindowsTaskID is the disabled-by-default MSIX startup task declared by
// the packaging tool and selected by the Windows backend.
const WindowsTaskID = "NSGoStartup"

// StorageAppGroupKey is the Info.plist entry that opts an iOS App Clip and
// its full app into persistent storage in their shared App Group container.
// Widget-only apps keep their existing sandbox storage.
const StorageAppGroupKey = "NSGoStorageAppGroup"
