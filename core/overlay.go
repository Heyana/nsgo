package core

import "github.com/nsgo-dev/nsgo/internal/driver"

// Windows over other apps on Android: the core's part of mobile.OpenOverlay, mobile.OverlayPermission and mobile.RequestOverlayPermission, which apps use. An overlay is a surface the app opens, as a panel is on a desktop, whose tree the platform attaches; elsewhere there is none.

// OverlayPermission is whether the app may show windows over other apps, Android's special access to draw over them: the core's part of mobile.OverlayPermission's answer, in NotificationPermission's words without its not asked, as the special access has no such state.
type OverlayPermission uint8

const (
	// OverlayDenied keeps the app from showing overlays until the user grants it in the system's settings (RequestOverlayPermission).
	OverlayDenied OverlayPermission = iota
	// OverlayGranted lets the app show them.
	OverlayGranted
	// OverlayUnavailable is a platform without overlays, every one but Android, an app whose manifest does not ask for SYSTEM_ALERT_WINDOW, or code its Access does not grant AccessOverlay.
	OverlayUnavailable
)

// String returns "denied", "granted" or "unavailable".
func (p OverlayPermission) String() string {
	switch p {
	case OverlayGranted:
		return "granted"
	case OverlayUnavailable:
		return "unavailable"
	}
	return "denied"
}

// OverlayEnd is why the platform closed an overlay itself, which OverlayOptions.OnEnd hears.
type OverlayEnd uint8

const (
	// OverlayEndDenied is an overlay opened without the permission: no window showed.
	OverlayEndDenied OverlayEnd = iota + 1
	// OverlayEndUnavailable is one opened where the app's manifest does not ask for the permission.
	OverlayEndUnavailable
	// OverlayEndRevoked is the user taking the permission away while it showed.
	OverlayEndRevoked
	// OverlayEndRemoved is the system or the user stopping the service that kept it.
	OverlayEndRemoved
	// OverlayEndFailed is the platform failing to show it or to start its service, as Android refuses a foreground service it does not allow.
	OverlayEndFailed
)

// String returns "denied", "unavailable", "revoked", "removed" or "failed".
func (e OverlayEnd) String() string {
	switch e {
	case OverlayEndDenied:
		return "denied"
	case OverlayEndUnavailable:
		return "unavailable"
	case OverlayEndRevoked:
		return "revoked"
	case OverlayEndRemoved:
		return "removed"
	}
	return "failed"
}

// OverlayOptions are an overlay's: the core's part of mobile.OverlayOptions, which apps use.
type OverlayOptions struct {
	// Width and Height are the content's size in points; 320 by 180 when either is zero.
	Width, Height Points
	// Frame, when it moves, opens the overlay there at its size instead of centered. The platform keeps every frame on the screen's usable area and reports the one it applied in the overlay's Env.WindowFrame; its content moves and resizes it with SetWindowFrame, as desktop.SetWindowFrame does a window.
	Frame WindowFrame
	// Hidden opens it without showing it, for Window.Show.
	Hidden bool
	// Focusable lets it take the focus, which text input needs; by default it takes touches and the keys stay with the app in front.
	Focusable bool
	// Passive lets every touch through to the apps below it, at most at the window opacity Android lets touches pass through (0.8 by default); it takes none and never the focus.
	Passive bool
	// Accessible shows its content to screen readers, which it is hidden from by default.
	Accessible bool
	// OnEnd hears, once, that the platform closed the overlay itself and why, as the overlay's owner hears it; Window.Close and its owner going do not call it.
	OnEnd func(OverlayEnd)
}

// OverlayPermission is the core's part of mobile.OverlayPermission, which apps use: done hears whether the app may show overlays, without asking the user.
func (cx *Scope) OverlayPermission(done func(OverlayPermission)) { cx.overlayPermission(false, done) }

// RequestOverlayPermission is the core's part of mobile.RequestOverlayPermission, which apps use: it opens the system's setting that lets the app show overlays and done hears the permission as the app comes back to the front, or at once where there is nothing to ask. Call it as the user does something that needs it.
func (cx *Scope) RequestOverlayPermission(done func(OverlayPermission)) {
	cx.overlayPermission(true, done)
}

func (cx *Scope) overlayPermission(ask bool, done func(OverlayPermission)) {
	answer := func(p OverlayPermission) {
		if done != nil {
			done(p)
		}
	}
	t := cx.tree()
	if t == nil {
		return
	}
	if !cx.overlays() {
		answer(OverlayUnavailable)
		return
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemOverlayPermission, On: ask}, func(ev driver.Event) {
		answer(OverlayPermission(ev.Index))
	})
}

// overlays reports whether code using cx may show overlays: on Android, where its Access grants AccessOverlay.
func (cx *Scope) overlays() bool {
	return cx.Env().hostUntracked() == OSAndroid && !cx.denied(serviceOverlay, "")
}

// OpenOverlay is the core's part of mobile.OpenOverlay, which apps use: it opens an overlay over other apps showing content, a tree of its own that belongs to cx and closes as cx goes. The app has one overlay at a time: it returns nil while another is open, off Android and where Access does not grant AccessOverlay. Without the permission the platform ends it at once (OverlayEndDenied).
func (cx *Scope) OpenOverlay(opts OverlayOptions, content func(cx *Scope) Node) *Window {
	rt := cx.rt
	if !cx.overlays() || rt.overlay != nil && !rt.overlay.closed {
		return nil
	}
	if opts.Width <= 0 || opts.Height <= 0 {
		opts.Width, opts.Height = 320, 180
	}
	if opts.Frame.Move && opts.Frame.Width > 0 && opts.Frame.Height > 0 {
		opts.Width, opts.Height = opts.Frame.Width, opts.Frame.Height
	} else {
		opts.Frame = WindowFrame{Move: opts.Frame.Move, X: opts.Frame.X, Y: opts.Frame.Y}
	}
	opts.Frame.Width, opts.Frame.Height = opts.Width, opts.Height
	passive := opts.Passive
	owner := cx
	w := rt.openSurface(owner, nil, driver.SurfaceConfig{Kind: driver.SurfaceOverlay, Width: opts.Width, Height: opts.Height,
		Frame: opts.Frame, Hidden: opts.Hidden, Focusable: opts.Focusable && !passive, Passive: passive,
		Accessible: opts.Accessible}, content)
	w.onEnd, w.endOwner = opts.OnEnd, owner
	rt.overlay = w
	return w
}

// end closes an overlay the platform ended for reason, then tells its OnEnd.
func (w *Window) end(reason OverlayEnd) {
	w.t.within(w.t.root, w.Close)
	if fn := w.onEnd; fn != nil {
		w.onEnd = nil
		owner := w.endOwner
		if owner.disposed {
			owner = w.t.rt.root
		}
		w.t.within(owner, func() { fn(reason) })
	}
}
