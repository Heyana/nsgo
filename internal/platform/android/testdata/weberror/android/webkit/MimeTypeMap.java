package android.webkit;

public class MimeTypeMap {
    private static final MimeTypeMap INSTANCE = new MimeTypeMap();

    public static MimeTypeMap getSingleton() {
        return INSTANCE;
    }

    public String getExtensionFromMimeType(String mimeType) {
        return "";
    }
}
