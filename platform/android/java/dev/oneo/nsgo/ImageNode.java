package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.ColorFilter;
import android.graphics.PorterDuff;
import android.graphics.PorterDuffColorFilter;
import android.graphics.RenderEffect;
import android.graphics.Shader;
import android.graphics.drawable.BitmapDrawable;
import android.os.Build;
import android.util.DisplayMetrics;
import android.widget.FrameLayout;
import android.widget.ImageView;

/**
 * An image node: its placeholder under its source, each an ImageView
 * fitted by the resize mode. Go numbers each change of its sources as a
 * request, and the node shows only the bitmaps of its current one. The
 * source fades in over the placeholder.
 */
final class ImageNode extends FrameLayout {
    private static final int DEFAULT_FADE = 300;

    private final ImageView placeholder, source;
    /** The request whose bitmaps the node shows. */
    int request;
    /** Whether the request's source is shown, which the placeholder then stays under. */
    private boolean loaded;
    private int fade = DEFAULT_FADE;

    ImageNode(Context context) {
        super(context);
        placeholder = image(context);
        source = image(context);
    }

    private ImageView image(Context context) {
        ImageView view = new ImageView(context);
        // The node itself is what assistive technology reads.
        view.setImportantForAccessibility(IMPORTANT_FOR_ACCESSIBILITY_NO);
        view.setScaleType(ImageView.ScaleType.CENTER_CROP);
        addView(view, new LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT));
        return view;
    }

    /**
     * Applies an ItemImage: the resize mode, the blur radius in pixels and
     * the fade in milliseconds, 0 for the default. A new request clears the
     * node until its bitmaps arrive.
     */
    void configure(int request, int mode, float blur, int fadeMillis) {
        ImageView.ScaleType type = scaleType(mode);
        placeholder.setScaleType(type);
        source.setScaleType(type);
        fade = fadeMillis > 0 ? fadeMillis : DEFAULT_FADE;
        // Blurring a view needs API 31; older systems show it sharp.
        if (Build.VERSION.SDK_INT >= 31) {
            RenderEffect effect = blur > 0 ? RenderEffect.createBlurEffect(blur, blur, Shader.TileMode.CLAMP) : null;
            placeholder.setRenderEffect(effect);
            source.setRenderEffect(effect);
        }
        if (request == this.request) return;
        this.request = request;
        loaded = false;
        source.animate().cancel();
        placeholder.setImageDrawable(null);
        source.setImageDrawable(null);
    }

    /** Recolors the opaque pixels with argb, or keeps their colors for 0. */
    void setTint(int argb) {
        ColorFilter filter = argb == 0 ? null : new PorterDuffColorFilter(argb, PorterDuff.Mode.SRC_IN);
        placeholder.setColorFilter(filter);
        source.setColorFilter(filter);
    }

    void showPlaceholder(Bitmap bitmap) {
        if (!loaded) placeholder.setImageDrawable(new BitmapDrawable(getResources(), bitmap));
    }

    /** Fades the source in, dropping the placeholder once it covers it. */
    void showSource(Bitmap bitmap) {
        loaded = true;
        source.setImageDrawable(new BitmapDrawable(getResources(), bitmap));
        source.setAlpha(0f);
        source.animate().alpha(1f).setDuration(fade).withEndAction(() -> placeholder.setImageDrawable(null));
    }

    private static ImageView.ScaleType scaleType(int mode) {
        switch (mode) {
            case Protocol.RESIZE_CONTAIN: return ImageView.ScaleType.FIT_CENTER;
            case Protocol.RESIZE_STRETCH: return ImageView.ScaleType.FIT_XY;
            // Its own size, centered, and fitted only when larger.
            case Protocol.RESIZE_CENTER: return ImageView.ScaleType.CENTER_INSIDE;
            default: return ImageView.ScaleType.CENTER_CROP;
        }
    }

    /**
     * Decodes an image off the UI thread, sampled down while it stays at
     * least width by height pixels, and puts its size in pixels in size.
     * Its density makes a point of the drawable scale of its pixels, as
     * "@2x" files declare. Null when data is no image.
     */
    static Bitmap decode(byte[] data, float scale, int width, int height, int[] size) {
        BitmapFactory.Options options = new BitmapFactory.Options();
        options.inJustDecodeBounds = true;
        BitmapFactory.decodeByteArray(data, 0, data.length, options);
        if (options.outWidth <= 0 || options.outHeight <= 0) return null;
        size[0] = options.outWidth;
        size[1] = options.outHeight;
        int sample = 1;
        while (options.outWidth / (sample * 2) >= Math.max(width, 1)
                && options.outHeight / (sample * 2) >= Math.max(height, 1)) {
            sample *= 2;
        }
        options.inJustDecodeBounds = false;
        options.inSampleSize = sample;
        Bitmap bitmap = BitmapFactory.decodeByteArray(data, 0, data.length, options);
        if (bitmap != null) bitmap.setDensity(Math.max(1, Math.round(DisplayMetrics.DENSITY_DEFAULT * scale / sample)));
        return bitmap;
    }
}
