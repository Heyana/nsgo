package android.content.pm;

public class PackageManager {
    public static final int PERMISSION_GRANTED = 0;
    public static final int PERMISSION_DENIED = -1;
    public static final int GET_PERMISSIONS = 4096;

    public static class NameNotFoundException extends Exception {}

    public PackageInfo info = new PackageInfo();

    public PackageInfo getPackageInfo(String pkg, int flags) throws NameNotFoundException {
        return info;
    }
}
