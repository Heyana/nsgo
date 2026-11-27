package dev.oneo.nsgo;

import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;
import java.lang.reflect.Proxy;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.atomic.AtomicBoolean;
import org.json.JSONArray;
import org.json.JSONObject;

/** Reflection bridge for Google Play Games Services v2, backing package gameservices on Android without adding a compile-time Play Services dependency to the NSGo runtime. When Play Games Services is not on the classpath, every command answers "unavailable" cleanly and bit 64 of the second capability byte stays clear. */
final class GameServices {
    private GameServices() {}

    interface Reporter {
        void report(byte[] json);
    }

    static final int ACHIEVEMENTS_REQUEST = 0x4e47;
    static final int LEADERBOARD_REQUEST = 0x4e48;

    private static final String PLAY_GAMES_CLASS = "com.google.android.gms.games.PlayGames";
    private static final String PLAY_GAMES_SDK_CLASS = "com.google.android.gms.games.PlayGamesSdk";
    private static final String LEADERBOARD_VARIANT_CLASS =
            "com.google.android.gms.games.leaderboard.LeaderboardVariant";
    private static final String ON_COMPLETE_LISTENER_CLASS =
            "com.google.android.gms.tasks.OnCompleteListener";

    private static final int MAX_SCORES_PER_PAGE = 25;

    // CommonStatusCodes / GamesClientStatusCodes values mapped to package gameservices errors.
    private static final int STATUS_SIGN_IN_REQUIRED = 4;
    private static final int STATUS_CANCELED = 16;
    private static final int STATUS_CLIENT_RECONNECT_REQUIRED = 26502;

    // LeaderboardVariant fallback constants when LeaderboardVariant cannot be read reflectively.
    private static final int TIME_SPAN_DAILY = 0;
    private static final int TIME_SPAN_WEEKLY = 1;
    private static final int TIME_SPAN_ALL_TIME = 2;
    private static final int COLLECTION_PUBLIC = 0;
    private static final int COLLECTION_FRIENDS = 3;

    /** Whether the last authentication check or sign-in found a signed-in Play Games player. */
    static volatile boolean signedIn;

    private static final AtomicBoolean initialized = new AtomicBoolean();

    /** Reports whether the Play Games Services v2 runtime classes are present on the classpath. */
    static boolean isSdkPresent() {
        try {
            Class.forName(PLAY_GAMES_CLASS);
            return true;
        } catch (Throwable ignored) {
            return false;
        }
    }

    /** Probes Play Games sign-in status and reports {"type":"state","available":...}. */
    static void probeState(Activity activity, Reporter reporter) {
        if (activity == null || !isSdkPresent()) {
            reportState(reporter, false);
            return;
        }
        ensureInitialized(activity);
        try {
            Object signInClient = getClient("getGamesSignInClient", activity);
            if (signInClient == null) {
                reportState(reporter, false);
                return;
            }
            Object task = signInClient.getClass().getMethod("isAuthenticated").invoke(signInClient);
            addCompleteListener(task, completed -> {
                try {
                    boolean authed = isTaskSuccessful(completed) && isAuthenticatedResult(taskResult(completed));
                    reportState(reporter, authed);
                } catch (Throwable ignored) {
                    reportState(reporter, false);
                }
            });
        } catch (Throwable ignored) {
            reportState(reporter, false);
        }
    }

    /** Dispatches one EXT_GAME_SERVICES JSON command from package gameservices. */
    static void handle(Activity activity, byte[] payload, Reporter reporter) {
        long id = 0;
        String op = "";
        try {
            JSONObject cmd = new JSONObject(new String(payload, StandardCharsets.UTF_8));
            id = cmd.optLong("id");
            op = cmd.optString("op");
            if (activity == null || !isSdkPresent()) {
                if (id != 0) replyError(reporter, id, replyTypeFor(op), "unavailable", "");
                return;
            }
            ensureInitialized(activity);
            switch (op) {
                case "authenticate":
                    authenticate(activity, id, reporter);
                    break;
                case "reportAchievement":
                    reportAchievement(
                            activity,
                            id,
                            cmd.optString("achievementId"),
                            cmd.optDouble("percent", 0),
                            reporter);
                    break;
                case "submitScore":
                    submitScore(activity, id, cmd.optString("leaderboardId"), cmd.optLong("score"), reporter);
                    break;
                case "loadScores":
                    loadScores(
                            activity,
                            id,
                            cmd.optString("leaderboardId"),
                            cmd.optString("playerScope"),
                            cmd.optString("timeScope"),
                            cmd.optInt("limit", MAX_SCORES_PER_PAGE),
                            reporter);
                    break;
                case "showAchievements":
                    showAchievements(activity);
                    break;
                case "showLeaderboard":
                    showLeaderboard(activity, cmd.optString("leaderboardId"));
                    break;
                default:
                    if (id != 0) replyError(reporter, id, "result", "unavailable", "unsupported op: " + op);
                    break;
            }
        } catch (Throwable t) {
            if (id != 0) {
                replyError(reporter, id, replyTypeFor(op), errorCodeFor(t), messageOf(t));
            }
        }
    }

