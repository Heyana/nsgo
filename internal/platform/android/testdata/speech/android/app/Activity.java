package android.app;

public class Activity extends android.content.Context {
    public String[] requestedPermissions;
    public int requestedCode;
    public int requestCalls;

    public void requestPermissions(String[] permissions, int requestCode) {
        this.requestedPermissions = permissions;
        this.requestedCode = requestCode;
        this.requestCalls++;
    }
}
