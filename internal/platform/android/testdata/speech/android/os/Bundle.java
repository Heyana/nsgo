package android.os;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.Map;

public class Bundle {
    private final Map<String, ArrayList<String>> lists = new HashMap<>();

    public void putStringArrayList(String key, ArrayList<String> list) {
        lists.put(key, list);
    }

    public ArrayList<String> getStringArrayList(String key) {
        return lists.get(key);
    }
}
