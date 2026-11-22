package dev.oneo.nsgo;

import java.io.BufferedInputStream;
import java.io.BufferedOutputStream;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.MalformedURLException;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Base64;
import java.util.Collections;
import java.util.HashMap;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;

/**
 * Pure-Java helpers for WebNode's downloads: filename sanitization, data: and base64 decoding, request-header and Referer construction, redirect-safe HTTP fetching, and cancellation tracking when a web view closes. Kept free of android.* imports so host JVM tests exercise the exact code packaged into the runtime.
 */
final class WebDownload {
    static final int MAX_REDIRECTS = 10;

    private WebDownload() {}

    /** Looks up the Cookie header value for url, or null for none. */
    interface CookieLookup {
        String cookieFor(String url);
    }

    /** Tracks active HTTP download connections for one WebNode so destroy() cancels them promptly. */
    static final class Tracker {
        private volatile boolean cancelled;
        private final Set<HttpURLConnection> active = Collections.newSetFromMap(new ConcurrentHashMap<>());

        boolean isCancelled() {
            return cancelled;
        }

        boolean track(HttpURLConnection connection) {
            active.add(connection);
            if (cancelled) {
                active.remove(connection);
                connection.disconnect();
                return false;
            }
            return true;
        }

        void untrack(HttpURLConnection connection) {
            if (connection != null) active.remove(connection);
        }

        void cancelAll() {
            cancelled = true;
            for (HttpURLConnection connection : new ArrayList<>(active)) {
                active.remove(connection);
                connection.disconnect();
            }
        }
    }

