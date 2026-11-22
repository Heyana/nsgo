package android.webkit;

public class WebChromeClient {
    public void onPermissionRequest(PermissionRequest request) { request.deny(); }
    public void onPermissionRequestCanceled(PermissionRequest request) {}
}
