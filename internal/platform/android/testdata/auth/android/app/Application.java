package android.app;
import android.os.Bundle;
import java.util.ArrayList;
import java.util.List;
public class Application {
    public interface ActivityLifecycleCallbacks {
        void onActivityCreated(Activity a, Bundle b);
        void onActivityStarted(Activity a);
        void onActivityResumed(Activity a);
        void onActivityPaused(Activity a);
        void onActivityStopped(Activity a);
        void onActivitySaveInstanceState(Activity a, Bundle b);
        void onActivityDestroyed(Activity a);
    }
    public final List<ActivityLifecycleCallbacks> callbacks = new ArrayList<>();
    public void registerActivityLifecycleCallbacks(ActivityLifecycleCallbacks c) { callbacks.add(c); }
    public void unregisterActivityLifecycleCallbacks(ActivityLifecycleCallbacks c) { callbacks.remove(c); }
    public void pause(Activity a) { for (var c : List.copyOf(callbacks)) c.onActivityPaused(a); }
    public void resume(Activity a) { for (var c : List.copyOf(callbacks)) c.onActivityResumed(a); }
    public void destroy(Activity a) { for (var c : List.copyOf(callbacks)) c.onActivityDestroyed(a); }
}
