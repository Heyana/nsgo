package android.content.res;

import java.io.ByteArrayInputStream;
import java.io.IOException;
import java.io.InputStream;

public class AssetManager {
    public AssetFileDescriptor openFd(String fileName) throws IOException {
        return new AssetFileDescriptor();
    }

    public InputStream open(String fileName) throws IOException {
        return new ByteArrayInputStream(new byte[0]);
    }
}
