// Package core is what NSGo apps are written with: elements, styles, the
// reactive graph and regions, the services an app asks for, and the shadow
// tree that turns them into mount items, laid out and with text prepared.
//
// The values it shares with the platforms, such as geometry, colors and
// configurations, are defined in [driver], the platform protocol, and
// aliased here, so apps import core alone. It never imports a platform
// package or cgo, so it builds and tests headless on every OS.
package core
