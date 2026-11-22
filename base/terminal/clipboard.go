package terminal

import (
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/base/terminal/internal/vt"
	"github.com/nsgo-dev/nsgo/core"
)

// clipboardHost is one mounted view's clipboard authority. Terminal.mu guards its ownership, the paste snapshot and outstanding requests. Native services run only through its scope, on the engine's goroutine.
type clipboardHost struct {
	t           *Terminal
	cx          *core.Scope
	read, write bool
	pending     map[*clipboardRead]struct{}
	paste       *clipboardContents
}

type clipboardContents struct{ data map[string][]byte }

func (c *clipboardContents) types() []string {
	var types []string
	for _, mime := range []string{"text/plain", "image/png"} {
		if len(c.data[mime]) > 0 {
			types = append(types, mime)
		}
	}
	return types
}

// bindClipboard replaces the previous view's authority before it can answer a late request. Rebuilding or disposing a view also revokes its paste password's snapshot, even when the terminal session itself remains alive.
func (v *view) bindClipboard() {
	t := v.t
	access := v.cx.CheckAccess(core.AccessRequest{Permission: core.AccessClipboard}) == nil
	h := &clipboardHost{t: t, cx: v.cx, read: v.p.AllowClipboardRead && access,
		write: v.p.AllowClipboardWrite && access, pending: make(map[*clipboardRead]struct{})}
	v.clipboard = h
	t.mu.Lock()
	if t.clipboard != nil {
		t.clipboard.revoke()
	}
	t.clipboard = h
	t.mu.Unlock()
	v.cx.Cleanup(func() {
		t.mu.Lock()
		h.revoke()
		if t.clipboard == h {
			t.clipboard = nil
		}
		t.mu.Unlock()
	})
}

// revoke is called with Terminal.mu held, after the parser has finished filling each deferred read. Refusal does not call into the graph or read native data.
func (h *clipboardHost) revoke() {
	h.paste = nil
	for r := range h.pending {
		r.finishLocked(false, nil)
	}
}

func (h *clipboardHost) live() bool {
	h.t.mu.Lock()
	defer h.t.mu.Unlock()
	return h.t.clipboard == h && !h.t.closed && h.t.term != nil
}

type clipboardRead struct {
	t        *Terminal
	host     *clipboardHost
	request  vt.ClipboardRead
	deferred *vt.DeferredClipboardRead
	snapshot *clipboardContents
	done     bool
}

// requestClipboard runs under Terminal.mu during parser output. Graph work is queued until Write returns, so the binding has captured the complete reply.
func (t *Terminal) requestClipboard(req vt.ClipboardRead) (vt.ClipboardReply, vt.ClipboardAnswer) {
	r := &clipboardRead{t: t, host: t.clipboard, request: req, deferred: &vt.DeferredClipboardRead{}}
	if r.host != nil {
		r.host.pending[r] = struct{}{}
		if req.Granted {
			r.snapshot = r.host.paste
			r.host.paste = nil
		}
	}
	t.events.clipboard = append(t.events.clipboard, r.dispatch)
	return vt.ClipboardReply{Deferred: r.deferred}, vt.ClipboardDeferred
}

func (r *clipboardRead) dispatch() {
	h := r.host
	if h == nil || r.request.Primary {
		r.finish(false, nil)
		return
	}
	h.cx.Post(func() {
		if !h.live() {
			r.finish(false, nil)
			return
		}
		if r.request.Granted {
			r.finish(r.snapshot != nil, r.snapshot)
			return
		}
		if !h.read || h.cx.CheckAccess(core.AccessRequest{Permission: core.AccessClipboard}) != nil {
			r.finish(false, nil)
			return
		}
		h.readContents(r.request.Mimes, r.request.List, func(c *clipboardContents) { r.finish(true, c) })
	})
}

func (r *clipboardRead) finish(allowed bool, c *clipboardContents) {
	r.t.mu.Lock()
	defer r.t.mu.Unlock()
	r.finishLocked(allowed, c)
}

