package dev.oneo.nsgo;

import android.app.Activity;
import android.content.ActivityNotFoundException;
import android.content.ClipData;
import android.content.ClipDescription;
import android.content.ClipboardManager;
import android.content.ComponentName;
import android.content.ContentResolver;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.database.Cursor;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.ImageDecoder;
import android.media.ExifInterface;
import android.net.Uri;
import android.os.Build;
import android.os.ext.SdkExtensions;
import android.provider.DocumentsContract;
import android.provider.MediaStore;
import android.provider.OpenableColumns;
import android.util.Base64;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.DataOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.lang.reflect.Method;
import java.lang.reflect.Proxy;
import java.nio.ByteBuffer;
import java.nio.channels.FileChannel;
import java.util.ArrayList;
import java.util.List;

/**
 * The services NsgoView performs for the app beyond its views: the
 * clipboard's images and files, opening URLs, the photo picker, whose
 * images it prepares as Go's driver.PickRequest says, the documents UI's
 * file and folder picks and saves, reading the folders picked for Go, and
 * opening and sharing the app's files, which FileProvider serves.
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

    /** The request codes of the photo picker's, the file picker's, the save dialog's and the folder picker's activities. */
    static final int PICK_REQUEST = 0x6e73, FILES_REQUEST = 0x6e74, SAVE_REQUEST = 0x6e75, FOLDER_REQUEST = 0x6e76;

    /** The request codes of the camera's activity and of the camera permission's prompt. */
    static final int PHOTO_REQUEST = 0x6e77, CAMERA_PERMISSION_REQUEST = 0x6e78;

    /** The directory in the app's cache the camera writes photos to, which FileProvider lets it write. */
    static final String CAMERA_DIR = "nsgo-camera";

    static final String CAMERA_PERMISSION = "android.permission.CAMERA";

    /** The directory in the app's cache picked files are copied to, as Go's File.Temporary says. */
    static final String FILES_DIR = "nsgo-files";

    /** The directory in the app's cache the clipboard's image is kept in. */
    static final String CLIP_DIR = "nsgo-clip";

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
     * size and its base64 ("toolarge" and its bytes past the limit, "error"
     * and why for an image it cannot read), or "uri" lines of local files' file: URLs; and
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
                    if (bitmap == null) {
                        out.setLength(0);
                        out.append("error\tno decoder reads its ").append(mime);
                        continue;
                    }
                    ByteArrayOutputStream png = new ByteArrayOutputStream();
                    bitmap.compress(Bitmap.CompressFormat.PNG, 100, png);
                    out.setLength(0);
                    if (png.size() > MAX_CLIPBOARD_IMAGE) {
                        out.append("toolarge\t").append(png.size());
                        break;
                    }
                    out.append("image\t").append(bitmap.getWidth()).append('\t').append(bitmap.getHeight()).append('\t')
                            .append(Base64.encodeToString(png.toByteArray(), Base64.NO_WRAP));
                    break;
                } catch (IOException | SecurityException e) {
                    // An image the app may not read fails its read, unless a later item reads.
                    out.setLength(0);
                    out.append("error\t").append(String.valueOf(e.getMessage()).replace('\n', ' '));
                }
            }
        }
        data[0] = out.toString();
    }

    /**
     * Puts the image bytes encode on the clipboard: a PNG in the app's
     * cache that FileProvider serves, which the clipboard lets the app
     * pasting it read.
     */
    static void setClipboardImage(Context context, ClipboardManager cm, byte[] bytes) {
        Bitmap bitmap = BitmapFactory.decodeByteArray(bytes, 0, bytes.length);
        if (bitmap == null || cm == null) return;
        File dir = new File(context.getCacheDir(), CLIP_DIR);
        dir.mkdirs();
        File[] old = dir.listFiles();
        if (old != null) for (File f : old) f.delete();
        File file = new File(dir, "image-" + System.currentTimeMillis() + ".png");
        try (FileOutputStream s = new FileOutputStream(file)) {
            bitmap.compress(Bitmap.CompressFormat.PNG, 100, s);
        } catch (IOException e) {
            return;
        }
        Uri uri;
        try {
            uri = FileProvider.uri(context, file, file.getName(), "image/png");
        } catch (IOException e) {
            return;
        }
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

    /**
     * Opens the file at path in the app the system associates with mime,
     * which reads it as name through a URI granted to it; false when no
     * app opens it. error says what failed otherwise.
     */
    static boolean openFile(Context context, String path, String name, String mime, String[] error) {
        Intent intent = new Intent(Intent.ACTION_VIEW);
        try {
            Uri uri = path.startsWith("content://") ? contentURI(path) : FileProvider.uri(context, new File(path), name, mime);
            if (mime.isEmpty()) mime = contentType(context, uri);
            intent.setDataAndType(uri, mime);
        } catch (IOException e) {
            error[0] = e.getMessage();
            return false;
        }
        intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
        if (!(context instanceof Activity)) intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        try {
            context.startActivity(intent);
            return true;
        } catch (ActivityNotFoundException e) {
            return false;
        } catch (RuntimeException e) {
            error[0] = String.valueOf(e.getMessage());
            return false;
        }
    }

    /**
     * Shows the system's share sheet for text and the files at paths,
     * which the app chosen reads under names through URIs granted to it;
     * it returns what failed, "" once the sheet shows.
     */
    static String share(Context context, String text, String[] paths, String[] names, String[] mimes) {
        ArrayList<Uri> uris = new ArrayList<>();
        try {
            for (int i = 0; i < paths.length; i++) {
                uris.add(paths[i].startsWith("content://") ? contentURI(paths[i]) : FileProvider.uri(context, new File(paths[i]), names[i], mimes[i]));
                if (mimes[i].isEmpty()) mimes[i] = contentType(context, uris.get(i));
            }
        } catch (IOException e) {
            return e.getMessage();
        }
        Intent intent = new Intent(uris.size() > 1 ? Intent.ACTION_SEND_MULTIPLE : Intent.ACTION_SEND);
        intent.setType(uris.isEmpty() ? "text/plain" : commonType(mimes));
        if (!text.isEmpty()) intent.putExtra(Intent.EXTRA_TEXT, text);
        if (uris.size() == 1) intent.putExtra(Intent.EXTRA_STREAM, uris.get(0));
        else if (uris.size() > 1) intent.putParcelableArrayListExtra(Intent.EXTRA_STREAM, uris);
        if (!uris.isEmpty()) {
            // The clip carries the grants through the chooser to the app
            // chosen, and the sheet's preview of the files.
            ClipData clip = new ClipData(names[0], mimes, new ClipData.Item(uris.get(0)));
            for (int i = 1; i < uris.size(); i++) clip.addItem(new ClipData.Item(uris.get(i)));
            intent.setClipData(clip);
            intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
        }
        Intent chooser = Intent.createChooser(intent, null);
        if (!(context instanceof Activity)) chooser.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        try {
            context.startActivity(chooser);
            return "";
        } catch (RuntimeException e) {
            return String.valueOf(e.getMessage());
        }
    }

    private static Uri contentURI(String text) throws IOException {
        Uri uri = Uri.parse(text);
        if (!"content".equals(uri.getScheme()) || uri.getAuthority() == null) {
            throw new IOException("shared URI is not a content URI");
        }
        return uri;
    }

    private static String contentType(Context context, Uri uri) {
        String mime = context.getContentResolver().getType(uri);
        return mime == null || mime.isEmpty() ? "application/octet-stream" : mime;
    }

    /** The permission an app installing packages asks for in its manifest. */
    private static final String INSTALL_PERMISSION = "android.permission.REQUEST_INSTALL_PACKAGES";

    /**
     * Hands the APK at path to the package installer, which reads it as
     * name through a URI granted to it and asks the user to confirm; it
     * returns what failed, "" once the installer starts.
     */
    static String installPackage(Context context, String path, String name) {
        if (!asksFor(context, INSTALL_PERMISSION)) {
            return "the app's manifest does not ask for " + INSTALL_PERMISSION
                    + ": add it to nsgo.yaml's android.permissions, or an embedding app's manifest";
        }
        String mime = "application/vnd.android.package-archive";
        Intent intent = new Intent(Intent.ACTION_VIEW);
        try {
            intent.setDataAndType(FileProvider.uri(context, new File(path), name, mime), mime);
        } catch (IOException e) {
            return e.getMessage();
        }
        intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
        if (!(context instanceof Activity)) intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        try {
            context.startActivity(intent);
            return "";
        } catch (RuntimeException e) {
            return String.valueOf(e.getMessage());
        }
    }

    /** Whether the app's manifest asks for permission. */
    private static boolean asksFor(Context context, String permission) {
        try {
            String[] asked = context.getPackageManager()
                    .getPackageInfo(context.getPackageName(), android.content.pm.PackageManager.GET_PERMISSIONS)
                    .requestedPermissions;
            if (asked != null) for (String p : asked) if (permission.equals(p)) return true;
        } catch (android.content.pm.PackageManager.NameNotFoundException e) {
            // The app's own package is always there.
        }
        return false;
    }

    /** Opens the setting that lets the app install packages; false when it cannot. */
    static boolean installSettings(Context context) {
        Intent intent = new Intent(android.provider.Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,
                Uri.parse("package:" + context.getPackageName()));
        if (!(context instanceof Activity)) intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        try {
            context.startActivity(intent);
            return true;
        } catch (RuntimeException e) {
            return false;
        }
    }

    /** The type a share of files of types says: theirs, their family's (image/*) or any. */
    private static String commonType(String[] types) {
        String type = types[0];
        for (String t : types) {
            if (t.equals(type)) continue;
            String family = type.substring(0, type.indexOf('/') + 1);
            if (family.isEmpty() || !t.startsWith(family)) return "*/*";
            type = family + "*";
        }
        return type;
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

    /** Whether the device has a camera an app takes photos with. */
    static boolean hasCamera(Context context) {
        return context.getPackageManager().hasSystemFeature(android.content.pm.PackageManager.FEATURE_CAMERA_ANY);
    }

    /**
     * Whether the camera app must wait for the user to allow the camera:
     * an app whose manifest asks for CAMERA, and has not been granted it,
     * cannot start one. One that does not ask needs no permission.
     */
    static boolean needsCameraPermission(Context context) {
        if (context.checkSelfPermission(CAMERA_PERMISSION) == android.content.pm.PackageManager.PERMISSION_GRANTED) {
            return false;
        }
        try {
            String[] asked = context.getPackageManager()
                    .getPackageInfo(context.getPackageName(), android.content.pm.PackageManager.GET_PERMISSIONS)
                    .requestedPermissions;
            return asked != null && java.util.Arrays.asList(asked).contains(CAMERA_PERMISSION);
        } catch (android.content.pm.PackageManager.NameNotFoundException e) {
            return false;
        }
    }

    /**
     * Starts the camera app to take one photo into a new file of the
     * cache's nsgo-camera, which it writes through FileProvider, and
     * returns the file; it fails when no activity takes photos.
     */
    static File startPhoto(Activity activity) throws IOException {
        File dir = new File(activity.getCacheDir(), CAMERA_DIR);
        if (!dir.isDirectory() && !dir.mkdirs()) throw new IOException("cannot make " + dir);
        String name = new java.text.SimpleDateFormat("'IMG_'yyyyMMdd_HHmmss", java.util.Locale.ROOT)
                .format(new java.util.Date());
        File photo = new File(dir, name + ".jpg");
        for (int n = 2; !photo.createNewFile(); n++) photo = new File(dir, name + "_" + n + ".jpg");
        Uri uri = photoURI(activity, photo);
        Intent intent = new Intent(MediaStore.ACTION_IMAGE_CAPTURE);
        intent.putExtra(MediaStore.EXTRA_OUTPUT, uri);
        // The clip carries the grant to camera apps that read it from there.
        intent.setClipData(ClipData.newRawUri("", uri));
        intent.addFlags(Intent.FLAG_GRANT_WRITE_URI_PERMISSION | Intent.FLAG_GRANT_READ_URI_PERMISSION);
        try {
            activity.startActivityForResult(intent, PHOTO_REQUEST);
        } catch (ActivityNotFoundException | SecurityException e) {
            photo.delete();
            throw new IOException("no camera app takes photos", e);
        }
        return photo;
    }

    /** The URI the camera app writes photo through, and the app reads it back from. */
    static Uri photoURI(Context context, File photo) throws IOException {
        return FileProvider.uri(context, photo, photo.getName(), "image/jpeg");
    }

    /** The images a picker's result names, at most the pick's limit. */
    static List<Uri> picked(Intent result, Pick p) {
        return picked(result, p.limit);
    }

    /** The documents a picker's result names, at most limit. */
    static List<Uri> picked(Intent result, int limit) {
        List<Uri> uris = new ArrayList<>();
        if (result == null) return uris;
        ClipData clip = result.getClipData();
        if (clip != null) {
            for (int i = 0; i < clip.getItemCount() && uris.size() < limit; i++) {
                if (clip.getItemAt(i).getUri() != null) uris.add(clip.getItemAt(i).getUri());
            }
        } else if (result.getData() != null) {
            uris.add(result.getData());
        }
        return uris;
    }

    /** A file or folder pick or a save the documents UI answers: Go's request number and what it asked for. */
    static final class Documents {
        final long count;
        final int limit;
        /** The file a save copies, null for a pick. */
        final String from;
        /** Whether it picks a folder. */
        final boolean folder;
        /** Whether a folder pick asks for a persistable read grant. */
        final boolean persist;
        /** The proposed document name and MIME type for a preflight save. */
        final String saveName, saveMime;

        Documents(long count, int limit, String from, boolean folder) {
            this(count, limit, from, folder, false, null, null);
        }

        Documents(long count, int limit, String from, boolean folder, boolean persist) {
            this(count, limit, from, folder, persist, null, null);
        }

        Documents(long count, int limit, String from, boolean folder, String saveName, String saveMime) {
            this(count, limit, from, folder, false, saveName, saveMime);
        }

        Documents(long count, int limit, String from, boolean folder, boolean persist, String saveName, String saveMime) {
            this.count = count;
            this.limit = Math.max(limit, 1);
            this.from = from;
            this.folder = folder;
            this.persist = persist;
            this.saveName = saveName;
            this.saveMime = saveMime;
        }

        /** The request code of the activity that answers it. */
        int request() {
            return folder || saveName != null ? FOLDER_REQUEST : from == null ? FILES_REQUEST : SAVE_REQUEST;
        }
    }

    /**
     * Starts the documents UI for the files of the media types given, or
     * any file for none; it needs no permission. It reports false when no
     * activity can pick.
     */
    static boolean startFiles(Activity activity, int limit, String[] types) {
        Intent intent = new Intent(Intent.ACTION_OPEN_DOCUMENT);
        intent.addCategory(Intent.CATEGORY_OPENABLE);
        intent.setType(types.length == 1 ? types[0] : "*/*");
        if (types.length > 1) intent.putExtra(Intent.EXTRA_MIME_TYPES, types);
        intent.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, limit > 1);
        return start(activity, intent, FILES_REQUEST);
    }

    /** Starts the documents UI to pick a folder, which grants its tree, not a path. */
    static boolean startFolder(Activity activity) {
        return startFolder(activity, false);
    }

    /** Starts the documents UI to pick a folder, requesting a persistable read grant when persist is set. */
    static boolean startFolder(Activity activity, boolean persist) {
        Intent intent = new Intent(Intent.ACTION_OPEN_DOCUMENT_TREE);
        if (persist) {
            intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION);
        }
        return start(activity, intent, FOLDER_REQUEST);
    }

    /**
     * Takes a persistable read URI permission for a picked document tree and
     * returns its serializable token (the tree URI), or "" if the provider did
     * not grant a persistable permission.
     */
    static String persistTree(Context context, Uri tree, Intent data) {
        int flags = (data != null ? data.getFlags() : 0) & Intent.FLAG_GRANT_READ_URI_PERMISSION;
        if (flags == 0) flags = Intent.FLAG_GRANT_READ_URI_PERMISSION;
        try {
            context.getContentResolver().takePersistableUriPermission(tree, flags);
            return tree.toString();
        } catch (SecurityException e) {
            return "";
        }
    }

    /**
     * The folder picked as Go's readFolders reads it: a line of its name,
     * the tree's URI, its root document's ID and, when persisted, its token.
     */
    static String pickedTree(Context context, Uri tree) {
        return pickedTree(context, tree, "");
    }

    static String pickedTree(Context context, Uri tree, String token) {
        String root = DocumentsContract.getTreeDocumentId(tree);
        String name = displayName(context.getContentResolver(), DocumentsContract.buildDocumentUriUsingTree(tree, root));
        String line = name.replace('\t', ' ').replace('\n', ' ') + '\t' + tree + '\t' + root;
        if (token != null && !token.isEmpty()) {
            line += '\t' + token.replace('\t', ' ').replace('\n', ' ');
        }
        return line + '\n';
    }

    /**
     * Verifies a persisted read grant for token and writes the folder's
     * display name, tree URI and root document ID as Go's openDirectory reads
     * them.
     */
    static void reopenTree(Context context, String token, DataOutputStream out) throws IOException {
        Uri tree = parseTreeToken(token);
        String root;
        try {
            root = DocumentsContract.getTreeDocumentId(tree);
        } catch (IllegalArgumentException e) {
            throw new IOException("invalid directory token", e);
        }
        ContentResolver resolver = context.getContentResolver();
        boolean held = false;
        for (android.content.UriPermission perm : resolver.getPersistedUriPermissions()) {
            if (tree.equals(perm.getUri()) && perm.isReadPermission()) {
                held = true;
                break;
            }
        }
        if (!held) {
            throw new IOException("directory grant is no longer held");
        }
        Uri doc = DocumentsContract.buildDocumentUriUsingTree(tree, root);
        String name;
        try (Cursor c = resolver.query(doc, new String[] {DocumentsContract.Document.COLUMN_DISPLAY_NAME}, null, null, null)) {
            if (c == null || !c.moveToFirst()) {
                throw new IOException("directory is no longer accessible");
            }
            name = c.isNull(0) ? null : c.getString(0);
        } catch (RuntimeException e) {
            throw new IOException(e.getMessage(), e);
        }
        if (name == null) name = doc.getLastPathSegment();
        if (name != null) name = name.replace('/', '_').replace('\0', '_').trim();
        if (name == null || name.isEmpty() || name.equals(".") || name.equals("..")) name = "file";
        Protocol.writeString(out, name);
        Protocol.writeString(out, tree.toString());
        Protocol.writeString(out, root);
    }

    /** Releases a persisted read grant for token, ignoring already-released or absent grants. */
    static void releaseTree(Context context, String token) throws IOException {
        Uri tree = parseTreeToken(token);
        try {
            context.getContentResolver().releasePersistableUriPermission(tree, Intent.FLAG_GRANT_READ_URI_PERMISSION);
        } catch (SecurityException ignored) {
            // Already released or never persisted.
        }
    }

    private static Uri parseTreeToken(String token) throws IOException {
        if (token == null || token.isEmpty()) {
            throw new IOException("directory token is empty");
        }
        Uri uri = Uri.parse(token);
        if (!"content".equals(uri.getScheme()) || uri.getAuthority() == null) {
            throw new IOException("directory token is not a content URI");
        }
        return uri;
    }

    /**
     * Lists the folder document of a tree as Go's tree.List reads it: the
     * count, then each child's name, document ID, whether it is a folder,
     * size (-1 when its provider does not say) and modification time in
     * milliseconds (0: unknown).
     */
    static void listFolder(Context context, String tree, String document, DataOutputStream out) throws IOException {
        Uri children = DocumentsContract.buildChildDocumentsUriUsingTree(Uri.parse(tree), document);
        String[] columns = {DocumentsContract.Document.COLUMN_DISPLAY_NAME, DocumentsContract.Document.COLUMN_DOCUMENT_ID,
                DocumentsContract.Document.COLUMN_MIME_TYPE, DocumentsContract.Document.COLUMN_SIZE,
                DocumentsContract.Document.COLUMN_LAST_MODIFIED};
        try (Cursor c = context.getContentResolver().query(children, columns, null, null, null)) {
            if (c == null) throw new IOException("no listing for " + children);
            out.writeInt(c.getCount());
            while (c.moveToNext()) {
                Protocol.writeString(out, c.isNull(0) ? "" : c.getString(0));
                Protocol.writeString(out, c.getString(1));
                out.writeByte(DocumentsContract.Document.MIME_TYPE_DIR.equals(c.getString(2)) ? 1 : 0);
                out.writeLong(c.isNull(3) ? -1 : c.getLong(3));
                out.writeLong(c.isNull(4) ? 0 : c.getLong(4));
            }
        } catch (RuntimeException e) {
            throw new IOException(e.getMessage(), e);
        }
    }

    /**
     * Reads at most n bytes of a tree's document from off, fewer only at
     * its end: in place where its provider gives a file, else skipping
     * through its stream.
     */
    static byte[] readDocument(Context context, String tree, String document, long off, int n) throws IOException {
        ContentResolver resolver = context.getContentResolver();
        Uri uri = DocumentsContract.buildDocumentUriUsingTree(Uri.parse(tree), document);
        ByteBuffer buf = ByteBuffer.allocate(n);
        try {
            try (android.os.ParcelFileDescriptor fd = resolver.openFileDescriptor(uri, "r")) {
                if (fd == null) throw new IOException("no file descriptor for " + uri);
                try (FileChannel ch = new FileInputStream(fd.getFileDescriptor()).getChannel()) {
                    while (buf.hasRemaining() && ch.read(buf, off + buf.position()) > 0) {}
                }
            } catch (IOException e) {
                // A pipe, which cannot read in place.
                return readDocumentStream(context, uri, off, n);
            }
        } catch (RuntimeException e) {
            throw new IOException(e.getMessage(), e);
        }
        return java.util.Arrays.copyOf(buf.array(), buf.position());
    }

    /**
     * Opens a tree's document once so repeated reads reuse its provider
     * descriptor until closed. Providers whose descriptor is a non-seekable
     * pipe fall back to stream reads after retiring that descriptor.
     */
    static FolderHandles.Document openDocument(Context context, String tree, String document) throws IOException {
        ContentResolver resolver = context.getContentResolver();
        Uri uri = DocumentsContract.buildDocumentUriUsingTree(Uri.parse(tree), document);
        try {
            android.os.ParcelFileDescriptor fd = resolver.openFileDescriptor(uri, "r");
            if (fd == null) throw new IOException("no file descriptor for " + uri);
            return new OpenDocument(context.getApplicationContext(), uri, fd);
        } catch (RuntimeException e) {
            throw new IOException(e.getMessage(), e);
        }
    }

    private static byte[] readDocumentStream(Context context, Uri uri, long off, int n) throws IOException {
        byte[] buf = new byte[n];
        int pos = 0;
        try (InputStream in = context.getContentResolver().openInputStream(uri)) {
            if (in == null) throw new IOException("no stream for " + uri);
            for (long skip = off; skip > 0; ) {
                long s = in.skip(skip);
                if (s <= 0) {
                    if (in.read() < 0) return new byte[0];
                    s = 1;
                }
                skip -= s;
            }
            for (int r; pos < n && (r = in.read(buf, pos, n - pos)) > 0; ) {
                pos += r;
            }
        } catch (RuntimeException e) {
            throw new IOException(e.getMessage(), e);
        }
        return java.util.Arrays.copyOf(buf, pos);
    }

    private static final class OpenDocument implements FolderHandles.Document {
        private final Context context;
        private final Uri uri;
        private android.os.ParcelFileDescriptor fd;
        private FileInputStream in;
        private FileChannel ch;
        private boolean streamOnly;
        private boolean closed;

        OpenDocument(Context context, Uri uri, android.os.ParcelFileDescriptor fd) {
            this.context = context;
            this.uri = uri;
            this.fd = fd;
            this.in = new FileInputStream(fd.getFileDescriptor());
            this.ch = this.in.getChannel();
        }

        @Override
        public byte[] read(long off, int n) throws IOException {
            FileChannel channel;
            synchronized (this) {
                if (closed) throw new IOException("document closed");
                channel = streamOnly ? null : ch;
            }
            if (channel != null) {
                ByteBuffer buf = ByteBuffer.allocate(n);
                try {
                    while (buf.hasRemaining() && channel.read(buf, off + buf.position()) > 0) {}
                    return java.util.Arrays.copyOf(buf.array(), buf.position());
                } catch (java.nio.channels.ClosedByInterruptException e) {
                    throw e;
                } catch (IOException e) {
                    if (Thread.currentThread().isInterrupted()) throw e;
                    // A pipe, which cannot read in place: retire the descriptor once and read from a stream.
                    synchronized (this) {
                        if (closed) throw e;
                        streamOnly = true;
                        closeDescriptorLocked();
                    }
                } catch (RuntimeException e) {
                    throw new IOException(e.getMessage(), e);
                }
            }
            return readDocumentStream(context, uri, off, n);
        }

        @Override
        public void close() {
            synchronized (this) {
                if (closed) return;
                closed = true;
                closeDescriptorLocked();
            }
        }

        private void closeDescriptorLocked() {
            FileInputStream stream = in;
            android.os.ParcelFileDescriptor pfd = fd;
            ch = null;
            in = null;
            fd = null;
            if (stream != null) {
                try {
                    stream.close();
                } catch (IOException ignored) {
                }
            }
            if (pfd != null) {
                try {
                    pfd.close();
                } catch (IOException ignored) {
                }
            }
        }
    }

    /** Starts the documents UI to create a document named name of type mime. */
    static boolean startSave(Activity activity, String name, String mime) {
        Intent intent = new Intent(Intent.ACTION_CREATE_DOCUMENT);
        intent.addCategory(Intent.CATEGORY_OPENABLE);
        intent.setType(mime.isEmpty() ? "application/octet-stream" : mime);
        intent.putExtra(Intent.EXTRA_TITLE, name);
        return start(activity, intent, SAVE_REQUEST);
    }

    private static boolean start(Activity activity, Intent intent, int request) {
        try {
            activity.startActivityForResult(intent, request);
            return true;
        } catch (ActivityNotFoundException e) {
            return false;
        }
    }

    /**
     * Copies the documents picked into a new directory of the cache, which
     * the Go process reads, streaming each: lines of the name, the MIME
     * type and the copy's path. A document that cannot be read removes
     * the copies and fails the pick.
     */
    static String copyPicked(Context context, List<Uri> uris) throws IOException {
        StringBuilder out = new StringBuilder();
        for (CachedFile file : copyToCache(context, uris, "pick")) {
            out.append(file.name.replace('\t', ' ').replace('\n', ' ')).append('\t')
                    .append(file.mime).append('\t').append(file.path).append('\n');
        }
        return out.toString();
    }

    static final class CachedFile {
        final String name, mime, path;

        CachedFile(String name, String mime, String path) {
            this.name = name;
            this.mime = mime;
            this.path = path;
        }
    }

    /** Streams documents into a private cache directory with their display names. */
    static List<CachedFile> copyToCache(Context context, List<Uri> uris, String prefix) throws IOException {
        ContentResolver resolver = context.getContentResolver();
        File base = new File(context.getCacheDir(), FILES_DIR);
        if (!base.isDirectory() && !base.mkdirs()) throw new IOException("cannot make " + base);
        File dir = java.nio.file.Files.createTempDirectory(base.toPath(), prefix).toFile();
        List<CachedFile> files = new ArrayList<>();
        try {
            for (Uri uri : uris) {
                String name = displayName(resolver, uri);
                File to = new File(dir, name);
                for (int n = 2; to.exists(); n++) to = new File(dir, n + " " + name);
                try (InputStream in = resolver.openInputStream(uri); OutputStream o = new FileOutputStream(to)) {
                    if (in == null) throw new IOException("no stream for " + uri);
                    copy(in, o);
                }
                String mime = resolver.getType(uri);
                files.add(new CachedFile(name, mime == null ? "" : mime.toLowerCase(java.util.Locale.ROOT),
                        to.getAbsolutePath()));
            }
        } catch (IOException | RuntimeException e) {
            remove(dir);
            throw e instanceof IOException ? (IOException) e : new IOException(e.getMessage(), e);
        }
        return files;
    }

    /** A document's name as the user saw it, safe as a file name. */
    private static String displayName(ContentResolver resolver, Uri uri) {
        String name = null;
        try (Cursor c = resolver.query(uri, new String[] {OpenableColumns.DISPLAY_NAME}, null, null, null)) {
            if (c != null && c.moveToFirst() && !c.isNull(0)) name = c.getString(0);
        } catch (RuntimeException e) {
            // A provider without the column leaves the URI's last segment.
        }
        if (name == null) name = uri.getLastPathSegment();
        if (name != null) name = name.replace('/', '_').replace('\0', '_').trim();
        return name == null || name.isEmpty() || name.equals(".") || name.equals("..") ? "file" : name;
    }

    /**
     * Copies the file at from into the document the user created,
     * streaming it; a copy that fails deletes the document, which holds
     * only part of the file.
     */
    static void save(Context context, String from, Uri to) throws IOException {
        ContentResolver resolver = context.getContentResolver();
        try (InputStream in = new java.io.FileInputStream(from); OutputStream o = resolver.openOutputStream(to, "w")) {
            if (o == null) throw new IOException("no stream for " + to);
            copy(in, o);
        } catch (IOException | RuntimeException e) {
            deleteCreatedDocument(resolver, to);
            throw e instanceof IOException ? (IOException) e : new IOException(e.getMessage(), e);
        }
    }

    /** Creates and writes the approved name under the user's directory grant. */
    static Uri createSave(Context context, Documents d, Uri tree) throws IOException {
        ContentResolver resolver = context.getContentResolver();
        Uri folder = DocumentsContract.buildDocumentUriUsingTree(tree, DocumentsContract.getTreeDocumentId(tree));
        Uri created = DocumentsContract.createDocument(resolver, folder, d.saveMime, d.saveName);
        if (created == null) throw new IOException("the document provider could not create " + d.saveName);
        try (Cursor cursor = resolver.query(created, new String[]{OpenableColumns.DISPLAY_NAME}, null, null, null)) {
            if (cursor == null || !cursor.moveToFirst() || !d.saveName.equals(cursor.getString(0))) {
                throw new IOException("the document provider changed the approved file name");
            }
        } catch (IOException | RuntimeException e) {
            deleteCreatedDocument(resolver, created);
            throw e instanceof IOException ? (IOException) e : new IOException(e.getMessage(), e);
        }
        save(context, d.from, created);
        return created;
    }

    private static void deleteCreatedDocument(ContentResolver resolver, Uri document) {
        try {
            DocumentsContract.deleteDocument(resolver, document);
        } catch (Exception ignored) {
            // The provider did not remove the document it created.
        }
    }

    private static void copy(InputStream in, OutputStream out) throws IOException {
        byte[] buf = new byte[1 << 16];
        for (int n; (n = in.read(buf)) > 0; ) out.write(buf, 0, n);
    }

    private static void remove(File f) {
        File[] children = f.listFiles();
        if (children != null) for (File c : children) remove(c);
        f.delete();
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

    /** The Play Review library is supplied by the embedding Android app. */
    private static final class PlayReview {
        static final PlayReview SDK = load();

        final Method create, request, addListener, successful, result, launch;
        final Class<?> listener;

        private PlayReview() throws ReflectiveOperationException {
            Class<?> factory = Class.forName("com.google.android.play.core.review.ReviewManagerFactory");
            Class<?> manager = Class.forName("com.google.android.play.core.review.ReviewManager");
            Class<?> info = Class.forName("com.google.android.play.core.review.ReviewInfo");
            Class<?> task = Class.forName("com.google.android.gms.tasks.Task");
            listener = Class.forName("com.google.android.gms.tasks.OnCompleteListener");
            create = factory.getMethod("create", Context.class);
            request = manager.getMethod("requestReviewFlow");
            addListener = task.getMethod("addOnCompleteListener", listener);
            successful = task.getMethod("isSuccessful");
            result = task.getMethod("getResult");
            launch = manager.getMethod("launchReviewFlow", Activity.class, info);
        }

        private static PlayReview load() {
            try {
                return new PlayReview();
            } catch (ReflectiveOperationException | LinkageError e) {
                return null;
            }
        }
    }

    /** Whether the host bundled the SDK and can reach an enabled Play Store. */
    static boolean canRequestReview(Activity activity) {
        if (activity == null || activity.isFinishing() || activity.isDestroyed() || PlayReview.SDK == null) {
            return false;
        }
        PackageManager packages = activity.getPackageManager();
        ComponentName wrapper = new ComponentName(activity, "com.google.android.play.core.common.PlayCoreDialogWrapperActivity");
        try {
            return packages.getApplicationInfo("com.android.vending", 0).enabled
                    && packages.getActivityInfo(wrapper, 0).enabled;
        } catch (PackageManager.NameNotFoundException e) {
            return false;
        }
    }

    /** Requests the Play Review card. The SDK delivers completion on the main thread. */
    static void requestReview(Activity activity) {
        if (!canRequestReview(activity)) return;
        PlayReview sdk = PlayReview.SDK;
        try {
            Object manager = sdk.create.invoke(null, activity);
            Object task = sdk.request.invoke(manager);
            Object listener = Proxy.newProxyInstance(sdk.listener.getClassLoader(), new Class<?>[] {sdk.listener},
                    (proxy, method, args) -> {
                        switch (method.getName()) {
                            case "hashCode": return System.identityHashCode(proxy);
                            case "equals": return proxy == args[0];
                            case "toString": return "NsgoReviewListener";
                            case "onComplete":
                                launchReview(activity, sdk, manager, args[0]);
                                return null;
                            default: return null;
                        }
                    });
            sdk.addListener.invoke(task, listener);
        } catch (ReflectiveOperationException | LinkageError ignored) {
        }
    }

    private static void launchReview(Activity activity, PlayReview sdk, Object manager, Object task) {
        if (activity.isFinishing() || activity.isDestroyed() || !activity.hasWindowFocus()) return;
        try {
            if (Boolean.TRUE.equals(sdk.successful.invoke(task))) {
                sdk.launch.invoke(manager, activity, sdk.result.invoke(task));
            }
        } catch (ReflectiveOperationException | LinkageError ignored) {
        }
    }
}
