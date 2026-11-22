package android.content;

import android.content.pm.PackageManager;

public class ContextWrapper extends Context {
    private final Context base;

    public ContextWrapper(Context base) {
        this.base = base;
    }

    public Context getBaseContext() {
        return base;
    }

    @Override
    public PackageManager getPackageManager() {
        return base.getPackageManager();
    }

    @Override
    public String getPackageName() {
        return base.getPackageName();
    }

    @Override
    public int checkSelfPermission(String permission) {
        return base.checkSelfPermission(permission);
    }

    @Override
    public void startActivity(Intent intent) {
        base.startActivity(intent);
    }
}
