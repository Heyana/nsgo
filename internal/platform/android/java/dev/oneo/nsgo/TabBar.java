package dev.oneo.nsgo;

import android.content.Context;
import android.content.res.ColorStateList;
import android.content.res.Configuration;
import android.content.res.Resources;
import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.graphics.Typeface;
import android.graphics.drawable.BitmapDrawable;
import android.graphics.drawable.Drawable;
import android.graphics.drawable.GradientDrawable;
import android.os.Build;
import android.util.TypedValue;
import android.text.TextUtils;
import android.util.Base64;
import android.view.Gravity;
import android.view.View;
import android.view.ViewTreeObserver;
import android.view.WindowInsets;
import android.view.accessibility.AccessibilityEvent;
import android.view.accessibility.AccessibilityNodeInfo;
import android.widget.FrameLayout;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.TextView;
import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;

/**
 * core.TabBar's view, as NSH's TabBarComponent: Material 3's navigation
 * bar drawn with framework views, in its baseline colors for the
 * appearance shown and the selected tab in the theme's accent. Each tab is an icon in an active indicator over its
 * label; a press selects it at once and tells the listener. Native
 * components build it by reflection, so its constructor, setters and
 * listener are public.
 */
public final class TabBar extends LinearLayout {
    /** Hears the tab the user chose, by index. */
    public interface OnChangeListener {
        void onChange(int index);
    }

    /** Reports the bar's natural height in points to the Go layout. */
    public interface OnSizeListener {
        void onSize(float height);
    }

    private OnChangeListener listener;
    private OnSizeListener sizeListener;
    private int selected;
    private JSONArray renderedItems = new JSONArray();
    private String label = "";
    /** The theme's accent as ARGB (0: none): the selected tab's icon, label and indicator. */
    private int tint;
    private int background;
    private int inactive;
    private int indicator;
    private int activeContent;
    private int badgeColor = 0xffb3261e;
    private JSONArray badges = new JSONArray();
    private final float density;
    /** The pixels of this bar over the system's navigation bar, kept clear as bottom padding. */
    private int bottomInset;
    /** Runs after every layout pass of the window, so a move of the bar or of any ancestor updates the overlap; padding changed there takes effect on the next pass instead of re-entering this one. */
    private final ViewTreeObserver.OnGlobalLayoutListener placement = this::updateNavigationInset;

    public TabBar(Context context) {
        super(context);
        density = context.getResources().getDisplayMetrics().density;
        setOrientation(HORIZONTAL);
        setBackgroundColor(night() ? 0xff211f26 : 0xfff3edf7);
    }

    /**
     * Material's 80 dp tall, as wide as it is placed: measured with no
     * width of its own, it asks for its height alone.
     */
    @Override
    protected void onMeasure(int widthSpec, int heightSpec) {
        int height = MeasureSpec.getMode(heightSpec) == MeasureSpec.EXACTLY ? MeasureSpec.getSize(heightSpec) : dp(80) + bottomInset;
        int width = MeasureSpec.getMode(widthSpec) == MeasureSpec.UNSPECIFIED ? 0 : MeasureSpec.getSize(widthSpec);
        super.onMeasure(MeasureSpec.makeMeasureSpec(width, MeasureSpec.EXACTLY),
                MeasureSpec.makeMeasureSpec(height, MeasureSpec.EXACTLY));
    }

    public void setOnChangeListener(OnChangeListener l) {
        listener = l;
    }

    public void setOnSizeListener(OnSizeListener l) {
        sizeListener = l;
    }

    @Override
    protected void onAttachedToWindow() {
        super.onAttachedToWindow();
        getViewTreeObserver().addOnGlobalLayoutListener(placement);
    }

    @Override
    protected void onDetachedFromWindow() {
        getViewTreeObserver().removeOnGlobalLayoutListener(placement);
        super.onDetachedFromWindow();
    }

    /** Makes the tabs from titles, resource icons and optional base64 canvas drawings. */
    public void setItems(String json) {
        JSONArray items;
        try {
            items = new JSONArray(json);
        } catch (JSONException e) {
            return;
        }
        if (updateTitles(items)) {
            renderedItems = items;
            // A retitled tab's description carries its badge announcement again.
            showBadges();
            return;
        }
        removeAllViews();
        renderedItems = items;
        for (int i = 0; i < items.length(); i++) {
            JSONObject tab = items.optJSONObject(i);
            if (tab == null) continue;
            addView(tab(i, tab.optString("title"), icon(tab)),
                    new LayoutParams(0, LayoutParams.MATCH_PARENT, 1));
        }
        restyle();
        showBadges();
    }

