package android.app;
import android.content.ActivityNotFoundException;
import android.content.Intent;
import android.content.pm.PackageManager;
import java.util.ArrayList;
import java.util.List;
public class Activity {
    public final Application application = new Application();
    public final PackageManager packages = new PackageManager();
    public final List<Intent> launched = new ArrayList<>();
    public boolean finishing, destroyed, noBrowser, noCustomTabs, denied;
    public boolean isFinishing() { return finishing; }
    public boolean isDestroyed() { return destroyed; }
    public Application getApplication() { return application; }
    public PackageManager getPackageManager() { return packages; }
    public String getPackageName() { return "dev.nsgo.fixture"; }
    public void startActivity(Intent intent) {
        if (denied) throw new SecurityException();
        if (noBrowser || noCustomTabs && intent.extras.containsKey("android.support.customtabs.extra.SESSION")) throw new ActivityNotFoundException();
        launched.add(intent);
    }
}
