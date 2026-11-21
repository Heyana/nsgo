package core

import (
	"fmt"
	"testing"
)

// A report that changes only fields read through Env signals commits without a full round, and a tree that reads none of them sends nothing.
func TestWindowEnvironmentReportsStayIncremental(t *testing.T) {
	h := newHost(t, Size{Width: 200, Height: 100}, func(*Scope) Node {
		return View{TestID: "static"}
	})
	h.commit()
	h.commit()
	for _, env := range []Environment{
		{WindowFrame: WindowFrame{X: 20, Y: 30}},
		{Maximized: true},
		{Lifecycle: LifecycleBackground},
		{TitleBar: Edges{Top: 24}},
		{ThermalState: ThermalFair},
		{LowPowerMode: LowPowerOn},
	} {
		h.tree.SetEnvironment(env)
		if h.tree.full {
			t.Fatalf("environment %+v forced a full resolve", env)
		}
		if batch := h.commit(); batch != nil && len(batch.Items) > 0 {
			t.Fatalf("environment %+v emitted items for a tree that reads none of its signals: %+v", env, batch.Items)
		}
	}
}

// What reads the window's frame, maximized state, lifecycle or title bar through Env follows each report without a full round.
func TestWindowEnvironmentReportsReachTheirReaders(t *testing.T) {
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		e := cx.Env()
		return View{Children: []Node{
			Text{TestID: "frame", ContentFunc: func() string { return fmt.Sprint(e.WindowFrame().X) }},
			Text{TestID: "maximized", ContentFunc: func() string { return fmt.Sprint(e.Maximized()) }},
			Text{TestID: "lifecycle", ContentFunc: func() string { return e.Lifecycle().String() }},
			Text{TestID: "titlebar", ContentFunc: func() string { return fmt.Sprint(e.TitleBar().Top) }},
		}}
	})
	h.commit()
	env := Environment{WindowFrame: WindowFrame{X: 20}, Maximized: true, Lifecycle: LifecycleBackground, TitleBar: Edges{Top: 24}}
	h.tree.SetEnvironment(env)
	if h.tree.full {
		t.Fatal("the window's report forced a full resolve")
	}
	h.commit()
	for id, want := range map[string]string{"frame": "20", "maximized": "true", "lifecycle": LifecycleBackground.String(), "titlebar": "24"} {
		if got := h.find(id).text.content; got != want {
			t.Errorf("%s shows %q, want %q", id, got, want)
		}
	}
}

func TestEnvironmentReportsThatAffectResolutionStayFull(t *testing.T) {
	if !environmentNeedsFullRound(Environment{}, Environment{Dark: true}) {
		t.Fatal("dark mode did not request a full resolve")
	}
	if environmentNeedsFullRound(Environment{}, Environment{WindowFrame: WindowFrame{X: 1}}) {
		t.Fatal("window frame requested a full resolve")
	}
}
