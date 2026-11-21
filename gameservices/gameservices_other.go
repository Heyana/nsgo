//go:build (!darwin || nsgowatch) && !android

package gameservices

func platformAuthenticate(done func(Player, error)) {
	done(Player{}, ErrUnsupported)
}

func platformReportAchievement(_ string, _ float64, done func(error)) {
	done(ErrUnsupported)
}

func platformSubmitScore(_ string, _ int64, done func(error)) {
	done(ErrUnsupported)
}

func platformShowAchievements() {}

func platformShowLeaderboard(_ string) {}

func platformLoadScores(_ string, _ PlayerScope, _ TimeScope, _ int, done func([]ScoreEntry, error)) {
	done(nil, ErrUnsupported)
}
