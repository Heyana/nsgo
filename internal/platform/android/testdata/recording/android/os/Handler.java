package android.os;
public final class Handler { public Handler(Looper l) {} public boolean post(Runnable r) { r.run(); return true; } }
