package dev.oneo.nsgo;

import android.annotation.SuppressLint;
import android.content.Context;
import android.graphics.Bitmap;
import android.os.Handler;
import android.os.Looper;
import android.view.MotionEvent;
import android.view.ViewGroup;
import android.webkit.JavascriptInterface;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import java.io.DataInputStream;
import java.io.IOException;
import java.util.Objects;

/**
 * A web view node's android.webkit.WebView, filling the node's view, as
 * NSH's WebBox. Pages post to the app through window.nsgo.postMessage: the
 * JavaScript interface INTERFACE is there from the page's first script,
 * and the bridge Go puts first in the script before content wraps it to
 * pass any value as a string. The view loads its source again when it or
 * the scripts change.
 */
@SuppressLint({"SetJavaScriptEnabled", "AddJavascriptInterface", "ClickableViewAccessibility"})
final class WebNode {
    /** The JavaScript interface pages post to, as webInterface in android.go. */
    static final String INTERFACE = "nsgo";

    /** Receives a web view's events; navigation is null for messages and has only the URL for errors. UI thread. */
    interface Reporter {
        void report(int kind, String text, Navigation navigation);
    }

    /** A page and its history, as core.WebNavigation. */
    static final class Navigation {
        final String url, title;
        final boolean loading, canGoBack, canGoForward;

        Navigation(String url, String title, boolean loading, boolean canGoBack, boolean canGoForward) {
            this.url = url;
            this.title = title;
            this.loading = loading;
            this.canGoBack = canGoBack;
            this.canGoForward = canGoForward;
        }
    }

    final WebView view;
    private final Reporter reporter;
    private final Handler ui = new Handler(Looper.getMainLooper());
    private boolean configured;
    private String src = "", html = "", baseURL = "";
    private String injected = "", injectedBefore = "", userAgent = "";
    private boolean noScroll;

    WebNode(Context context, Reporter reporter) {
        this.reporter = reporter;
        view = new WebView(context);
        view.getSettings().setDomStorageEnabled(true);
        view.addJavascriptInterface(new Bridge(), INTERFACE);
        view.setWebViewClient(new Client());
        view.setOnTouchListener((v, e) -> {
            if (e.getActionMasked() == MotionEvent.ACTION_DOWN) {
                // The page scrolls itself: keep enclosing scroll views out.
                v.getParent().requestDisallowInterceptTouchEvent(true);
            }
            // Without moves the page cannot scroll; taps still reach it.
            return noScroll && e.getActionMasked() == MotionEvent.ACTION_MOVE;
        });
    }

    /** Called from the page on a binder thread. */
    private final class Bridge {
        @JavascriptInterface
        public void postMessage(String data) {
            ui.post(() -> reporter.report(Protocol.EVENT_MESSAGE, data != null ? data : "", null));
        }
    }

    private final class Client extends WebViewClient {
        @Override
        public void onPageStarted(WebView web, String url, Bitmap favicon) {
            // android.webkit cannot run a script at document start: only
            // androidx's WebViewCompat.addDocumentStartJavaScript can, which
            // the runtime does without. The script before content (the bridge,
            // then the app's) runs as the page starts loading instead, so a
            // page's earliest scripts may run before it.
            if (!injectedBefore.isEmpty()) web.evaluateJavascript(injectedBefore, null);
            state(Protocol.EVENT_LOAD_START, url, true);
        }

        @Override
        public void onPageFinished(WebView web, String url) {
            if (!injected.isEmpty()) web.evaluateJavascript(injected, null);
            state(Protocol.EVENT_LOAD, url, false);
        }

        @Override
        public void onReceivedError(WebView web, WebResourceRequest request, WebResourceError error) {
            if (!request.isForMainFrame()) return;
            Navigation nav = new Navigation(request.getUrl().toString(), "", false, false, false);
            reporter.report(Protocol.EVENT_ERROR, String.valueOf(error.getDescription()), nav);
        }
    }

    /** Reports the page's state as kind and as a navigation state change. */
    private void state(int kind, String url, boolean loading) {
        Navigation nav = new Navigation(url != null ? url : "", Objects.toString(view.getTitle(), ""),
                loading, view.canGoBack(), view.canGoForward());
        reporter.report(kind, "", nav);
        reporter.report(Protocol.EVENT_NAVIGATION_STATE_CHANGE, "", nav);
    }

    /** Applies an ItemWeb's fields, in the order android.go writes them. */
    void configure(DataInputStream in) throws IOException {
        String src = Protocol.readString(in), html = Protocol.readString(in), baseURL = Protocol.readString(in);
        String injected = Protocol.readString(in), injectedBefore = Protocol.readString(in);
        String userAgent = Protocol.readString(in);
        int flags = in.readUnsignedByte();
        boolean noScroll = Protocol.flag(flags, 0), noJavaScript = Protocol.flag(flags, 1);

        boolean reload = !configured || !src.equals(this.src) || !html.equals(this.html)
                || !baseURL.equals(this.baseURL) || !injected.equals(this.injected)
                || !injectedBefore.equals(this.injectedBefore);
        configured = true;
        this.src = src;
        this.html = html;
        this.baseURL = baseURL;
        this.injected = injected;
        this.injectedBefore = injectedBefore;
        this.noScroll = noScroll;

        view.setVerticalScrollBarEnabled(!noScroll);
        view.setHorizontalScrollBarEnabled(!noScroll);
        WebSettings settings = view.getSettings();
        settings.setJavaScriptEnabled(!noJavaScript);
        if (!userAgent.equals(this.userAgent)) {
            this.userAgent = userAgent;
            settings.setUserAgentString(userAgent.isEmpty() ? null : userAgent); // null: the default
        }
        if (reload) load();
    }

    private void load() {
        // Apps load their own files; pages from elsewhere may not.
        WebSettings settings = view.getSettings();
        if (!html.isEmpty()) {
            settings.setAllowFileAccess(baseURL.startsWith("file:"));
            view.loadDataWithBaseURL(baseURL.isEmpty() ? null : baseURL, html, "text/html", "UTF-8", null);
        } else if (!src.isEmpty()) {
            settings.setAllowFileAccess(src.startsWith("file:"));
            view.loadUrl(src);
        }
    }

    /** Does a core.WebCommand, with value for messages and scripts. */
    void command(int command, String value) {
        switch (command) {
            case Protocol.WEB_RELOAD: view.reload(); break;
            case Protocol.WEB_GO_BACK: view.goBack(); break;
            case Protocol.WEB_GO_FORWARD: view.goForward(); break;
            case Protocol.WEB_STOP_LOADING: view.stopLoading(); break;
            // Go writes the script dispatching a message as the value.
            case Protocol.WEB_POST_MESSAGE:
            case Protocol.WEB_INJECT_JAVASCRIPT:
                view.evaluateJavascript(value, null);
                break;
            default:
        }
    }

    /** Stops the page and frees the WebView. */
    void destroy() {
        view.stopLoading();
        // A WebView is destroyed out of the view hierarchy.
        if (view.getParent() instanceof ViewGroup) ((ViewGroup) view.getParent()).removeView(view);
        view.setWebViewClient(new WebViewClient());
        view.removeJavascriptInterface(INTERFACE);
        view.destroy();
    }
}
