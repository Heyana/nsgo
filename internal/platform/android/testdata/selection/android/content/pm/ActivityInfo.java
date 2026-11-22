package android.content.pm;

public class ActivityInfo {
    public String packageName;
    public String name;
    public boolean exported;
    public String permission;

    public ActivityInfo(String packageName, String name, boolean exported, String permission) {
        this.packageName = packageName;
        this.name = name;
        this.exported = exported;
        this.permission = permission;
    }
}
