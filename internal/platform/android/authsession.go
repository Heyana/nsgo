package android

import (
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// filterAuthSessions passes b to a runtime that reads ItemAuthSession (capable); for one built before it, it answers each session's start unavailable through dispatch, as before Android had auth sessions, and returns a copy of b without them.
func filterAuthSessions(capable bool, b *driver.MountBatch, dispatch func(driver.Event)) *driver.MountBatch {
	auth := func(it driver.MountItem) bool { return it.Kind == driver.ItemAuthSession }
	if capable || !slices.ContainsFunc(b.Items, auth) {
		return b
	}
	for i := range b.Items {
		if ev, ok := driver.NoAuthSession(&b.Items[i]); ok {
			dispatch(ev)
		}
	}
	out := *b
	out.Items = slices.DeleteFunc(slices.Clone(b.Items), auth)
	return &out
}

// writeAuthRequest is the request AuthSession.java reads from a batch.
func writeAuthRequest(w *writer, id uint32, r *driver.AuthRequest) {
	if r == nil {
		r = &driver.AuthRequest{Command: driver.AuthCancel}
	}
	w.u32(id)
	w.byte(byte(r.Command))
	w.str(r.Scheme)
	w.str(r.Host)
	w.str(r.Path)
	w.flags(r.Ephemeral)
	w.str(r.URL)
}

// readAuthSession decodes the phase and its two strings: redirect, or error code and message/callback. The original callback stays intact.
func readAuthSession(ev driver.Event, data string) *driver.AuthEvent {
	switch ev.Index {
	case int(driver.AuthRedirect):
		return &driver.AuthEvent{Phase: driver.AuthRedirect, Redirect: ev.Text}
	case int(driver.AuthEnded):
		answer := &driver.AuthEvent{Phase: driver.AuthEnded, Error: ev.Text}
		if ev.Text == "" {
			answer.Callback = data
		} else {
			answer.Message = data
		}
		return answer
	default:
		return &driver.AuthEvent{Phase: driver.AuthEnded, Error: driver.AuthFailed, Message: "invalid Android authentication response"}
	}
}
