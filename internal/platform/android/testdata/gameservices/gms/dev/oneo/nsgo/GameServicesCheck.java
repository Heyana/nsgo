package dev.oneo.nsgo;

import android.app.Activity;
import com.google.android.gms.common.api.ApiException;
import com.google.android.gms.games.AnnotatedData;
import com.google.android.gms.games.AuthenticationResult;
import com.google.android.gms.games.LeaderboardsClient;
import com.google.android.gms.games.PlayGames;
import com.google.android.gms.games.PlayGamesSdk;
import com.google.android.gms.games.Player;
import com.google.android.gms.games.leaderboard.LeaderboardScore;
import com.google.android.gms.games.leaderboard.LeaderboardScoreBuffer;
import com.google.android.gms.tasks.Task;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

/** Exercises GameServices against fake Play Games Services v2 classes via reflection. */
public final class GameServicesCheck {
    private static void require(boolean cond, String msg) {
        if (!cond) throw new AssertionError(msg);
    }

    private static void send(Activity activity, String json, List<String> out) {
        GameServices.handle(activity, json.getBytes(StandardCharsets.UTF_8), bytes -> out.add(new String(bytes, StandardCharsets.UTF_8)));
    }

    public static void main(String[] args) {
        require(GameServices.isSdkPresent(), "SDK should be detected when PlayGames is on the classpath");
        Activity activity = new Activity();
        List<String> reports = new ArrayList<>();

        // 1. probeState signed-out then signed-in.
        PlayGames.signInClient.isAuthenticatedTask = Task.forResult(new AuthenticationResult(false));
        GameServices.probeState(activity, bytes -> reports.add(new String(bytes, StandardCharsets.UTF_8)));
        require(!GameServices.signedIn, "signedIn should be false after signed-out probe");
        require(PlayGamesSdk.initCount == 1, "PlayGamesSdk.initialize should run once, got " + PlayGamesSdk.initCount);

        PlayGames.signInClient.isAuthenticatedTask = Task.forResult(new AuthenticationResult(true));
        GameServices.probeState(activity, bytes -> reports.add(new String(bytes, StandardCharsets.UTF_8)));
        require(GameServices.signedIn, "signedIn should be true after signed-in probe");

        // 2. authenticate success.
        PlayGames.signInClient.signInTask = Task.forResult(new AuthenticationResult(true));
        PlayGames.playersClient.currentPlayerTask = Task.forResult(new Player("p-ada", "Ada"));
        send(activity, "{\"op\":\"authenticate\",\"id\":1}", reports);

        // 3. authenticate not signed in -> unauthenticated.
        PlayGames.signInClient.signInTask = Task.forResult(new AuthenticationResult(false));
        send(activity, "{\"op\":\"authenticate\",\"id\":2}", reports);
        require(!GameServices.signedIn, "signedIn should clear when sign-in returns false");

        // 4. authenticate canceled task -> cancelled.
        PlayGames.signInClient.signInTask = Task.forCanceled();
        send(activity, "{\"op\":\"authenticate\",\"id\":3}", reports);

        // 5. authenticate ApiException(CANCELED=16) -> cancelled.
        PlayGames.signInClient.signInTask = Task.forException(new ApiException(16, "user dismissed"));
        send(activity, "{\"op\":\"authenticate\",\"id\":4}", reports);

        // 6. reportAchievement: unlock at 100%, incremental steps at 42.5% and 99.9%, 0% when signed in and out, and SIGN_IN_REQUIRED error.
        send(activity, "{\"op\":\"reportAchievement\",\"id\":5,\"achievementId\":\"ach-unlock\",\"percent\":100}", reports);
        send(activity, "{\"op\":\"reportAchievement\",\"id\":6,\"achievementId\":\"ach-inc\",\"percent\":42.5}", reports);
        send(activity, "{\"op\":\"reportAchievement\",\"id\":7,\"achievementId\":\"ach-almost\",\"percent\":99.9}", reports);
        PlayGames.signInClient.isAuthenticatedTask = Task.forResult(new AuthenticationResult(true));
        send(activity, "{\"op\":\"reportAchievement\",\"id\":8,\"achievementId\":\"ach-zero\",\"percent\":0}", reports);
        PlayGames.signInClient.isAuthenticatedTask = Task.forResult(new AuthenticationResult(false));
        send(activity, "{\"op\":\"reportAchievement\",\"id\":9,\"achievementId\":\"ach-zero-out\",\"percent\":0}", reports);
        PlayGames.achievementsClient.unlockTask = Task.forException(new ApiException(4, "sign in required"));
        send(activity, "{\"op\":\"reportAchievement\",\"id\":10,\"achievementId\":\"ach-fail\",\"percent\":100}", reports);
        PlayGames.achievementsClient.unlockTask = Task.forResult(null);

        require(PlayGames.achievementsClient.unlocked.equals(Arrays.asList("ach-unlock", "ach-fail")),
                "unexpected unlocked calls: " + PlayGames.achievementsClient.unlocked);
        require(PlayGames.achievementsClient.stepsCalls.equals(Arrays.asList("ach-inc:43", "ach-almost:99")),
                "unexpected setStepsImmediate calls: " + PlayGames.achievementsClient.stepsCalls);

        // 7. submitScore success and failure.
        send(activity, "{\"op\":\"submitScore\",\"id\":11,\"leaderboardId\":\"arcade\",\"score\":1250}", reports);
        PlayGames.leaderboardsClient.submitTask = Task.forException(new ApiException(26502, "reconnect"));
        send(activity, "{\"op\":\"submitScore\",\"id\":12,\"leaderboardId\":\"arcade\",\"score\":500}", reports);
        PlayGames.leaderboardsClient.submitTask = Task.forResult(new Object());
        require(PlayGames.leaderboardsClient.submitted.equals(Arrays.asList("arcade:1250", "arcade:500")),
                "unexpected submitted scores: " + PlayGames.leaderboardsClient.submitted);

        // 8. loadScores with global/allTime (clamping limit 50 -> 25), friends/week, and today + failure.
        LeaderboardsClient.LeaderboardScores firstBatch = new LeaderboardsClient.LeaderboardScores(
                new LeaderboardScoreBuffer(Arrays.asList(
                        new LeaderboardScore(1200, 1, "Linus", new Player("p-linus", "Linus")),
                        new LeaderboardScore(1000, 2, "", new Player("p-ada", "Ada")),
                        new LeaderboardScore(800, -1, "Anon", null))));
        PlayGames.leaderboardsClient.loadTopTask = Task.forResult(new AnnotatedData<>(firstBatch));
        send(activity, "{\"op\":\"loadScores\",\"id\":13,\"leaderboardId\":\"arcade\",\"playerScope\":\"global\",\"timeScope\":\"allTime\",\"limit\":50}", reports);
        require(firstBatch.released, "LeaderboardScores must be released after reading");

        LeaderboardsClient.LeaderboardScores friendBatch = new LeaderboardsClient.LeaderboardScores(
                new LeaderboardScoreBuffer(Arrays.asList(
                        new LeaderboardScore(900, 1, "Grace", new Player("p-grace", "Grace")))));
        PlayGames.leaderboardsClient.loadTopTask = Task.forResult(new AnnotatedData<>(friendBatch));
        send(activity, "{\"op\":\"loadScores\",\"id\":14,\"leaderboardId\":\"arcade\",\"playerScope\":\"friends\",\"timeScope\":\"week\",\"limit\":10}", reports);
        require(friendBatch.released, "friends LeaderboardScores must be released");

        PlayGames.leaderboardsClient.loadTopTask = Task.forException(new ApiException(17, "api not connected"));
        send(activity, "{\"op\":\"loadScores\",\"id\":15,\"leaderboardId\":\"arcade\",\"playerScope\":\"global\",\"timeScope\":\"today\",\"limit\":5}", reports);

        require(PlayGames.leaderboardsClient.loadQueries.equals(Arrays.asList(
                "arcade:2:0:25",
                "arcade:1:3:10",
                "arcade:0:0:5")),
                "unexpected loadTopScores queries: " + PlayGames.leaderboardsClient.loadQueries);

        // 9. showAchievements and showLeaderboard launch intents, and skip when finishing.
        send(activity, "{\"op\":\"showAchievements\"}", reports);
        send(activity, "{\"op\":\"showLeaderboard\",\"leaderboardId\":\"arcade\"}", reports);
        activity.finishing = true;
        send(activity, "{\"op\":\"showAchievements\"}", reports);
        require(activity.launchedActions.equals(Arrays.asList("play.games.ACHIEVEMENTS", "play.games.LEADERBOARD")),
                "unexpected launched intents: " + activity.launchedActions);
        require(activity.launchedCodes.equals(Arrays.asList(GameServices.ACHIEVEMENTS_REQUEST, GameServices.LEADERBOARD_REQUEST)),
                "unexpected request codes: " + activity.launchedCodes);

        // Ensure SDK initialization still ran only once across all commands.
        require(PlayGamesSdk.initCount == 1, "PlayGamesSdk.initialize ran more than once: " + PlayGamesSdk.initCount);

        for (String line : reports) {
            System.out.println(line);
        }
    }
}
