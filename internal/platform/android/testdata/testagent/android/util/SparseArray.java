package android.util;

import java.util.HashMap;
import java.util.Map;

public class SparseArray<E> {
    private final Map<Integer, E> map = new HashMap<>();
    public E get(int key) { return map.get(key); }
    public void put(int key, E val) { map.put(key, val); }
}
