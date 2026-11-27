package org.json;

import java.util.ArrayList;
import java.util.List;

/** Minimal org.json.JSONArray stub for GameServices JVM tests. */
public class JSONArray {
    private final List<Object> items = new ArrayList<>();

    public JSONArray put(Object value) {
        items.add(value);
        return this;
    }

    public int length() {
        return items.size();
    }

    public Object get(int index) throws JSONException {
        if (index < 0 || index >= items.size()) {
            throw new JSONException("index out of range: " + index);
        }
        return items.get(index);
    }

    @Override
    public String toString() {
        StringBuilder sb = new StringBuilder("[");
        for (int i = 0; i < items.size(); i++) {
            if (i > 0) sb.append(',');
            JSONObject.appendValue(sb, items.get(i));
        }
        return sb.append(']').toString();
    }
}
