package org.json;

import java.util.Iterator;
import java.util.LinkedHashMap;
import java.util.Map;

public class JSONObject {
    private final Map<String, Object> fields = new LinkedHashMap<>();

    public JSONObject() {}

    // Minimal flat JSON object parser sufficient for TestAgent ops in JVM tests.
    public JSONObject(String src) throws JSONException {
        String s = src.trim();
        if (!s.startsWith("{") || !s.endsWith("}")) throw new JSONException("not an object: " + src);
        String body = s.substring(1, s.length() - 1).trim();
        if (body.isEmpty()) return;
        int i = 0;
        while (i < body.length()) {
            while (i < body.length() && (body.charAt(i) == ' ' || body.charAt(i) == ',')) i++;
            if (i >= body.length()) break;
            if (body.charAt(i) != '"') throw new JSONException("expected key quote at " + i);
            int keyEnd = body.indexOf('"', i + 1);
            String key = body.substring(i + 1, keyEnd);
            int colon = body.indexOf(':', keyEnd + 1);
            i = colon + 1;
            while (i < body.length() && body.charAt(i) == ' ') i++;
            if (body.charAt(i) == '"') {
                int valEnd = body.indexOf('"', i + 1);
                fields.put(key, body.substring(i + 1, valEnd));
                i = valEnd + 1;
            } else if (body.charAt(i) == '{') {
                int depth = 0, start = i;
                do {
                    if (body.charAt(i) == '{') depth++;
                    else if (body.charAt(i) == '}') depth--;
                    i++;
                } while (depth > 0 && i < body.length());
                fields.put(key, new JSONObject(body.substring(start, i)));
            } else {
                int valEnd = i;
                while (valEnd < body.length() && body.charAt(valEnd) != ',') valEnd++;
                String raw = body.substring(i, valEnd).trim();
                if ("true".equals(raw) || "false".equals(raw)) fields.put(key, Boolean.parseBoolean(raw));
                else fields.put(key, Double.parseDouble(raw));
                i = valEnd;
            }
        }
    }

    public JSONObject put(String k, Object v) throws JSONException { fields.put(k, v); return this; }
    public String getString(String k) throws JSONException {
        Object v = fields.get(k);
        if (v == null) throw new JSONException("missing " + k);
        return String.valueOf(v);
    }
    public double getDouble(String k) throws JSONException {
        Object v = fields.get(k);
        if (v instanceof Number) return ((Number) v).doubleValue();
        throw new JSONException("missing double " + k);
    }
    public int getInt(String k) throws JSONException { return (int) getDouble(k); }
    public long optLong(String k) {
        Object v = fields.get(k);
        return v instanceof Number ? ((Number) v).longValue() : 0L;
    }
    public boolean optBoolean(String k) {
        Object v = fields.get(k);
        return v instanceof Boolean && (Boolean) v;
    }
    public JSONObject optJSONObject(String k) {
        Object v = fields.get(k);
        return v instanceof JSONObject ? (JSONObject) v : null;
    }
    public JSONArray optJSONArray(String k) {
        Object v = fields.get(k);
        return v instanceof JSONArray ? (JSONArray) v : null;
    }
    public JSONArray getJSONArray(String k) throws JSONException {
        JSONArray a = optJSONArray(k);
        if (a == null) throw new JSONException("missing array " + k);
        return a;
    }
    public Iterator<String> keys() { return fields.keySet().iterator(); }
    @Override public String toString() { return fields.toString(); }
}