    private static void ensureInitialized(Activity activity) {
        if (activity == null || !initialized.compareAndSet(false, true)) {
            return;
        }
        try {
            Context appContext = activity.getApplicationContext();
            Context ctx = appContext != null ? appContext : activity;
            Class<?> sdk = Class.forName(PLAY_GAMES_SDK_CLASS);
            sdk.getMethod("initialize", Context.class).invoke(null, ctx);
        } catch (Throwable ignored) {}
    }

    private static Object getClient(String methodName, Activity activity) {
        try {
            Class<?> playGames = Class.forName(PLAY_GAMES_CLASS);
            Method method = playGames.getMethod(methodName, Activity.class);
            return method.invoke(null, activity);
        } catch (Throwable ignored) {
            return null;
        }
    }

    private static void authenticate(Activity activity, long id, Reporter reporter) throws Exception {
        Object signInClient = getClient("getGamesSignInClient", activity);
        if (signInClient == null) {
            replyError(reporter, id, "authenticate", "unavailable", "");
            return;
        }
        Object task = signInClient.getClass().getMethod("signIn").invoke(signInClient);
        addCompleteListener(task, completed -> {
            try {
                if (!isTaskSuccessful(completed)) {
                    reportState(reporter, false);
                    replyTaskError(reporter, id, "authenticate", completed);
                    return;
                }
                if (!isAuthenticatedResult(taskResult(completed))) {
                    reportState(reporter, false);
                    replyError(reporter, id, "authenticate", "unauthenticated", "");
                    return;
                }
                reportState(reporter, true);
                Object playersClient = getClient("getPlayersClient", activity);
                if (playersClient == null) {
                    replyError(reporter, id, "authenticate", "unavailable", "");
                    return;
                }
                Object playerTask = playersClient.getClass().getMethod("getCurrentPlayer").invoke(playersClient);
                addCompleteListener(playerTask, playerDone -> {
                    try {
                        if (!isTaskSuccessful(playerDone)) {
                            replyTaskError(reporter, id, "authenticate", playerDone);
                            return;
                        }
                        Object player = taskResult(playerDone);
                        if (player == null) {
                            replyError(reporter, id, "authenticate", "unauthenticated", "");
                            return;
                        }
                        String playerId = (String) player.getClass().getMethod("getPlayerId").invoke(player);
                        String displayName = (String) player.getClass().getMethod("getDisplayName").invoke(player);
                        JSONObject p = new JSONObject();
                        p.put("id", playerId != null ? playerId : "");
                        p.put("displayName", displayName != null ? displayName : "");
                        p.put("authenticated", true);
                        JSONObject r = new JSONObject();
                        r.put("type", "authenticate");
                        r.put("id", id);
                        r.put("player", p);
                        reporter.report(r.toString().getBytes(StandardCharsets.UTF_8));
                    } catch (Throwable t) {
                        replyError(reporter, id, "authenticate", errorCodeFor(t), messageOf(t));
                    }
                });
            } catch (Throwable t) {
                replyError(reporter, id, "authenticate", errorCodeFor(t), messageOf(t));
            }
        });
    }