    /** Updates titles without replacing the tabs that have the same icons. */
    private boolean updateTitles(JSONArray items) {
        if (items.length() != getChildCount() || items.length() != renderedItems.length()) return false;
        for (int i = 0; i < items.length(); i++) {
            JSONObject before = renderedItems.optJSONObject(i), after = items.optJSONObject(i);
            if (before == null || after == null || !before.optString("icon").equals(after.optString("icon"))
                    || !before.optString("drawing").equals(after.optString("drawing"))) {
                return false;
            }
        }
        for (int i = 0; i < items.length(); i++) {
            String title = items.optJSONObject(i).optString("title");
            LinearLayout item = (LinearLayout) getChildAt(i);
            TextView label = (TextView) item.getChildAt(1);
            if (!title.contentEquals(label.getText())) {
                label.setText(title);
                item.setContentDescription(title);
            }
        }
        return true;
    }

    public void setLabel(String title) {
        label = title == null ? "" : title;
        sendAccessibilityEvent(AccessibilityEvent.TYPE_WINDOW_CONTENT_CHANGED);
    }

    @Override
    public void onInitializeAccessibilityNodeInfo(AccessibilityNodeInfo info) {
        super.onInitializeAccessibilityNodeInfo(info);
        if (!label.isEmpty()) info.setPaneTitle(label);
    }

    public void setSelectedIndex(int index) {
        selected = index;
        restyle();
    }

    /** Takes the theme's accent as ARGB, 0 for Material's colors. */
    public void setTint(long argb) {
        tint = (int) argb;
        restyle();
    }

    public void setBackground(long argb) {
        background = (int) argb;
        setBackgroundColor(background != 0 ? background : (night() ? 0xff211f26 : 0xfff3edf7));
    }

    public void setInactive(long argb) { inactive = (int) argb; restyle(); }
    public void setIndicator(long argb) { indicator = (int) argb; restyle(); }
    public void setActiveContent(long argb) { activeContent = (int) argb; restyle(); }

    public void setBadgeColor(long argb) {
        badgeColor = argb == 0 ? 0xffb3261e : (int) argb;
        showBadges();
    }

    /** Badge updates leave the tab views and their focus in place. */
    public void setBadges(String json) {
        try {
            badges = new JSONArray(json);
        } catch (JSONException e) {
            badges = new JSONArray();
        }
        showBadges();
    }

    private void showBadges() {
        for (int i = 0; i < getChildCount(); i++) {
            LinearLayout item = (LinearLayout) getChildAt(i);
            FrameLayout pill = (FrameLayout) item.getChildAt(0);
            TextView mark = (TextView) pill.getChildAt(1);
            TextView title = (TextView) item.getChildAt(1);
            JSONObject badge = badges.optJSONObject(i);
            String text = badge == null ? "" : badge.optString("text");
            boolean dot = text.isEmpty() && badge != null && badge.optBoolean("dot");
            String announcement = badge == null ? "" : badge.optString("announcement");
            item.setContentDescription(announcement.isEmpty() ? title.getText() : title.getText() + ", " + announcement);
            mark.setText(text);
            mark.setVisibility(dot || !text.isEmpty() ? VISIBLE : GONE);
            if (mark.getVisibility() == GONE) continue;
            int size = dp(dot ? 6 : 16);
            mark.setMinWidth(size);
            mark.setPadding(dp(dot ? 0 : 4), 0, dp(dot ? 0 : 4), 0);
            GradientDrawable shape = (GradientDrawable) mark.getBackground();
            shape.setColor(badgeColor);
            shape.setCornerRadius(size / 2f);
            FrameLayout.LayoutParams lp = (FrameLayout.LayoutParams) mark.getLayoutParams();
            lp.width = dot ? size : LayoutParams.WRAP_CONTENT;
            lp.height = size;
            lp.setMarginEnd(dp(dot ? 16 : 10));
            mark.setLayoutParams(lp);
        }
    }

    private View tab(int index, String title, Drawable icon) {
        LinearLayout item = new LinearLayout(getContext());
        item.setOrientation(VERTICAL);
        item.setGravity(Gravity.CENTER);
        item.setPadding(0, dp(12), 0, dp(16));
        FrameLayout pill = new FrameLayout(getContext());
        GradientDrawable shape = new GradientDrawable();
        shape.setCornerRadius(dp(16));
        pill.setBackground(shape);
        ImageView image = new ImageView(getContext());
        if (icon != null) image.setImageDrawable(icon.mutate());
        pill.addView(image, new FrameLayout.LayoutParams(dp(24), dp(24), Gravity.CENTER));
        TextView badge = new TextView(getContext());
        badge.setBackground(new GradientDrawable());
        badge.setTextColor(0xffffffff);
        badge.setTextSize(TypedValue.COMPLEX_UNIT_SP, 10);
        badge.setGravity(Gravity.CENTER);
        badge.setIncludeFontPadding(false);
        badge.setSingleLine(true);
        badge.setMaxWidth(dp(50));
        badge.setEllipsize(TextUtils.TruncateAt.END);
        badge.setImportantForAccessibility(IMPORTANT_FOR_ACCESSIBILITY_NO);
        badge.setVisibility(GONE);
        pill.addView(badge, new FrameLayout.LayoutParams(0, 0, Gravity.TOP | Gravity.END));
        item.addView(pill, new LayoutParams(dp(64), dp(32)));
        TextView label = new TextView(getContext());
        label.setText(title);
        label.setTextSize(TypedValue.COMPLEX_UNIT_SP, 12);
        label.setTypeface(Typeface.create("sans-serif-medium", Typeface.NORMAL));
        label.setSingleLine(true);
        label.setGravity(Gravity.CENTER);
        LayoutParams lp = new LayoutParams(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT);
        lp.topMargin = dp(4);
        item.addView(label, lp);
        item.setContentDescription(title);
        item.setClickable(true);
        item.setOnClickListener(v -> {
            setSelectedIndex(index);
            if (listener != null) listener.onChange(index);
        });
        return item;
    }

