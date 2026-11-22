package android.webkit;

import android.content.Context;
import android.view.ViewGroup;
import java.util.Map;

public class WebView extends ViewGroup {
    private final WebSettings settings = new WebSettings();
    public WebViewClient client;

    public WebView(Context context) {
        super(context);
    }

    public WebSettings getSettings() {
        return settings;
    }

    public void addJavascriptInterface(Object object, String name) {}

    public void removeJavascriptInterface(String name) {}

    public void setWebViewClient(WebViewClient client) {
        this.client = client;
    }

    public void setWebChromeClient(WebChromeClient client) {}

    public void setDownloadListener(DownloadListener listener) {}

    public void evaluateJavascript(String script, ValueCallback<String> resultCallback) {}

    public String getUrl() {
        return "";
    }

    public String getTitle() {
        return "";
    }

    public boolean canGoBack() {
        return false;
    }

    public boolean canGoForward() {
        return false;
    }

    public void loadDataWithBaseURL(String baseUrl, String data, String mimeType, String encoding, String historyUrl) {}

    public void loadUrl(String url, Map<String, String> additionalHttpHeaders) {}

    public void reload() {}

    public void goBack() {}

    public void goForward() {}

    public void stopLoading() {}

    public void destroy() {}
}
