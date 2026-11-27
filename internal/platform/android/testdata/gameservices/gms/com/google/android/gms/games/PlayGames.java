package com.google.android.gms.games;

import android.app.Activity;

public final class PlayGames {
    public static final GamesSignInClient signInClient = new GamesSignInClient();
    public static final PlayersClient playersClient = new PlayersClient();
    public static final AchievementsClient achievementsClient = new AchievementsClient();
    public static final LeaderboardsClient leaderboardsClient = new LeaderboardsClient();

    public static GamesSignInClient getGamesSignInClient(Activity activity) {
        return signInClient;
    }

    public static PlayersClient getPlayersClient(Activity activity) {
        return playersClient;
    }

    public static AchievementsClient getAchievementsClient(Activity activity) {
        return achievementsClient;
    }

    public static LeaderboardsClient getLeaderboardsClient(Activity activity) {
        return leaderboardsClient;
    }
}
