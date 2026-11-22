package devtools

import (
	"slices"
	"testing"
)

type stubSurface struct {
	id, graph int
}

func (s *stubSurface) ID() int                               { return s.id }
func (s *stubSurface) Graph() int                            { return s.graph }
func (s *stubSurface) Post(fn func())                        { fn() }
func (s *stubSurface) Elements() Node                        { return Node{} }
func (s *stubSurface) Owners() Owner                         { return Owner{} }
func (s *stubSurface) Inspect(uint32)                        {}
func (s *stubSurface) Highlight(bool)                        {}
func (s *stubSurface) Layout(bool)                           {}
func (s *stubSurface) WatchLayout(func(bool))                {}
func (s *stubSurface) Pick(bool, func(uint32), func(uint32)) {}
func (s *stubSurface) Record(bool, func(Step))               {}
func (s *stubSurface) Set(uint64, string) error              { return nil }
func (s *stubSurface) Hot(int) []Computation                 { return nil }
func (s *stubSurface) Follow(Follower)                       {}

type stubAgent struct {
	registered   []Surface
	unregistered []Surface
}

func (a *stubAgent) Registered(s Surface, _ bool) { a.registered = append(a.registered, s) }
func (a *stubAgent) Unregistered(s Surface)       { a.unregistered = append(a.unregistered, s) }
func (a *stubAgent) Do([]byte) []byte             { return nil }

func TestRegisterAndUnregisterSurfaces(t *testing.T) {
	prevAuto := Automation
	agent := &stubAgent{}
	Automation = agent
	t.Cleanup(func() { Automation = prevAuto })

	var (
		hidden  Surface = &stubSurface{id: 0, graph: 1}
		mainWin Surface = &stubSurface{id: 1, graph: 1}
		secWin  Surface = &stubSurface{id: 2, graph: 1}
		embed   Surface = &stubSurface{id: 0, graph: 2}
	)

	Register(hidden, false)
	Register(mainWin, true)
	Register(secWin, true)
	Register(embed, true)
	t.Cleanup(func() {
		Unregister(hidden)
		Unregister(mainWin)
		Unregister(secWin)
		Unregister(embed)
	})

	listed := Surfaces()
	if !slices.Contains(listed, mainWin) || !slices.Contains(listed, secWin) || !slices.Contains(listed, embed) {
		t.Fatalf("Surfaces() = %v, missing listed surfaces", listed)
	}
	if slices.Contains(listed, hidden) {
		t.Fatalf("Surfaces() = %v, should not list hidden app surface", listed)
	}
	if len(agent.registered) != 4 {
		t.Fatalf("agent registered %d surfaces, want 4 (including hidden)", len(agent.registered))
	}

	Unregister(secWin)
	Unregister(secWin) // idempotent
	if slices.Contains(Surfaces(), secWin) {
		t.Fatal("Unregister left secWin in Surfaces()")
	}
	if len(agent.unregistered) != 1 || agent.unregistered[0] != secWin {
		t.Fatalf("agent unregistered %v, want only secWin once", agent.unregistered)
	}
}
