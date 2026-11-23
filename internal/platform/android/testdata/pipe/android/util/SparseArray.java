package android.util;

import java.util.HashMap;

public final class SparseArray<E> {
    private final HashMap<Integer, E> values = new HashMap<>();

    public E get(int key) { return values.get(key); }
    public void put(int key, E value) { values.put(key, value); }
    public void remove(int key) { values.remove(key); }
}
