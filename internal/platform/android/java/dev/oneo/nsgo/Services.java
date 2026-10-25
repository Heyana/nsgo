package dev.oneo.nsgo;

import android.app.Activity;
import android.content.ActivityNotFoundException;
import android.content.ClipData;
import android.content.ClipDescription;
import android.content.ClipboardManager;
import android.content.ContentResolver;
import android.content.Context;
import android.content.Intent;
import android.database.Cursor;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.ImageDecoder;
import android.media.ExifInterface;
import android.net.Uri;
import android.os.Build;
import android.os.ext.SdkExtensions;
import android.provider.MediaStore;
import android.provider.OpenableColumns;
import android.util.Base64;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.nio.ByteBuffer;
import java.util.ArrayList;
import java.util.List;

/**
 * The services NsgoView performs for the app beyond its views: the
 * clipboard's images and files, opening URLs and the photo picker, whose
 * images it prepares as Go's driver.PickRequest says.
 */
final class Services {
    private Services() {}

    // What a clipboard read asks for, as driver.ClipboardRead.
    static final int CLIPBOARD_TEXT = 0;
    static final int CLIPBOARD_HAS_TEXT = 1;
    static final int CLIPBOARD_IMAGE = 2;
    static final int CLIPBOARD_HAS_IMAGE = 3;
    static final int CLIPBOARD_FILES = 4;

    /** The largest PNG a clipboard read answers with, as driver.MaxClipboardImage. */
    private static final int MAX_CLIPBOARD_IMAGE = 48 << 20;

    /** The request code of the photo picker's activity. */
    static final int PICK_REQUEST = 0x6e73;

    static byte[] read(ContentResolver resolver, Uri uri) throws IOException {
        try (InputStream s = resolver.openInputStream(uri)) {
            if (s == null) throw new IOException("no stream for " + uri);
            ByteArrayOutputStream b = new ByteArrayOutputStream();
            byte[] buf = new byte[65536];
            for (int n; (n = s.read(buf)) > 0; ) b.write(buf, 0, n);
            return b.toByteArray();
        }
    }

    /**
     * Answers a clipboard read: the text, or in data "image", the PNG's
     * size and its base64, or "uri" lines of local files' file: URLs; and
     * in finished whether it has text or an image. Asking whether it has
     * one reads only the clip's description, so Android shows no paste
     * notice.
     */
    static void readClipboard(Context context, ClipboardManager cm, int what, String[] text, String[] data,
            boolean[] has) {
        ClipDescription d = cm == null ? null : cm.getPrimaryClipDescription();
        switch (what) {
            case CLIPBOARD_HAS_TEXT:
                has[0] = d != null && (d.hasMimeType("text/*") || d.hasMimeType(ClipDescription.MIMETYPE_TEXT_URILIST));
                return;
            case CLIPBOARD_HAS_IMAGE:
                has[0] = d != null && d.hasMimeType("image/*");
                return;
        }
        ClipData clip = cm == null ? null : cm.getPrimaryClip();
        if (clip == null) return;
        StringBuilder out = new StringBuilder();
        for (int i = 0; i < clip.getItemCount(); i++) {
            ClipData.Item item = clip.getItemAt(i);
            Uri uri = item.getUri();
            if (what == CLIPBOARD_TEXT) {
                CharSequence t = item.coerceToText(context);
                if (t != null) text[0] = t.toString();
                return;
            }
            if (what == CLIPBOARD_FILES && uri != null && "file".equals(uri.getScheme())) {
                out.append("uri\t").append(uri).append('\n');
            }
            if (what == CLIPBOARD_IMAGE && uri != null) {
                String mime = context.getContentResolver().getType(uri);
                if (mime == null || !mime.startsWith("image/")) continue;
                try {
                    byte[] bytes = read(context.getContentResolver(), uri);
                    Bitmap bitmap = BitmapFactory.decodeByteArray(bytes, 0, bytes.length);
                    if (bitmap == null) continue;
                    ByteArrayOutputStream png = new ByteArrayOutputStream();
                    bitmap.compress(Bitmap.CompressFormat.PNG, 100, png);
                    if (png.size() > MAX_CLIPBOARD_IMAGE) return;
                    out.append("image\t").append(bitmap.getWidth()).append('\t').append(bitmap.getHeight()).append('\t')
                            .append(Base64.encodeToString(png.toByteArray(), Base64.NO_WRAP));
                    break;
                } catch (IOException | SecurityException e) {
                    // An image the app may not read is none.
                }
            }
        }
        data[0] = out.toString();
    }

