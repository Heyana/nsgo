package android.content;

import android.content.pm.PackageManager;
import android.content.res.TypedArray;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

public class Context {
    public PackageManager packageManager;
    public String packageName = "dev.oneo.nsgo.test";
    public final Set<String> grantedPermissions = new HashSet<>();
    public final List<Intent> started = new ArrayList<>();
    public RuntimeException startFailure;

    public TypedArray obtainStyledAttributes(int[] attrs) {
        return new TypedArray();
    }

    public <T> T getSystemService(Class<T> service) {
        throw new UnsupportedOperationException("no Android services on the JVM");
    }

    public PackageManager getPackageManager() {
        return packageManager;
    }

    public String getPackageName() {
        return packageName;
    }

    public int checkSelfPermission(String permission) {
        return grantedPermissions.contains(permission)
                ? PackageManager.PERMISSION_GRANTED
                : PackageManager.PERMISSION_DENIED;
    }

    public void startActivity(Intent intent) {
        if (startFailure != null) throw startFailure;
        started.add(intent);
    }
}