    private static void reportAchievement(
            Activity activity, long id, String achievementId, double percent, Reporter reporter) throws Exception {
        if (achievementId == null || achievementId.isEmpty()) {
            replyError(reporter, id, "result", "failed", "achievement ID is required");
            return;
        }
        if (Double.isNaN(percent) || percent < 0) {
            percent = 0;
        } else if (percent > 100) {
            percent = 100;
        }
        if (percent <= 0) {
            // Play Games rejects zero-step updates; verify sign-in so unauthenticated callers still see ErrUnauthenticated.
            Object signInClient = getClient("getGamesSignInClient", activity);
            if (signInClient == null) {
                replyError(reporter, id, "result", "unavailable", "");
                return;
            }
            Object task = signInClient.getClass().getMethod("isAuthenticated").invoke(signInClient);
            addCompleteListener(task, completed -> {
                try {
                    if (!isTaskSuccessful(completed)) {
                        replyTaskError(reporter, id, "result", completed);
                        return;
                    }
                    if (!isAuthenticatedResult(taskResult(completed))) {
                        signedIn = false;
                        replyError(reporter, id, "result", "unauthenticated", "");
                        return;
                    }
                    replyOk(reporter, id);
                } catch (Throwable t) {
                    replyError(reporter, id, "result", errorCodeFor(t), messageOf(t));
                }
            });
            return;
        }
        Object client = getClient("getAchievementsClient", activity);
        if (client == null) {
            replyError(reporter, id, "result", "unavailable", "");
            return;
        }
        Object task;
        if (percent >= 100.0) {
            task = client.getClass().getMethod("unlockImmediate", String.class).invoke(client, achievementId);
        } else {
            int steps = Math.max(1, Math.min(99, (int) Math.round(percent)));
            task = client.getClass()
                    .getMethod("setStepsImmediate", String.class, int.class)
                    .invoke(client, achievementId, steps);
        }
        addCompleteListener(task, completed -> {
            try {
                if (!isTaskSuccessful(completed)) {
                    replyTaskError(reporter, id, "result", completed);
                    return;
                }
                replyOk(reporter, id);
            } catch (Throwable t) {
                replyError(reporter, id, "result", errorCodeFor(t), messageOf(t));
            }
        });
    }

    private static void submitScore(
            Activity activity, long id, String leaderboardId, long score, Reporter reporter) throws Exception {
        if (leaderboardId == null || leaderboardId.isEmpty()) {
            replyError(reporter, id, "result", "failed", "leaderboard ID is required");
            return;
        }
        Object client = getClient("getLeaderboardsClient", activity);
        if (client == null) {
            replyError(reporter, id, "result", "unavailable", "");
            return;
        }
        Object task = client.getClass()
                .getMethod("submitScoreImmediate", String.class, long.class)
                .invoke(client, leaderboardId, score);
        addCompleteListener(task, completed -> {
            try {
                if (!isTaskSuccessful(completed)) {
                    replyTaskError(reporter, id, "result", completed);
                    return;
                }
                replyOk(reporter, id);
            } catch (Throwable t) {
                replyError(reporter, id, "result", errorCodeFor(t), messageOf(t));
            }
        });
    }

    private static void loadScores(
            Activity activity,
            long id,
            String leaderboardId,
            String playerScope,
            String timeScope,
            int limit,
            Reporter reporter)
            throws Exception {
        if (leaderboardId == null || leaderboardId.isEmpty()) {
            replyError(reporter, id, "scores", "failed", "leaderboard ID is required");
            return;
        }
        Object client = getClient("getLeaderboardsClient", activity);
        if (client == null) {
            replyError(reporter, id, "scores", "unavailable", "");
            return;
        }
        int maxResults = limit <= 0 ? MAX_SCORES_PER_PAGE : Math.min(limit, MAX_SCORES_PER_PAGE);
        int span = timeSpanFor(timeScope);
        int collection = collectionFor(playerScope);
        Method loadTop = client.getClass()
                .getMethod("loadTopScores", String.class, int.class, int.class, int.class);
        Object task = loadTop.invoke(client, leaderboardId, span, collection, maxResults);
        addCompleteListener(task, completed -> {
            try {
                if (!isTaskSuccessful(completed)) {
                    replyTaskError(reporter, id, "scores", completed);
                    return;
                }
                Object annotated = taskResult(completed);
                Object lbScores = annotated != null ? annotated.getClass().getMethod("get").invoke(annotated) : null;
                JSONArray scores = new JSONArray();
                try {
                    if (lbScores != null) {
                        Object buf = lbScores.getClass().getMethod("getScores").invoke(lbScores);
                        if (buf != null) {
                            int count = ((Number) buf.getClass().getMethod("getCount").invoke(buf)).intValue();
                            Method get = buf.getClass().getMethod("get", int.class);
                            int n = Math.min(count, maxResults);
                            for (int i = 0; i < n; i++) {
                                Object row = get.invoke(buf, i);
                                if (row == null) continue;
                                long rawScore = ((Number) row.getClass().getMethod("getRawScore").invoke(row)).longValue();
                                long rank = ((Number) row.getClass().getMethod("getRank").invoke(row)).longValue();
                                String displayName = (String) row.getClass().getMethod("getScoreHolderDisplayName").invoke(row);
                                String playerId = "";
                                Object holder = row.getClass().getMethod("getScoreHolder").invoke(row);
                                if (holder != null) {
                                    String pid = (String) holder.getClass().getMethod("getPlayerId").invoke(holder);
                                    if (pid != null) playerId = pid;
                                    if (displayName == null || displayName.isEmpty()) {
                                        String holderName = (String) holder.getClass().getMethod("getDisplayName").invoke(holder);
                                        if (holderName != null) displayName = holderName;
                                    }
                                }
                                JSONObject item = new JSONObject();
                                item.put("leaderboardId", leaderboardId);
                                item.put("playerId", playerId);
                                item.put("displayName", displayName != null ? displayName : "");
                                item.put("score", rawScore);
                                item.put("rank", rank > 0 ? rank : 0);
                                scores.put(item);
                            }
                        }
                    }
                } finally {
                    releaseQuietly(lbScores);
                }
                JSONObject r = new JSONObject();
                r.put("type", "scores");
                r.put("id", id);
                r.put("scores", scores);
                reporter.report(r.toString().getBytes(StandardCharsets.UTF_8));
            } catch (Throwable t) {
                replyError(reporter, id, "scores", errorCodeFor(t), messageOf(t));
            }
        });
    }

