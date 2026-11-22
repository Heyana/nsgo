package android.content.pm;
import android.content.Intent;
import java.util.List;
public class PackageManager {
    public static final int MATCH_DEFAULT_ONLY = 0x10000;
    public boolean claimed = true;
    public List<ResolveInfo> queryIntentActivities(Intent intent, int flags) {
        return claimed ? List.of(new ResolveInfo()) : List.of();
    }
}
