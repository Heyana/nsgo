package driver

import (
	"errors"
	"sync"
)

// Sentinel errors returned by package gameservices and its platform backends.
var (
	// ErrGameServicesUnsupported is returned on platforms without a native game service backend (Windows, Linux, Web, watchOS, mini programs).
	ErrGameServicesUnsupported = errors.New("gameservices: not supported on this platform")
	// ErrGameServicesUnavailable is returned when the platform game service framework or runtime bridge is not available or could not be reached.
	ErrGameServicesUnavailable = errors.New("gameservices: platform game service is unavailable")
	// ErrGameServicesUnauthenticated is returned when an operation requires a signed-in player and none is authenticated.
	ErrGameServicesUnauthenticated = errors.New("gameservices: player is not authenticated")
	// ErrGameServicesCancelled is returned when the user cancels a platform game service sign-in flow.
	ErrGameServicesCancelled = errors.New("gameservices: sign-in was cancelled")
)

// GamePlayer identifies the local player signed into a platform game service.
type GamePlayer struct {
	ID            string
	DisplayName   string
	Authenticated bool
}

// GameAchievement describes an achievement's reported completion progress.
type GameAchievement struct {
	ID              string
	PercentComplete float64
	Unlocked        bool
}

// GameScoreEntry is one ranked score row returned for a leaderboard query.
type GameScoreEntry struct {
	LeaderboardID string
	PlayerID      string
	DisplayName   string
	Score         int64
	Rank          int
}

// GameTimeScope selects the time window for leaderboard score queries.
type GameTimeScope uint8

const (
	// GameTimeAllTime queries all-time leaderboard scores.
	GameTimeAllTime GameTimeScope = iota
	// GameTimeWeek queries scores recorded in the current week.
	GameTimeWeek
	// GameTimeToday queries scores recorded today.
	GameTimeToday
)

// String returns the scope's name: allTime, week or today.
func (s GameTimeScope) String() string {
	switch s {
	case GameTimeWeek:
		return "week"
	case GameTimeToday:
		return "today"
	}
	return "allTime"
}

// GamePlayerScope selects which players a leaderboard score query includes.
type GamePlayerScope uint8

const (
	// GamePlayersGlobal queries scores across all players.
	GamePlayersGlobal GamePlayerScope = iota
	// GamePlayersFriends queries scores among the local player's friends.
	GamePlayersFriends
)

// String returns the scope's name: global or friends.
func (s GamePlayerScope) String() string {
	if s == GamePlayersFriends {
		return "friends"
	}
	return "global"
}

// GameServices is a per-app platform game services backend, such as nsgotest's
// isolated in-memory fake. Package gameservices validates arguments, clamps
// achievement percentages to 0..100, checks core.Env.CanUseGameServices and
// marshals all callbacks back onto the engine goroutine via core.Scope.Post.
type GameServices interface {
	Authenticate(done func(GamePlayer, error))
	ReportAchievement(id string, percent float64, done func(error))
	SubmitScore(leaderboardID string, score int64, done func(error))
	ShowAchievements()
	ShowLeaderboard(leaderboardID string)
	LoadScores(leaderboardID string, scope GamePlayerScope, timeScope GameTimeScope, limit int, done func([]GameScoreEntry, error))
}

// gameServicesBridge carries JSON commands and reports between package
// gameservices and Android's Java runtime over the backend's pipe, matching
// recordings.go and liveupdates.go.
var gameServicesBridge struct {
	sync.Mutex
	send    func([]byte)
	queue   [][]byte
	hear    func([]byte)
	reports [][]byte
}

// SendGameServices sends package gameservices' msg to the connected Android
// runtime bridge, queuing it until ConnectGameServices connects.
func SendGameServices(msg []byte) {
	gameServicesBridge.Lock()
	send := gameServicesBridge.send
	if send == nil {
		gameServicesBridge.queue = append(gameServicesBridge.queue, msg)
	}
	gameServicesBridge.Unlock()
	if send != nil {
		send(msg)
	}
}

// ConnectGameServices connects the Android backend's sender for package
// gameservices messages, flushing any queued messages first. Passing nil
// disconnects the sender.
func ConnectGameServices(send func([]byte)) {
	gameServicesBridge.Lock()
	var queue [][]byte
	if send != nil {
		queue, gameServicesBridge.queue = gameServicesBridge.queue, nil
	}
	gameServicesBridge.send = send
	gameServicesBridge.Unlock()
	for _, msg := range queue {
		send(msg)
	}
}

// GameServicesConnected reports whether a game services runtime bridge is connected.
func GameServicesConnected() bool {
	gameServicesBridge.Lock()
	defer gameServicesBridge.Unlock()
	return gameServicesBridge.send != nil
}

// HearGameServices registers package gameservices' report handler, delivering
// any reports that arrived before registration first.
func HearGameServices(fn func([]byte)) {
	gameServicesBridge.Lock()
	reports := gameServicesBridge.reports
	gameServicesBridge.hear, gameServicesBridge.reports = fn, nil
	gameServicesBridge.Unlock()
	for _, r := range reports {
		fn(r)
	}
}

// ReportGameServices delivers a report from the Android runtime to package
// gameservices.
func ReportGameServices(report []byte) {
	gameServicesBridge.Lock()
	hear := gameServicesBridge.hear
	if hear == nil {
		gameServicesBridge.reports = append(gameServicesBridge.reports, report)
	}
	gameServicesBridge.Unlock()
	if hear != nil {
		hear(report)
	}
}
