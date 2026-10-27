package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.ImageDecoder;
import android.graphics.ColorFilter;
import android.graphics.Matrix;
import android.graphics.PorterDuff;
import android.graphics.PorterDuffColorFilter;
import android.graphics.RenderEffect;
import android.graphics.Shader;
import android.graphics.drawable.AnimatedImageDrawable;
import android.graphics.drawable.BitmapDrawable;
import android.graphics.drawable.Drawable;
import android.os.Build;
import android.util.DisplayMetrics;
import android.widget.FrameLayout;
import android.widget.ImageView;
import java.nio.ByteBuffer;

/**
 * An image node: its placeholder under its source, each an ImageView
 * whose matrix fits its drawable by the resize mode and position, as
 * core.ResizeMode.Fit does. Go numbers each change of its sources as a
 * request, and the node shows only the bitmaps of its current one. The
 * source fades in over the placeholder. Animated GIF and WebP play with
 * AnimatedImageDrawable, each node its own copy.
 */
final class ImageNode extends FrameLayout {
    private static final int DEFAULT_FADE = 300;

    private final ImageView placeholder, source;
    /** The request whose bitmaps the node shows. */
    int request;
    /** Whether the request's source is shown, which the placeholder then stays under. */
    private boolean loaded;
    private int fade = DEFAULT_FADE;
    private int mode = Protocol.RESIZE_COVER;
    /** The position on each axis: a fraction of the free space plus pixels. */
    private float xFraction = 0.5f, x, yFraction = 0.5f, y;
    /**
     * Before Android 12: the blur radius in pixels, and each view's sharp
     * bitmap with the scale and radius its blurred copy was made at.
     */
    private float blur;
    private final Sharp placeholderSharp = new Sharp(), sourceSharp = new Sharp();

    private static final class Sharp {
        Drawable image;
        float scale, radius;
    }

    ImageNode(Context context) {
        super(context);
        placeholder = image(context);
        source = image(context);
    }

