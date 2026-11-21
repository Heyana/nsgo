// Package gameservices provides a thin, provider-neutral API for platform game
// services: local player authentication, achievements, leaderboards, and native
// Game Center / Play Games UI overlays.
//
// Operations run asynchronously and invoke their callbacks on the engine
// goroutine via core.Scope.Post so frames are never blocked and callbacks can
// safely update reactive signals. Check cx.Env().CanUseGameServices() to adapt
// UI on platforms without a native game service backend (Windows, Linux, the
// web, watchOS and mini programs), where calls answer ErrUnsupported.
package gameservices

import (
	"errors"
	"math"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Types shared across package gameservices, core and nsgotest.
type (
	// Player identifies the local player signed into the platform game service.
	Player = driver.GamePlayer
	// Achievement describes an achievement's reported completion progress (0..100).
	Achievement = driver.GameAchievement
	// ScoreEntry is one ranked leaderboard row returned by LoadScores.
	ScoreEntry = driver.GameScoreEntry
	// TimeScope selects the time window for LoadScores: AllTime, Week or Today.
	TimeScope = driver.GameTimeScope
	// PlayerScope selects which players LoadScores includes: Global or Friends.
	PlayerScope = driver.GamePlayerScope
)

// Leaderboard time scopes.
const (
	AllTime = driver.GameTimeAllTime
	Week    = driver.GameTimeWeek
	Today   = driver.GameTimeToday
)

// Leaderboard player scopes.
const (
	Global  = driver.GamePlayersGlobal
	Friends = driver.GamePlayersFriends
)

// Sentinel errors returned by package gameservices.
var (
	// ErrUnsupported is returned when the platform has no native game service
	// backend (core.Env.CanUseGameServices is false).
	ErrUnsupported = driver.ErrGameServicesUnsupported
	// ErrUnavailable is returned when the platform game service cannot be
	// reached or is not enabled in the current environment.
	ErrUnavailable = driver.ErrGameServicesUnavailable
	// ErrUnauthenticated is returned when an operation requires a signed-in
	// player and none is authenticated.
	ErrUnauthenticated = driver.ErrGameServicesUnauthenticated
	// ErrCancelled is returned when the user dismisses or cancels sign-in.
	ErrCancelled = driver.ErrGameServicesCancelled
)

const (
	defaultScoreLimit = 25
	maxScoreLimit     = 100
)

// clampPercent clamps an achievement completion percentage to [0, 100],
// treating NaN as 0.
func clampPercent(percent float64) float64 {
	if math.IsNaN(percent) || percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}

// normalizeLimit clamps a leaderboard query limit to [1, maxScoreLimit],
// using defaultScoreLimit when limit <= 0.
func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultScoreLimit
	}
	if limit > maxScoreLimit {
		return maxScoreLimit
	}
	return limit
}

// deliver schedules fn on cx's engine goroutine when cx is non-nil.
func deliver(cx *core.Scope, fn func()) {
	if fn == nil {
		return
	}
	if cx == nil {
		fn()
		return
	}
	cx.Post(fn)
}

// canUse reports whether cx's environment supports platform game services.
func canUse(cx *core.Scope) bool {
	return cx != nil && core.Untracked(cx, cx.Env().CanUseGameServices)
}

// Authenticate signs the local player into the platform game service (such as
// Game Center on Apple platforms or Play Games Services on Android) and calls
// done on the engine goroutine with the authenticated Player or an error.
func Authenticate(cx *core.Scope, done func(Player, error)) {
	reply := func(p Player, err error) {
		if done != nil {
			deliver(cx, func() { done(p, err) })
		}
	}
	if !canUse(cx) {
		reply(Player{}, ErrUnsupported)
		return
	}
	if b := core.GameServicesOf(cx); b != nil {
		b.Authenticate(reply)
		return
	}
	platformAuthenticate(reply)
}

// ReportAchievement reports completion progress for achievement id, clamping
// percent to the range 0..100 (100 unlocks the achievement), and calls done on
// the engine goroutine once the platform accepts or rejects the update.
func ReportAchievement(cx *core.Scope, id string, percent float64, done func(error)) {
	reply := func(err error) {
		if done != nil {
			deliver(cx, func() { done(err) })
		}
	}
	if !canUse(cx) {
		reply(ErrUnsupported)
		return
	}
	if id == "" {
		reply(errors.New("gameservices: achievement ID is required"))
		return
	}
	percent = clampPercent(percent)
	if b := core.GameServicesOf(cx); b != nil {
		b.ReportAchievement(id, percent, reply)
		return
	}
	platformReportAchievement(id, percent, reply)
}

// SubmitScore submits score to leaderboardID for the authenticated local
// player and calls done on the engine goroutine with the result.
func SubmitScore(cx *core.Scope, leaderboardID string, score int64, done func(error)) {
	reply := func(err error) {
		if done != nil {
			deliver(cx, func() { done(err) })
		}
	}
	if !canUse(cx) {
		reply(ErrUnsupported)
		return
	}
	if leaderboardID == "" {
		reply(errors.New("gameservices: leaderboard ID is required"))
		return
	}
	if b := core.GameServicesOf(cx); b != nil {
		b.SubmitScore(leaderboardID, score, reply)
		return
	}
	platformSubmitScore(leaderboardID, score, reply)
}

// ShowAchievements presents the platform's native achievements UI overlay when
// supported and authenticated; it does nothing where unsupported.
func ShowAchievements(cx *core.Scope) {
	if !canUse(cx) {
		return
	}
	if b := core.GameServicesOf(cx); b != nil {
		b.ShowAchievements()
		return
	}
	platformShowAchievements()
}

// ShowLeaderboard presents the platform's native leaderboard UI overlay for
// leaderboardID when supported and authenticated; it does nothing when
// unsupported or when leaderboardID is empty.
func ShowLeaderboard(cx *core.Scope, leaderboardID string) {
	if !canUse(cx) || leaderboardID == "" {
		return
	}
	if b := core.GameServicesOf(cx); b != nil {
		b.ShowLeaderboard(leaderboardID)
		return
	}
	platformShowLeaderboard(leaderboardID)
}

// LoadScores loads up to limit ranked entries from leaderboardID filtered by
// scope and timeScope, and calls done on the engine goroutine with the entries
// or an error. Non-positive limits default to 25 and values above 100 are
// clamped to 100.
func LoadScores(cx *core.Scope, leaderboardID string, scope PlayerScope, timeScope TimeScope, limit int, done func([]ScoreEntry, error)) {
	reply := func(entries []ScoreEntry, err error) {
		if done != nil {
			deliver(cx, func() { done(entries, err) })
		}
	}
	if !canUse(cx) {
		reply(nil, ErrUnsupported)
		return
	}
	if leaderboardID == "" {
		reply(nil, errors.New("gameservices: leaderboard ID is required"))
		return
	}
	limit = normalizeLimit(limit)
	if b := core.GameServicesOf(cx); b != nil {
		b.LoadScores(leaderboardID, scope, timeScope, limit, reply)
		return
	}
	platformLoadScores(leaderboardID, scope, timeScope, limit, reply)
}
