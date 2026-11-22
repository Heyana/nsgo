package android.content;

import java.util.HashMap;
import java.util.Map;

public interface SharedPreferences {
    String getString(String key, String defValue);
    Editor edit();

    interface Editor {
        Editor putString(String key, String value);
        Editor remove(String key);
        void apply();
    }

    final class InMemory implements SharedPreferences {
        private final Map<String, String> values = new HashMap<>();

        @Override
        public String getString(String key, String defValue) {
            return values.getOrDefault(key, defValue);
        }

        @Override
        public Editor edit() {
            return new Editor() {
                private final Map<String, String> pending = new HashMap<>();

                @Override
                public Editor putString(String key, String value) {
                    pending.put(key, value);
                    return this;
                }

                @Override
                public Editor remove(String key) {
                    pending.put(key, null);
                    return this;
                }

                @Override
                public void apply() {
                    for (Map.Entry<String, String> e : pending.entrySet()) {
                        if (e.getValue() == null) values.remove(e.getKey());
                        else values.put(e.getKey(), e.getValue());
                    }
                }
            };
        }
    }
}
