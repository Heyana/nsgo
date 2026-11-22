package android.content;

import java.io.File;
import java.util.HashMap;
import java.util.Map;

/** Minimal Context stub for JVM Secrets tests. */
public class Context {
    public static final int MODE_PRIVATE = 0;

    private final File noBackupDir;
    private final Map<String, SharedPreferences> prefs = new HashMap<>();

    public Context(File noBackupDir) {
        this.noBackupDir = noBackupDir;
    }

    public File getNoBackupFilesDir() {
        return noBackupDir;
    }

    public SharedPreferences getSharedPreferences(String name, int mode) {
        return prefs.computeIfAbsent(name, k -> new SharedPreferences());
    }
}
