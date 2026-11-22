package android.content;

import java.util.HashMap;
import java.util.HashSet;
import java.util.Map;
import java.util.Set;

/** In-memory SharedPreferences stub for JVM Secrets tests. */
public class SharedPreferences {
    private final Map<String, String> values = new HashMap<>();
    public int editCount;

    public boolean contains(String key) {
        return values.containsKey(key);
    }

    public String getString(String key, String defValue) {
        return values.containsKey(key) ? values.get(key) : defValue;
    }

    public void put(String key, String value) {
        values.put(key, value);
    }

    public Editor edit() {
        editCount++;
        return new Editor(values);
    }

    public static final class Editor {
        private final Map<String, String> target;
        private final Map<String, String> puts = new HashMap<>();
        private final Set<String> removes = new HashSet<>();

        Editor(Map<String, String> target) {
            this.target = target;
        }

        public Editor putString(String key, String value) {
            if (value == null) {
                removes.add(key);
                puts.remove(key);
            } else {
                puts.put(key, value);
                removes.remove(key);
            }
            return this;
        }

        public Editor remove(String key) {
            removes.add(key);
            puts.remove(key);
            return this;
        }

        public boolean commit() {
            for (String k : removes) target.remove(k);
            target.putAll(puts);
            return true;
        }

        public void apply() {
            commit();
        }
    }
}
