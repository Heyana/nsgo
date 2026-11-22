package android.content;

import java.util.HashMap;
import java.util.Map;

public class Intent {
    public final String action;
    public final Map<String, Object> extras = new HashMap<>();

    public Intent(String action) { this.action = action; }
    public Intent putExtra(String name, String value) { extras.put(name, value); return this; }
    public Intent putExtra(String name, boolean value) { extras.put(name, value); return this; }
    public Intent putExtra(String name, int value) { extras.put(name, value); return this; }
}
