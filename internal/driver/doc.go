// Package driver is the protocol between core and the platform backends:
// the mount items a commit sends, the events backends report, the
// TextEngine they implement, and the values both sides share. It also
// holds what every backend runs the same way on its main thread: the
// animation graph's Animator, the selection model and text field edits.
//
// Backends import driver, never core; core aliases the values apps use.
package driver
