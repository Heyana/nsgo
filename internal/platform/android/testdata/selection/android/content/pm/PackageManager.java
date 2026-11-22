package android.content.pm;

import android.content.Intent;
import java.util.Collections;
import java.util.List;

public class PackageManager {
    public static final int PERMISSION_GRANTED = 0;
    public static final int PERMISSION_DENIED = -1;

    public Intent lastQuery;
    public int lastFlags;
    private final List<ResolveInfo> activities;

    public PackageManager(List<ResolveInfo> activities) {
        this.activities = activities;
    }

    public List<ResolveInfo> queryIntentActivities(Intent intent, int flags) {
        lastQuery = intent;
        lastFlags = flags;
        return activities == null ? Collections.emptyList() : activities;
    }
}
