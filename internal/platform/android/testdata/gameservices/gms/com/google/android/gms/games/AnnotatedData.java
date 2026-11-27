package com.google.android.gms.games;

public final class AnnotatedData<T> {
    private final T data;

    public AnnotatedData(T data) {
        this.data = data;
    }

    public T get() {
        return data;
    }
}
