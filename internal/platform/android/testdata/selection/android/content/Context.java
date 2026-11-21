package android.content;

import android.content.res.TypedArray;

public class Context {
    public TypedArray obtainStyledAttributes(int[] attrs) {
        return new TypedArray();
    }

    public <T> T getSystemService(Class<T> service) {
        throw new UnsupportedOperationException("no Android services on the JVM");
    }
}
