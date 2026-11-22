package org.json;

public class JSONObject {
    public JSONObject put(String name, Object value) throws JSONException {
        return this;
    }

    public static String quote(String data) {
        return "\"" + data + "\"";
    }
}
