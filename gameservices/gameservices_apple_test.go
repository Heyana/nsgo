//go:build darwin && !nsgowatch

package gameservices

import (
	"testing"

	"github.com/ebitengine/purego/objc"
)

func TestAppleGameKitHelpers(t *testing.T) {
	if !loadGameKit() {
		t.Skip("GameKit.framework not loadable in this environment")
	}

	// NSString <-> Go string round trip (empty, ASCII, Unicode).
	for _, want := range []string{"", "leaderboard.high_score", "玩家 🏆"} {
		ns := newNSString(want)
		if ns == 0 {
			t.Fatalf("newNSString(%q) returned 0", want)
		}
		got := goString(ns)
		ns.Send(sel("release"))
		if got != want {
			t.Fatalf("round-trip %q: got %q", want, got)
		}
	}

	// Nil safety for responds and readPlayer.
	if responds(0, sel("isAuthenticated")) {
		t.Fatal("responds(0, ...) should be false")
	}
	if p := readPlayer(0); p != (Player{}) {
		t.Fatalf("readPlayer(0) = %+v, want zero Player", p)
	}

	// Scope mappings match GameKit constants.
	if gkPlayerScope(Global) != gkPlayerScopeGlobal || gkPlayerScope(Friends) != gkPlayerScopeFriends {
		t.Fatal("unexpected player scope mapping")
	}
	if gkTimeScope(Today) != gkTimeScopeToday || gkTimeScope(Week) != gkTimeScopeWeek || gkTimeScope(AllTime) != gkTimeScopeAllTime {
		t.Fatal("unexpected time scope mapping")
	}

	// Delegate class registers cleanly without crashing.
	if d := gameCenterDelegate(); d == 0 {
		t.Fatal("gameCenterDelegate returned 0")
	}

	// Calling localPlayer and reading its authenticated state must never crash.
	if lp := localPlayer(); lp != 0 {
		_ = readPlayer(lp)
	}
	if nsError(objc.ID(0)) != nil {
		t.Fatal("nsError(0) should be nil")
	}
}
