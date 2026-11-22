package dev.oneo.nsgo;

import java.io.ByteArrayInputStream;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.io.RandomAccessFile;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.security.GeneralSecurityException;
import java.security.cert.CertificateException;
import java.security.cert.CertificateFactory;
import java.security.cert.X509Certificate;
import java.util.Map;
import java.util.regex.Matcher;
import java.util.regex.Pattern;
import javax.net.ssl.HttpsURLConnection;
import javax.net.ssl.SSLContext;
import javax.net.ssl.TrustManager;
import javax.net.ssl.X509TrustManager;

/** Streams a GET into a partial file. Only the service may publish the result. */
final class BackgroundTransfer {
    private static final Pattern RANGE = Pattern.compile("bytes ([0-9]+)-([0-9]+)/([0-9]+)");

    interface Progress {
        void update(long loaded, long total);
    }

    final File partial;
    private final String url, certificate;
    private final Map<String, String> headers;
    private String validator;
    volatile boolean stopped;

    BackgroundTransfer(File partial, String url, Map<String, String> headers, String certificate) {
        this.partial = partial;
        this.url = url;
        this.headers = headers;
        this.certificate = certificate;
    }

    /** Returns a failure description, or null when the file is ready or the service stopped this attempt. */
    String run(Progress progress) {
        long offset = validator == null ? 0 : partial.length();
        HttpURLConnection connection = null;
        try {
            connection = connect(offset);
            int status = connection.getResponseCode();
            if (status == 416 && offset > 0) {
                connection.disconnect();
                offset = 0;
                connection = connect(0);
                status = connection.getResponseCode();
            }
            long total = connection.getContentLengthLong();
            if (status == 206 && offset > 0) {
                total = rangeTotal(connection.getHeaderField("Content-Range"), offset, total);
                if (total < 0) return "the server returned an invalid byte range";
                String current = validator(connection);
                if (current != null && !validator.equals(current)) return "the server's file changed during the download";
            } else if (status == 200) {
                offset = 0;
            } else {
                return "the server returned HTTP " + status;
            }
            String encoding = connection.getContentEncoding();
            if (encoding != null && !encoding.equalsIgnoreCase("identity")) {
                return "the server encoded a byte-range download";
            }
            long loaded = offset;
            progress.update(loaded, total);
            if (stopped) return null;
            try (InputStream body = connection.getInputStream();
                    RandomAccessFile file = new RandomAccessFile(partial, "rw")) {
                file.setLength(offset);
                if (offset == 0) validator = validator(connection);
                file.seek(offset);
                byte[] buffer = new byte[128 * 1024];
                long last = System.nanoTime();
                while (!stopped) {
                    int size = body.read(buffer);
                    if (size == -1) break;
                    if (total >= 0 && size > total - loaded) {
                        return "the response exceeded its advertised length";
                    }
                    file.write(buffer, 0, size);
                    loaded += size;
                    long now = System.nanoTime();
                    if (now - last >= 250_000_000) {
                        progress.update(loaded, total);
                        last = now;
                    }
                }
                file.getFD().sync();
            }
            progress.update(loaded, total);
            return stopped || total < 0 || loaded == total ? null : "the response ended before its advertised length";
        } catch (GeneralSecurityException e) {
            return "the pinned certificate is invalid";
        } catch (IOException | RuntimeException e) {
            // A URL or headers can carry credentials. Persist the error class alone.
            return "download failed (" + e.getClass().getSimpleName() + ")";
        } finally {
            if (connection != null) connection.disconnect();
        }
    }

    private HttpURLConnection connect(long offset) throws IOException, GeneralSecurityException {
        HttpURLConnection connection = (HttpURLConnection) new URL(url).openConnection();
        connection.setInstanceFollowRedirects(false);
        connection.setUseCaches(false);
        connection.setConnectTimeout(15_000);
        connection.setReadTimeout(15_000);
        if (!certificate.isEmpty()) {
            HttpsURLConnection secure = (HttpsURLConnection) connection;
            secure.setSSLSocketFactory(pin(certificate).getSocketFactory());
        }
        for (Map.Entry<String, String> header : headers.entrySet()) {
            connection.setRequestProperty(header.getKey(), header.getValue());
        }
        connection.setRequestProperty("Accept-Encoding", "identity");
        if (offset > 0) {
            connection.setRequestProperty("Range", "bytes=" + offset + "-");
            connection.setRequestProperty("If-Range", validator);
        }
        return connection;
    }

    private static String validator(HttpURLConnection connection) {
        String etag = connection.getHeaderField("ETag");
        if (etag != null && etag.startsWith("\"")) return etag;
        return connection.getHeaderField("Last-Modified");
    }

    private static long rangeTotal(String value, long offset, long length) {
        if (value == null) return -1;
        Matcher match = RANGE.matcher(value);
        if (!match.matches()) return -1;
        try {
            long start = Long.parseLong(match.group(1)), end = Long.parseLong(match.group(2));
            long total = Long.parseLong(match.group(3));
            return start == offset && start <= end && end == total - 1 &&
                    (length < 0 || length == total - offset) ? total : -1;
        } catch (NumberFormatException e) {
            return -1;
        }
    }

    private static SSLContext pin(String pem) throws GeneralSecurityException {
        CertificateFactory factory = CertificateFactory.getInstance("X.509");
        X509Certificate expected = (X509Certificate) factory.generateCertificate(
                new ByteArrayInputStream(pem.getBytes(StandardCharsets.UTF_8)));
        X509TrustManager trust = new X509TrustManager() {
            @Override public void checkClientTrusted(X509Certificate[] chain, String auth) throws CertificateException {
                throw new CertificateException("client trust is unavailable");
            }

            @Override public void checkServerTrusted(X509Certificate[] chain, String auth) throws CertificateException {
                if (chain == null || chain.length == 0 || !expected.equals(chain[0])) {
                    throw new CertificateException("the server certificate differs from the pinned certificate");
                }
                chain[0].checkValidity();
            }

            @Override public X509Certificate[] getAcceptedIssuers() {
                return new X509Certificate[] {expected};
            }
        };
        SSLContext context = SSLContext.getInstance("TLS");
        context.init(null, new TrustManager[] {trust}, null);
        // HttpsURLConnection still verifies the hostname.
        return context;
    }
}
