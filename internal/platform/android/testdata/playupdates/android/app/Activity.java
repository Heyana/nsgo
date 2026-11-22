package android.app;
public class Activity extends android.content.Context {
    public boolean finishing, destroyed, focused = true;
    public boolean isFinishing() { return finishing; }
    public boolean isDestroyed() { return destroyed; }
    public boolean hasWindowFocus() { return focused; }
    public android.content.Context getApplicationContext() { return this; }
}
