package linux

// preeditAction is how the Linux backend responds to a GTK preedit-changed signal.
type preeditAction uint8

const (
	preeditNone   preeditAction = iota
	preeditUpdate               // report active preedit text
	preeditEnd                  // end composition immediately
	preeditDefer                // focus is leaving; hold end until leave runs
)

// preeditState tracks a GTK 4 editor's active IME preedit so that losing focus
// or becoming non-editable commits an uncommitted preedit before ending the
// composition and blurring, without duplicating text the IME already committed.
type preeditState struct {
	text      string
	active    bool
	leaving   bool
	forcing   bool
	committed bool
}

// Active reports whether a preedit is currently active or waiting to finish on leave.
func (p *preeditState) Active() bool { return p.active || p.leaving }

// BeginForceFinish marks an in-progress reset (such as turning an editor
// disabled or read-only) so a preedit-changed("") emitted during the reset
// defers to Finish instead of discarding the uncommitted preedit.
func (p *preeditState) BeginForceFinish() {
	if p.active {
		p.forcing = true
	}
}

// Changed handles GTK's preedit-changed signal with preedit text while
// focused reports whether the editor still owns keyboard focus.
func (p *preeditState) Changed(text string, focused bool) preeditAction {
	if text != "" {
		p.active = true
		p.text = text
		p.leaving = false
		p.committed = false
		return preeditUpdate
	}
	if !p.active {
		return preeditNone
	}
	if p.committed || (focused && !p.forcing) {
		*p = preeditState{}
		return preeditEnd
	}
	// Focus is leaving (or a forced finish is underway) and the IME cleared
	// the preedit without a prior commit. Defer ending the composition until
	// Finish runs so a commit emitted right after preedit-changed("") is not
	// duplicated, and an uncommitted preedit is committed before the end and blur.
	p.active = false
	p.leaving = true
	return preeditDefer
}

// Edited records an actual user or IME edit to the editor's text.
func (p *preeditState) Edited() {
	if p.active || p.leaving {
		p.committed = true
	}
}

// Finish concludes any active or deferred composition on blur or when the
// editor stops taking text. It returns the preedit text to commit when the
// IME did not already commit text, and whether a composition was in progress.
func (p *preeditState) Finish() (commit string, wasActive bool) {
	if !p.active && !p.leaving {
		p.forcing = false
		return "", false
	}
	prev := p.text
	wasCommitted := p.committed
	*p = preeditState{}
	if !wasCommitted && prev != "" {
		return prev, true
	}
	return "", true
}

// Reset clears preedit tracking when an editor is dropped.
func (p *preeditState) Reset() { *p = preeditState{} }
