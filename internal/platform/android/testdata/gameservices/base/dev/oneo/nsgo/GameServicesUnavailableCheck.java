package dev.oneo.nsgo;

import android.app.Activity;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;

/** Exercises GameServices when Play Games Services v2 is absent from the classpath. */
public final class GameServicesUnavailableCheck {
    private static void require(boolean cond, String msg) {
        if (!cond) throw new AssertionError(msg);
    }

    public static void main(String[] args) {
        require(!GameServices.isSdkPresent(), "SDK must be reported absent without play-services-games-v2");
        Activity activity = new Activity();
        List<String> reports = new ArrayList<>();
        GameServices.Reporter reporter = bytes -> reports.add(new String(bytes, StandardCharsets.UTF_8));

        GameServices.probeState(activity, reporter);
        require(reports.size() == 1, "probeState should emit one report");
        require(!GameServices.signedIn, "signedIn must stay false");
        System.out.println(reports.get(0));

        reports.clear();
        String[] commands = {
            "{\"op\":\"authenticate\",\"id\":1}",
            "{\"op\":\"reportAchievement\",\"id\":2,\"achievementId\":\"ach-1\",\"percent\":100}",
            "{\"op\":\"submitScore\",\"id\":3,\"leaderboardId\":\"lb-1\",\"score\":500}",
            "{\"op\":\"loadScores\",\"id\":4,\"leaderboardId\":\"lb-1\",\"playerScope\":\"global\",\"timeScope\":\"allTime\",\"limit\":10}",
            "{\"op\":\"showAchievements\"}",
            "{\"op\":\"showLeaderboard\",\"leaderboardId\":\"lb-1\"}"
        };
        for (String cmd : commands) {
            GameServices.handle(activity, cmd.getBytes(StandardCharsets.UTF_8), reporter);
        }
        require(reports.size() == 4, "expected 4 unavailable replies (fire-and-forget UI ops emit none), got " + reports.size());
        for (String r : reports) {
            System.out.println(r);
        }
        require(activity.launchedActions.isEmpty(), "UI intents must not launch without SDK");
    }
}