    private ImageView image(Context context) {
        ImageView view = new ImageView(context);
        // The node itself is what assistive technology reads.
        view.setImportantForAccessibility(IMPORTANT_FOR_ACCESSIBILITY_NO);
        view.setScaleType(ImageView.ScaleType.MATRIX);
        addView(view, new LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT));
        return view;
    }

    /**
     * Applies an ItemImage: the resize mode, the blur radius in pixels and
     * the fade in milliseconds, 0 for the default. A new request clears the
     * node until its bitmaps arrive.
     */
    void configure(int request, int mode, float blur, int fadeMillis) {
        this.mode = mode;
        fade = fadeMillis > 0 ? fadeMillis : DEFAULT_FADE;
        // Blurring a view needs API 31; before it fit blurs the bitmaps.
        if (Build.VERSION.SDK_INT >= 31) {
            RenderEffect effect = blur > 0 ? RenderEffect.createBlurEffect(blur, blur, Shader.TileMode.CLAMP) : null;
            placeholder.setRenderEffect(effect);
            source.setRenderEffect(effect);
        } else {
            this.blur = blur;
        }
        fit(placeholder);
        fit(source);
        if (request == this.request) return;
        this.request = request;
        loaded = false;
        source.animate().cancel();
        show(placeholder, null);
        show(source, null);
    }

    /** Sets the position: each axis a fraction of the free space plus pixels. */
    void position(float xFraction, float x, float yFraction, float y) {
        this.xFraction = xFraction;
        this.x = x;
        this.yFraction = yFraction;
        this.y = y;
        fit(placeholder);
        fit(source);
    }

    @Override
    protected void onSizeChanged(int w, int h, int oldw, int oldh) {
        super.onSizeChanged(w, h, oldw, oldh);
        fit(placeholder);
        fit(source);
    }

    /**
     * Fits view's drawable in the node as core.ResizeMode.Fit does: scaled
     * by the mode (Center keeps the natural size, ScaleDown shrinks only a
     * larger one), then placed at the position; it may reach outside the
     * node, which clips it. The drawable's intrinsic size is its natural
     * size in pixels.
     */
    private void fit(ImageView view) {
        Drawable d = view.getDrawable();
        int w = getWidth(), h = getHeight();
        if (d == null || w == 0 || h == 0) return;
        float nw = d.getIntrinsicWidth(), nh = d.getIntrinsicHeight();
        if (mode == Protocol.RESIZE_STRETCH || nw <= 0 || nh <= 0) {
            view.setScaleType(ImageView.ScaleType.FIT_XY);
            if (nw > 0 && nh > 0) blurOld(view, (w / nw + h / nh) / 2);
            return;
        }
        float contain = Math.min(w / nw, h / nh);
        float s;
        switch (mode) {
            case Protocol.RESIZE_CONTAIN: s = contain; break;
            case Protocol.RESIZE_CENTER: s = 1; break;
            case Protocol.RESIZE_SCALE_DOWN: s = Math.min(contain, 1); break;
            default: s = Math.max(w / nw, h / nh);
        }
        float freeX = w - nw * s, freeY = h - nh * s;
        Matrix m = new Matrix();
        m.setScale(s, s);
        m.postTranslate(xFraction * freeX + x, yFraction * freeY + y);
        view.setScaleType(ImageView.ScaleType.MATRIX);
        view.setImageMatrix(m);
        blurOld(view, s);
    }

    /**
     * Before Android 12, shows view's bitmap blurred as RenderEffect would
     * blur it drawn at scale: shrunk to that scale first, which costs less
     * and loses only what the blur hides, and drawn at the sharp one's size.
     */
    private void blurOld(ImageView view, float scale) {
        if (Build.VERSION.SDK_INT >= 31) return;
        Sharp sharp = view == placeholder ? placeholderSharp : sourceSharp;
        if (!(sharp.image instanceof BitmapDrawable)) return;
        if (blur <= 0) {
            if (view.getDrawable() != sharp.image) view.setImageDrawable(sharp.image);
            sharp.radius = 0;
            return;
        }
        if (sharp.radius == blur && sharp.scale == scale) return;
        sharp.radius = blur;
        sharp.scale = scale;
        BitmapDrawable image = (BitmapDrawable) sharp.image;
        Bitmap bitmap = image.getBitmap();
        int bw = bitmap.getWidth(), bh = bitmap.getHeight();
        int iw = image.getIntrinsicWidth(), ih = image.getIntrinsicHeight();
        // The pixels a bitmap pixel covers on screen: the drawable's
        // intrinsic size may differ from the bitmap's by their densities.
        float drawn = scale * iw / Math.max(bw, 1);
        float shrink = Math.min(drawn, 1);
        Bitmap small = shrink < 1
                ? Bitmap.createScaledBitmap(bitmap, Math.max(1, Math.round(bw * shrink)), Math.max(1, Math.round(bh * shrink)), true)
                : bitmap;
        Bitmap blurred = Blur.blurred(small, Blur.stackRadius(blur * shrink / drawn));
        view.setImageDrawable(new BitmapDrawable(getResources(), blurred) {
            @Override
            public int getIntrinsicWidth() {
                return iw;
            }

            @Override
            public int getIntrinsicHeight() {
                return ih;
            }
        });
    }

    /** Recolors the opaque pixels with argb, or keeps their colors for 0. */
    void setTint(int argb) {
        ColorFilter filter = argb == 0 ? null : new PorterDuffColorFilter(argb, PorterDuff.Mode.SRC_IN);
        placeholder.setColorFilter(filter);
        source.setColorFilter(filter);
    }

    void showPlaceholder(Drawable image) {
        if (!loaded) show(placeholder, image);
    }

    /** Fades the source in, dropping the placeholder once it covers it. */
    void showSource(Drawable image) {
        loaded = true;
        show(source, image);
        source.setAlpha(0f);
        source.animate().alpha(1f).setDuration(fade).withEndAction(() -> show(placeholder, null));
    }

    /** Shows image in view, fitted, playing it when it is animated and stopping what it showed. */
    private void show(ImageView view, Drawable image) {
        Drawable old = view.getDrawable();
        if (old instanceof AnimatedImageDrawable && old != image) ((AnimatedImageDrawable) old).stop();
        (view == placeholder ? placeholderSharp : sourceSharp).image = image;
        (view == placeholder ? placeholderSharp : sourceSharp).radius = 0;
        view.setImageDrawable(image);
        fit(view);
        if (image instanceof AnimatedImageDrawable) ((AnimatedImageDrawable) image).start();
    }

    /** A decoded image: a drawable and its size in pixels. */
    static final class Decoded {
        Drawable drawable;
        int width, height;
    }

    /**
     * Decodes an image off the UI thread as decode does, but GIF and WebP
     * with ImageDecoder, so animated ones come as an AnimatedImageDrawable,
     * sampled down while they stay at least width by height pixels. Null
     * when data is no image.
     */
    static Decoded decodeDrawable(Context context, byte[] data, float scale, int width, int height) {
        Decoded d = new Decoded();
        if (animatable(data)) {
            try {
                Drawable drawable = ImageDecoder.decodeDrawable(ImageDecoder.createSource(ByteBuffer.wrap(data)),
                        (decoder, info, src) -> {
                            d.width = info.getSize().getWidth();
                            d.height = info.getSize().getHeight();
                            int sample = 1;
                            while (d.width / (sample * 2) >= Math.max(width, 1)
                                    && d.height / (sample * 2) >= Math.max(height, 1)) {
                                sample *= 2;
                            }
                            decoder.setTargetSampleSize(sample);
                        });
                if (drawable instanceof AnimatedImageDrawable) {
                    d.drawable = drawable;
                    return d;
                }
            } catch (Exception e) {
                // Leave it to BitmapFactory.
            }
        }
        int[] size = new int[2];
        Bitmap bitmap = decode(data, scale, width, height, size);
        if (bitmap == null) return null;
        d.drawable = new BitmapDrawable(context.getResources(), bitmap);
        d.width = size[0];
        d.height = size[1];
        return d;
    }

    /** Whether data is a GIF or a WebP, which may be animated. */
    private static boolean animatable(byte[] data) {
        boolean gif = data.length >= 6 && data[0] == 'G' && data[1] == 'I' && data[2] == 'F' && data[3] == '8';
        boolean webp = data.length >= 12 && data[0] == 'R' && data[1] == 'I' && data[2] == 'F' && data[3] == 'F'
                && data[8] == 'W' && data[9] == 'E' && data[10] == 'B' && data[11] == 'P';
        return gif || webp;
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
