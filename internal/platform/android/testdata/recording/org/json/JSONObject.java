package org.json;
import java.util.*;
/** Only command construction and report serialization; never a substitute parser. */
public class JSONObject {
    private final Map<String,Object> values = new LinkedHashMap<>();
    public JSONObject put(String key, Object value) throws JSONException { if (key == null) throw new JSONException("key"); values.put(key, value); return this; }
    public String optString(String key) { Object v = values.get(key); return v == null ? "" : v.toString(); }
    public int optInt(String key) { return optInt(key, 0); }
    public int optInt(String key, int fallback) { Object v = values.get(key); return v instanceof Number ? ((Number)v).intValue() : fallback; }
    public long optLong(String key) { Object v = values.get(key); return v instanceof Number ? ((Number)v).longValue() : 0; }
    public boolean optBoolean(String key) { return Boolean.TRUE.equals(values.get(key)); }
    private static String quoted(String value) { return "\"" + value.replace("\\", "\\\\").replace("\"", "\\\"") + "\""; }
    @Override public String toString() {
        StringJoiner out = new StringJoiner(",", "{", "}");
        for (Map.Entry<String,Object> e : values.entrySet()) { Object v = e.getValue(); out.add(quoted(e.getKey()) + ":" + (v instanceof String ? quoted((String)v) : String.valueOf(v))); }
        return out.toString();
    }
}
