package gameservices_test

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gameservices"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

var withGameServices = nsgotest.Environment(core.Environment{CanUseGameServices: true})

func scoped(t *testing.T, opts ...nsgotest.Option) (*nsgotest.App, *core.Scope) {
	t.Helper()
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		return core.View{}
	}, opts...)
	return app, scope
}

func TestScopeNames(t *testing.T) {
	if gameservices.AllTime.String() != "allTime" || gameservices.Week.String() != "week" || gameservices.Today.String() != "today" {
		t.Fatalf("unexpected time scopes: %q %q %q", gameservices.AllTime, gameservices.Week, gameservices.Today)
	}
	if gameservices.Global.String() != "global" || gameservices.Friends.String() != "friends" {
		t.Fatalf("unexpected player scopes: %q %q", gameservices.Global, gameservices.Friends)
	}
}

func TestAuthenticateAndSignInFlow(t *testing.T) {
	app, cx := scoped(t, withGameServices)
	if !cx.Env().CanUseGameServices() {
		t.Fatal("CanUseGameServices should be true")
	}

	// No player configured: Authenticate reports ErrUnauthenticated.
	var gotPlayer gameservices.Player
	var gotErr error
	gameservices.Authenticate(cx, func(p gameservices.Player, err error) {
		gotPlayer, gotErr = p, err
	})
	app.Settle()
	if !errors.Is(gotErr, gameservices.ErrUnauthenticated) || gotPlayer.Authenticated {
		t.Fatalf("without player: player=%+v err=%v", gotPlayer, gotErr)
	}

	// Configure a pending player (not yet signed in): operations fail until Authenticate runs.
	app.SetGameServicesPlayer(gameservices.Player{ID: "p-ada", DisplayName: "Ada", Authenticated: false})
	var submitErr error
	gameservices.SubmitScore(cx, "high-score", 100, func(err error) { submitErr = err })
	app.Settle()
	if !errors.Is(submitErr, gameservices.ErrUnauthenticated) {
		t.Fatalf("before Authenticate: err=%v, want ErrUnauthenticated", submitErr)
	}

	// Authenticate signs the player in.
	gameservices.Authenticate(cx, func(p gameservices.Player, err error) {
		gotPlayer, gotErr = p, err
	})
	app.Settle()
	if gotErr != nil || !gotPlayer.Authenticated || gotPlayer.ID != "p-ada" || gotPlayer.DisplayName != "Ada" {
		t.Fatalf("after Authenticate: player=%+v err=%v", gotPlayer, gotErr)
	}

	// Subsequent score submission succeeds.
	gameservices.SubmitScore(cx, "high-score", 100, func(err error) { submitErr = err })
	app.Settle()
	if submitErr != nil {
		t.Fatalf("after Authenticate SubmitScore: %v", submitErr)
	}
}

func TestReportAchievementClampingAndValidation(t *testing.T) {
	app, cx := scoped(t, withGameServices)
	app.SetGameServicesPlayer(gameservices.Player{ID: "p1", DisplayName: "Ada", Authenticated: true})

	var errs []error
	record := func(err error) { errs = append(errs, err) }

	gameservices.ReportAchievement(cx, "first-win", 40, record)
	gameservices.ReportAchievement(cx, "first-win", 20, record) // lower progress does not regress
	gameservices.ReportAchievement(cx, "over-100", 175.5, record)
	gameservices.ReportAchievement(cx, "under-0", -25, record)
	gameservices.ReportAchievement(cx, "nan-val", math.NaN(), record)
	gameservices.ReportAchievement(cx, "exact-unlock", 100, record)
	app.Settle()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
	}

	got := app.GameServicesAchievements()
	want := []gameservices.Achievement{
		{ID: "exact-unlock", PercentComplete: 100, Unlocked: true},
		{ID: "first-win", PercentComplete: 40, Unlocked: false},
		{ID: "nan-val", PercentComplete: 0, Unlocked: false},
		{ID: "over-100", PercentComplete: 100, Unlocked: true},
		{ID: "under-0", PercentComplete: 0, Unlocked: false},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("achievements:\ngot:  %+v\nwant: %+v", got, want)
	}

	// Empty achievement ID is rejected cleanly.
	var emptyErr error
	gameservices.ReportAchievement(cx, "", 50, func(err error) { emptyErr = err })
	app.Settle()
	if emptyErr == nil {
		t.Fatal("expected error for empty achievement ID")
	}
}