    private static void showAchievements(Activity activity) throws Exception {
        Object client = getClient("getAchievementsClient", activity);
        if (client == null) return;
        Object task = client.getClass().getMethod("getAchievementsIntent").invoke(client);
        addCompleteListener(task, completed -> launchIntentTask(activity, completed, ACHIEVEMENTS_REQUEST));
    }

    private static void showLeaderboard(Activity activity, String leaderboardId) throws Exception {
        if (leaderboardId == null || leaderboardId.isEmpty()) return;
        Object client = getClient("getLeaderboardsClient", activity);
        if (client == null) return;
        Object task = client.getClass().getMethod("getLeaderboardIntent", String.class).invoke(client, leaderboardId);
        addCompleteListener(task, completed -> launchIntentTask(activity, completed, LEADERBOARD_REQUEST));
    }

    private static void launchIntentTask(Activity activity, Object completed, int requestCode) {
        try {
            if (!isTaskSuccessful(completed)) return;
            Object result = taskResult(completed);
            if (!(result instanceof Intent)) return;
            Intent intent = (Intent) result;
            activity.runOnUiThread(() -> {
                if (activity.isFinishing() || activity.isDestroyed()) return;
                try {
                    activity.startActivityForResult(intent, requestCode);
                } catch (Throwable ignored) {}
            });
        } catch (Throwable ignored) {}
    }

    private static int timeSpanFor(String timeScope) {
        if ("today".equals(timeScope)) {
            return variantConst("TIME_SPAN_DAILY", TIME_SPAN_DAILY);
        }
        if ("week".equals(timeScope)) {
            return variantConst("TIME_SPAN_WEEKLY", TIME_SPAN_WEEKLY);
        }
        return variantConst("TIME_SPAN_ALL_TIME", TIME_SPAN_ALL_TIME);
    }

    private static int collectionFor(String playerScope) {
        if ("friends".equals(playerScope)) {
            return variantConst("COLLECTION_FRIENDS", COLLECTION_FRIENDS);
        }
        return variantConst("COLLECTION_PUBLIC", COLLECTION_PUBLIC);
    }

    private static int variantConst(String field, int fallback) {
        try {
            Class<?> variant = Class.forName(LEADERBOARD_VARIANT_CLASS);
            return variant.getField(field).getInt(null);
        } catch (Throwable ignored) {
            return fallback;
        }
    }

    private static boolean isTaskSuccessful(Object task) throws Exception {
        return task != null && Boolean.TRUE.equals(task.getClass().getMethod("isSuccessful").invoke(task));
    }

    private static boolean isTaskCanceled(Object task) {
        if (task == null) return false;
        try {
            return Boolean.TRUE.equals(task.getClass().getMethod("isCanceled").invoke(task));
        } catch (Throwable ignored) {
            return false;
        }
    }

    private static Object taskResult(Object task) throws Exception {
        return task.getClass().getMethod("getResult").invoke(task);
    }

