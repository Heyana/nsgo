package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.Canvas;
import android.graphics.ColorFilter;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.PixelFormat;
import android.graphics.Typeface;
import android.graphics.drawable.Drawable;
import android.text.TextUtils;
import android.util.TypedValue;
import android.view.ContextThemeWrapper;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.widget.FrameLayout;
import android.widget.TextView;
import android.widget.Toolbar;
import java.io.DataInputStream;
import java.io.IOException;
import java.util.function.IntFunction;

/**
 * A screen's system header, as NSH's StackBox.ScreenBox: a Material 3 top app
 * bar over the top of the screen's view. A regular bar pins a 64 dp Toolbar
 * below the status bar; a large-title bar adds an 88 dp band below the pinned
 * Toolbar holding a 28 sp headline that collapses into the 22 sp toolbar title
 * as the screen's top scroll view scrolls under it. The core keeps the stack;
 * the toolbar's arrow reports back for the screen.
 */
final class Header {
    /** Material 3's top app bar height, in points. */
    static final float BAR = HeaderLayout.BAR;
    /** Material 3's large top app bar height before it collapses, in points. */
    static final float LARGE_BAR = HeaderLayout.LARGE_BAR;

    /** Root header container added to the screen, carrying the bar's background and visibility. */
    final FrameLayout view;
    private final Toolbar toolbar;
    private final FrameLayout largeBand;
    private final TextView largeTitle;
    private final BackArrow arrow;
    private final float density;
    /** The bar's height and filling titleView width last reported to the engine, in points. */
    float reported;
    /** The screen insets last reported to the engine (top, right, bottom, left), in points. */
    float[] reportedInsets;
    float reportedTitleWidth;
    /** The titleView's holder, if any, and whether it fills the room between the ends. */
    private View titleHolder;
    private boolean titleFills;
    /** Whether the screen asked for a large title and trailing actions beside it, and how far it collapsed, in points. */
    private boolean large, rightWithLargeTitle;
    private View customTitle, trailing;
    private final int toolbarEndInset;
    private Typeface toolbarTypeface;
    private final Typeface largeTypeface;
    private HeaderFont titleFont = new HeaderFont(), largeTitleFont = new HeaderFont();
    private float collapsed, scrolled;
    private float largeHeight = HeaderLayout.COLLAPSE;
    /** The toolbar's measured bar height (at least BAR), in points. */
    private float measuredBar = BAR;
    /** The header area's height the core set, status bar included, in pixels. */
    private int full;
    /** The header area's height the core set, in points. */
    float heightPoints;

    /** The colors the core set, 0 for the theme's, and whether the bar is transparent. */
    private int background, titleColor, tint;
    private boolean transparent;
    /** Whether the theme's colors are the dark theme's. */
    private boolean dark;
    /** Resolved ARGB title color before collapse alpha, and the last color applied to the toolbar title. */
    private int resolvedTitleColor, lastToolbarTitleColor;
    private boolean titleColorApplied;
    /** Last accessibility mode applied: -1 unset, 0 regular, 1 large title shown, 2 collapsed toolbar title shown. */
    private int a11yState = -1;

    Header(Context context, float density, Runnable back) {
        this.density = density;
        view = new FrameLayout(context) {
            @Override protected void onLayout(boolean changed, int left, int top, int right, int bottom) {
                super.onLayout(changed, left, top, right, bottom);
                placeTrailing();
            }
        };
        // Over the screen's content, which the core inserts after it, and
        // without the shadow its elevation would cast.
        view.setOutlineProvider(null);
        view.setTranslationZ(1);
        view.setClipChildren(true);

        toolbar = new Toolbar(context);
        toolbarEndInset = toolbar.getContentInsetEnd();
        toolbar.setBackgroundColor(0);
        arrow = new BackArrow(density);
        toolbar.setNavigationOnClickListener(v -> back.run());
        view.addView(toolbar, new FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, Math.round(BAR * density), Gravity.TOP));

        largeBand = new FrameLayout(context);
        largeBand.setClipChildren(true);
        largeBand.setVisibility(View.GONE);