    /** Returns the lowercase RFC 3986 scheme of url, or "" when none. */
    static String schemeOf(String url) {
        if (url == null) return "";
        int colon = url.indexOf(':');
        if (colon <= 0) return "";
        for (int i = 0; i < colon; i++) {
            char c = url.charAt(i);
            boolean ok = (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
                    || (i > 0 && ((c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.'));
            if (!ok) return "";
        }
        return url.substring(0, colon).toLowerCase(Locale.ROOT);
    }

    /** Strips any path prefix and keeps the name inside its download directory. */
    static String sanitizeName(String raw) {
        if (raw == null || raw.isEmpty()) return "download";
        int slash = Math.max(raw.lastIndexOf('/'), raw.lastIndexOf('\\'));
        String name = slash >= 0 ? raw.substring(slash + 1) : raw;
        if (name.isEmpty() || ".".equals(name) || "..".equals(name)) return "download";
        return name;
    }

    /** The name a data: or blob: download saves as: the page's name, else "download" with extension. */
    static String localName(String pageName, String extension) {
        if (pageName != null && !pageName.isEmpty()) {
            int slash = Math.max(pageName.lastIndexOf('/'), pageName.lastIndexOf('\\'));
            String name = slash >= 0 ? pageName.substring(slash + 1) : pageName;
            if (!name.isEmpty() && !".".equals(name) && !"..".equals(name)) return name;
        }
        return extension == null || extension.isEmpty() ? "download" : "download." + extension;
    }

    /** The media type a data: URL names, without its parameters, or "". */
    static String dataType(String url) {
        if (url == null) return "";
        int comma = url.indexOf(',');
        if (comma < 5 || !url.regionMatches(true, 0, "data:", 0, 5)) return "";
        String type = url.substring(5, comma);
        int semicolon = type.indexOf(';');
        return (semicolon >= 0 ? type.substring(0, semicolon) : type).trim();
    }

    /** Decodes a data: URL's bytes (RFC 2397): percent-encoded, then base64 when it says so. */
    static byte[] decodeDataURL(String url) throws IOException {
        if (url == null) throw new IOException("Malformed data: URL.");
        int comma = url.indexOf(',');
        if (!url.regionMatches(true, 0, "data:", 0, 5) || comma < 0) {
            throw new IOException("Malformed data: URL.");
        }
        boolean base64 = false;
        for (String parameter : url.substring(5, comma).split(";")) {
            if (parameter.trim().equalsIgnoreCase("base64")) base64 = true;
        }
        byte[] bytes = percentDecode(url.substring(comma + 1));
        return base64 ? decodeBase64(bytes) : bytes;
    }

    /** Decodes standard base64 text, ignoring ASCII whitespace as MIME/data URLs may wrap lines. */
    static byte[] decodeBase64(String text) {
        if (text == null) throw new IllegalArgumentException("Missing base64 data.");
        return decodeBase64(text.getBytes(StandardCharsets.US_ASCII));
    }

    static byte[] decodeBase64(byte[] input) {
        byte[] compact = new byte[input.length];
        int n = 0;
        for (byte b : input) {
            if (b == ' ' || b == '\t' || b == '\r' || b == '\n') continue;
            compact[n++] = b;
        }
        byte[] trimmed = n == compact.length ? compact : java.util.Arrays.copyOf(compact, n);
        return Base64.getDecoder().decode(trimmed);
    }

    private static byte[] percentDecode(String s) {
        byte[] in = s.getBytes(StandardCharsets.UTF_8);
        ByteArrayOutputStream out = new ByteArrayOutputStream(in.length);
        for (int i = 0; i < in.length; i++) {
            if (in[i] == '%' && i + 2 < in.length) {
                int high = Character.digit(in[i + 1], 16), low = Character.digit(in[i + 2], 16);
                if (high >= 0 && low >= 0) {
                    out.write(high << 4 | low);
                    i += 2;
                    continue;
                }
            }
            out.write(in[i]);
        }
        return out.toByteArray();
    }

    /** Reports whether a and b are http(s) URLs with the same scheme, host and effective port. */
    static boolean sameOrigin(String a, String b) {
        URL x = parseHTTP(a), y = parseHTTP(b);
        return x != null && y != null
                && x.getProtocol().equalsIgnoreCase(y.getProtocol())
                && x.getHost().equalsIgnoreCase(y.getHost())
                && effectivePort(x) == effectivePort(y);
    }

    /**
     * The fields a download's own request sends, as the page's would have: the user agent, the cookies, the view's Headers when the download is on its source's origin, so they reach no other host, and a Referer as browsers send one by default (strict-origin-when-cross-origin): the page without its fragment and user name on its own origin, only its origin elsewhere, and none from https to http.
     */
    static Map<String, String> requestHeaders(String url, String src, Map<String, String> appHeaders,
            String userAgent, String cookies, String page) {
        Map<String, String> request = new HashMap<>();
        if (appHeaders != null && sameOrigin(url, src)) request.putAll(appHeaders);
        if (userAgent != null && !userAgent.isEmpty()) request.put("User-Agent", userAgent);
        if (cookies != null && !cookies.isEmpty()) request.put("Cookie", cookies);
        String referer = refererHeader(url, page);
        if (referer != null) request.put("Referer", referer);
        return request;
    }

    static String refererHeader(String targetUrl, String pageUrl) {
        URL from = parseHTTP(pageUrl);
        if (from == null) return null;
        String scheme = from.getProtocol().toLowerCase(Locale.ROOT);
        if (scheme.equals("https") && schemeOf(targetUrl).equals("http")) return null;
        String origin = scheme + "://" + from.getHost() + (from.getPort() >= 0 ? ":" + from.getPort() : "");
        if (!sameOrigin(targetUrl, pageUrl)) return origin + "/";
        String path = from.getPath();
        String query = from.getQuery();
        return origin + (path != null ? path : "") + (query != null ? "?" + query : "");
    }

    private static URL parseHTTP(String raw) {
        if (raw == null || raw.isEmpty()) return null;
        String scheme = schemeOf(raw);
        if (!scheme.equals("http") && !scheme.equals("https")) return null;
        try {
            URL u = new URL(raw);
            return u.getHost() != null && !u.getHost().isEmpty() ? u : null;
        } catch (MalformedURLException e) {
            return null;
        }
    }

    private static int effectivePort(URL u) {
        if (u.getPort() >= 0) return u.getPort();
        return "https".equalsIgnoreCase(u.getProtocol()) ? 443 : 80;
    }

    /**
     * Fetches an http(s) download into partial and atomically renames it to path, following http(s) redirects while keeping app Headers on src's origin and recomputing Cookie and Referer per hop. Returns bytes written, or -1 if tracker cancelled the download.
     */
    static long fetch(String url, String src, Map<String, String> appHeaders, String userAgent, String page,
            CookieLookup cookies, File partial, File path, Tracker tracker) throws IOException {
        HttpURLConnection connection = null;
        String current = url;
        long bytes = 0;
        try {
            for (int redirects = 0; ; redirects++) {
                if (tracker != null && tracker.isCancelled()) return -1;
                String cookie = cookies != null ? cookies.cookieFor(current) : null;
                Map<String, String> request = requestHeaders(current, src, appHeaders, userAgent, cookie, page);
                connection = (HttpURLConnection) new URL(current).openConnection();
                if (tracker != null && !tracker.track(connection)) {
                    connection = null;
                    return -1;
                }
                connection.setUseCaches(false);
                connection.setInstanceFollowRedirects(false);
                for (Map.Entry<String, String> field : request.entrySet()) {
                    connection.setRequestProperty(field.getKey(), field.getValue());
                }
                int status = connection.getResponseCode();
                if (tracker != null && tracker.isCancelled()) return -1;
                if (status == 301 || status == 302 || status == 303 || status == 307 || status == 308) {
                    String location = connection.getHeaderField("Location");
                    if (location == null || location.isEmpty()) throw new IOException("HTTP " + status);
                    if (redirects >= MAX_REDIRECTS) throw new IOException("Too many redirects.");
                    URL next = new URL(new URL(current), location);
                    String nextScheme = schemeOf(next.toString());
                    if (!nextScheme.equals("http") && !nextScheme.equals("https")) {
                        throw new IOException("Unsupported redirect scheme: " + nextScheme);
                    }
                    if (tracker != null) tracker.untrack(connection);
                    connection.disconnect();
                    connection = null;
                    current = next.toString();
                    continue;
                }
                if (status < 200 || status >= 300) throw new IOException("HTTP " + status);
                try (InputStream input = new BufferedInputStream(connection.getInputStream());
                        OutputStream output = new BufferedOutputStream(new FileOutputStream(partial))) {
                    byte[] buffer = new byte[64 << 10];
                    for (int n; (n = input.read(buffer)) >= 0; ) {
                        if (tracker != null && tracker.isCancelled()) return -1;
                        if (n == 0) continue;
                        output.write(buffer, 0, n);
                        bytes += n;
                    }
                }
                if (tracker != null && tracker.isCancelled()) return -1;
                if (!partial.renameTo(path)) throw new IOException("Cannot finish the download.");
                return bytes;
            }
        } finally {
            if (tracker != null) tracker.untrack(connection);
            if (connection != null) connection.disconnect();
            partial.delete();
        }
    }

    static String errorMessage(Throwable t) {
        String message = t.getMessage();
        return message != null && !message.isEmpty() ? message : t.getClass().getSimpleName();
    }
}
