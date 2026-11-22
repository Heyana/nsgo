package org.json;

import java.util.LinkedHashMap;
import java.util.Map;

/** Minimal org.json.JSONObject parser/builder for flat expo-secure-store payloads in JVM tests. */
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

    public JSONObject put(String key, Object value) {
        map.put(key, value == null ? NULL : value);
        return this;
    }

    public Object opt(String key) {
        return map.get(key);
    }

    public String getString(String key) throws JSONException {
        Object v = map.get(key);
        if (v instanceof String s) return s;
        throw new JSONException("missing string " + key);
    }

    public String optString(String key, String fallback) {
        Object v = map.get(key);
        if (v == null || v == NULL) return fallback;
        return v.toString();
    }

    public int getInt(String key) throws JSONException {
        Object v = map.get(key);
        if (v instanceof Number n) return n.intValue();
        throw new JSONException("missing int " + key);
    }

    public boolean optBoolean(String key, boolean fallback) {
        Object v = map.get(key);
        if (v instanceof Boolean b) return b;
        return fallback;
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
            Object v = e.getValue();
            if (v == null || v == NULL) {
                sb.append("null");
            } else if (v instanceof String s) {
                appendString(sb, s);
            } else {
                sb.append(v);
            }
        }
        return sb.append('}').toString();
    }

    private static void appendString(StringBuilder sb, String s) {
        sb.append('"');
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            switch (c) {
                case '"' -> sb.append("\\\"");
                case '\\' -> sb.append("\\\\");
                case '\n' -> sb.append("\\n");
                case '\r' -> sb.append("\\r");
                case '\t' -> sb.append("\\t");
                default -> sb.append(c);
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
                        case '"', '\\', '/' -> sb.append(e);
                        case 'n' -> sb.append('\n');
                        case 'r' -> sb.append('\r');
                        case 't' -> sb.append('\t');
                        case 'b' -> sb.append('\b');
                        case 'f' -> sb.append('\f');
                        case 'u' -> {
                            if (i + 4 > s.length()) throw new JSONException("bad unicode escape");
                            sb.append((char) Integer.parseInt(s.substring(i, i + 4), 16));
                            i += 4;
                        }
                        default -> throw new JSONException("bad escape \\" + e);
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
                while (i < s.length() && ((s.charAt(i) >= '0' && s.charAt(i) <= '9'))) i++;
                return Integer.parseInt(s.substring(start, i));
            }
            throw new JSONException("unexpected char '" + c + "'");
        }
    }
}
