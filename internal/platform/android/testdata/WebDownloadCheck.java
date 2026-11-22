package dev.oneo.nsgo;

import java.io.File;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.Arrays;
import java.util.Collections;
import java.util.HashMap;
import java.util.Map;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.AtomicReference;

/**
 * Exercises WebDownload's filename sanitization, data:/base64 decoding, Referer/header construction, HTTP fetch with same-origin and cross-origin redirects, and cancellation on the host JVM.
 */
public final class WebDownloadCheck {
    public static void main(String[] args) throws Exception {
        checkNames();
        checkDataURLs();
        checkOriginsAndHeaders();

        // args: <dir> <originA> <originB>
        if (args.length < 3) throw new AssertionError("expected <dir> <originA> <originB>");
        File dir = new File(args[0]);
        String originA = args[1];
        String originB = args[2];
        checkHTTPFetch(dir, originA, originB);
    }

    private static void checkNames() {
        eq("report.pdf", WebDownload.sanitizeName("report.pdf"), "plain name");
        eq("secret.txt", WebDownload.sanitizeName("../../secret.txt"), "unix traversal");
        eq("secret.txt", WebDownload.sanitizeName("..\\..\\secret.txt"), "windows traversal");
        eq("download", WebDownload.sanitizeName(".."), "dotdot");
        eq("download", WebDownload.sanitizeName("."), "dot");
        eq("download", WebDownload.sanitizeName(""), "empty");
        eq("download", WebDownload.sanitizeName(null), "null");
        eq("download", WebDownload.sanitizeName("/"), "slash");

        eq("notes.txt", WebDownload.localName("../notes.txt", "bin"), "page name wins");
        eq("download.pdf", WebDownload.localName("", "pdf"), "extension fallback");
        eq("download.pdf", WebDownload.localName("..", "pdf"), "dotdot falls back to extension");
        eq("download", WebDownload.localName("", ""), "empty fallback");
    }

    private static void checkDataURLs() throws Exception {
        eq("text/plain", WebDownload.dataType("data:text/plain;charset=utf-8;base64,aGk="), "dataType");
        eq("application/json", WebDownload.dataType("DATA:application/json,{\"a\":1}"), "dataType uppercase");
        eq("", WebDownload.dataType("https://example.com/a,b"), "non-data scheme");
        eq("", WebDownload.dataType("data:,hello"), "empty media type");

        eqBytes("hello world".getBytes(StandardCharsets.UTF_8),
                WebDownload.decodeDataURL("data:text/plain;base64,aGVs\nbG8g\r\nd29ybGQ="), "wrapped base64");
        eqBytes("a b+c%20".getBytes(StandardCharsets.UTF_8),
                WebDownload.decodeDataURL("data:text/plain,a%20b+c%2520"), "percent-encoded");
        eqBytes(new byte[0], WebDownload.decodeDataURL("data:,"), "empty data");

        boolean threw = false;
        try {
            WebDownload.decodeDataURL("https://example.com/not-data");
        } catch (IOException expected) {
            threw = true;
        }
        if (!threw) throw new AssertionError("expected IOException for non-data URL");

        threw = false;
        try {
            WebDownload.decodeDataURL("data:text/plain;base64,!!!not-base64!!!");
        } catch (IllegalArgumentException expected) {
            threw = true;
        }
        if (!threw) throw new AssertionError("expected IllegalArgumentException for invalid base64");

        eqBytes("blob bytes".getBytes(StandardCharsets.UTF_8),
                WebDownload.decodeBase64("Ymxv\nYiBieXRlcw=="), "decodeBase64");
    }

    private static void checkOriginsAndHeaders() {
        check(WebDownload.sameOrigin("https://Example.COM:443/a", "https://example.com/b"), "default https port");
        check(WebDownload.sameOrigin("http://example.com:80/a", "http://example.com/b"), "default http port");
        check(!WebDownload.sameOrigin("https://example.com/a", "http://example.com/a"), "different scheme");
        check(!WebDownload.sameOrigin("https://example.com:8443/a", "https://example.com/a"), "different port");
        check(!WebDownload.sameOrigin("data:text/plain,hi", "data:text/plain,hi"), "non-http");

        Map<String, String> appHeaders = Collections.singletonMap("Authorization", "Bearer secret");
        Map<String, String> same = WebDownload.requestHeaders(
                "https://app.example/dl/file.bin",
                "https://app.example/home",
                appHeaders,
                "TestUA/1.0",
                "sid=1",
                "https://user:pass@app.example/page?x=1#frag");
        eq("Bearer secret", same.get("Authorization"), "same-origin app header");
        eq("TestUA/1.0", same.get("User-Agent"), "user agent");
        eq("sid=1", same.get("Cookie"), "cookie");
        eq("https://app.example/page?x=1", same.get("Referer"), "same-origin referer strips userinfo and fragment");

        Map<String, String> cross = WebDownload.requestHeaders(
                "https://cdn.other.example/file.bin",
                "https://app.example/home",
                appHeaders,
                "TestUA/1.0",
                "cdn=2",
                "https://app.example/private/path?secret=1#frag");
        if (cross.containsKey("Authorization")) throw new AssertionError("leaked app header cross-origin");
        eq("https://app.example/", cross.get("Referer"), "cross-origin referer is origin only");

        Map<String, String> downgrade = WebDownload.requestHeaders(
                "http://other.example/file.bin",
                "https://app.example/home",
                appHeaders,
                "TestUA/1.0",
                "",
                "https://app.example/page");
        if (downgrade.containsKey("Referer")) throw new AssertionError("sent https referer to http target");
    }