        largeTitle = new TextView(context);
        largeTypeface = largeTitle.getTypeface();
        largeTitle.setTextSize(TypedValue.COMPLEX_UNIT_SP, HeaderLayout.LARGE_TITLE);
        largeTitle.setSingleLine(true);
        largeTitle.setEllipsize(TextUtils.TruncateAt.END);
        largeTitle.setTextAlignment(View.TEXT_ALIGNMENT_VIEW_START);
        int padH = Math.round(HeaderLayout.LARGE_PADDING_HORIZONTAL * density);
        int padB = Math.round(HeaderLayout.LARGE_PADDING_BOTTOM * density);
        largeTitle.setPaddingRelative(padH, 0, padH, padB);
        largeBand.addView(largeTitle, new FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT,
                Gravity.BOTTOM | Gravity.START));
        view.addView(largeBand, new FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, 0, Gravity.TOP));
    }

    /** Whether the header is hidden or transparent. */
    boolean hidden() { return toolbar.getVisibility() != View.VISIBLE; }
    boolean transparent() { return transparent; }
    /** Colors the bar: the core's colors, else the light or dark theme's, as the app shows. */
    void colors(boolean dark) {
        this.dark = dark;
        Context context = new ContextThemeWrapper(view.getContext(), dark
                ? android.R.style.Theme_DeviceDefault_NoActionBar : android.R.style.Theme_DeviceDefault_Light_NoActionBar);
        int text = themeColor(context, android.R.attr.textColorPrimary, dark ? 0xffffffff : 0xff1f2328);
        resolvedTitleColor = titleColor != 0 ? Text.argb(titleColor) : text;
        largeTitle.setTextColor(resolvedTitleColor);
        int themeFill = themeColor(context, android.R.attr.colorBackground, dark ? 0xff000000 : 0xffffffff);
        int customFill = background != 0 ? Text.argb(background) : 0;
        view.setBackgroundColor(HeaderLayout.backgroundColor(customFill, transparent, themeFill));
        arrow.setColor(tint != 0 ? Text.argb(tint) : text);
        titleColorApplied = false;
        applyTitleState();
    }

    /**
     * Applies an ItemScreen as android.go writes it; wrap returns the
     * holder of a header view, or null.
     */
    void configure(DataInputStream in, IntFunction<View> wrap) throws IOException {
        String title = Protocol.readString(in);
        int flags = in.readUnsignedByte();
        boolean hidden = Protocol.flag(flags, 0), transparent = Protocol.flag(flags, 1);
        large = Protocol.flag(flags, 2);
        boolean canGoBack = Protocol.flag(flags, 3), backHidden = Protocol.flag(flags, 4);
        rightWithLargeTitle = Protocol.flag(flags, 7);
        // A second flags byte follows the first, which is full.
        titleFills = Protocol.flag(in.readUnsignedByte(), 0);
        String backTitle = Protocol.readString(in);
        int background = in.readInt(), titleColor = in.readInt(), tint = in.readInt();
        int left = in.readInt(), right = in.readInt(), titleView = in.readInt();
        float height = in.readFloat();
        titleFont = HeaderFont.read(in);
        largeTitleFont = HeaderFont.read(in);

        this.background = background;
        this.titleColor = titleColor;
        this.tint = tint;
        this.transparent = transparent;
        boolean hasTitleView = titleView != 0;
        if (!large) collapsed = 0;
        heightPoints = height;
        full = Math.round(height * density);

        view.setVisibility(hidden ? View.GONE : View.VISIBLE);
        String shownTitle = hasTitleView ? null : title;
        toolbar.setTitle(shownTitle);
        largeTitle.setText(title);
        largeTitleFont.apply(largeTitle, HeaderLayout.LARGE_TITLE, largeTypeface);
        styleToolbarTitle();

        boolean back = canGoBack && !backHidden;
        toolbar.setNavigationIcon(back ? arrow : null);
        toolbar.setNavigationContentDescription(back ? upDescription(backTitle) : null);
        // The header views: the core sized left and right, and a filling titleView takes MATCH_PARENT after both ends are measured so Toolbar leaves it the exact room between them.
        if (titleHolder instanceof Holder) ((Holder) titleHolder).fills = false;
        for (int i = toolbar.getChildCount() - 1; i >= 0; i--) {
            if (toolbar.getChildAt(i) instanceof Holder) toolbar.removeViewAt(i);
        }
        if (trailing != null && trailing.getParent() == view) view.removeView(trailing);
        trailing = null;
        add(wrap, left, Gravity.START, false);
        int reserved = 0;
        if (large && rightWithLargeTitle && right != 0) {
            trailing = wrap.apply(right);
            if (trailing != null) {
                if (trailing.getParent() instanceof ViewGroup) ((ViewGroup) trailing.getParent()).removeView(trailing);
                ViewGroup.LayoutParams size = trailing.getLayoutParams();
                int width = size != null ? Math.max(1, size.width) : 1;
                int actionHeight = size != null ? Math.max(1, size.height) : 1;
                FrameLayout.LayoutParams p = new FrameLayout.LayoutParams(width, actionHeight, Gravity.TOP | Gravity.END);
                p.setMarginEnd(Math.round(12 * density));
                view.addView(trailing, p);
                reserved = width + Math.round(28 * density);
            }
        } else add(wrap, right, Gravity.END, false);
        // The title goes in last, so a filling one takes the room the ends leave.
        customTitle = titleHolder = add(wrap, titleView, Gravity.START, titleFills);
        toolbar.setContentInsetsRelative(toolbar.getContentInsetStart(), Math.max(toolbarEndInset, reserved));
        largeTitle.setPaddingRelative(Math.round(HeaderLayout.LARGE_PADDING_HORIZONTAL * density), 0,
                Math.max(Math.round(HeaderLayout.LARGE_PADDING_HORIZONTAL * density), reserved),
                Math.round(HeaderLayout.LARGE_PADDING_BOTTOM * density));

        a11yState = -1;
        colors(dark);
        collapse();
    }

    /** Collapses a large title as the scroll view at the top of the screen moves its content scrolled points under the header, as Material's large top app bar does: the bar shrinks with the content, so the core keeps the expanded height and the content's inset. */
    void follow(float scrolled) {
        if (!large) return;
        this.scrolled = scrolled;
        float c = HeaderLayout.collapse(true, scrolled, largeHeight);
        if (c == collapsed) return;
        collapsed = c;
        collapse();
    }

    /** Sizes the header container, pins the toolbar below the status bar, and shrinks the large-title band below it. */
    private void collapse() {
        int h = HeaderLayout.containerHeightPx(full, collapsed, density);
        ViewGroup.LayoutParams p = view.getLayoutParams();
        if (p == null) {
            view.setLayoutParams(new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, h, Gravity.TOP));
        } else if (p.height != h) {
            p.height = h;
            view.setLayoutParams(p);
        }

        int barPx = Math.round(measuredBar * density);
        float expanded = HeaderLayout.expandedBar(large, measuredBar, largeHeight);
        int barTop = HeaderLayout.barTopPx(full, expanded, density);
        FrameLayout.LayoutParams tp = (FrameLayout.LayoutParams) toolbar.getLayoutParams();
        if (tp.height != barPx || tp.topMargin != barTop) {
            tp.height = barPx;
            tp.topMargin = barTop;
            toolbar.setLayoutParams(tp);
        }

        boolean showLarge = large;
        int bandHeight = showLarge ? HeaderLayout.largeBandHeightPx(h, barTop, barPx) : 0;
        if (bandHeight > 0) {
            int bandTop = barTop + barPx;
            FrameLayout.LayoutParams bp = (FrameLayout.LayoutParams) largeBand.getLayoutParams();
            if (bp.height != bandHeight || bp.topMargin != bandTop) {
                bp.height = bandHeight;
                bp.topMargin = bandTop;
                largeBand.setLayoutParams(bp);
            }
            if (largeBand.getVisibility() != View.VISIBLE) largeBand.setVisibility(View.VISIBLE);
        } else if (largeBand.getVisibility() != View.GONE) {
            largeBand.setVisibility(View.GONE);
        }

        applyTitleState();
        placeTrailing();
    }

    /** Keeps the trailing view beside the headline until it reaches the pinned toolbar. */
    private void placeTrailing() {
        if (trailing == null) return;
        float expanded = HeaderLayout.expandedBar(large, measuredBar, largeHeight);
        int barTop = HeaderLayout.barTopPx(full, expanded, density);
        int padB = largeTitle.getPaddingBottom();
        int titleHeight = Math.max(largeTitle.getLineHeight(), largeTitle.getMeasuredHeight() - padB);
        int actionHeight = trailing.getLayoutParams().height;
        trailing.setTranslationY(HeaderLayout.actionTopPx(
                HeaderLayout.containerHeightPx(full, collapsed, density), barTop,
                Math.round(measuredBar * density), titleHeight, padB, actionHeight));
    }

    /** Updates the large and collapsed title opacities and accessibility visibility for the current collapse fraction. */
    private void applyTitleState() {
        boolean showLarge = large;
        float fraction = HeaderLayout.fraction(showLarge, collapsed, largeHeight);
        largeTitle.setAlpha(showLarge ? HeaderLayout.largeTitleAlpha(fraction) : 0f);
        if (customTitle != null) {
            float alpha = HeaderLayout.toolbarTitleAlpha(showLarge, fraction);
            customTitle.setAlpha(alpha);
            customTitle.setVisibility(alpha > 0 ? View.VISIBLE : View.INVISIBLE);
            customTitle.setImportantForAccessibility(alpha > 0 ? View.IMPORTANT_FOR_ACCESSIBILITY_AUTO
                    : View.IMPORTANT_FOR_ACCESSIBILITY_NO_HIDE_DESCENDANTS);
        }
        int argb = HeaderLayout.withAlpha(resolvedTitleColor, HeaderLayout.toolbarTitleAlpha(showLarge, fraction));
        if (!titleColorApplied || argb != lastToolbarTitleColor) {
            titleColorApplied = true;
            lastToolbarTitleColor = argb;
            toolbar.setTitleTextColor(argb);
        }
        int nextA11y = !showLarge ? 0 : (fraction < HeaderLayout.CROSSFADE ? 1 : 2);
        if (nextA11y != a11yState) {
            a11yState = nextA11y;
            TextView titleView = toolbarTitleView();
            if (nextA11y == 0) {
                largeTitle.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_NO);
                if (titleView != null) titleView.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_AUTO);
            } else if (nextA11y == 1) {
                largeTitle.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_YES);
                if (titleView != null) titleView.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_NO);
            } else {
                largeTitle.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_NO);
                if (titleView != null) titleView.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_YES);
            }
        }
    }

    /** Applies the compact font overrides, with Material 3's 22 sp title as the default. */
    private void styleToolbarTitle() {
        TextView titleView = toolbarTitleView();
        if (titleView != null) {
            if (toolbarTypeface == null) toolbarTypeface = titleView.getTypeface();
            titleFont.apply(titleView, HeaderLayout.TITLE, toolbarTypeface);
        }
    }

    /** Returns the Toolbar's own title TextView if created (Holders are FrameLayouts, never TextViews). */
    private TextView toolbarTitleView() {
        for (int i = 0; i < toolbar.getChildCount(); i++) {
            View child = toolbar.getChildAt(i);
            if (child instanceof TextView) return (TextView) child;
        }
        return null;
    }

    /** The bar's height as the toolbar lays its title and header views out across width pixels, in points: at least BAR, and LARGE_BAR with a large title, which the core keeps as the bar collapses. */
    float measure(int width) {
        toolbar.measure(View.MeasureSpec.makeMeasureSpec(Math.max(width, 1), View.MeasureSpec.EXACTLY),
                View.MeasureSpec.makeMeasureSpec(0, View.MeasureSpec.UNSPECIFIED));
        int actionHeight = trailing == null ? 0 : trailing.getLayoutParams().height;
        float bar = Math.max(BAR, Math.round(Math.max(toolbar.getMeasuredHeight(), actionHeight) / density));
        float band = HeaderLayout.COLLAPSE;
        if (large) {
            largeTitle.measure(View.MeasureSpec.makeMeasureSpec(Math.max(width, 1), View.MeasureSpec.EXACTLY),
                    View.MeasureSpec.makeMeasureSpec(0, View.MeasureSpec.UNSPECIFIED));
            band = Math.max(band, (float) Math.ceil(largeTitle.getMeasuredHeight() / density));
        }
        if (bar != measuredBar || band != largeHeight) {
            measuredBar = bar;
            largeHeight = band;
            collapsed = HeaderLayout.collapse(large, scrolled, largeHeight);
            collapse();
        }
        return HeaderLayout.expandedBar(large, bar, largeHeight);
    }

    /**
     * The room the toolbar left for its filling titleView during the last
     * measure, in points, or 0 when no filling titleView is set.
     */
    float titleWidth() {
        if (titleHolder == null || !titleFills) return 0;
        int w = titleHolder.getMeasuredWidth();
        return w > 0 ? w / density : 0;
    }

    private View add(IntFunction<View> wrap, int id, int gravity, boolean fills) {
        View holder = id != 0 ? wrap.apply(id) : null;
        if (holder == null) return null;
        if (holder.getParent() instanceof ViewGroup) ((ViewGroup) holder.getParent()).removeView(holder);
        int w = 1, h = 1;
        if (holder instanceof Holder) {
            Holder hh = (Holder) holder;
            hh.fills = fills;
            w = hh.width;
            h = hh.height;
        } else {
            ViewGroup.LayoutParams size = holder.getLayoutParams();
            if (size != null) {
                w = Math.max(1, size.width);
                h = Math.max(1, size.height);
            }
        }
        Toolbar.LayoutParams p = new Toolbar.LayoutParams(
                fills ? ViewGroup.LayoutParams.MATCH_PARENT : w, h,
                gravity | Gravity.CENTER_VERTICAL);
        if (gravity == Gravity.END) p.setMarginEnd(Math.round(12 * density));
        toolbar.addView(holder, p);
        return holder;
    }

    /** Font overrides written by writeHeaderFont; unset fields retain native defaults. */
    private static final class HeaderFont {
        String family = "";
        float size;
        int weight;
        boolean italic;

        static HeaderFont read(DataInputStream in) throws IOException {
            HeaderFont font = new HeaderFont();
            font.family = Protocol.readString(in);
            font.size = in.readFloat();
            font.weight = in.readUnsignedShort();
            font.italic = Protocol.flag(in.readUnsignedByte(), 0);
            return font;
        }

        void apply(TextView view, float defaultSize, Typeface defaultTypeface) {
            view.setTextSize(TypedValue.COMPLEX_UNIT_SP, size > 0 ? size : defaultSize);
            view.setTypeface(family.isEmpty() && weight == 0 && !italic ? defaultTypeface
                    : Text.typeface(family, weight > 0 ? weight : defaultTypeface != null ? defaultTypeface.getWeight() : 400, italic));
        }
    }

    /** Sizes the holder of a header view to its frame, in pixels. */
    static void size(View holder, int width, int height) {
        int w = Math.max(1, width);
        int h = Math.max(1, height);
        boolean fills = false;
        if (holder instanceof Holder) {
            Holder hh = (Holder) holder;
            hh.width = w;
            hh.height = h;
            fills = hh.fills;
        }
        int lw = fills ? ViewGroup.LayoutParams.MATCH_PARENT : w;
        ViewGroup.LayoutParams p = holder.getLayoutParams();
        if (p == null) {
            holder.setLayoutParams(new FrameLayout.LayoutParams(lw, h));
            return;
        }
        p.width = lw;
        p.height = h;
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

    /** Holds a header view at its frame's size, or MATCH_PARENT width when filling, where the toolbar puts it. */
    static final class Holder extends FrameLayout {
        boolean fills;
        int width = 1, height = 1;

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
        public boolean onLayoutDirectionChanged(int layoutDirection) {
            invalidateSelf();
            return true;
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
