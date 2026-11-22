package android.content;

import android.content.pm.PackageManager;

public class Context {
    public PackageManager pm = new PackageManager();
    public int selfPermission = PackageManager.PERMISSION_GRANTED;

    public PackageManager getPackageManager() { return pm; }
    public String getPackageName() { return "dev.oneo.nsgo.test"; }
    public int checkSelfPermission(String perm) { return selfPermission; }
}