func TestSubmitAndLoadScores(t *testing.T) {
	app, cx := scoped(t, withGameServices)
	app.SetGameServicesPlayer(gameservices.Player{ID: "p-ada", DisplayName: "Ada", Authenticated: true})
	app.SetGameServicesFriends("p-grace")
	app.SeedLeaderboardScores("arcade",
		gameservices.ScoreEntry{PlayerID: "p-grace", DisplayName: "Grace", Score: 900},
		gameservices.ScoreEntry{PlayerID: "p-linus", DisplayName: "Linus", Score: 1200},
		gameservices.ScoreEntry{PlayerID: "p-alan", DisplayName: "Alan", Score: 500},
	)

	var submitErr error
	gameservices.SubmitScore(cx, "arcade", 1000, func(err error) { submitErr = err })
	gameservices.SubmitScore(cx, "arcade", 750, func(err error) { submitErr = err })
	app.Settle()
	if submitErr != nil {
		t.Fatalf("SubmitScore: %v", submitErr)
	}

	// Both raw submissions were recorded in order.
	subs := app.GameServicesSubmittedScores()
	if len(subs) != 2 || subs[0].Score != 1000 || subs[1].Score != 750 {
		t.Fatalf("submitted scores: %+v", subs)
	}

	// Global leaderboard with limit 3 returns top 3 ranked descending.
	var global []gameservices.ScoreEntry
	var loadErr error
	gameservices.LoadScores(cx, "arcade", gameservices.Global, gameservices.AllTime, 3, func(entries []gameservices.ScoreEntry, err error) {
		global, loadErr = entries, err
	})
	app.Settle()
	if loadErr != nil {
		t.Fatalf("LoadScores global: %v", loadErr)
	}
	wantGlobal := []gameservices.ScoreEntry{
		{LeaderboardID: "arcade", PlayerID: "p-linus", DisplayName: "Linus", Score: 1200, Rank: 1},
		{LeaderboardID: "arcade", PlayerID: "p-ada", DisplayName: "Ada", Score: 1000, Rank: 2},
		{LeaderboardID: "arcade", PlayerID: "p-grace", DisplayName: "Grace", Score: 900, Rank: 3},
	}
	if !slices.Equal(global, wantGlobal) {
		t.Fatalf("global scores:\ngot:  %+v\nwant: %+v", global, wantGlobal)
	}

	// Friends scope includes only local player and configured friends.
	var friends []gameservices.ScoreEntry
	gameservices.LoadScores(cx, "arcade", gameservices.Friends, gameservices.Week, 0, func(entries []gameservices.ScoreEntry, err error) {
		friends, loadErr = entries, err
	})
	app.Settle()
	if loadErr != nil {
		t.Fatalf("LoadScores friends: %v", loadErr)
	}
	wantFriends := []gameservices.ScoreEntry{
		{LeaderboardID: "arcade", PlayerID: "p-ada", DisplayName: "Ada", Score: 1000, Rank: 1},
		{LeaderboardID: "arcade", PlayerID: "p-grace", DisplayName: "Grace", Score: 900, Rank: 2},
	}
	if !slices.Equal(friends, wantFriends) {
		t.Fatalf("friend scores:\ngot:  %+v\nwant: %+v", friends, wantFriends)
	}

	// Empty leaderboard ID is rejected for both SubmitScore and LoadScores.
	var badSubmit, badLoad error
	gameservices.SubmitScore(cx, "", 10, func(err error) { badSubmit = err })
	gameservices.LoadScores(cx, "", gameservices.Global, gameservices.Today, 10, func(_ []gameservices.ScoreEntry, err error) { badLoad = err })
	app.Settle()
	if badSubmit == nil || badLoad == nil {
		t.Fatalf("expected errors for empty leaderboard ID: submit=%v load=%v", badSubmit, badLoad)
	}
}

func TestShowAchievementsAndShowLeaderboard(t *testing.T) {
	app, cx := scoped(t, withGameServices)
	app.SetGameServicesPlayer(gameservices.Player{ID: "p1", DisplayName: "Ada", Authenticated: true})

	gameservices.ShowAchievements(cx)
	gameservices.ShowAchievements(cx)
	gameservices.ShowLeaderboard(cx, "daily")
	gameservices.ShowLeaderboard(cx, "") // empty ID ignored
	gameservices.ShowLeaderboard(cx, "weekly")

	if got := app.GameServicesShownAchievements(); got != 2 {
		t.Fatalf("shown achievements: got %d, want 2", got)
	}
	if got := app.GameServicesShownLeaderboards(); !slices.Equal(got, []string{"daily", "weekly"}) {
		t.Fatalf("shown leaderboards: got %v, want [daily weekly]", got)
	}
}

func TestUnavailableAndCancelledErrors(t *testing.T) {
	app, cx := scoped(t, withGameServices)
	app.SetGameServicesPlayer(gameservices.Player{ID: "p1", DisplayName: "Ada", Authenticated: true})

	app.SetGameServicesAuthError(gameservices.ErrCancelled)
	var authErr error
	gameservices.Authenticate(cx, func(_ gameservices.Player, err error) { authErr = err })
	app.Settle()
	if !errors.Is(authErr, gameservices.ErrCancelled) {
		t.Fatalf("Authenticate cancelled: got %v, want ErrCancelled", authErr)
	}
	app.SetGameServicesAuthError(nil)

	app.SetGameServicesError(gameservices.ErrUnavailable)
	var achErr, scoreErr, loadErr error
	gameservices.ReportAchievement(cx, "ach-1", 100, func(err error) { achErr = err })
	gameservices.SubmitScore(cx, "lb-1", 42, func(err error) { scoreErr = err })
	gameservices.LoadScores(cx, "lb-1", gameservices.Global, gameservices.AllTime, 5, func(_ []gameservices.ScoreEntry, err error) { loadErr = err })
	app.Settle()
	for name, err := range map[string]error{"achievement": achErr, "score": scoreErr, "load": loadErr} {
		if !errors.Is(err, gameservices.ErrUnavailable) {
			t.Fatalf("%s: got %v, want ErrUnavailable", name, err)
		}
	}
}