func (r *clipboardRead) finishLocked(allowed bool, c *clipboardContents) {
	if r.done {
		return
	}
	r.done = true
	if r.host != nil {
		delete(r.host.pending, r)
		allowed = allowed && r.t.clipboard == r.host && !r.t.closed
	}
	if r.t.term == nil {
		return
	}
	var contents map[string][]byte
	var available []string
	if c != nil {
		contents, available = c.data, c.types()
	}
	r.t.term.AnswerClipboard(r.deferred, allowed, contents, available)
}

// writeClipboard preserves the Mirror's pass-through and adds native writes only for the current view's explicit permission. Its protocol reply is synchronous; the actual native operation is posted to the owning scope.
func (t *Terminal) writeClipboard(text string, primary bool) bool {
	if primary {
		return false
	}
	for _, m := range t.mirrors {
		m.clipOut = queueClip(m.clipOut, text)
	}
	h := t.clipboard
	allowed := h != nil && h.write && !t.closed
	if allowed {
		t.events.clipboard = append(t.events.clipboard, func() {
			h.cx.Post(func() {
				if h.live() && h.cx.CheckAccess(core.AccessRequest{Permission: core.AccessClipboard}) == nil {
					h.cx.SetClipboard(text)
				}
			})
		})
	}
	return allowed || len(t.mirrors) > 0
}

// readContents uses NSGo's portable text/PNG clipboard services. Every callback checks ownership before issuing the next read, and copies native data before retaining it as a paste snapshot. No extra clipboard representations are read.
func (h *clipboardHost) readContents(mimes []string, list bool, done func(*clipboardContents)) {
	c := &clipboardContents{data: make(map[string][]byte)}
	image := func() {
		if !h.live() {
			done(nil)
			return
		}
		if !list && !slices.Contains(mimes, "image/png") {
			done(c)
			return
		}
		h.cx.ReadClipboardImage(func(img core.ClipboardImage, ok bool, err error) {
			if !h.live() {
				done(nil)
				return
			}
			if ok && err == nil {
				c.data["image/png"] = append([]byte(nil), img.PNG...)
			}
			done(c)
		})
	}
	if !list && !slices.Contains(mimes, "text/plain") {
		image()
		return
	}
	h.cx.ReadClipboard(func(text string) {
		if text != "" {
			c.data["text/plain"] = []byte(text)
		}
		image()
	})
}

// pasteClipboard is an explicit user paste. Mode 5522 receives an offer and a password for this snapshot; other programs receive ordinary bracketed text.
func (h *clipboardHost) pasteClipboard() {
	h.cx.Post(func() {
		if !h.live() {
			return
		}
		h.t.mu.Lock()
		kitty := h.t.term.Mode(vt.ModePasteEvents)
		h.t.mu.Unlock()
		h.readContents([]string{"text/plain"}, kitty, h.offer)
	})
}

func (h *clipboardHost) offer(c *clipboardContents) {
	if c == nil || !h.live() {
		return
	}
	h.t.mu.Lock()
	if h.t.clipboard != h || h.t.closed || h.t.term == nil {
		h.t.mu.Unlock()
		return
	}
	offered := h.t.term.PasteEvent(c.types())
	h.paste = nil
	if offered {
		h.paste = c
		h.t.term.ScrollToBottom()
		h.t.term.SetSelection(nil)
	}
	h.t.mu.Unlock()
	if !offered && len(c.data["text/plain"]) > 0 {
		h.t.Paste(string(c.data["text/plain"]))
	}
	h.t.redraw()
}

// pasted retains the trusted Input paste event's own snapshot. PNG data already supplied by the native input needs no second read of the system clipboard.
func (v *view) pasted(p core.PasteEvent) {
	h := v.clipboard
	if h == nil || !h.live() {
		return
	}
	c := &clipboardContents{data: make(map[string][]byte)}
	if p.Text != "" {
		c.data["text/plain"] = []byte(p.Text)
	}
	normalizeImage := false
	for _, f := range p.Files {
		normalizeImage = normalizeImage || strings.HasPrefix(f.MIME, "image/")
		if f.MIME == "image/png" && len(f.Data) > 0 && len(f.Data) <= core.MaxClipboardImage {
			c.data["image/png"] = append([]byte(nil), f.Data...)
			break
		}
	}
	if len(c.types()) == 0 && normalizeImage {
		// Native clipboard services normalize supported image encodings to PNG.
		h.pasteClipboard()
		return
	}
	h.offer(c)
}
