package dev.oneo.nsgo;

import android.content.Context;
import android.net.Uri;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebViewClient;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.util.ArrayList;
import java.util.List;
import java.util.function.Consumer;

/**
 * Exercises WebNode.Client.onReceivedError and shouldOverrideUrlLoading on the
 * JVM and writes each reported error in NsgoView.event's wire format so Go's
 * readEvent decodes the exact bytes Java produces.
 */
public final class WebErrorCheck {
    private static final int NODE_ID = 77;

    public static void main(String[] args) throws Exception {
        List<byte[]> frames = new ArrayList<>();
        WebNode.Reporter reporter = new WebNode.Reporter() {
            @Override
            public void report(int kind, String text, WebNode.Navigation navigation) {
                if (kind == Protocol.EVENT_ERROR) {
                    throw new AssertionError("WebNode.Client should route errors through Reporter.error");
                }
            }

            @Override
            public void error(String text, WebNode.Navigation nav, String domain, int code) {
                frames.add(encodeEvent(Protocol.EVENT_ERROR, NODE_ID, text, code, nav, domain != null ? domain : ""));
            }

            @Override
            public void message(String data, String url) {}

            @Override
            public void download(String data) {}

            @Override
            public void askPermissions(String[] permissions, Consumer<Boolean> answer) {
                answer.accept(false);
            }
        };

        WebNode node = new WebNode(new Context(), reporter);
        configureRestricted(node, "allowed.example");
        WebViewClient client = node.view.client;
        if (client == null) {
            throw new AssertionError("WebNode did not install a WebViewClient");
        }

        // Main-frame WebResourceError callbacks carry domain and code.
        client.onReceivedError(node.view, request("https://missing.invalid/", true),
                error(WebViewClient.ERROR_HOST_LOOKUP, "net::ERR_NAME_NOT_RESOLVED"));
        client.onReceivedError(node.view, request("https://closed.invalid:8443/", true),
                error(WebViewClient.ERROR_CONNECT, "net::ERR_CONNECTION_REFUSED"));
        client.onReceivedError(node.view, request("https://slow.invalid/", true),
                error(WebViewClient.ERROR_TIMEOUT, "net::ERR_TIMED_OUT"));
        client.onReceivedError(node.view, request("https://tls.invalid/", true),
                error(WebViewClient.ERROR_FAILED_SSL_HANDSHAKE, "net::ERR_SSL_PROTOCOL_ERROR"));
        client.onReceivedError(node.view, request("file:///nonexistent.html", true),
                error(WebViewClient.ERROR_FILE_NOT_FOUND, "net::ERR_FILE_NOT_FOUND"));
        client.onReceivedError(node.view, request("https://unsafe.invalid/", true),
                error(WebViewClient.ERROR_UNSAFE_RESOURCE, "net::ERR_BLOCKED_BY_CLIENT"));
        client.onReceivedError(node.view, request("https://empty-desc.invalid/", true),
                error(WebViewClient.ERROR_UNKNOWN, null));

        // Subframe errors are ignored.
        client.onReceivedError(node.view, request("https://subframe.invalid/img.png", false),
                error(WebViewClient.ERROR_HOST_LOOKUP, "net::ERR_NAME_NOT_RESOLVED"));

        // Navigation policy checks: disallowed main-frame reports an error with empty domain and code 0.
        if (!client.shouldOverrideUrlLoading(node.view, request("https://evil.invalid/page", true))) {
            throw new AssertionError("expected disallowed main-frame URL to be refused");
        }
        if (client.shouldOverrideUrlLoading(node.view, request("https://allowed.example/ok", true))) {
            throw new AssertionError("expected allowed main-frame URL to be permitted");
        }
        if (client.shouldOverrideUrlLoading(node.view, request("https://evil.invalid/frame", false))) {
            throw new AssertionError("expected subframe navigation not to be refused");
        }

        DataOutputStream out = new DataOutputStream(System.out);
        out.writeInt(frames.size());
        for (byte[] frame : frames) {
            out.writeInt(frame.length);
            out.write(frame);
        }
        out.flush();
    }

    private static void configureRestricted(WebNode node, String hosts) throws IOException {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream d = new DataOutputStream(bytes);
        Protocol.writeString(d, ""); // src
        Protocol.writeString(d, ""); // html
        Protocol.writeString(d, ""); // baseURL
        Protocol.writeString(d, ""); // injected
        Protocol.writeString(d, ""); // injectedBefore
        Protocol.writeString(d, ""); // userAgent
        Protocol.writeString(d, ""); // userAgentSuffix
        d.writeByte(1 << 2); // restricted = true
        Protocol.writeString(d, hosts);
        Protocol.writeString(d, ""); // schemes
        Protocol.writeString(d, ""); // downloadDir
        d.writeInt(0); // headerCount
        node.configure(new DataInputStream(new ByteArrayInputStream(bytes.toByteArray())));
    }

    private static WebResourceRequest request(String url, boolean mainFrame) {
        return new WebResourceRequest() {
            @Override
            public Uri getUrl() {
                return Uri.parse(url);
            }

            @Override
            public boolean isForMainFrame() {
                return mainFrame;
            }
        };
    }

    private static WebResourceError error(int code, CharSequence description) {
        return new WebResourceError() {
            @Override
            public int getErrorCode() {
                return code;
            }

            @Override
            public CharSequence getDescription() {
                return description;
            }
        };
    }

    /** Matches NsgoView.event's exact binary layout. */
    private static byte[] encodeEvent(int kind, int id, String text, int index, WebNode.Navigation nav, String data) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeInt(0); // surface
            d.writeByte(kind);
            d.writeInt(id);
            d.writeInt(0); // count
            d.writeFloat(0); // x
            d.writeFloat(0); // y
            Protocol.writeString(d, text);
            d.writeFloat(0); // width
            d.writeFloat(0); // height
            d.writeInt(index);
            d.writeByte(0); // phase
            d.writeFloat(0); // vx
            d.writeFloat(0); // vy
            d.writeFloat(0); // value
            if (nav == null) {
                d.writeByte(0);
            } else {
                d.writeByte(1 | (nav.loading ? 2 : 0) | (nav.canGoBack ? 4 : 0) | (nav.canGoForward ? 8 : 0));
                Protocol.writeString(d, nav.url);
                Protocol.writeString(d, nav.title);
            }
            d.writeLong(0); // time
            d.writeLong(0); // duration
            d.writeByte(0); // buffering
            Protocol.writeString(d, data);
            d.writeByte(0); // finished
            d.writeFloat(0); // pageX
            d.writeFloat(0); // pageY
            d.writeFloat(0); // positionX
            d.writeFloat(0); // positionY
            d.writeByte(0); // mods
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        return bytes.toByteArray();
    }
}
