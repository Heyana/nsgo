package dev.oneo.nsgo;

import android.content.Context;
import android.content.res.AssetFileDescriptor;
import android.media.MediaPlayer;
import android.net.Uri;

import java.io.File;
import java.io.FileInputStream;
import java.io.FileNotFoundException;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;

/** Resolves the audio package's sources on the media workers. Go stages its byte sources as files. */
final class AudioSource {
    static final class Limit extends IOException {
        Limit() { super("the sound has more than 8 MiB"); }
    }

    static boolean remote(String source) {
        String scheme = Uri.parse(source).getScheme();
        return "http".equalsIgnoreCase(scheme) || "https".equalsIgnoreCase(scheme);
    }

    private static String asset(String source) {
        return source.substring("asset:".length()).replaceFirst("^/+", "");
    }

    /** Gives an uncompressed asset's descriptor to MediaPlayer. An APK-compressed asset needs a seekable copy. */
    static void player(Context context, MediaPlayer player, String source) throws IOException {
        if (!source.startsWith("asset:")) {
            player.setDataSource(context, Uri.parse(source));
            return;
        }
        String path = asset(source);
        AssetFileDescriptor file;
        try {
            file = context.getAssets().openFd(path);
        } catch (FileNotFoundException compressed) {
            File copy = soundFile(context, source, Long.MAX_VALUE);
            try (FileInputStream in = new FileInputStream(copy)) {
                player.setDataSource(in.getFD());
            } finally {
                copy.delete();
            }
            return;
        }
        try (AssetFileDescriptor fd = file) {
            player.setDataSource(fd);
        }
    }

    /** Reads at most limit plus one byte into the file the native decoders can seek. */
    static File soundFile(Context context, String source, long limit) throws IOException {
        File file = File.createTempFile("nsgo-sound-", null, context.getCacheDir());
        boolean copied = false;
        try {
            if (remote(source)) {
                download(source, file, limit);
            } else {
                try (InputStream in = open(context, source)) { copy(in, file, limit); }
            }
            copied = true;
            return file;
        } finally {
            if (!copied) file.delete();
        }
    }

    private static InputStream open(Context context, String source) throws IOException {
        if (source.startsWith("asset:")) return context.getAssets().open(asset(source));
        Uri uri = Uri.parse(source);
        if (uri.getScheme() == null) return new FileInputStream(source);
        InputStream in = context.getContentResolver().openInputStream(uri);
        if (in == null) throw new FileNotFoundException("cannot open the audio source");
        return in;
    }

    private static void download(String source, File file, long limit) throws IOException {
        HttpURLConnection connection = (HttpURLConnection) new URL(source).openConnection();
        try {
            connection.setConnectTimeout(15000);
            connection.setReadTimeout(15000);
            if (connection.getContentLengthLong() > limit) throw new Limit();
            try (InputStream in = connection.getInputStream()) { copy(in, file, limit); }
        } finally {
            connection.disconnect();
        }
    }

    private static void copy(InputStream in, File file, long limit) throws IOException {
        try (FileOutputStream out = new FileOutputStream(file)) {
            byte[] buffer = new byte[64 << 10];
            long left = limit;
            for (;;) {
                if (Thread.currentThread().isInterrupted()) throw new IOException("audio load cancelled");
                int n = in.read(buffer, 0, (int) Math.min(buffer.length, left == Long.MAX_VALUE ? left : left + 1));
                if (n < 0) return;
                if (n > left) throw new Limit();
                out.write(buffer, 0, n);
                left -= n;
            }
        }
    }
}
