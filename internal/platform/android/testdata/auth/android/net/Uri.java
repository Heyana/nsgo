package android.net;
import java.net.URI;
public class Uri {
    private final URI uri;
    private Uri(String value) { uri = URI.create(value); }
    public static Uri parse(String value) { return new Uri(value); }
    public String getScheme() { return uri.getScheme(); }
    public String getHost() { return uri.getHost(); }
    public String toString() { return uri.toString(); }
}
