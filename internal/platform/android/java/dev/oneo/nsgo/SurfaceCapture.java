package dev.oneo.nsgo;

import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.graphics.Matrix;
import android.graphics.Rect;
import android.graphics.RectF;
import android.os.Build;
import android.os.Handler;
import android.os.HandlerThread;
import android.view.PixelCopy;
import android.view.TextureView;
import android.view.View;
import android.view.ViewGroup;
import android.view.Window;
import java.io.ByteArrayOutputStream;
import java.util.function.BiConsumer;

/** Copies an app window's displayed pixels and encodes them away from the UI thread. */
final class SurfaceCapture {
    private static Handler worker;

    private static Handler worker() {
        if (worker == null) {
            HandlerThread thread = new HandlerThread("nsgo-capture");
            thread.start();
            worker = new Handler(thread.getLooper());
        }
        return worker;
    }

    static void take(Window window, View surface, View target, BiConsumer<byte[], String> done) {
        if (target == null || !target.isShown() || !target.isAttachedToWindow()
                || target.getWindowToken() != surface.getWindowToken()) {
            done.accept(null, "android: capture view is outside the window");
            return;
        }
        View root = window != null ? window.getDecorView() : surface.getRootView();
        if (root == null || root.getWidth() <= 0 || root.getHeight() <= 0) {
            done.accept(null, "android: capture has no visible pixels");
            return;
        }
        Rect region = bounds(target), visible = bounds(surface);
        if (!region.intersect(visible) || !region.intersect(0, 0, root.getWidth(), root.getHeight())) {
            done.accept(null, "android: capture has no visible pixels");
            return;
        }
        if (window != null) {
            copyWindow(window, surface, region, true, done);
        } else if (Build.VERSION.SDK_INT >= 34) {
            copyViewWindow(surface, region, true, done);
        } else {
            copySoftware(root, region, done);
        }
    }

    private static Rect bounds(View view) {
        Matrix matrix = new Matrix();
        view.transformMatrixToGlobal(matrix);
        RectF bounds = new RectF(0, 0, view.getWidth(), view.getHeight());
        matrix.mapRect(bounds);
        int[] screen = new int[2], window = new int[2];
        view.getLocationOnScreen(screen);
        view.getLocationInWindow(window);
        bounds.offset(window[0] - screen[0], window[1] - screen[1]);
        Rect rounded = new Rect();
        bounds.roundOut(rounded);
        return rounded;
    }

    private static void encode(Bitmap bitmap, BiConsumer<byte[], String> done) {
        ByteArrayOutputStream png = new ByteArrayOutputStream();
        if (!bitmap.compress(Bitmap.CompressFormat.PNG, 100, png)) {
            done.accept(null, "android: cannot encode the window's pixels");
        } else if (png.size() > Protocol.MAX_MESSAGE_SIZE - 12) {
            done.accept(null, "android: captured PNG is too large for the platform bridge");
        } else {
            done.accept(png.toByteArray(), "");
        }
    }

    private static void copyWindow(Window window, View surface, Rect region, boolean retry, BiConsumer<byte[], String> done) {
        Bitmap bitmap = Bitmap.createBitmap(region.width(), region.height(), Bitmap.Config.ARGB_8888);
        try {
            PixelCopy.request(window, region, bitmap, result -> {
                try {
                    if (result == PixelCopy.ERROR_SOURCE_NO_DATA && retry && surface.isAttachedToWindow()) {
                        surface.postOnAnimation(() -> copyWindow(window, surface, region, false, done));
                    } else if (result != PixelCopy.SUCCESS) {
                        done.accept(null, "android: copying the window failed (PixelCopy " + result + ")");
                    } else {
                        encode(bitmap, done);
                    }
                } finally {
                    bitmap.recycle();
                }
            }, worker());
        } catch (IllegalArgumentException error) {
            bitmap.recycle();
            done.accept(null, "android: copying the window failed (" + error.getMessage() + ")");
        }
    }

    /** Captures an overlay or non-Activity window on API 34+ through PixelCopy.Request.Builder.ofWindow(View). */
    private static void copyViewWindow(View surface, Rect region, boolean retry, BiConsumer<byte[], String> done) {
        if (Build.VERSION.SDK_INT < 34) {
            copySoftware(surface.getRootView(), region, done);
            return;
        }
        Bitmap bitmap = Bitmap.createBitmap(region.width(), region.height(), Bitmap.Config.ARGB_8888);
        try {
            PixelCopy.Request request = PixelCopy.Request.Builder.ofWindow(surface)
                    .setSourceRect(region)
                    .setDestinationBitmap(bitmap)
                    .build();
            PixelCopy.request(request, worker()::post, result -> {
                int status = result.getStatus();
                try {
                    if (status == PixelCopy.ERROR_SOURCE_NO_DATA && retry && surface.isAttachedToWindow()) {
                        surface.postOnAnimation(() -> copyViewWindow(surface, region, false, done));
                    } else if (status != PixelCopy.SUCCESS) {
                        done.accept(null, "android: copying the window failed (PixelCopy " + status + ")");
                    } else {
                        encode(bitmap, done);
                    }
                } finally {
                    bitmap.recycle();
                }
            });
        } catch (IllegalArgumentException error) {
            bitmap.recycle();
            done.accept(null, "android: copying the window failed (" + error.getMessage() + ")");
        }
    }

    /** Fallback for overlay windows on API < 34: renders the view tree and composites visible TextureViews (shaders and GPU views), then encodes PNG off the UI thread. */
    private static void copySoftware(View root, Rect region, BiConsumer<byte[], String> done) {
        Bitmap bitmap = Bitmap.createBitmap(region.width(), region.height(), Bitmap.Config.ARGB_8888);
        try {
            Canvas canvas = new Canvas(bitmap);
            canvas.translate(-region.left, -region.top);
            root.draw(canvas);
            drawTextureViews(root, canvas);
        } catch (RuntimeException error) {
            bitmap.recycle();
            done.accept(null, "android: drawing the window failed (" + error.getMessage() + ")");
            return;
        }
        worker().post(() -> {
            try {
                encode(bitmap, done);
            } finally {
                bitmap.recycle();
            }
        });
    }

    private static void drawTextureViews(View view, Canvas canvas) {
        if (!view.isShown()) return;
        if (view instanceof TextureView) {
            TextureView tv = (TextureView) view;
            if (tv.isAvailable() && tv.getWidth() > 0 && tv.getHeight() > 0) {
                Bitmap tex = tv.getBitmap();
                if (tex != null) {
                    try {
                        Rect b = bounds(tv);
                        canvas.drawBitmap(tex, null, b, null);
                    } finally {
                        tex.recycle();
                    }
                }
            }
            return;
        }
        if (view instanceof ViewGroup) {
            ViewGroup group = (ViewGroup) view;
            for (int i = 0; i < group.getChildCount(); i++) {
                drawTextureViews(group.getChildAt(i), canvas);
            }
        }
    }
}