    private static Throwable taskException(Object task) {
        if (task == null) return null;
        try {
            Object ex = task.getClass().getMethod("getException").invoke(task);
            return ex instanceof Throwable ? (Throwable) ex : null;
        } catch (Throwable ignored) {
            return null;
        }
    }

    private static boolean isAuthenticatedResult(Object authResult) throws Exception {
        return authResult != null
                && Boolean.TRUE.equals(authResult.getClass().getMethod("isAuthenticated").invoke(authResult));
    }

    private static void releaseQuietly(Object releasable) {
        if (releasable == null) return;
        try {
            releasable.getClass().getMethod("release").invoke(releasable);
        } catch (Throwable ignored) {}
    }

    private static String replyTypeFor(String op) {
        if ("authenticate".equals(op)) return "authenticate";
        if ("loadScores".equals(op)) return "scores";
        return "result";
    }

    private static void replyTaskError(Reporter reporter, long id, String type, Object task) {
        if (isTaskCanceled(task)) {
            replyError(reporter, id, type, "cancelled", "");
            return;
        }
        Throwable ex = taskException(task);
        String code = errorCodeFor(ex);
        if ("unauthenticated".equals(code)) {
            signedIn = false;
        }
        replyError(reporter, id, type, code, messageOf(ex));
    }

    private static String errorCodeFor(Throwable error) {
        Throwable cause = unwrap(error);
        if (cause == null) return "unavailable";
        Integer status = statusCodeOf(cause);
        if (status != null) {
            if (status == STATUS_SIGN_IN_REQUIRED || status == STATUS_CLIENT_RECONNECT_REQUIRED) {
                return "unauthenticated";
            }
            if (status == STATUS_CANCELED) {
                return "cancelled";
            }
        }
        return "unavailable";
    }

    private static Integer statusCodeOf(Throwable error) {
        try {
            Method getStatusCode = error.getClass().getMethod("getStatusCode");
            Object value = getStatusCode.invoke(error);
            if (value instanceof Number) {
                return ((Number) value).intValue();
            }
        } catch (Throwable ignored) {}
        return null;
    }

    private static Throwable unwrap(Throwable error) {
        while (error instanceof InvocationTargetException && error.getCause() != null) {
            error = error.getCause();
        }
        return error;
    }

    private static String messageOf(Throwable error) {
        Throwable cause = unwrap(error);
        if (cause == null) return "";
        String msg = cause.getMessage();
        return msg != null && !msg.isEmpty() ? msg : cause.toString();
    }

    private interface TaskCallback {
        void onComplete(Object task);
    }

    private static void addCompleteListener(Object task, TaskCallback cb) throws Exception {
        Class<?> listenerClass = Class.forName(ON_COMPLETE_LISTENER_CLASS);
        Object proxy = Proxy.newProxyInstance(
                listenerClass.getClassLoader(),
                new Class<?>[] {listenerClass},
                (p, method, args) -> {
                    switch (method.getName()) {
                        case "hashCode":
                            return System.identityHashCode(p);
                        case "equals":
                            return args != null && args.length == 1 && p == args[0];
                        case "toString":
                            return "NsgoGameServicesListener";
                        case "onComplete":
                            if (args != null && args.length == 1) {
                                cb.onComplete(args[0]);
                            }
                            return null;
                        default:
                            return null;
                    }
                });
        task.getClass().getMethod("addOnCompleteListener", listenerClass).invoke(task, proxy);
    }

    private static void reportState(Reporter reporter, boolean available) {
        signedIn = available;
        try {
            JSONObject s = new JSONObject();
            s.put("type", "state");
            s.put("available", available);
            reporter.report(s.toString().getBytes(StandardCharsets.UTF_8));
        } catch (Throwable ignored) {}
    }

    private static void replyOk(Reporter reporter, long id) {
        try {
            JSONObject r = new JSONObject();
            r.put("type", "result");
            r.put("id", id);
            reporter.report(r.toString().getBytes(StandardCharsets.UTF_8));
        } catch (Throwable ignored) {}
    }

    private static void replyError(Reporter reporter, long id, String type, String code, String detail) {
        try {
            JSONObject r = new JSONObject();
            r.put("type", type);
            r.put("id", id);
            r.put("error", code);
            if (detail != null && !detail.isEmpty()) r.put("detail", detail);
            reporter.report(r.toString().getBytes(StandardCharsets.UTF_8));
        } catch (Throwable ignored) {}
    }
}
