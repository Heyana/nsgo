package android.util;

import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.nio.file.Files;

/** Minimal AtomicFile stub backed by java.nio.file for JVM Secrets tests. */
public class AtomicFile {
    private final File file;

    public AtomicFile(File file) {
        this.file = file;
    }

    public byte[] readFully() throws IOException {
        return Files.readAllBytes(file.toPath());
    }

    public FileOutputStream startWrite() throws IOException {
        return new FileOutputStream(file);
    }

    public void finishWrite(FileOutputStream stream) throws IOException {
        stream.close();
    }

    public void failWrite(FileOutputStream stream) {
        try {
            stream.close();
        } catch (IOException ignored) {
        }
        file.delete();
    }

    public void delete() {
        file.delete();
    }
}
