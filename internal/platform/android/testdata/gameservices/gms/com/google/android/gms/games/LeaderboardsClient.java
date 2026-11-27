package com.google.android.gms.games;

import android.content.Intent;
import com.google.android.gms.games.leaderboard.LeaderboardScoreBuffer;
import com.google.android.gms.tasks.Task;
import java.util.ArrayList;
import java.util.List;

public final class LeaderboardsClient {
    public static final class LeaderboardScores {
        private final LeaderboardScoreBuffer scores;
        public boolean released;

        public LeaderboardScores(LeaderboardScoreBuffer scores) {
            this.scores = scores;
        }

        public LeaderboardScoreBuffer getScores() {
            return scores;
        }

        public void release() {
            released = true;
        }
    }

    public final List<String> submitted = new ArrayList<>();
    public final List<String> loadQueries = new ArrayList<>();
    public Task<Object> submitTask = Task.forResult(new Object());
    public Task<AnnotatedData<LeaderboardScores>> loadTopTask;
    public Task<Intent> intentTask = Task.forResult(new Intent("play.games.LEADERBOARD"));

    public Task<Object> submitScoreImmediate(String leaderboardId, long score) {
        submitted.add(leaderboardId + ":" + score);
        return submitTask;
    }

    public Task<AnnotatedData<LeaderboardScores>> loadTopScores(
            String leaderboardId, int span, int collection, int maxResults) {
        loadQueries.add(leaderboardId + ":" + span + ":" + collection + ":" + maxResults);
        return loadTopTask;
    }

    public Task<Intent> getLeaderboardIntent(String leaderboardId) {
        return intentTask;
    }
}
