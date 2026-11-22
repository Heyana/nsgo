package android.content;
import android.app.Activity;
import android.net.Uri;
import android.os.Bundle;
public class Intent {
    public static final String ACTION_VIEW = "view", CATEGORY_BROWSABLE = "browsable";
    public static final int FLAG_ACTIVITY_CLEAR_TOP = 0x04000000, FLAG_ACTIVITY_SINGLE_TOP = 0x20000000;
    public final Bundle extras = new Bundle();
    public Uri uri;
    public int flags;
    public Class<?> target;
    public Intent(String action, Uri uri) { this.uri = uri; }
    public Intent(Activity activity, Class<?> target) { this.target = target; }
    public Intent addCategory(String category) { return this; }
    public Intent setPackage(String pkg) { return this; }
    public Intent putExtras(Bundle bundle) { extras.putAll(bundle); return this; }
    public Intent putExtra(String key, boolean value) { extras.put(key, value); return this; }
    public Intent addFlags(int value) { flags |= value; return this; }
}