func TestUnsupportedPlatformReturnsErrUnsupported(t *testing.T) {
	// Default environment has CanUseGameServices == false (Windows/Linux/Web/watchOS).
	app, cx := scoped(t)
	if cx.Env().CanUseGameServices() {
		t.Fatal("default environment should report CanUseGameServices() == false")
	}

	var authErr, achErr, scoreErr, loadErr error
	gameservices.Authenticate(cx, func(_ gameservices.Player, err error) { authErr = err })
	gameservices.ReportAchievement(cx, "ach-1", 100, func(err error) { achErr = err })
	gameservices.SubmitScore(cx, "lb-1", 500, func(err error) { scoreErr = err })
	gameservices.LoadScores(cx, "lb-1", gameservices.Global, gameservices.AllTime, 10, func(_ []gameservices.ScoreEntry, err error) { loadErr = err })
	gameservices.ShowAchievements(cx)
	gameservices.ShowLeaderboard(cx, "lb-1")
	app.Settle()

	for name, err := range map[string]error{"auth": authErr, "achievement": achErr, "score": scoreErr, "load": loadErr} {
		if !errors.Is(err, gameservices.ErrUnsupported) {
			t.Fatalf("%s: got %v, want ErrUnsupported", name, err)
		}
	}
	if app.GameServicesShownAchievements() != 0 || len(app.GameServicesShownLeaderboards()) != 0 {
		t.Fatalf("unsupported platform recorded UI presentations: achievements=%d leaderboards=%v",
			app.GameServicesShownAchievements(), app.GameServicesShownLeaderboards())
	}
}

func TestCallbacksUpdateReactiveSignals(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		status := core.NewSignal(cx, "signed-out")
		return core.View{
			Children: []core.Node{
				core.Text{TestID: "status", ContentFunc: status.Get},
				core.View{
					TestID: "signin",
					Style:  core.Style{Width: core.Pt(120), Height: core.Pt(40)},
					OnPress: func(core.PressEvent) {
						gameservices.Authenticate(cx, func(p gameservices.Player, err error) {
							if err != nil {
								status.Set("err:" + err.Error())
								return
							}
							status.Set(fmt.Sprintf("hello:%s", p.DisplayName))
						})
					},
				},
			},
		}
	}, withGameServices)

	app.SetGameServicesPlayer(gameservices.Player{ID: "p-ada", DisplayName: "Ada", Authenticated: false})
	app.Find("text:signed-out")
	app.Press("id:signin")
	app.Find("text:hello:Ada")
}

func TestPerAppIsolation(t *testing.T) {
	appA, cxA := scoped(t, withGameServices)
	appB, cxB := scoped(t, withGameServices)

	appA.SetGameServicesPlayer(gameservices.Player{ID: "a", DisplayName: "Alice", Authenticated: true})
	appB.SetGameServicesPlayer(gameservices.Player{ID: "b", DisplayName: "Bob", Authenticated: true})

	gameservices.ReportAchievement(cxA, "only-a", 100, nil)
	gameservices.SubmitScore(cxA, "board", 400, nil)
	gameservices.ShowLeaderboard(cxA, "board")
	gameservices.ReportAchievement(cxB, "only-b", 50, nil)
	gameservices.ShowAchievements(cxB)

	appA.Settle()
	appB.Settle()

	if achs := appA.GameServicesAchievements(); len(achs) != 1 || achs[0].ID != "only-a" {
		t.Fatalf("appA achievements leaked: %+v", achs)
	}
	if achs := appB.GameServicesAchievements(); len(achs) != 1 || achs[0].ID != "only-b" {
		t.Fatalf("appB achievements leaked: %+v", achs)
	}
	if len(appA.GameServicesScores("board")) != 1 || len(appB.GameServicesScores("board")) != 0 {
		t.Fatalf("leaderboard scores leaked: A=%+v B=%+v", appA.GameServicesScores("board"), appB.GameServicesScores("board"))
	}
	if appA.GameServicesShownAchievements() != 0 || appB.GameServicesShownAchievements() != 1 {
		t.Fatalf("shown achievements leaked: A=%d B=%d", appA.GameServicesShownAchievements(), appB.GameServicesShownAchievements())
	}
	if len(appA.GameServicesShownLeaderboards()) != 1 || len(appB.GameServicesShownLeaderboards()) != 0 {
		t.Fatalf("shown leaderboards leaked: A=%v B=%v", appA.GameServicesShownLeaderboards(), appB.GameServicesShownLeaderboards())
	}
}
