package driver

// PointerLock identifies a request for native relative motion.
type PointerLock struct {
	ID    NodeID
	Count uint32
}

func (p PointerLock) Matches(id NodeID, count uint32) bool { return p.ID == id && p.Count == count }

func (p PointerLock) Unlocked() Event {
	return Event{Kind: EventPointerUnlocked, ID: p.ID, Count: p.Count}
}

func (p PointerLock) Motion(dx, dy float64) Event {
	return Event{Kind: EventPointerDelta, ID: p.ID, Count: p.Count, Data: PointerDelta{X: dx, Y: dy}}
}

// PointerDelta is a relative movement in the platform's input units.
type PointerDelta struct{ X, Y float64 }
