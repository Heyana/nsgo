package android.app;
public final class NotificationManager {
    public static final int IMPORTANCE_LOW = 2;
    public static final NotificationManager instance = new NotificationManager();
    private NotificationChannel channel;
    public NotificationChannel getNotificationChannel(String id) { return channel; }
    public void createNotificationChannel(NotificationChannel c) { channel = c; }
}
