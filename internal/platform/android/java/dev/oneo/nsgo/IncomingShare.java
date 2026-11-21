package dev.oneo.nsgo;

import android.content.ClipData;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.Bundle;
import android.os.Process;
import android.util.Log;
import java.io.IOException;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;

/** Copies the content granted by another app while the receiving activity holds its URI permission. */
final class IncomingShare {
    private static final String TAG = "NsgoShare";
    // Keep intents in arrival order and leave provider I/O off the UI thread.
    private static final ExecutorService worker = Executors.newSingleThreadExecutor();

    static void receive(Context context, NsgoView view, Intent intent, String mime) {
        List<String> texts = new ArrayList<>();
        List<Uri> streams = new ArrayList<>();
        try {
            Bundle extras = intent.getExtras();
            if (extras != null) {
                Object text = extras.get(Intent.EXTRA_TEXT);
                Object stream = extras.get(Intent.EXTRA_STREAM);
                if (text instanceof List<?>) {
                    for (Object item : (List<?>) text) addText(texts, item);
                } else {
                    addText(texts, text);
                }
                if (stream instanceof List<?>) {
                    for (Object item : (List<?>) stream) addStream(streams, item);
                } else {
                    addStream(streams, stream);
                }
            }
            ClipData clip = intent.getClipData();
            if (clip != null) {
                boolean needText = texts.isEmpty(), needStreams = streams.isEmpty();
                for (int i = 0; i < clip.getItemCount(); i++) {
                    ClipData.Item item = clip.getItemAt(i);
                    if (needText) addText(texts, item.getText());
                    if (needStreams) addStream(streams, item.getUri());
                }
            }
        } catch (RuntimeException e) {
            Log.w(TAG, "invalid incoming share", e);
            return;
        }
        if (!texts.isEmpty() || !streams.isEmpty()) {
            worker.execute(() -> copyAndDeliver(context, view, mime, texts, streams));
        }
    }

    private static void addText(List<String> texts, Object item) {
        if (item instanceof CharSequence && ((CharSequence) item).length() != 0) texts.add(item.toString());
    }

    private static void addStream(List<Uri> streams, Object item) {
        // A sender must grant access through a content provider. A file:
        // URI could point into the receiving app's own private files.
        if (item instanceof Uri && "content".equals(((Uri) item).getScheme())) streams.add((Uri) item);
    }

    private static void copyAndDeliver(Context context, NsgoView view, String mime,
            List<String> texts, List<Uri> streams) {
        JSONArray items = new JSONArray();
        try {
            for (String text : texts) items.put(new JSONObject().put("Text", text).put("MIME", mime));
            streams.removeIf(uri -> !granted(context, uri));
            if (!streams.isEmpty()) {
                try {
                    for (Services.CachedFile file : Services.copyToCache(context, streams, "share")) {
                        items.put(new JSONObject().put("Path", file.path).put("Name", file.name)
                                .put("MIME", file.mime.isEmpty() ? mime : file.mime));
                    }
                } catch (IOException e) {
                    Log.w(TAG, "cannot read the shared files", e);
                }
            }
        } catch (JSONException e) {
            throw new AssertionError(e);
        }
        if (items.length() != 0) view.post(() -> view.receiveShare(items.toString()));
    }

    private static boolean granted(Context context, Uri uri) {
        String authority = uri.getAuthority();
        if (authority == null || authority.isEmpty()) return false;
        // A provider authority can start with a user ID when shared across profiles.
        if (FileProvider.authority(context).equals(authority.substring(authority.lastIndexOf('@') + 1))) return false;
        // Do not rely on the app's own provider permissions to read a sender's URI.
        try {
            return context.checkUriPermission(uri, Process.myPid(), Process.myUid(),
                    Intent.FLAG_GRANT_READ_URI_PERMISSION) == PackageManager.PERMISSION_GRANTED;
        } catch (RuntimeException e) {
            return false;
        }
    }
}
