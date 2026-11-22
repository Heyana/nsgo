package android.app;
import android.content.*;
public final class PendingIntent {
    public static final int FLAG_IMMUTABLE = 1;
    public static PendingIntent getActivity(Context c, int request, Intent i, int flags) { return new PendingIntent(); }
}
