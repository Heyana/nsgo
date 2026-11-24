// Package appstartup holds identities shared by native backends and packaging without linking the runtime into the build tool.
package appstartup

// WindowsTaskID is the disabled-by-default MSIX startup task declared by
// the packaging tool and selected by the Windows backend.
const WindowsTaskID = "NSGoStartup"

// StorageAppGroupKey is the Info.plist entry that opts an iOS App Clip and
// its full app into persistent storage in their shared App Group container.
// Widget-only apps keep their existing sandbox storage.
const StorageAppGroupKey = "NSGoStorageAppGroup"

// LocalNetworkProbeType is the Bonjour service type the Apple backends advertise and browse to ask for Local Network access, which nsgo build adds to NSBonjourServices whenever apple.info has NSLocalNetworkUsageDescription, as iOS refuses to browse a type the Info.plist does not list.
const LocalNetworkProbeType = "_nsgo-lnp._tcp"
