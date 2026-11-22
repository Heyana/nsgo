package android.net;

import java.net.URI;

public final class Uri {
    private final String raw;
    private final URI uri;

    private Uri(String raw) {
        this.raw = raw;
        URI parsed = null;
        try {
            parsed = URI.create(raw);
        } catch (IllegalArgumentException ignored) {
        }
        this.uri = parsed;
    }

    public static Uri parse(String s) {
        return new Uri(s == null ? "" : s);
    }

    public String getScheme() {
        return uri != null ? uri.getScheme() : null;
    }

    public String getHost() {
        return uri != null ? uri.getHost() : null;
    }

    public int getPort() {
        return uri != null ? uri.getPort() : -1;
    }

    public String getEncodedPath() {
        return uri != null ? uri.getRawPath() : null;
    }

    public String getEncodedQuery() {
        return uri != null ? uri.getRawQuery() : null;
    }

    @Override
    public String toString() {
        return raw;
    }
}
