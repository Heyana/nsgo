package android.content.pm;

public class ResolveInfo {
    public ActivityInfo activityInfo;
    private final CharSequence label;

    public ResolveInfo(ActivityInfo activityInfo, CharSequence label) {
        this.activityInfo = activityInfo;
        this.label = label;
    }

    public CharSequence loadLabel(PackageManager pm) {
        return label;
    }
}
