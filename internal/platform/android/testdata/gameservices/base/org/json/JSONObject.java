package org.json;

import java.util.LinkedHashMap;
import java.util.Map;

/** Minimal org.json.JSONObject parser/builder for GameServices JVM tests. */
public class JSONObject {
    public static final Object NULL = new Object();

    private final Map<String, Object> map = new LinkedHashMap<>();

    public JSONObject() {}

    public JSONObject(String json) throws JSONException {
        if (json == null) throw new JSONException("null JSON");
        Parser p = new Parser(json);
        p.skipWS();
        if (!p.consume('{')) throw new JSONException("expected '{'");
        p.skipWS();
        if (p.consume('}')) {
            p.skipWS();
            if (!p.eof()) throw new JSONException("trailing characters");
            return;
        }
        while (true) {
            p.skipWS();
            String key = p.readString();
            p.skipWS();
            if (!p.consume(':')) throw new JSONException("expected ':'");
            p.skipWS();
            map.put(key, p.readValue());
            p.skipWS();
            if (p.consume('}')) break;
            if (!p.consume(',')) throw new JSONException("expected ',' or '}'");
        }
        p.skipWS();
        if (!p.eof()) throw new JSONException("trailing characters");
    }

    public JSONObject put(String key, Object value) throws JSONException {
        if (key == null) throw new JSONException("null key");
        map.put(key, value == null ? NULL : value);
        return this;
    }

    public JSONObject put(String key, long value) throws JSONException {
        return put(key, Long.valueOf(value));
    }

    public JSONObject put(String key, int value) throws JSONException {
        return put(key, Integer.valueOf(value));
    }

    public JSONObject put(String key, boolean value) throws JSONException {
        return put(key, Boolean.valueOf(value));
    }

    public String optString(String key) {
        return optString(key, "");
    }

    public String optString(String key, String fallback) {
        Object v = map.get(key);
        if (v == null || v == NULL) return fallback;
        return v.toString();
    }

    public long optLong(String key) {
        Object v = map.get(key);
        return v instanceof Number ? ((Number) v).longValue() : 0L;
    }

    public int optInt(String key, int fallback) {
        Object v = map.get(key);
        return v instanceof Number ? ((Number) v).intValue() : fallback;
    }

    public double optDouble(String key, double fallback) {
        Object v = map.get(key);
        return v instanceof Number ? ((Number) v).doubleValue() : fallback;
    }

    public boolean optBoolean(String key) {
        return Boolean.TRUE.equals(map.get(key));
    }

    @Override
    public String toString() {
        StringBuilder sb = new StringBuilder("{");
        boolean first = true;
        for (Map.Entry<String, Object> e : map.entrySet()) {
            if (!first) sb.append(',');
            first = false;
            appendString(sb, e.getKey());
            sb.append(':');
            appendValue(sb, e.getValue());
        }
        return sb.append('}').toString();
    }

    static void appendValue(StringBuilder sb, Object v) {
        if (v == null || v == NULL) {
            sb.append("null");
        } else if (v instanceof String) {
            appendString(sb, (String) v);
        } else {
            sb.append(v);
        }
    }

    private static void appendString(StringBuilder sb, String s) {
        sb.append('"');
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            switch (c) {
                case '"': sb.append("\\\""); break;
                case '\\': sb.append("\\\\"); break;
                case '\n': sb.append("\\n"); break;
                case '\r': sb.append("\\r"); break;
                case '\t': sb.append("\\t"); break;
                default: sb.append(c); break;
            }
        }
        sb.append('"');
    }

    private static final class Parser {
        private final String s;
        private int i;

        Parser(String s) { this.s = s; }
        boolean eof() { return i >= s.length(); }
        void skipWS() { while (i < s.length() && Character.isWhitespace(s.charAt(i))) i++; }

        boolean consume(char c) {
            if (i < s.length() && s.charAt(i) == c) {
                i++;
                return true;
            }
            return false;
        }

        String readString() throws JSONException {
            if (!consume('"')) throw new JSONException("expected '\"'");
            StringBuilder sb = new StringBuilder();
            while (i < s.length()) {
                char c = s.charAt(i++);
                if (c == '"') return sb.toString();
                if (c == '\\') {
                    if (i >= s.length()) throw new JSONException("unterminated escape");
                    char e = s.charAt(i++);
                    switch (e) {
                        case '"': case '\\': case '/': sb.append(e); break;
                        case 'n': sb.append('\n'); break;
                        case 'r': sb.append('\r'); break;
                        case 't': sb.append('\t'); break;
                        case 'b': sb.append('\b'); break;
                        case 'f': sb.append('\f'); break;
                        case 'u':
                            if (i + 4 > s.length()) throw new JSONException("bad unicode escape");
                            sb.append((char) Integer.parseInt(s.substring(i, i + 4), 16));
                            i += 4;
                            break;
                        default:
                            throw new JSONException("bad escape \\" + e);
                    }
                } else {
                    sb.append(c);
                }
            }
            throw new JSONException("unterminated string");
        }

        Object readValue() throws JSONException {
            if (eof()) throw new JSONException("unexpected EOF");
            char c = s.charAt(i);
            if (c == '"') return readString();
            if (s.startsWith("true", i)) { i += 4; return Boolean.TRUE; }
            if (s.startsWith("false", i)) { i += 5; return Boolean.FALSE; }
            if (s.startsWith("null", i)) { i += 4; return NULL; }
            if (c == '-' || (c >= '0' && c <= '9')) {
                int start = i++;
                boolean decimal = false;
                while (i < s.length()) {
                    char ch = s.charAt(i);
                    if (ch >= '0' && ch <= '9') {
                        i++;
                    } else if (ch == '.' || ch == 'e' || ch == 'E' || ch == '+' || ch == '-') {
                        decimal = true;
                        i++;
                    } else {
                        break;
                    }
                }
                String raw = s.substring(start, i);
                return decimal ? Double.valueOf(raw) : Long.valueOf(raw);
            }
            throw new JSONException("unexpected char '" + c + "'");
        }
    }
}
