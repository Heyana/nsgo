package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.os.Handler;
import android.os.Looper;
import android.text.Editable;
import android.text.InputType;
import android.text.TextWatcher;
import android.util.Log;
import android.util.SparseArray;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.view.accessibility.AccessibilityNodeInfo;
import android.view.inputmethod.EditorInfo;
import android.widget.EditText;
import android.widget.FrameLayout;
import java.io.BufferedInputStream;
import java.io.BufferedOutputStream;
import java.io.BufferedReader;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.File;
import java.io.IOException;
import java.io.InputStreamReader;

/**
 * A surface showing one NSGo app. It starts the app's Go executable
 * (libnsgo.so in the native library directory), answers its text
 * measurements on a reader thread and mounts its batches on the UI thread.
 * Points are density-independent pixels.
 */
public class NsgoView extends FrameLayout {
    private static final String TAG = "nsgo";

    // Mount item kinds and tags, as in core/mount.go and core/element.go.
    private static final int ITEM_CREATE = 1, ITEM_PROPS = 2, ITEM_INSERT = 3, ITEM_FRAME = 4;
    private static final int TAG_TEXT = 2, TAG_INPUT = 3;
    private static final int ROLE_BUTTON = 1;

    private final float density;
    private final Handler ui = new Handler(Looper.getMainLooper());
    private final SparseArray<View> views = new SparseArray<>();
    private Process process;
    private DataOutputStream out;

    public NsgoView(Context context) {
        super(context);
        density = context.getResources().getDisplayMetrics().density;
    }

    @Override
    protected void onSizeChanged(int w, int h, int oldw, int oldh) {
        super.onSizeChanged(w, h, oldw, oldh);
        boolean first = process == null;
        if (first) start();
        send(first ? Protocol.START : Protocol.RESIZE, w / density, h / density);
    }

    private void start() {
        File exe = new File(getContext().getApplicationInfo().nativeLibraryDir, "libnsgo.so");
        try {
            process = new ProcessBuilder(exe.getAbsolutePath()).start();
        } catch (IOException e) {
            throw new IllegalStateException("cannot start " + exe, e);
        }
        out = new DataOutputStream(new BufferedOutputStream(process.getOutputStream()));
        Thread reader = new Thread(this::read, "nsgo-reader");
        reader.setDaemon(true);
        reader.start();
        Thread logger = new Thread(this::log, "nsgo-log");
        logger.setDaemon(true);
        logger.start();
    }

    /** Ends the app's process. */
    public void stop() {
        if (process != null) process.destroy();
    }

    private void read() {
        try (DataInputStream in = new DataInputStream(new BufferedInputStream(process.getInputStream()))) {
            while (true) {
                int kind = in.readUnsignedByte();
                byte[] payload = new byte[in.readInt()];
                in.readFully(payload);
                if (kind == Protocol.BATCH) {
                    ui.post(() -> mount(payload));
                } else if (kind == Protocol.MEASURE) {
                    measure(new DataInputStream(new ByteArrayInputStream(payload)));
                }
            }
        } catch (IOException e) {
            Log.i(TAG, "app process ended: " + e);
        }
    }

    private void log() {
        try (BufferedReader err = new BufferedReader(new InputStreamReader(process.getErrorStream()))) {
            for (String line; (line = err.readLine()) != null; ) Log.i(TAG, line);
        } catch (IOException ignored) {
            // The process ended.
        }
    }

    private void measure(DataInputStream in) throws IOException {
        float size = in.readFloat();
        int weight = in.readInt();
        float maxWidth = in.readFloat();
        String text = Protocol.readString(in);
        float[] px = Text.measure(text, size * density, weight, maxWidth < 0 ? -1 : maxWidth * density);
        send(Protocol.MEASURED, px[0] / density, px[1] / density);
    }

    private synchronized void write(int kind, byte[] payload) {
        if (out == null) return;
        try {
            out.writeByte(kind);
            out.writeInt(payload.length);
            out.write(payload);
            out.flush();
        } catch (IOException e) {
            Log.w(TAG, "app process unreachable", e);
        }
    }

