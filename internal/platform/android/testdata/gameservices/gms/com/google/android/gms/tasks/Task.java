package com.google.android.gms.tasks;

public final class Task<T> {
    private final boolean successful;
    private final boolean canceled;
    private final T result;
    private final Exception exception;

    private Task(boolean successful, boolean canceled, T result, Exception exception) {
        this.successful = successful;
        this.canceled = canceled;
        this.result = result;
        this.exception = exception;
    }

    public static <T> Task<T> forResult(T result) {
        return new Task<>(true, false, result, null);
    }

    public static <T> Task<T> forException(Exception exception) {
        return new Task<>(false, false, null, exception);
    }

    public static <T> Task<T> forCanceled() {
        return new Task<>(false, true, null, null);
    }

    public boolean isSuccessful() {
        return successful;
    }

    public boolean isCanceled() {
        return canceled;
    }

    public T getResult() {
        return result;
    }

    public Exception getException() {
        return exception;
    }

    public Task<T> addOnCompleteListener(OnCompleteListener<T> listener) {
        // Exercise Proxy's Object methods just like the real Tasks implementation may.
        if (listener.toString() == null || !listener.equals(listener) || listener.hashCode() == 0 && false) {
            throw new AssertionError("broken proxy");
        }
        listener.onComplete(this);
        return this;
    }
}
