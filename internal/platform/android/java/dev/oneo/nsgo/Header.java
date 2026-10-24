package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.Canvas;
import android.graphics.ColorFilter;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.PixelFormat;
import android.graphics.drawable.Drawable;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.widget.FrameLayout;
import android.widget.Toolbar;
import java.io.DataInputStream;
import java.io.IOException;
import java.util.function.IntFunction;

/**
 * A screen's system header, as NSH's StackBox.ScreenBox: a Toolbar over
 * the top of the screen's view, as tall as the area the core keeps above
 * the content, its bar at the bottom of it. The core keeps the stack; the
 * toolbar's arrow reports back for the screen.
 */
final class Header {
    /** The toolbar's height, in points. */
    static final float BAR = 56;

    final Toolbar toolbar;
    private final BackArrow arrow;
    private final float density;

    Header(Context context, float density, Runnable back) {
        this.density = density;
        toolbar = new Toolbar(context);
        // Over the screen's content, which the core inserts after it, and
        // without the shadow its elevation would cast.
        toolbar.setOutlineProvider(null);
        toolbar.setTranslationZ(1);
        arrow = new BackArrow(density);
        toolbar.setNavigationOnClickListener(v -> back.run());
    }

    /**
     * Applies an ItemScreen as android.go writes it; wrap returns the
     * holder of a header view, or null.
     */
    void configure(DataInputStream in, IntFunction<View> wrap) throws IOException {
        String title = Protocol.readString(in);
        int flags = in.readUnsignedByte();
        boolean hidden = Protocol.flag(flags, 0), transparent = Protocol.flag(flags, 1);
        boolean canGoBack = Protocol.flag(flags, 3), backHidden = Protocol.flag(flags, 4);
        String backTitle = Protocol.readString(in);
        int background = in.readInt(), titleColor = in.readInt(), tint = in.readInt();
        int left = in.readInt(), right = in.readInt(), titleView = in.readInt();
        float height = in.readFloat();

        Context context = toolbar.getContext();
        toolbar.setVisibility(hidden ? View.GONE : View.VISIBLE);
        toolbar.setTitle(titleView != 0 ? null : title);
        int h = Math.round(height * density);
        toolbar.setPadding(0, Math.max(0, h - Math.round(BAR * density)), 0, 0);
        toolbar.setLayoutParams(new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, h, Gravity.TOP));
        int text = themeColor(context, android.R.attr.textColorPrimary, 0xff1f2328);
        toolbar.setTitleTextColor(titleColor != 0 ? Text.argb(titleColor) : text);
        int fill = background != 0 ? Text.argb(background) : themeColor(context, android.R.attr.colorBackground, 0xffffffff);
        toolbar.setBackgroundColor(transparent ? 0 : fill);
        arrow.setColor(tint != 0 ? Text.argb(tint) : text);
        boolean back = canGoBack && !backHidden;
        toolbar.setNavigationIcon(back ? arrow : null);
        toolbar.setNavigationContentDescription(back ? upDescription(backTitle) : null);
        // The header views: the core sized them, the toolbar places them.
        for (int i = toolbar.getChildCount() - 1; i >= 0; i--) {
            if (toolbar.getChildAt(i) instanceof Holder) toolbar.removeViewAt(i);
        }
        add(wrap, left, Gravity.START);
        add(wrap, titleView, Gravity.START);
        add(wrap, right, Gravity.END);
    }

    private void add(IntFunction<View> wrap, int id, int gravity) {
        View holder = id != 0 ? wrap.apply(id) : null;
        if (holder == null) return;
        if (holder.getParent() instanceof ViewGroup) ((ViewGroup) holder.getParent()).removeView(holder);
        ViewGroup.LayoutParams size = holder.getLayoutParams();
        Toolbar.LayoutParams p = new Toolbar.LayoutParams(
                size != null ? Math.max(1, size.width) : 1, size != null ? Math.max(1, size.height) : 1,
                gravity | Gravity.CENTER_VERTICAL);
        if (gravity == Gravity.END) p.setMarginEnd(Math.round(12 * density));
        toolbar.addView(holder, p);
    }

    /** Sizes the holder of a header view to its frame, in pixels. */
    static void size(View holder, int width, int height) {
        ViewGroup.LayoutParams p = holder.getLayoutParams();
        if (p == null) {
            holder.setLayoutParams(new FrameLayout.LayoutParams(width, height));
            return;
        }
        p.width = width;
        p.height = height;
        holder.setLayoutParams(p);
    }

    /** What the arrow reads as: the label of the screen it leads back to, else the system's words. */
    private static CharSequence upDescription(String backTitle) {
        if (!backTitle.isEmpty()) return backTitle;
        android.content.res.Resources system = android.content.res.Resources.getSystem();
        int up = system.getIdentifier("action_bar_up_description", "string", "android");
        return up != 0 ? system.getString(up) : null;
    }

    private static int themeColor(Context context, int attr, int fallback) {
        TypedValue v = new TypedValue();
        if (!context.getTheme().resolveAttribute(attr, v, true)) return fallback;
        if (v.resourceId != 0) {
            try {
                return context.getColorStateList(v.resourceId).getDefaultColor();
            } catch (Exception e) {
                return fallback;
            }
        }
        return v.data;
    }

    /** Holds a header view at its frame's size, where the toolbar puts it. */
    static final class Holder extends FrameLayout {
        Holder(Context context) {
            super(context);
            setClipChildren(false);
        }
    }

    /** Material's "arrow back" icon, drawn from its path, as NSH's. */
    private static final class BackArrow extends Drawable {
        private final Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG);
        private final Path path = new Path();
        private final int size;

        BackArrow(float density) {
            size = Math.round(24 * density);
            float s = density;
            // M20,11H7.83l5.59-5.59L12,4l-8,8 8,8 1.41-1.41L7.83,13H20v-2z
            path.moveTo(20 * s, 11 * s);
            path.lineTo(7.83f * s, 11 * s);
            path.lineTo(13.42f * s, 5.41f * s);
            path.lineTo(12 * s, 4 * s);
            path.lineTo(4 * s, 12 * s);
            path.lineTo(12 * s, 20 * s);
            path.lineTo(13.41f * s, 18.59f * s);
            path.lineTo(7.83f * s, 13 * s);
            path.lineTo(20 * s, 13 * s);
            path.close();
            setAutoMirrored(true);
        }

        void setColor(int color) {
            paint.setColor(color);
            invalidateSelf();
        }

        @Override
        public void draw(Canvas canvas) {
            canvas.save();
            canvas.translate(getBounds().left, getBounds().top);
            if (isAutoMirrored() && getLayoutDirection() == View.LAYOUT_DIRECTION_RTL) {
                canvas.scale(-1, 1, size / 2f, size / 2f);
            }
            canvas.drawPath(path, paint);
            canvas.restore();
        }

        @Override
        public int getIntrinsicWidth() {
            return size;
        }

        @Override
        public int getIntrinsicHeight() {
            return size;
        }

        @Override
        public void setAlpha(int alpha) {}

        @Override
        public void setColorFilter(ColorFilter filter) {}

        @Override
        @SuppressWarnings("deprecation")
        public int getOpacity() {
            return PixelFormat.TRANSLUCENT;
        }
    }
}
