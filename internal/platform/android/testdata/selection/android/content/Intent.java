package android.content;

import java.util.HashMap;
import java.util.Map;

public class Intent {
    public static final String ACTION_PROCESS_TEXT = "android.intent.action.PROCESS_TEXT";
    public static final String EXTRA_PROCESS_TEXT = "android.intent.extra.PROCESS_TEXT";
    public static final String EXTRA_PROCESS_TEXT_READONLY = "android.intent.extra.PROCESS_TEXT_READONLY";
    public static final int FLAG_ACTIVITY_NEW_TASK = 0x10000000;

    private final String action;
    private String type;
    private String packageName;
    private String className;
    private int flags;
    private final Map<String, Object> extras = new HashMap<>();

    public Intent(String action) {
        this.action = action;
    }

    public String getAction() { return action; }

    public Intent setType(String type) {
        this.type = type;
        return this;
    }

    public String getType() { return type; }

    public Intent setClassName(String packageName, String className) {
        this.packageName = packageName;
        this.className = className;
        return this;
    }

    public String getPackageName() { return packageName; }

    public String getClassName() { return className; }

    public Intent putExtra(String name, String value) {
        extras.put(name, value);
        return this;
    }

    public Intent putExtra(String name, boolean value) {
        extras.put(name, value);
        return this;
    }

    public String getStringExtra(String name) {
        Object v = extras.get(name);
        return v instanceof String ? (String) v : null;
    }

    public boolean getBooleanExtra(String name, boolean fallback) {
        Object v = extras.get(name);
        return v instanceof Boolean ? (Boolean) v : fallback;
    }

    public Intent addFlags(int flags) {
        this.flags |= flags;
        return this;
    }

    public int getFlags() { return flags; }
}
