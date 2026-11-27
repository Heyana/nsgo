package android.net;

public class Uri {
    private final String raw;
    private Uri(String raw) { this.raw = raw; }
    public static Uri parse(String s) { return new Uri(s); }
    public String getScheme() {
        int i = raw.indexOf(':');
        return i > 0 ? raw.substring(0, i) : null;
    }
}
