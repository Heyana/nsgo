package nsgotest_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestFakeGameServicesIsPerAppIsolated(t *testing.T) {
	var cx1, cx2 *core.Scope
	app1 := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx1 = cx
		return core.View{}
	}, nsgotest.Environment(core.Environment{CanUseGameServices: true}))
	app2 := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx2 = cx
		return core.View{}
	}, nsgotest.Environment(core.Environment{CanUseGameServices: true}))

	b1, b2 := core.GameServicesOf(cx1), core.GameServicesOf(cx2)
	if b1 == nil || b2 == nil || b1 == b2 {
		t.Fatalf("expected distinct per-app backends: b1=%v b2=%v", b1, b2)
	}

	app1.SetGameServicesPlayer(core.GamePlayer{ID: "p1", DisplayName: "One", Authenticated: true})
	app1.SeedLeaderboardScores("lb", core.GameScoreEntry{PlayerID: "p0", DisplayName: "Zero", Score: 250})

	var err1, err2 error
	b1.SubmitScore("lb", 500, func(err error) { err1 = err })
	b2.SubmitScore("lb", 100, func(err error) { err2 = err })
	if err1 != nil {
		t.Fatalf("app1 SubmitScore: %v", err1)
	}
	if err2 == nil {
		t.Fatal("app2 SubmitScore should fail while unauthenticated")
	}

	scores1 := app1.GameServicesScores("lb")
	want := []core.GameScoreEntry{
		{LeaderboardID: "lb", PlayerID: "p1", DisplayName: "One", Score: 500, Rank: 1},
		{LeaderboardID: "lb", PlayerID: "p0", DisplayName: "Zero", Score: 250, Rank: 2},
	}
	if !slices.Equal(scores1, want) {
		t.Fatalf("app1 scores: got %+v, want %+v", scores1, want)
	}
	if len(app2.GameServicesScores("lb")) != 0 {
		t.Fatalf("app2 scores should be empty, got %+v", app2.GameServicesScores("lb"))
	}

	sentinel := errors.New("boom")
	app1.SetGameServicesError(sentinel)
	b1.ReportAchievement("ach", 100, func(err error) { err1 = err })
	if !errors.Is(err1, sentinel) {
		t.Fatalf("expected configured error %v, got %v", sentinel, err1)
	}
}
