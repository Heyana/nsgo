package dev.oneo.nsgo;

import android.content.Context;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.os.Bundle;
import android.util.Log;
import org.json.JSONObject;

import java.io.File;
import java.io.IOException;
import java.io.PrintWriter;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.StandardCopyOption;

/** Keeps Java crashes in the release app's files/logs/crashes, shared with Go. */
final class CrashReports {
    private static boolean installed;

    static synchronized void install(Context context) {
        if (installed) return;
        installed = true;
        try {
            PackageInfo info = context.getPackageManager().getPackageInfo(context.getPackageName(), PackageManager.GET_META_DATA);
            Bundle metadata = info.applicationInfo.metaData;
            String build = metadata == null ? null : metadata.getString("dev.oneo.nsgo.crash_report_build");
            if (build == null) return;
            JSONObject header = new JSONObject();
            header.put("platform", "android");
            header.put("kind", "java");
            header.put("version", info.versionName == null ? "" : info.versionName);
            header.put("build", build);
            String line = header.toString();
            File directory = new File(context.getFilesDir(), "logs/crashes");
            Thread.UncaughtExceptionHandler previous = Thread.getDefaultUncaughtExceptionHandler();
            Thread.setDefaultUncaughtExceptionHandler((thread, error) -> {
                try {
                    record(directory, line, thread, error);
                } catch (Exception failure) {
                    Log.e("nsgo", "cannot record uncaught Java exception", failure);
                } finally {
                    if (previous != null) previous.uncaughtException(thread, error);
                }
            });
        } catch (Exception error) {
            Log.e("nsgo", "cannot install Java crash reporting", error);
        }
    }

    private static void record(File directory, String header, Thread thread, Throwable error) throws IOException {
        if (!directory.isDirectory() && !directory.mkdirs() && !directory.isDirectory()) {
            throw new IOException("cannot create crash directory");
        }
        File tmp = File.createTempFile(".report-", ".tmp", directory);
        try {
            try (PrintWriter out = new PrintWriter(Files.newBufferedWriter(tmp.toPath(), StandardCharsets.UTF_8))) {
                out.println(header);
                out.println("Uncaught exception on " + thread.getName());
                error.printStackTrace(out);
                if (out.checkError()) throw new IOException("cannot write crash trace");
            }
            String name = tmp.getName();
            File report = new File(directory, name.substring(1, name.length() - 4) + ".log");
            Files.move(tmp.toPath(), report.toPath(), StandardCopyOption.ATOMIC_MOVE);
        } finally {
            tmp.delete();
        }
    }
}
