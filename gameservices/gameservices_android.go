//go:build android

package gameservices

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// On Android, package gameservices talks to the Java runtime's Play Games
// Services reflection hook over internal/driver's game services channel
// (matching package microphone and package widget). When Play Games Services
// is not linked or the player is not signed in, operations answer
// ErrUnavailable or ErrUnauthenticated cleanly.

type androidCommand struct {
	Op            string  `json:"op"`
	ID            uint64  `json:"id,omitempty"`
	AchievementID string  `json:"achievementId,omitempty"`
	Percent       float64 `json:"percent,omitempty"`
	LeaderboardID string  `json:"leaderboardId,omitempty"`
	Score         int64   `json:"score,omitempty"`
	PlayerScope   string  `json:"playerScope,omitempty"`
	TimeScope     string  `json:"timeScope,omitempty"`
	Limit         int     `json:"limit,omitempty"`
}

type androidReport struct {
	Type      string       `json:"type"` // state, authenticate, result, scores
	ID        uint64       `json:"id"`
	Available bool         `json:"available"`
	Player    *Player      `json:"player,omitempty"`
	Scores    []ScoreEntry `json:"scores,omitempty"`
	Error     string       `json:"error,omitempty"`
	Detail    string       `json:"detail,omitempty"`
}

type androidPending struct {
	onAuth   func(Player, error)
	onResult func(error)
	onScores func([]ScoreEntry, error)
}

var androidBridge struct {
	once      sync.Once
	mu        sync.Mutex
	nextID    uint64
	available bool
	pending   map[uint64]androidPending
}

func listenAndroid() {
	androidBridge.once.Do(func() {
		androidBridge.mu.Lock()
		androidBridge.pending = map[uint64]androidPending{}
		androidBridge.mu.Unlock()
		driver.HearGameServices(hearAndroid)
	})
}

func hearAndroid(data []byte) {
	var r androidReport
	if json.Unmarshal(data, &r) != nil {
		return
	}
	if r.Type == "state" {
		androidBridge.mu.Lock()
		androidBridge.available = r.Available
		androidBridge.mu.Unlock()
		return
	}
	androidBridge.mu.Lock()
	p, ok := androidBridge.pending[r.ID]
	delete(androidBridge.pending, r.ID)
	androidBridge.mu.Unlock()
	if !ok {
		return
	}
	err := androidError(r.Error, r.Detail)
	switch {
	case p.onAuth != nil:
		var player Player
		if r.Player != nil {
			player = *r.Player
		}
		p.onAuth(player, err)
	case p.onResult != nil:
		p.onResult(err)
	case p.onScores != nil:
		p.onScores(r.Scores, err)
	}
}

func androidError(code, detail string) error {
	var base error
	switch code {
	case "":
		return nil
	case "unsupported":
		base = ErrUnsupported
	case "unavailable":
		base = ErrUnavailable
	case "unauthenticated":
		base = ErrUnauthenticated
	case "cancelled":
		base = ErrCancelled
	default:
		if detail != "" {
			return errors.New("gameservices: " + detail)
		}
		return ErrUnavailable
	}
	if detail != "" {
		return fmt.Errorf("%w: %s", base, detail)
	}
	return base
}

func sendAndroid(cmd androidCommand, p *androidPending) bool {
	listenAndroid()
	if !driver.GameServicesConnected() {
		return false
	}
	if p != nil {
		androidBridge.mu.Lock()
		androidBridge.nextID++
		cmd.ID = androidBridge.nextID
		androidBridge.pending[cmd.ID] = *p
		androidBridge.mu.Unlock()
	}
	msg, err := json.Marshal(cmd)
	if err != nil {
		if p != nil {
			androidBridge.mu.Lock()
			delete(androidBridge.pending, cmd.ID)
			androidBridge.mu.Unlock()
		}
		return false
	}
	driver.SendGameServices(msg)
	return true
}

func platformAuthenticate(done func(Player, error)) {
	if !sendAndroid(androidCommand{Op: "authenticate"}, &androidPending{onAuth: done}) {
		done(Player{}, ErrUnavailable)
	}
}

func platformReportAchievement(id string, percent float64, done func(error)) {
	if !sendAndroid(androidCommand{Op: "reportAchievement", AchievementID: id, Percent: percent}, &androidPending{onResult: done}) {
		done(ErrUnavailable)
	}
}

func platformSubmitScore(leaderboardID string, score int64, done func(error)) {
	if !sendAndroid(androidCommand{Op: "submitScore", LeaderboardID: leaderboardID, Score: score}, &androidPending{onResult: done}) {
		done(ErrUnavailable)
	}
}

func platformShowAchievements() {
	sendAndroid(androidCommand{Op: "showAchievements"}, nil)
}

func platformShowLeaderboard(leaderboardID string) {
	sendAndroid(androidCommand{Op: "showLeaderboard", LeaderboardID: leaderboardID}, nil)
}

func platformLoadScores(leaderboardID string, scope PlayerScope, timeScope TimeScope, limit int, done func([]ScoreEntry, error)) {
	cmd := androidCommand{
		Op:            "loadScores",
		LeaderboardID: leaderboardID,
		PlayerScope:   scope.String(),
		TimeScope:     timeScope.String(),
		Limit:         limit,
	}
	if !sendAndroid(cmd, &androidPending{onScores: done}) {
		done(nil, ErrUnavailable)
	}
}
