package com.google.android.gms.games.leaderboard;

import com.google.android.gms.games.Player;

public final class LeaderboardScore {
    private final long rawScore;
    private final long rank;
    private final String displayName;
    private final Player holder;

    public LeaderboardScore(long rawScore, long rank, String displayName, Player holder) {
        this.rawScore = rawScore;
        this.rank = rank;
        this.displayName = displayName;
        this.holder = holder;
    }

    public long getRawScore() {
        return rawScore;
    }

    public long getRank() {
        return rank;
    }

    public String getScoreHolderDisplayName() {
        return displayName;
    }

    public Player getScoreHolder() {
        return holder;
    }
}
