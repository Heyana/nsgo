package dev.oneo.nsgo;

import android.app.Activity;
import android.util.Base64;
import java.lang.reflect.Method;
import java.nio.charset.StandardCharsets;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import org.json.JSONArray;
import org.json.JSONObject;

/** Reflection bridge for Google Play Games Services v2 Saved Games (SnapshotsClient), backing package cloud on Android without adding a compile-time Play Services dependency to the NSGo runtime. When Play Games Services is not on the classpath or the user is not signed in, every operation reports "unavailable" and writes are never silently dropped. */
final class CloudStorage {
    private CloudStorage() {}

    interface Reporter {
        void report(byte[] json);
    }

    private static final String PLAY_GAMES_CLASS = "com.google.android.gms.games.PlayGames";
    private static final String METADATA_CHANGE_BUILDER =
            "com.google.android.gms.games.snapshot.SnapshotMetadataChange$Builder";
    private static final String KV_SNAPSHOT = "__nsgo_cloud_kv__";

    /** Whether the last probe found Play Games signed in, which the environment's second capability byte carries (bit 128). */
    static volatile boolean signedIn;

    /** Pending conflict handles keyed by conflict ID while Go resolves a conflict. */
    private static final Map<String, Object> pendingConflicts = new ConcurrentHashMap<>();

