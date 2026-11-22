package org.json;

import java.util.ArrayList;
import java.util.List;

public class JSONArray {
    private final List<Object> items = new ArrayList<>();
    public int length() { return items.size(); }
    public String getString(int i) throws JSONException { return String.valueOf(items.get(i)); }
    public JSONObject getJSONObject(int i) throws JSONException { return (JSONObject) items.get(i); }
}
