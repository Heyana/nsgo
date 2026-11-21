package nsgotest

import (
	"cmp"
	"math"
	"slices"
	"sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// fakeGameServices is a per-app isolated in-memory implementation of
// driver.GameServices used by nsgotest.App so parallel tests never interfere.
type fakeGameServices struct {
	mu                sync.Mutex
	player            core.GamePlayer
	friends           map[string]bool
	opErr             error
	authErr           error
	achievements      map[string]core.GameAchievement
	submittedScores   []core.GameScoreEntry
	leaderboards      map[string][]core.GameScoreEntry
	shownAchievements int
	shownLeaderboards []string
}

func newFakeGameServices() *fakeGameServices {
	return &fakeGameServices{
		friends:      map[string]bool{},
		achievements: map[string]core.GameAchievement{},
		leaderboards: map[string][]core.GameScoreEntry{},
	}
}

func (f *fakeGameServices) Authenticate(done func(driver.GamePlayer, error)) {
	f.mu.Lock()
	var (
		player core.GamePlayer
		err    error
	)
	switch {
	case f.authErr != nil:
		err = f.authErr
	case f.opErr != nil:
		err = f.opErr
	case f.player.Authenticated:
		player = f.player
	case f.player.ID != "":
		f.player.Authenticated = true
		player = f.player
	default:
		err = driver.ErrGameServicesUnauthenticated
	}
	f.mu.Unlock()
	done(player, err)
}

func (f *fakeGameServices) ReportAchievement(id string, percent float64, done func(error)) {
	if math.IsNaN(percent) || percent < 0 {
		percent = 0
	} else if percent > 100 {
		percent = 100
	}
	f.mu.Lock()
	var err error
	switch {
	case f.opErr != nil:
		err = f.opErr
	case !f.player.Authenticated:
		err = driver.ErrGameServicesUnauthenticated
	default:
		prev := f.achievements[id]
		if percent < prev.PercentComplete {
			percent = prev.PercentComplete
		}
		f.achievements[id] = core.GameAchievement{
			ID:              id,
			PercentComplete: percent,
			Unlocked:        prev.Unlocked || percent >= 100,
		}
	}
	f.mu.Unlock()
	done(err)
}

func (f *fakeGameServices) SubmitScore(leaderboardID string, score int64, done func(error)) {
	f.mu.Lock()
	var err error
	switch {
	case f.opErr != nil:
		err = f.opErr
	case !f.player.Authenticated:
		err = driver.ErrGameServicesUnauthenticated
	default:
		entry := core.GameScoreEntry{
			LeaderboardID: leaderboardID,
			PlayerID:      f.player.ID,
			DisplayName:   f.player.DisplayName,
			Score:         score,
		}
		f.submittedScores = append(f.submittedScores, entry)
		entries := f.leaderboards[leaderboardID]
		if idx := slices.IndexFunc(entries, func(e core.GameScoreEntry) bool { return e.PlayerID == f.player.ID }); idx >= 0 {
			if score > entries[idx].Score {
				entries[idx].Score = score
			}
			if f.player.DisplayName != "" {
				entries[idx].DisplayName = f.player.DisplayName
			}
			f.leaderboards[leaderboardID] = entries
		} else {
			f.leaderboards[leaderboardID] = append(entries, entry)
		}
	}
	f.mu.Unlock()
	done(err)
}

func (f *fakeGameServices) ShowAchievements() {
	f.mu.Lock()
	f.shownAchievements++
	f.mu.Unlock()
}

func (f *fakeGameServices) ShowLeaderboard(leaderboardID string) {
	f.mu.Lock()
	f.shownLeaderboards = append(f.shownLeaderboards, leaderboardID)
	f.mu.Unlock()
}

func (f *fakeGameServices) LoadScores(leaderboardID string, scope driver.GamePlayerScope, _ driver.GameTimeScope, limit int, done func([]driver.GameScoreEntry, error)) {
	f.mu.Lock()
	var (
		out []core.GameScoreEntry
		err error
	)
	switch {
	case f.opErr != nil:
		err = f.opErr
	case !f.player.Authenticated:
		err = driver.ErrGameServicesUnauthenticated
	default:
		out = f.rankedLocked(leaderboardID, scope, limit)
	}
	f.mu.Unlock()
	done(out, err)
}

func (f *fakeGameServices) rankedLocked(leaderboardID string, scope driver.GamePlayerScope, limit int) []core.GameScoreEntry {
	src := f.leaderboards[leaderboardID]
	out := make([]core.GameScoreEntry, 0, len(src))
	for _, e := range src {
		if scope == driver.GamePlayersFriends && e.PlayerID != f.player.ID && !f.friends[e.PlayerID] {
			continue
		}
		e.LeaderboardID = leaderboardID
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b core.GameScoreEntry) int {
		if c := cmp.Compare(b.Score, a.Score); c != 0 {
			return c
		}
		return cmp.Compare(a.PlayerID, b.PlayerID)
	})
	for i := range out {
		out[i].Rank = i + 1
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// SetGameServicesPlayer sets the local player for this app's fake game
// services backend. When p.Authenticated is true the player is immediately
// signed in; when p.ID is non-empty and p.Authenticated is false, calling
// gameservices.Authenticate signs the player in; passing a zero GamePlayer
// signs the player out.
func (a *App) SetGameServicesPlayer(p core.GamePlayer) {
	a.gameServices.mu.Lock()
	a.gameServices.player = p
	a.gameServices.mu.Unlock()
}

// SetGameServicesFriends sets the player IDs treated as friends of the local
// player when gameservices.LoadScores is called with Friends scope.
func (a *App) SetGameServicesFriends(playerIDs ...string) {
	a.gameServices.mu.Lock()
	clear(a.gameServices.friends)
	for _, id := range playerIDs {
		a.gameServices.friends[id] = true
	}
	a.gameServices.mu.Unlock()
}

// SetGameServicesError configures an error returned by subsequent game
// services operations on this app (such as gameservices.ErrUnavailable). Pass
// nil to clear the error.
func (a *App) SetGameServicesError(err error) {
	a.gameServices.mu.Lock()
	a.gameServices.opErr = err
	a.gameServices.mu.Unlock()
}

// SetGameServicesAuthError configures an error returned by subsequent
// gameservices.Authenticate calls on this app (such as
// gameservices.ErrCancelled). Pass nil to clear the error.
func (a *App) SetGameServicesAuthError(err error) {
	a.gameServices.mu.Lock()
	a.gameServices.authErr = err
	a.gameServices.mu.Unlock()
}

// SeedLeaderboardScores seeds entries into leaderboardID for this app's fake
// game services backend, replacing any prior entries for that leaderboard.
func (a *App) SeedLeaderboardScores(leaderboardID string, entries ...core.GameScoreEntry) {
	cloned := make([]core.GameScoreEntry, len(entries))
	for i, e := range entries {
		if e.LeaderboardID == "" {
			e.LeaderboardID = leaderboardID
		}
		cloned[i] = e
	}
	a.gameServices.mu.Lock()
	a.gameServices.leaderboards[leaderboardID] = cloned
	a.gameServices.mu.Unlock()
}

// GameServicesAchievements returns the achievements recorded for this app,
// sorted by ID.
func (a *App) GameServicesAchievements() []core.GameAchievement {
	a.gameServices.mu.Lock()
	out := make([]core.GameAchievement, 0, len(a.gameServices.achievements))
	for _, ach := range a.gameServices.achievements {
		out = append(out, ach)
	}
	a.gameServices.mu.Unlock()
	slices.SortFunc(out, func(a, b core.GameAchievement) int { return cmp.Compare(a.ID, b.ID) })
	return out
}

// GameServicesSubmittedScores returns every score submitted by this app via
// gameservices.SubmitScore, oldest first.
func (a *App) GameServicesSubmittedScores() []core.GameScoreEntry {
	a.gameServices.mu.Lock()
	out := slices.Clone(a.gameServices.submittedScores)
	a.gameServices.mu.Unlock()
	return out
}

// GameServicesScores returns the current ranked entries for leaderboardID
// (combining seeded and submitted scores), sorted descending by score.
func (a *App) GameServicesScores(leaderboardID string) []core.GameScoreEntry {
	a.gameServices.mu.Lock()
	out := a.gameServices.rankedLocked(leaderboardID, driver.GamePlayersGlobal, 0)
	a.gameServices.mu.Unlock()
	return out
}

// GameServicesShownAchievements returns how many times this app called
// gameservices.ShowAchievements.
func (a *App) GameServicesShownAchievements() int {
	a.gameServices.mu.Lock()
	n := a.gameServices.shownAchievements
	a.gameServices.mu.Unlock()
	return n
}

// GameServicesShownLeaderboards returns the leaderboard IDs passed to
// gameservices.ShowLeaderboard by this app, oldest first.
func (a *App) GameServicesShownLeaderboards() []string {
	a.gameServices.mu.Lock()
	out := slices.Clone(a.gameServices.shownLeaderboards)
	a.gameServices.mu.Unlock()
	return out
}
