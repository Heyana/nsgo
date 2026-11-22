package android.content;

public class Context {
    public static final String CLIPBOARD_SERVICE = "clipboard";
    public static final String INPUT_METHOD_SERVICE = "input_method";
    public Object getSystemService(String name) { return null; }
    public ContentResolver getContentResolver() { return new ContentResolver(); }
}