    /** Reports whether the Play Games v2 runtime classes are present on the classpath. */
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
        try {
            Class<?> playGames = Class.forName(PLAY_GAMES_CLASS);
            Method getSignIn = playGames.getMethod("getGamesSignInClient", Activity.class);
            Object signInClient = getSignIn.invoke(null, activity);
            Object task = signInClient.getClass().getMethod("isAuthenticated").invoke(signInClient);
            addCompleteListener(task, completedTask -> {
                try {
                    boolean successful = (Boolean) completedTask.getClass().getMethod("isSuccessful").invoke(completedTask);
                    if (!successful) {
                        reportState(reporter, false);
                        return;
                    }
                    Object result = completedTask.getClass().getMethod("getResult").invoke(completedTask);
                    boolean authed = result != null
                            && Boolean.TRUE.equals(result.getClass().getMethod("isAuthenticated").invoke(result));
                    reportState(reporter, authed);
                } catch (Throwable ignored) {
                    reportState(reporter, false);
                }
            });
        } catch (Throwable ignored) {
            reportState(reporter, false);
        }
    }

    /** Dispatches one EXT_CLOUD_STORAGE JSON command from package cloud. */
    static void handle(Activity activity, byte[] payload, Reporter reporter) {
        long id = 0;
        try {
            JSONObject cmd = new JSONObject(new String(payload, StandardCharsets.UTF_8));
            id = cmd.optLong("id");
            String op = cmd.optString("op");
            if (activity == null || !isSdkPresent()) {
                replyError(reporter, id, "unavailable", "");
                return;
            }
            Object client = getSnapshotsClient(activity);
            if (client == null) {
                replyError(reporter, id, "unavailable", "");
                return;
            }
            switch (op) {
                case "read_file":
                    readSnapshot(client, id, cmd.optString("name"), reporter);
                    break;
                case "write_file": {
                    byte[] data = Base64.decode(cmd.optString("data"), Base64.DEFAULT);
                    writeSnapshot(client, id, cmd.optString("name"), data, reporter);
                    break;
                }
                case "delete_file":
                    deleteSnapshot(client, id, cmd.optString("name"), reporter);
                    break;
                case "list_files":
                    listSnapshots(client, id, reporter);
                    break;
                case "get":
                    readKv(client, id, cmd.optString("key"), reporter);
                    break;
                case "set": {
                    byte[] val = Base64.decode(cmd.optString("data"), Base64.DEFAULT);
                    writeKv(client, id, cmd.optString("key"), val, false, reporter);
                    break;
                }
                case "delete":
                    writeKv(client, id, cmd.optString("key"), null, true, reporter);
                    break;
                case "keys":
                    listKvKeys(client, id, reporter);
                    break;
                case "resolve": {
                    byte[] chosen = Base64.decode(cmd.optString("data"), Base64.DEFAULT);
                    resolveConflict(client, id, cmd.optString("conflictId"), chosen, reporter);
                    break;
                }
                default:
                    replyError(reporter, id, "unavailable", "unsupported op: " + op);
            }
        } catch (Throwable t) {
            replyError(reporter, id, "failed", t.getMessage() != null ? t.getMessage() : t.toString());
        }
    }

    private static Object getSnapshotsClient(Activity activity) {
        try {
            Class<?> playGames = Class.forName(PLAY_GAMES_CLASS);
            Method getSnapshots = playGames.getMethod("getSnapshotsClient", Activity.class);
            return getSnapshots.invoke(null, activity);
        } catch (Throwable ignored) {
            return null;
        }
    }

    private static void readSnapshot(Object client, long id, String name, Reporter reporter) throws Exception {
        // SnapshotsClient.open(name, false) -> Task<DataOrConflict<Snapshot>>
        Method open = client.getClass().getMethod("open", String.class, boolean.class);
        Object task = open.invoke(client, name, false);
        addCompleteListener(task, completed -> {
            try {
                if (!Boolean.TRUE.equals(completed.getClass().getMethod("isSuccessful").invoke(completed))) {
                    replyError(reporter, id, "not_found", "");
                    return;
                }
                Object doc = completed.getClass().getMethod("getResult").invoke(completed);
                if (Boolean.TRUE.equals(doc.getClass().getMethod("isConflict").invoke(doc))) {
                    Object conflict = doc.getClass().getMethod("getConflict").invoke(doc);
                    reportSnapshotConflict(id, name, conflict, reporter);
                    return;
                }
                Object snapshot = doc.getClass().getMethod("getData").invoke(doc);
                if (snapshot == null) {
                    replyError(reporter, id, "not_found", "");
                    return;
                }
                Object contents = snapshot.getClass().getMethod("getSnapshotContents").invoke(snapshot);
                byte[] bytes = (byte[]) contents.getClass().getMethod("readFully").invoke(contents);
                JSONObject r = new JSONObject();
                r.put("type", "reply");
                r.put("id", id);
                r.put("data", Base64.encodeToString(bytes != null ? bytes : new byte[0], Base64.NO_WRAP));
                reporter.report(r.toString().getBytes(StandardCharsets.UTF_8));
            } catch (Throwable t) {
                replyError(reporter, id, "failed", String.valueOf(t.getMessage()));
            }
        });
    }

    private static void writeSnapshot(Object client, long id, String name, byte[] data, Reporter reporter)
            throws Exception {
        Method open = client.getClass().getMethod("open", String.class, boolean.class);
        Object task = open.invoke(client, name, true);
        addCompleteListener(task, completed -> {
            try {
                if (!Boolean.TRUE.equals(completed.getClass().getMethod("isSuccessful").invoke(completed))) {
                    replyError(reporter, id, "unavailable", "");
                    return;
                }
                Object doc = completed.getClass().getMethod("getResult").invoke(completed);
                if (Boolean.TRUE.equals(doc.getClass().getMethod("isConflict").invoke(doc))) {
                    Object conflict = doc.getClass().getMethod("getConflict").invoke(doc);
                    reportSnapshotConflict(id, name, conflict, reporter);
                    return;
                }
                Object snapshot = doc.getClass().getMethod("getData").invoke(doc);
                Object contents = snapshot.getClass().getMethod("getSnapshotContents").invoke(snapshot);
                contents.getClass().getMethod("writeBytes", byte[].class).invoke(contents, (Object) data);
                Object change = Class.forName(METADATA_CHANGE_BUILDER).getDeclaredConstructor().newInstance();
                Object built = change.getClass().getMethod("build").invoke(change);
                Method commit = findMethod(client.getClass(), "commitAndClose", 2);
                Object commitTask = commit.invoke(client, snapshot, built);
                addCompleteListener(commitTask, done -> {
                    try {
                        boolean ok = Boolean.TRUE.equals(done.getClass().getMethod("isSuccessful").invoke(done));
                        if (!ok) {
                            replyError(reporter, id, "failed", "commitAndClose failed");
                            return;
                        }
                        replyOk(reporter, id);
                    } catch (Throwable t) {
                        replyError(reporter, id, "failed", String.valueOf(t.getMessage()));
                    }
                });
            } catch (Throwable t) {
                replyError(reporter, id, "failed", String.valueOf(t.getMessage()));
            }
        });
    }

    private static void deleteSnapshot(Object client, long id, String name, Reporter reporter) throws Exception {
        Method open = client.getClass().getMethod("open", String.class, boolean.class);
        Object task = open.invoke(client, name, false);
        addCompleteListener(task, completed -> {
            try {
                if (!Boolean.TRUE.equals(completed.getClass().getMethod("isSuccessful").invoke(completed))) {
                    replyOk(reporter, id);
                    return;
                }
                Object doc = completed.getClass().getMethod("getResult").invoke(completed);
                Object snapshot = doc.getClass().getMethod("getData").invoke(doc);
                if (snapshot == null) {
                    replyOk(reporter, id);
                    return;
                }
                Object meta = snapshot.getClass().getMethod("getMetadata").invoke(snapshot);
                Method del = findMethod(client.getClass(), "delete", 1);
                Object delTask = del.invoke(client, meta);
                addCompleteListener(delTask, done -> replyOk(reporter, id));
            } catch (Throwable t) {
                replyError(reporter, id, "failed", String.valueOf(t.getMessage()));
            }
        });
    }

    private static void listSnapshots(Object client, long id, Reporter reporter) throws Exception {
        Method load = client.getClass().getMethod("load", boolean.class);
        Object task = load.invoke(client, false);
        addCompleteListener(task, completed -> {
            try {
                if (!Boolean.TRUE.equals(completed.getClass().getMethod("isSuccessful").invoke(completed))) {
                    replyError(reporter, id, "unavailable", "");
                    return;
                }
                Object annotated = completed.getClass().getMethod("getResult").invoke(completed);
                Object buf = annotated.getClass().getMethod("get").invoke(annotated);
                JSONArray files = new JSONArray();
                if (buf != null) {
                    int count = (Integer) buf.getClass().getMethod("getCount").invoke(buf);
                    Method get = buf.getClass().getMethod("get", int.class);
                    for (int i = 0; i < count; i++) {
                        Object meta = get.invoke(buf, i);
                        String uniqueName = (String) meta.getClass().getMethod("getUniqueName").invoke(meta);
                        if (uniqueName != null && !KV_SNAPSHOT.equals(uniqueName)) {
                            files.put(uniqueName);
                        }
                    }
                    buf.getClass().getMethod("release").invoke(buf);
                }
                JSONObject r = new JSONObject();
                r.put("type", "reply");
                r.put("id", id);
                r.put("files", files);
                reporter.report(r.toString().getBytes(StandardCharsets.UTF_8));
            } catch (Throwable t) {
                replyError(reporter, id, "failed", String.valueOf(t.getMessage()));
            }
        });
    }

    private static void readKv(Object client, long id, String key, Reporter reporter) throws Exception {
        readSnapshot(client, id, KV_SNAPSHOT, raw -> {
            try {
                JSONObject resp = new JSONObject(new String(raw, StandardCharsets.UTF_8));
                if (resp.has("error") && !resp.optString("error").isEmpty()) {
                    reporter.report(raw);
                    return;
                }
                byte[] blob = Base64.decode(resp.optString("data"), Base64.DEFAULT);
                JSONObject kv = blob.length > 0 ? new JSONObject(new String(blob, StandardCharsets.UTF_8)) : new JSONObject();
                if (!kv.has(key)) {
                    replyError(reporter, id, "not_found", "");
                    return;
                }
                JSONObject r = new JSONObject();
                r.put("type", "reply");
                r.put("id", id);
                r.put("data", kv.getString(key));
                reporter.report(r.toString().getBytes(StandardCharsets.UTF_8));
            } catch (Throwable t) {
                replyError(reporter, id, "failed", String.valueOf(t.getMessage()));
            }
        });
    }

    private static void writeKv(Object client, long id, String key, byte[] value, boolean delete, Reporter reporter)
            throws Exception {
        readSnapshot(client, id, KV_SNAPSHOT, raw -> {
            try {
                JSONObject resp = new JSONObject(new String(raw, StandardCharsets.UTF_8));
                JSONObject kv = new JSONObject();
                String err = resp.optString("error");
                if (err.isEmpty()) {
                    byte[] blob = Base64.decode(resp.optString("data"), Base64.DEFAULT);
                    if (blob.length > 0) {
                        kv = new JSONObject(new String(blob, StandardCharsets.UTF_8));
                    }
                } else if (!"not_found".equals(err)) {
                    reporter.report(raw);
                    return;
                }
                if (delete) {
                    kv.remove(key);
                } else {
                    kv.put(key, Base64.encodeToString(value != null ? value : new byte[0], Base64.NO_WRAP));
                }
                writeSnapshot(client, id, KV_SNAPSHOT, kv.toString().getBytes(StandardCharsets.UTF_8), reporter);
            } catch (Throwable t) {
                replyError(reporter, id, "failed", String.valueOf(t.getMessage()));
            }
        });
    }

    private static void listKvKeys(Object client, long id, Reporter reporter) throws Exception {
        readSnapshot(client, id, KV_SNAPSHOT, raw -> {
            try {
                JSONObject resp = new JSONObject(new String(raw, StandardCharsets.UTF_8));
                JSONArray keys = new JSONArray();
                String err = resp.optString("error");
                if (err.isEmpty()) {
                    byte[] blob = Base64.decode(resp.optString("data"), Base64.DEFAULT);
                    if (blob.length > 0) {
                        JSONObject kv = new JSONObject(new String(blob, StandardCharsets.UTF_8));
                        JSONArray names = kv.names();
                        if (names != null) keys = names;
                    }
                } else if (!"not_found".equals(err)) {
                    reporter.report(raw);
                    return;
                }
                JSONObject r = new JSONObject();
                r.put("type", "reply");
                r.put("id", id);
                r.put("keys", keys);
                reporter.report(r.toString().getBytes(StandardCharsets.UTF_8));
            } catch (Throwable t) {
                replyError(reporter, id, "failed", String.valueOf(t.getMessage()));
            }
        });
    }

    private static void reportSnapshotConflict(long id, String name, Object conflict, Reporter reporter) throws Exception {
        String conflictId = (String) conflict.getClass().getMethod("getConflictId").invoke(conflict);
        Object localSnap = conflict.getClass().getMethod("getSnapshot").invoke(conflict);
        Object remoteSnap = conflict.getClass().getMethod("getConflictingSnapshot").invoke(conflict);
        Object localContents = localSnap.getClass().getMethod("getSnapshotContents").invoke(localSnap);
        Object remoteContents = remoteSnap.getClass().getMethod("getSnapshotContents").invoke(remoteSnap);
        byte[] localBytes = (byte[]) localContents.getClass().getMethod("readFully").invoke(localContents);
        byte[] remoteBytes = (byte[]) remoteContents.getClass().getMethod("readFully").invoke(remoteContents);
        Object localMeta = localSnap.getClass().getMethod("getMetadata").invoke(localSnap);
        Object remoteMeta = remoteSnap.getClass().getMethod("getMetadata").invoke(remoteSnap);
        long localMs = (Long) localMeta.getClass().getMethod("getLastModifiedTimestamp").invoke(localMeta);
        long remoteMs = (Long) remoteMeta.getClass().getMethod("getLastModifiedTimestamp").invoke(remoteMeta);
        pendingConflicts.put(conflictId, conflict);

        JSONObject c = new JSONObject();
        c.put("type", "conflict");
        c.put("conflictId", conflictId);
        c.put("name", name);
        c.put("local", Base64.encodeToString(localBytes != null ? localBytes : new byte[0], Base64.NO_WRAP));
        c.put("remote", Base64.encodeToString(remoteBytes != null ? remoteBytes : new byte[0], Base64.NO_WRAP));
        c.put("localTimeMs", localMs);
        c.put("remoteTimeMs", remoteMs);
        reporter.report(c.toString().getBytes(StandardCharsets.UTF_8));

        JSONObject r = new JSONObject();
        r.put("type", "reply");
        r.put("id", id);
        r.put("error", "conflict");
        r.put("name", name);
        r.put("local", Base64.encodeToString(localBytes != null ? localBytes : new byte[0], Base64.NO_WRAP));
        r.put("remote", Base64.encodeToString(remoteBytes != null ? remoteBytes : new byte[0], Base64.NO_WRAP));
        r.put("localTimeMs", localMs);
        r.put("remoteTimeMs", remoteMs);
        reporter.report(r.toString().getBytes(StandardCharsets.UTF_8));
    }

    private static void resolveConflict(Object client, long id, String conflictId, byte[] chosen, Reporter reporter)
            throws Exception {
        Object conflict = pendingConflicts.remove(conflictId);
        if (conflict == null) {
            replyError(reporter, id, "not_found", "unknown conflict id");
            return;
        }
        Object resolutionContents = conflict.getClass().getMethod("getResolutionSnapshotContents").invoke(conflict);
        resolutionContents.getClass().getMethod("writeBytes", byte[].class).invoke(resolutionContents, (Object) chosen);
        Object localSnap = conflict.getClass().getMethod("getSnapshot").invoke(conflict);
        Object meta = localSnap.getClass().getMethod("getMetadata").invoke(localSnap);
        String uniqueName = (String) meta.getClass().getMethod("getUniqueName").invoke(meta);
        Object change = Class.forName(METADATA_CHANGE_BUILDER).getDeclaredConstructor().newInstance();
        Object built = change.getClass().getMethod("build").invoke(change);
        Method resolve = findMethod(client.getClass(), "resolveConflict", 4);
        Object task = resolve.invoke(client, conflictId, uniqueName, built, resolutionContents);
        addCompleteListener(task, done -> {
            try {
                boolean ok = Boolean.TRUE.equals(done.getClass().getMethod("isSuccessful").invoke(done));
                if (!ok) {
                    replyError(reporter, id, "failed", "resolveConflict failed");
                    return;
                }
                replyOk(reporter, id);
            } catch (Throwable t) {
                replyError(reporter, id, "failed", String.valueOf(t.getMessage()));
            }
        });
    }

    private interface TaskCallback {
        void onComplete(Object task);
    }

    private static void addCompleteListener(Object task, TaskCallback cb) throws Exception {
        Class<?> listenerClass = Class.forName("com.google.android.gms.tasks.OnCompleteListener");
        Object proxy = java.lang.reflect.Proxy.newProxyInstance(
                listenerClass.getClassLoader(),
                new Class<?>[] {listenerClass},
                (p, method, args) -> {
                    if ("onComplete".equals(method.getName()) && args != null && args.length == 1) {
                        cb.onComplete(args[0]);
                    }
                    return null;
                });
        task.getClass().getMethod("addOnCompleteListener", listenerClass).invoke(task, proxy);
    }

    private static Method findMethod(Class<?> cls, String name, int paramCount) throws NoSuchMethodException {
        for (Method m : cls.getMethods()) {
            if (m.getName().equals(name) && m.getParameterCount() == paramCount) {
                return m;
            }
        }
        throw new NoSuchMethodException(cls.getName() + "." + name + "/" + paramCount);
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
            r.put("type", "reply");
            r.put("id", id);
            reporter.report(r.toString().getBytes(StandardCharsets.UTF_8));
        } catch (Throwable ignored) {}
    }

    private static void replyError(Reporter reporter, long id, String code, String detail) {
        try {
            JSONObject r = new JSONObject();
            r.put("type", "reply");
            r.put("id", id);
            r.put("error", code);
            if (detail != null && !detail.isEmpty()) r.put("detail", detail);
            reporter.report(r.toString().getBytes(StandardCharsets.UTF_8));
        } catch (Throwable ignored) {}
    }
}