    private static void checkHTTPFetch(File dir, String originA, String originB) throws Exception {
        Map<String, String> appHeaders = new HashMap<>();
        appHeaders.put("X-App-Token", "secret-token");
        WebDownload.CookieLookup cookies = target -> target.startsWith(originA) ? "session=originA" : "session=originB";
        WebDownload.Tracker tracker = new WebDownload.Tracker();

        // 1. Direct same-origin download.
        File dst1 = new File(dir, "direct.txt");
        File part1 = new File(dir, ".direct.txt.part");
        long n1 = WebDownload.fetch(originA + "/file", originA + "/page", appHeaders, "NSGoTest/1",
                originA + "/page?q=1#hash", cookies, part1, dst1, tracker);
        String body1 = Files.readString(dst1.toPath(), StandardCharsets.UTF_8);
        eq(String.valueOf(body1.length()), String.valueOf(n1), "direct byte count");
        check(body1.contains("auth=secret-token"), "same-origin header sent: " + body1);
        check(body1.contains("cookie=session=originA"), "same-origin cookie sent: " + body1);
        check(body1.contains("ua=NSGoTest/1"), "ua sent: " + body1);
        check(body1.contains("referer=" + originA + "/page?q=1"), "same-origin referer: " + body1);
        check(!part1.exists(), "partial deleted after direct download");

        // 2. Same-origin redirect preserves app header and follows Location.
        File dst2 = new File(dir, "same-redirect.txt");
        File part2 = new File(dir, ".same-redirect.txt.part");
        long n2 = WebDownload.fetch(originA + "/redirect-same", originA + "/page", appHeaders, "NSGoTest/1",
                originA + "/page", cookies, part2, dst2, tracker);
        String body2 = Files.readString(dst2.toPath(), StandardCharsets.UTF_8);
        check(n2 == body2.length() && body2.contains("auth=secret-token"), "same-origin redirect keeps header: " + body2);

        // 3. Cross-origin redirect strips app header, switches cookie, trims Referer.
        File dst3 = new File(dir, "cross-redirect.txt");
        File part3 = new File(dir, ".cross-redirect.txt.part");
        long n3 = WebDownload.fetch(originA + "/redirect-cross", originA + "/page", appHeaders, "NSGoTest/1",
                originA + "/secret/path?k=v", cookies, part3, dst3, tracker);
        String body3 = Files.readString(dst3.toPath(), StandardCharsets.UTF_8);
        check(n3 == body3.length(), "cross-origin redirect byte count");
        check(body3.contains("auth=" + "\n") || body3.contains("auth=\n"), "cross-origin redirect strips app header: " + body3);
        check(body3.contains("cookie=session=originB"), "cross-origin redirect uses target cookie: " + body3);
        check(body3.contains("referer=" + originA + "/\n"), "cross-origin redirect trims referer: " + body3);

        // 4. Redirect to file: is refused.
        File dst4 = new File(dir, "bad-scheme.txt");
        File part4 = new File(dir, ".bad-scheme.txt.part");
        boolean refused = false;
        try {
            WebDownload.fetch(originA + "/redirect-file", originA + "/page", appHeaders, "NSGoTest/1",
                    originA + "/page", cookies, part4, dst4, tracker);
        } catch (IOException expected) {
            refused = expected.getMessage() != null && expected.getMessage().contains("Unsupported redirect scheme");
        }
        check(refused && !dst4.exists() && !part4.exists(), "file: redirect refused cleanly");

        // 5. HTTP 404 fails and leaves no files.
        File dst5 = new File(dir, "missing.txt");
        File part5 = new File(dir, ".missing.txt.part");
        boolean failed404 = false;
        try {
            WebDownload.fetch(originA + "/missing", originA + "/page", appHeaders, "NSGoTest/1",
                    originA + "/page", cookies, part5, dst5, tracker);
        } catch (IOException expected) {
            failed404 = "HTTP 404".equals(expected.getMessage());
        }
        check(failed404 && !dst5.exists() && !part5.exists(), "404 fails cleanly");

        // 6. Cancelling via Tracker aborts an in-flight slow stream and deletes partial.
        WebDownload.Tracker cancelTracker = new WebDownload.Tracker();
        File dst6 = new File(dir, "cancelled.bin");
        File part6 = new File(dir, ".cancelled.bin.part");
        CountDownLatch finished = new CountDownLatch(1);
        AtomicLong result = new AtomicLong(-99);
        AtomicReference<Throwable> err = new AtomicReference<>();
        Thread t = new Thread(() -> {
            try {
                result.set(WebDownload.fetch(originA + "/slow", originA + "/page", appHeaders, "NSGoTest/1",
                        originA + "/page", cookies, part6, dst6, cancelTracker));
            } catch (Throwable ex) {
                err.set(ex);
            } finally {
                finished.countDown();
            }
        });
        t.start();
        Thread.sleep(150);
        cancelTracker.cancelAll();
        check(finished.await(5, TimeUnit.SECONDS), "cancelled fetch stopped promptly");
        check(!dst6.exists() && !part6.exists(), "cancelled fetch left no files");
        // Either returned -1 cleanly or threw IOException from disconnect() while tracker is cancelled.
        check(result.get() == -1 || (err.get() instanceof IOException && cancelTracker.isCancelled()),
                "cancelled fetch outcome: result=" + result.get() + " err=" + err.get());
    }

    private static void eq(String want, String got, String what) {
        if (!want.equals(got)) throw new AssertionError(what + ": got " + got + ", want " + want);
    }

    private static void eqBytes(byte[] want, byte[] got, String what) {
        if (!Arrays.equals(want, got)) {
            throw new AssertionError(what + ": got " + Arrays.toString(got) + ", want " + Arrays.toString(want));
        }
    }

    private static void check(boolean cond, String what) {
        if (!cond) throw new AssertionError(what);
    }
}
