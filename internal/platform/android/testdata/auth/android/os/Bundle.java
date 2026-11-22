package android.os;
import java.util.HashMap;
public class Bundle extends HashMap<String, Object> {
    public void putBinder(String key, Object value) { put(key, value); }
}