    /**
     * Puts the image bytes encode on the clipboard: a PNG in the app's
     * cache that ClipProvider serves, which the clipboard lets the app
     * pasting it read.
     */
    static void setClipboardImage(Context context, ClipboardManager cm, byte[] bytes) {
        Bitmap bitmap = BitmapFactory.decodeByteArray(bytes, 0, bytes.length);
        if (bitmap == null || cm == null) return;
        File dir = new File(context.getCacheDir(), ClipProvider.DIR);
        dir.mkdirs();
        File[] old = dir.listFiles();
        if (old != null) for (File f : old) f.delete();
        File file = new File(dir, "image-" + System.currentTimeMillis() + ".png");
        try (FileOutputStream s = new FileOutputStream(file)) {
            bitmap.compress(Bitmap.CompressFormat.PNG, 100, s);
        } catch (IOException e) {
            return;
        }
        Uri uri = new Uri.Builder().scheme("content").authority(ClipProvider.authority(context))
                .appendPath(file.getName()).build();
        cm.setPrimaryClip(ClipData.newUri(context.getContentResolver(), "Image", uri));
    }

    /** Opens url in the app that handles it; false when none does. */
    static boolean openURL(Context context, String url) {
        Intent intent = new Intent(Intent.ACTION_VIEW, Uri.parse(url));
        if (!(context instanceof Activity)) intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        try {
            context.startActivity(intent);
            return true;
        } catch (ActivityNotFoundException | SecurityException e) {
            return false;
        }
    }

    /** A pick the photo picker answers: Go's request and how to prepare its images. */
    static final class Pick {
        final long count;
        final int limit, maxDimension;
        final float quality;
        final boolean current;

        Pick(long count, int limit, int maxDimension, float quality, boolean current) {
            this.count = count;
            this.limit = Math.max(limit, 1);
            this.maxDimension = maxDimension;
            this.quality = quality;
            this.current = current;
        }
    }