    private void send(int kind, float a, float b) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream(8);
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeFloat(a);
            d.writeFloat(b);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        write(kind, bytes.toByteArray());
    }

    private void send(int kind, int id, String text) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeInt(id);
            if (text != null) Protocol.writeString(d, text);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        write(kind, bytes.toByteArray());
    }

    private int px(float points) {
        return Math.round(points * density);
    }

    private void mount(byte[] batch) {
        DataInputStream in = new DataInputStream(new ByteArrayInputStream(batch));
        try {
            int root = in.readInt();
            int count = in.readInt();
            for (int i = 0; i < count; i++) {
                int kind = in.readUnsignedByte();
                int id = in.readInt();
                switch (kind) {
                    case ITEM_CREATE:
                        views.put(id, create(id, in.readUnsignedByte()));
                        break;
                    case ITEM_PROPS:
                        props(id, views.get(id), in);
                        break;
                    case ITEM_INSERT: {
                        ViewGroup parent = (ViewGroup) views.get(in.readInt());
                        int index = in.readInt();
                        parent.addView(views.get(id), Math.min(index, parent.getChildCount()));
                        break;
                    }
                    case ITEM_FRAME: {
                        View v = views.get(id);
                        int x = px(in.readFloat()), y = px(in.readFloat());
                        int w = px(in.readFloat()), h = px(in.readFloat());
                        LayoutParams lp = new LayoutParams(w, h);
                        lp.leftMargin = x;
                        lp.topMargin = y;
                        v.setLayoutParams(lp);
                        break;
                    }
                    default:
                        throw new IOException("unknown mount item " + kind);
                }
            }
            View rootView = views.get(root);
            if (rootView.getParent() != this) addView(rootView);
        } catch (IOException e) {
            Log.e(TAG, "bad batch", e);
        }
    }

    private View create(int id, int tag) {
        if (tag == TAG_TEXT) return new TextNode(getContext());
        FrameLayout view = new FrameLayout(getContext());
        if (tag == TAG_INPUT) {
            EditText field = new EditText(getContext());
            field.setBackground(null);
            field.setPadding(0, 0, 0, 0);
            field.setSingleLine(true);
            field.setInputType(InputType.TYPE_CLASS_TEXT);
            field.setImeOptions(EditorInfo.IME_ACTION_DONE);
            field.setGravity(Gravity.CENTER_VERTICAL);
            field.addTextChangedListener(new TextWatcher() {
                @Override public void beforeTextChanged(CharSequence s, int a, int b, int c) {}
                @Override public void onTextChanged(CharSequence s, int a, int b, int c) {}
                @Override public void afterTextChanged(Editable s) {
                    if (field.getTag() == null) send(Protocol.CHANGE, id, s.toString());
                }
            });
            field.setOnEditorActionListener((v, action, event) -> {
                send(Protocol.SUBMIT, id, field.getText().toString());
                return false;
            });
            view.addView(field, new LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT));
        }
        return view;
    }

    private void props(int id, View view, DataInputStream in) throws IOException {
        int background = in.readInt();
        float radius = in.readFloat();
        boolean pressable = in.readUnsignedByte() != 0;
        int role = in.readUnsignedByte();
        String text = Protocol.readString(in);
        float fontSize = in.readFloat();
        int weight = in.readInt();
        int color = in.readInt();
        int top = px(in.readFloat()), right = px(in.readFloat());
        int bottom = px(in.readFloat()), left = px(in.readFloat());
        String placeholder = Protocol.readString(in);

        GradientDrawable bg = new GradientDrawable();
        bg.setColor(Text.argb(background));
        bg.setCornerRadius(radius * density);
        view.setBackground(bg);
        view.setClickable(pressable);
        view.setOnClickListener(pressable ? v -> send(Protocol.PRESS, id, null) : null);
        if (role == ROLE_BUTTON) {
            view.setAccessibilityDelegate(new View.AccessibilityDelegate() {
                @Override
                public void onInitializeAccessibilityNodeInfo(View host, AccessibilityNodeInfo info) {
                    super.onInitializeAccessibilityNodeInfo(host, info);
                    info.setClassName("android.widget.Button");
                }
            });
        }

        if (view instanceof TextNode) {
            ((TextNode) view).setText(text, Text.paint(fontSize * density, weight, Text.argb(color)), left, top, right);
        } else if (fontSize > 0 && ((ViewGroup) view).getChildAt(0) instanceof EditText) {
            EditText field = (EditText) ((ViewGroup) view).getChildAt(0);
            view.setPadding(left, top, right, bottom);
            field.setTextSize(TypedValue.COMPLEX_UNIT_PX, fontSize * density);
            field.setTypeface(Typeface.create(Typeface.DEFAULT, weight, false));
            field.setTextColor(Text.argb(color));
            field.setHint(placeholder);
            // Setting the text the user just typed would move the caret.
            if (!field.getText().toString().equals(text)) {
                field.setTag(Boolean.TRUE);
                field.setText(text);
                field.setSelection(text.length());
                field.setTag(null);
            }
        }
    }
}
