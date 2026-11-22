package android.app;
import android.content.Context;
public class Notification {
    public static final String CATEGORY_SERVICE = "service";
    public static final int FOREGROUND_SERVICE_IMMEDIATE = 1;
    public static final class Builder {
        public Builder(Context c, String channel) {}
        public Builder setSmallIcon(int icon) { return this; }
        public Builder setContentTitle(CharSequence s) { return this; }
        public Builder setContentText(CharSequence s) { return this; }
        public Builder setCategory(String s) { return this; }
        public Builder setOngoing(boolean b) { return this; }
        public Builder setContentIntent(PendingIntent p) { return this; }
        public Builder setForegroundServiceBehavior(int b) { return this; }
        public Notification build() { return new Notification(); }
    }
}
