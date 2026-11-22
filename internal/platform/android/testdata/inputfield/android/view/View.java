package android.view;

import android.content.Context;
import android.content.res.Resources;
import android.graphics.Rect;
import android.graphics.drawable.Drawable;
import java.util.ArrayList;
import java.util.List;

public class View {
    public static final int SCROLLBARS_OUTSIDE_OVERLAY = 0x02000000;
    public static final int IMPORTANT_FOR_ACCESSIBILITY_NO = 2;
    public static final int IMPORTANT_FOR_AUTOFILL_AUTO = 0;
    public static final int IMPORTANT_FOR_AUTOFILL_YES = 1;
    public static final int IMPORTANT_FOR_AUTOFILL_NO = 2;

    public interface OnFocusChangeListener {
        void onFocusChange(View v, boolean hasFocus);
    }

    private final Context context;
    private final Resources resources = new Resources();
    private final List<Runnable> posted = new ArrayList<>();
    private OnFocusChangeListener focusListener;
    private boolean focused;
    private boolean enabled = true;

    public View(Context context) { this.context = context; }
    public Context getContext() { return context; }
    public Resources getResources() { return resources; }
    public void setBackground(Drawable bg) {}
    public void setPadding(int l, int t, int r, int b) {}
    public void setVerticalScrollBarEnabled(boolean enabled) {}
    public void setHorizontalScrollBarEnabled(boolean enabled) {}
    public void setScrollBarStyle(int style) {}
    public void setNestedScrollingEnabled(boolean enabled) {}
    public void setFocusable(boolean focusable) {}
    public void setFocusableInTouchMode(boolean focusable) {}
    public void setImportantForAccessibility(int mode) {}
    public void setImportantForAutofill(int mode) {}
    public void setAutofillHints(String... hints) {}
    public int getLeft() { return 0; }
    public int getTop() { return 0; }
    public int getScrollX() { return 0; }
    public int getScrollY() { return 0; }
    public boolean hasFocus() { return focused; }
    public boolean isEnabled() { return enabled; }

    public void setOnFocusChangeListener(OnFocusChangeListener l) {
        this.focusListener = l;
    }

    public void setEnabled(boolean enabled) {
        this.enabled = enabled;
        if (!enabled && focused) {
            clearFocus();
        }
    }

    public boolean requestFocus() {
        if (!enabled || focused) return focused;
        focused = true;
        onFocusChanged(true, 0, null);
        return true;
    }

    public void clearFocus() {
        if (!focused) return;
        focused = false;
        onFocusChanged(false, 0, null);
    }

    protected void onFocusChanged(boolean gainFocus, int direction, Rect previouslyFocusedRect) {
        if (focusListener != null) {
            focusListener.onFocusChange(this, gainFocus);
        }
    }

    protected void onScrollChanged(int l, int t, int oldl, int oldt) {}

    public boolean post(Runnable action) {
        posted.add(action);
        return true;
    }

    public void flushPosted() {
        while (!posted.isEmpty()) {
            Runnable r = posted.remove(0);
            r.run();
        }
    }
}
