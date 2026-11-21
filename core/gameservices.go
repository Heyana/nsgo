package core

import "github.com/nsgo-dev/nsgo/internal/driver"

// GameServices, GamePlayer, GameAchievement, GameScoreEntry, GameTimeScope and
// GamePlayerScope are the core's part of package gameservices and nsgotest's
// per-app fake, which apps use through package gameservices.
type (
	GameServices    = driver.GameServices
	GamePlayer      = driver.GamePlayer
	GameAchievement = driver.GameAchievement
	GameScoreEntry  = driver.GameScoreEntry
	GameTimeScope   = driver.GameTimeScope
	GamePlayerScope = driver.GamePlayerScope
)

// GameServicesOf returns the per-app game services backend supplied to
// RootOptions.GameServices (such as nsgotest's fake), or nil when none was
// configured so package gameservices uses the platform backend. It is the
// core's part of package gameservices, which apps use.
func GameServicesOf(cx *Scope) GameServices {
	if cx == nil || cx.rt == nil {
		return nil
	}
	return cx.rt.gameServices
}
