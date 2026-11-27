package android

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func TestGameServicesEnvironmentFlag(t *testing.T) {
	for _, tc := range []struct {
		name string
		more byte
		want bool
	}{
		{"absent", 0, false},
		{"cloud only", 128, false},
		{"game services only", 64, true},
		{"game services and cloud", 64 | 128, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var w writer
			for range 7 {
				w.points(0)
			}
			w.byte(0)
			w.byte(tc.more)
			env := readEnvironment(&reader{buf: w.buf})
			if env.CanUseGameServices != tc.want {
				t.Fatalf("more=%08b decoded CanUseGameServices=%v, want %v", tc.more, env.CanUseGameServices, tc.want)
			}
		})
	}

	testhost.RequireSource(t)
	view, err := os.ReadFile(filepath.Join("java", "dev", "oneo", "nsgo", "NsgoView.java"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(view)
	for _, want := range []string{
		"(GameServices.isSdkPresent() ? 64 : 0)",
		"GameServices.probeState(activity(),",
		"GameServices.handle(activity(),",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("NsgoView.java missing expected GameServices wiring %q", want)
		}
	}
}

func TestGameServicesJava(t *testing.T) {
	javac, java := androidJDK(t)
	baseDir := t.TempDir()
	gmsDir := t.TempDir()

	collect := func(root string, extra ...string) []string {
		t.Helper()
		files := append([]string(nil), extra...)
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err == nil && !entry.IsDir() && filepath.Ext(path) == ".java" {
				files = append(files, path)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return files
	}

	// Compile GameServices.java against Android + org.json stubs only (no Play Games SDK on classpath).
	baseSources := collect(filepath.Join("testdata", "gameservices", "base"), filepath.Join("java", "dev", "oneo", "nsgo", "GameServices.java"))
	if out, err := exec.CommandContext(t.Context(), javac, append([]string{"-d", baseDir}, baseSources...)...).CombinedOutput(); err != nil {
		t.Fatalf("javac base: %v\n%s", err, out)
	}

	// 1. Run without Play Games Services classes on the classpath: every call answers unavailable cleanly.
	out, err := exec.CommandContext(t.Context(), java, "-cp", baseDir, "dev.oneo.nsgo.GameServicesUnavailableCheck").CombinedOutput()
	if err != nil {
		t.Fatalf("GameServicesUnavailableCheck: %v\n%s", err, out)
	}
	type report struct {
		Type      string                  `json:"type"`
		ID        uint64                  `json:"id"`
		Available bool                    `json:"available"`
		Player    *driver.GamePlayer      `json:"player"`
		Scores    []driver.GameScoreEntry `json:"scores"`
		Error     string                  `json:"error"`
		Detail    string                  `json:"detail"`
	}
	parseReports := func(raw []byte) []report {
		t.Helper()
		var list []report
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var r report
			if err := json.Unmarshal([]byte(line), &r); err != nil {
				t.Fatalf("unmarshal report %q: %v", line, err)
			}
			list = append(list, r)
		}
		return list
	}

	unavail := parseReports(out)
	if len(unavail) != 5 {
		t.Fatalf("unavailable reports = %d (%+v), want 5", len(unavail), unavail)
	}
	if unavail[0].Type != "state" || unavail[0].Available {
		t.Fatalf("unavailable probeState = %+v, want state available=false", unavail[0])
	}
	wantTypes := []string{"authenticate", "result", "result", "scores"}
	for i, wantType := range wantTypes {
		r := unavail[i+1]
		if r.Type != wantType || r.ID != uint64(i+1) || r.Error != "unavailable" {
			t.Errorf("unavailable reply %d = %+v, want type=%q id=%d error=unavailable", i+1, r, wantType, i+1)
		}
	}

	// 2. Compile fake Play Games Services v2 classes separately and run with both directories on the classpath.
	gmsSources := collect(filepath.Join("testdata", "gameservices", "gms"))
	if out, err := exec.CommandContext(t.Context(), javac, append([]string{"-cp", baseDir, "-d", gmsDir}, gmsSources...)...).CombinedOutput(); err != nil {
		t.Fatalf("javac gms: %v\n%s", err, out)
	}
	cp := baseDir + string(os.PathListSeparator) + gmsDir
	out, err = exec.CommandContext(t.Context(), java, "-cp", cp, "dev.oneo.nsgo.GameServicesCheck").CombinedOutput()
	if err != nil {
		t.Fatalf("GameServicesCheck: %v\n%s", err, out)
	}
	reports := parseReports(out)
	byID := map[uint64]report{}
	var states []bool
	for _, r := range reports {
		if r.Type == "state" {
			states = append(states, r.Available)
			continue
		}
		byID[r.ID] = r
	}
	// Probes (false, true) + authenticate transitions (true, false, false, false).
	if !slices.Equal(states, []bool{false, true, true, false, false, false}) {
		t.Fatalf("state reports = %v, want [false true true false false false]", states)
	}
	// ID 1: authenticate success.
	if r := byID[1]; r.Type != "authenticate" || r.Error != "" || r.Player == nil || *r.Player != (driver.GamePlayer{ID: "p-ada", DisplayName: "Ada", Authenticated: true}) {
		t.Fatalf("authenticate success = %+v", r)
	}
	// ID 2: authenticate unauthenticated.
	if r := byID[2]; r.Type != "authenticate" || r.Error != "unauthenticated" {
		t.Fatalf("authenticate unauthenticated = %+v", r)
	}
	// ID 3 & 4: authenticate canceled task & ApiException(CANCELED).
	if r := byID[3]; r.Error != "cancelled" {
		t.Fatalf("authenticate canceled task = %+v", r)
	}
	if r := byID[4]; r.Error != "cancelled" || r.Detail != "user dismissed" {
		t.Fatalf("authenticate ApiException(16) = %+v", r)
	}
	// ID 5..8: reportAchievement ok (100%, 42.5%, 99.9%, 0% signed in).
	for _, id := range []uint64{5, 6, 7, 8} {
		if r := byID[id]; r.Type != "result" || r.Error != "" {
			t.Fatalf("reportAchievement id=%d = %+v, want ok", id, r)
		}
	}
	// ID 9 & 10: reportAchievement unauthenticated (0% signed out, SIGN_IN_REQUIRED).
	if r := byID[9]; r.Error != "unauthenticated" {
		t.Fatalf("reportAchievement 0%% signed out = %+v", r)
	}
	if r := byID[10]; r.Error != "unauthenticated" || r.Detail != "sign in required" {
		t.Fatalf("reportAchievement SIGN_IN_REQUIRED = %+v", r)
	}
	// ID 11 & 12: submitScore ok and CLIENT_RECONNECT_REQUIRED -> unauthenticated.
	if r := byID[11]; r.Type != "result" || r.Error != "" {
		t.Fatalf("submitScore ok = %+v", r)
	}
	if r := byID[12]; r.Error != "unauthenticated" || r.Detail != "reconnect" {
		t.Fatalf("submitScore reconnect = %+v", r)
	}
	// ID 13: loadScores global/allTime with fallback display name & null score holder.
	wantScores := []driver.GameScoreEntry{
		{LeaderboardID: "arcade", PlayerID: "p-linus", DisplayName: "Linus", Score: 1200, Rank: 1},
		{LeaderboardID: "arcade", PlayerID: "p-ada", DisplayName: "Ada", Score: 1000, Rank: 2},
		{LeaderboardID: "arcade", PlayerID: "", DisplayName: "Anon", Score: 800, Rank: 0},
	}
	if r := byID[13]; r.Type != "scores" || r.Error != "" || !slices.Equal(r.Scores, wantScores) {
		t.Fatalf("loadScores global = %+v, want %+v", r, wantScores)
	}
	// ID 14: loadScores friends/week.
	wantFriends := []driver.GameScoreEntry{
		{LeaderboardID: "arcade", PlayerID: "p-grace", DisplayName: "Grace", Score: 900, Rank: 1},
	}
	if r := byID[14]; r.Type != "scores" || r.Error != "" || !slices.Equal(r.Scores, wantFriends) {
		t.Fatalf("loadScores friends = %+v, want %+v", r, wantFriends)
	}
	// ID 15: loadScores ApiException(17) -> unavailable with detail.
	if r := byID[15]; r.Type != "scores" || r.Error != "unavailable" || r.Detail != "api not connected" {
		t.Fatalf("loadScores failure = %+v", r)
	}
}