    /** The drawable the icon names, else the SVG's drawing rendered at screen density for ImageView to tint. */
    private Drawable icon(JSONObject tab) {
        // A name shared with iOS, such as an SF Symbol's, names no drawable here, so the SVG stands in for it.
        Drawable resource = drawable(tab.optString("icon"));
        if (resource != null) return resource;
        String drawing = tab.optString("drawing");
        if (drawing.isEmpty()) return null;
        byte[] ops = Base64.decode(drawing, Base64.DEFAULT);
        int size = Math.max(1, dp(24));
        Bitmap bitmap = Bitmap.createBitmap(size, size, Bitmap.Config.ARGB_8888);
        Canvas canvas = new Canvas(bitmap);
        canvas.scale(size / 24f, size / 24f);
        CanvasNode.play(canvas, 24, 24, ops);
        return new BitmapDrawable(getResources(), bitmap);
    }

    /** Colors each tab as selected or not. */
    private void restyle() {
        boolean night = night();
        int idle = night ? 0xffcac4d0 : 0xff49454f;
        int indicator = night ? 0xff4a4458 : 0xffe8def8;
        int activeIcon = night ? 0xffe8def8 : 0xff1d192b;
        int activeLabel = night ? 0xffe6e0e9 : 0xff1d1b20;
        if (inactive != 0) idle = inactive;
        if (tint != 0) {
            // The icon and label in the accent, over a faint pill of it.
            indicator = (tint & 0x00ffffff) | (((tint >>> 24) * 0x1f / 0xff) << 24);
            activeIcon = tint;
            activeLabel = tint;
        }
        if (this.indicator != 0) indicator = this.indicator;
        if (activeContent != 0) {
            activeIcon = activeContent;
            activeLabel = activeContent;
        }
        for (int i = 0; i < getChildCount(); i++) {
            LinearLayout item = (LinearLayout) getChildAt(i);
            boolean on = i == selected;
            item.setSelected(on);
            FrameLayout pill = (FrameLayout) item.getChildAt(0);
            ((GradientDrawable) pill.getBackground()).setColor(on ? indicator : 0);
            ((ImageView) pill.getChildAt(0)).setImageTintList(ColorStateList.valueOf(on ? activeIcon : idle));
            TextView label = (TextView) item.getChildAt(1);
            label.setTextColor(on ? activeLabel : idle);
        }
    }

    /** The drawable an icon names: "android:name" the system's, else the app's. */
    private Drawable drawable(String name) {
        if (name == null || name.isEmpty()) return null;
        Resources res = getContext().getResources();
        int id;
        if (name.startsWith("android:")) {
            res = Resources.getSystem();
            id = res.getIdentifier(name.substring(8), "drawable", "android");
        } else {
            id = res.getIdentifier(name, "drawable", getContext().getPackageName());
        }
        if (id == 0) return null;
        try {
            return res.getDrawable(id, null);
        } catch (Resources.NotFoundException e) {
            return null;
        }
    }

    /** Keeps only the part of this bar under navigation clear, never the IME. */
    private void updateNavigationInset() {
        if (!isAttachedToWindow()) return;
        View root = getRootView();
        WindowInsets in = getRootWindowInsets();
        int inset = 0;
        if (in != null && root.getHeight() > 0) {
            int nav = Build.VERSION.SDK_INT >= 30 ? in.getInsets(WindowInsets.Type.navigationBars()).bottom
                    : Math.min(in.getSystemWindowInsetBottom(), in.getStableInsetBottom());
            int[] at = new int[2], window = new int[2];
            getLocationInWindow(at);
            root.getLocationInWindow(window);
            int bottom = window[1] + root.getHeight();
            inset = Math.max(0, Math.min(nav, at[1] + getHeight() - (bottom - nav)));
        }
        if (inset == bottomInset) return;
        bottomInset = inset;
        setPadding(0, 0, 0, inset);
        if (sizeListener != null) sizeListener.onSize((dp(80) + inset) / density);
    }

    private boolean night() {
        int mode = getContext().getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK;
        return mode == Configuration.UI_MODE_NIGHT_YES;
    }

    private int dp(float v) {
        return Math.round(v * density);
    }
}