    /**
     * Starts the system's photo picker, which needs no permission: the
     * Photo Picker where the system has it, else the documents UI for
     * images. It reports false when no activity can pick.
     */
    static boolean startPick(Activity activity, Pick p) {
        Intent intent;
        boolean photoPicker = Build.VERSION.SDK_INT >= 33
                || Build.VERSION.SDK_INT >= 30 && SdkExtensions.getExtensionVersion(Build.VERSION_CODES.R) >= 2;
        if (photoPicker) {
            intent = new Intent(MediaStore.ACTION_PICK_IMAGES);
            if (p.limit > 1) intent.putExtra(MediaStore.EXTRA_PICK_IMAGES_MAX, Math.min(p.limit, MediaStore.getPickImagesMaxLimit()));
        } else {
            intent = new Intent(Intent.ACTION_OPEN_DOCUMENT);
            intent.addCategory(Intent.CATEGORY_OPENABLE);
            intent.setType("image/*");
            intent.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, p.limit > 1);
        }
        try {
            activity.startActivityForResult(intent, PICK_REQUEST);
            return true;
        } catch (ActivityNotFoundException e) {
            return false;
        }
    }

    /** The images a picker's result names, at most the pick's limit. */
    static List<Uri> picked(Intent result, Pick p) {
        List<Uri> uris = new ArrayList<>();
        if (result == null) return uris;
        ClipData clip = result.getClipData();
        if (clip != null) {
            for (int i = 0; i < clip.getItemCount() && uris.size() < p.limit; i++) {
                if (clip.getItemAt(i).getUri() != null) uris.add(clip.getItemAt(i).getUri());
            }
        } else if (result.getData() != null) {
            uris.add(result.getData());
        }
        return uris;
    }

    /**
     * Reads and prepares the images picked as the pick asks, as Go's
     * PickRequest.Encoding decides: lines of the name, type, width,
     * height, modification time in milliseconds (0: unknown) and bytes in
     * base64.
     */
    static String prepare(Context context, List<Uri> uris, Pick p) {
        ContentResolver resolver = context.getContentResolver();
        StringBuilder out = new StringBuilder();
        for (Uri uri : uris) {
            try {
                byte[] bytes = read(resolver, uri);
                String mime = resolver.getType(uri);
                if (mime == null) mime = "";
                mime = mime.toLowerCase();
                String name = "image";
                long modified = 0;
                try (Cursor c = resolver.query(uri, null, null, null, null)) {
                    if (c != null && c.moveToFirst()) {
                        int n = c.getColumnIndex(OpenableColumns.DISPLAY_NAME);
                        if (n >= 0 && c.getString(n) != null) name = c.getString(n);
                        int m = c.getColumnIndex(MediaStore.MediaColumns.DATE_MODIFIED);
                        if (m >= 0) modified = c.getLong(m) * 1000;
                    }
                } catch (RuntimeException e) {
                    // A provider without these columns leaves the defaults.
                }
                BitmapFactory.Options bounds = new BitmapFactory.Options();
                bounds.inJustDecodeBounds = true;
                BitmapFactory.decodeByteArray(bytes, 0, bytes.length, bounds);
                int w = bounds.outWidth, h = bounds.outHeight;
                if (turned(bytes)) {
                    int t = w;
                    w = h;
                    h = t;
                }
                boolean png = mime.equals("image/png"), jpeg = mime.equals("image/jpeg");
                boolean foreign = !(jpeg || png || mime.equals("image/gif") || mime.equals("image/webp"));
                boolean scale = p.maxDimension > 0 && Math.max(w, h) > p.maxDimension;
                boolean encode = scale || !p.current && foreign || p.quality > 0 && jpeg;
                if (encode) {
                    final int max = p.maxDimension;
                    Bitmap bitmap = ImageDecoder.decodeBitmap(ImageDecoder.createSource(ByteBuffer.wrap(bytes)),
                            (decoder, info, source) -> {
                                decoder.setAllocator(ImageDecoder.ALLOCATOR_SOFTWARE);
                                int iw = info.getSize().getWidth(), ih = info.getSize().getHeight();
                                int longer = Math.max(iw, ih);
                                if (max > 0 && longer > max) {
                                    decoder.setTargetSize(Math.max(1, Math.round((float) iw * max / longer)),
                                            Math.max(1, Math.round((float) ih * max / longer)));
                                }
                            });
                    ByteArrayOutputStream b = new ByteArrayOutputStream();
                    boolean asPNG = png && scale;
                    float quality = p.quality > 0 ? Math.min(p.quality, 1f) : 0.9f;
                    bitmap.compress(asPNG ? Bitmap.CompressFormat.PNG : Bitmap.CompressFormat.JPEG,
                            Math.round(quality * 100), b);
                    bytes = b.toByteArray();
                    mime = asPNG ? "image/png" : "image/jpeg";
                    w = bitmap.getWidth();
                    h = bitmap.getHeight();
                }
                out.append(name.replace('\t', ' ').replace('\n', ' ')).append('\t').append(mime).append('\t')
                        .append(w).append('\t').append(h).append('\t').append(modified).append('\t')
                        .append(Base64.encodeToString(bytes, Base64.NO_WRAP)).append('\n');
            } catch (IOException | RuntimeException e) {
                // An image that cannot be read or decoded is left out.
            }
        }
        return out.toString();
    }

    /** Whether an image's EXIF orientation turns it a quarter. */
    private static boolean turned(byte[] bytes) {
        try {
            int o = new ExifInterface(new ByteArrayInputStream(bytes))
                    .getAttributeInt(ExifInterface.TAG_ORIENTATION, ExifInterface.ORIENTATION_NORMAL);
            return o >= ExifInterface.ORIENTATION_TRANSPOSE && o <= ExifInterface.ORIENTATION_ROTATE_270;
        } catch (IOException | RuntimeException e) {
            return false;
        }
    }
}
