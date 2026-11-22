// Package appstartup holds startup identities shared by native backends and
// packaging without linking the runtime into the build tool.
package appstartup

// WindowsTaskID is the disabled-by-default MSIX startup task declared by
// the packaging tool and selected by the Windows backend.
const WindowsTaskID = "NSGoStartup"
