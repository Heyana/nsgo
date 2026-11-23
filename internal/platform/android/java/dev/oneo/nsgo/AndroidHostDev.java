package dev.oneo.nsgo;

import android.content.pm.ApplicationInfo;
import android.net.LocalServerSocket;
import android.net.LocalSocket;
import android.util.Log;

import java.io.BufferedReader;
import java.io.DataInputStream;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.io.InputStreamReader;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;

/** Accepts the host Go process from adb for a dev-only APK. */
final class AndroidHostDev implements AutoCloseable {
    private final NsgoActivity activity;
    private final LocalServerSocket server;
    private final byte[] token;
    private volatile boolean closed;

    static AndroidHostDev open(NsgoActivity activity) {
        if ((activity.getApplicationInfo().flags & ApplicationInfo.FLAG_DEBUGGABLE) == 0) return null;
        try (BufferedReader asset = new BufferedReader(new InputStreamReader(activity.getAssets().open("__nsgo_android_host")))) {
            String endpoint = asset.readLine();
            String token = asset.readLine();
            if (endpoint == null || !endpoint.matches("nsgo-dev-[a-f0-9]{32}")
                    || token == null || !token.matches("[a-f0-9]{32}")) {
                throw new IOException("invalid development socket configuration");
            }
            return new AndroidHostDev(activity, endpoint, token.getBytes(StandardCharsets.US_ASCII));
        } catch (FileNotFoundException absent) {
            return null;
        } catch (IOException error) {
            throw new IllegalStateException("cannot open the development socket", error);
        }
    }

    private AndroidHostDev(NsgoActivity activity, String endpoint, byte[] token) throws IOException {
        this.activity = activity;
        this.token = token;
        server = new LocalServerSocket(endpoint);
        Thread reader = new Thread(this::accept, "nsgo-dev-socket");
        reader.setDaemon(true);
        reader.start();
    }

    private void accept() {
        while (!closed) {
            LocalSocket socket;
            try {
                socket = server.accept();
            } catch (IOException error) {
                if (!closed) Log.e("nsgo", "development socket stopped accepting connections", error);
                return;
            }
            try {
                if (authorized(socket)) {
                    socket.getOutputStream().write(0xa7);
                    activity.runOnUiThread(() -> {
                        if (closed) closeSocket(socket);
                        else activity.showHostEngine(socket);
                    });
                    continue;
                }
            } catch (IOException error) {
                if (!closed) Log.i("nsgo", "development process disconnected before attaching", error);
            }
            closeSocket(socket);
        }
    }

    private boolean authorized(LocalSocket socket) throws IOException {
        int uid = socket.getPeerCredentials().getUid();
        // adbd runs as shell (UID 2000), or root on an adb-rooted emulator.
        if (uid != 2000 && uid != 0) return false;
        byte[] presented = new byte[token.length];
        socket.setSoTimeout(1000);
        new DataInputStream(socket.getInputStream()).readFully(presented);
        socket.setSoTimeout(0);
        return MessageDigest.isEqual(token, presented);
    }

    static void closeSocket(LocalSocket socket) {
        try { socket.close(); } catch (IOException ignored) {}
    }

    @Override
    public void close() {
        closed = true;
        try { server.close(); } catch (IOException ignored) {}
    }
}
