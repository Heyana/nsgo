package android.net;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;

/** The JVM reader test serves child processes only, never a development socket. */
public class LocalSocket {
    public InputStream getInputStream() throws IOException { throw new IOException("no development socket"); }
    public OutputStream getOutputStream() throws IOException { throw new IOException("no development socket"); }
    public void close() throws IOException {}
}
