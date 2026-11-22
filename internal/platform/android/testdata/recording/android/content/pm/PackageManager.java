package android.content.pm;
import android.content.*;
public class PackageManager {
    public static final int PERMISSION_GRANTED = 0;
    public static boolean declared = true;
    public static class NameNotFoundException extends Exception {}
    public ServiceInfo getServiceInfo(ComponentName c, int flags) throws NameNotFoundException {
        if (!declared) throw new NameNotFoundException(); return new ServiceInfo();
    }
    public Intent getLaunchIntentForPackage(String name) { return new Intent(); }
}
