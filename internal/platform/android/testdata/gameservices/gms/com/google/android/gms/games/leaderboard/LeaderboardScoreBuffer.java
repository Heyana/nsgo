package com.google.android.gms.games.leaderboard;

import java.util.List;

public final class LeaderboardScoreBuffer {
    private final List<LeaderboardScore> items;

    public LeaderboardScoreBuffer(List<LeaderboardScore> items) {
        this.items = items;
    }

    public int getCount() {
        return items.size();
    }

    public LeaderboardScore get(int index) {
        return items.get(index);
    }
}
