package dev.oneo.nsgo;

import android.graphics.Bitmap;
import android.graphics.Matrix;
import android.graphics.Rect;
import android.graphics.RectF;
import android.os.Handler;
import android.os.HandlerThread;
import android.view.PixelCopy;
import android.view.View;
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
        if (window == null) {
            done.accept(null, "this platform has no surface capture");
            return;
        }
        if (target == null || !target.isShown() || !target.isAttachedToWindow()
                || target.getWindowToken() != surface.getWindowToken()) {
            done.accept(null, "android: capture view is outside the window");
            return;
        }
        Rect region = bounds(target), visible = bounds(surface);
        if (!region.intersect(visible)
                || !region.intersect(0, 0, window.getDecorView().getWidth(), window.getDecorView().getHeight())) {
            done.accept(null, "android: capture has no visible pixels");
            return;
        }
        copy(window, surface, region, true, done);
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

    private static void copy(Window window, View surface, Rect region, boolean retry, BiConsumer<byte[], String> done) {
        Bitmap bitmap = Bitmap.createBitmap(region.width(), region.height(), Bitmap.Config.ARGB_8888);
        try {
            PixelCopy.request(window, region, bitmap, result -> {
                try {
                    if (result == PixelCopy.ERROR_SOURCE_NO_DATA && retry && surface.isAttachedToWindow()) {
                        surface.postOnAnimation(() -> copy(window, surface, region, false, done));
                    } else if (result != PixelCopy.SUCCESS) {
                        done.accept(null, "android: copying the window failed (PixelCopy " + result + ")");
                    } else {
                        ByteArrayOutputStream png = new ByteArrayOutputStream();
                        if (!bitmap.compress(Bitmap.CompressFormat.PNG, 100, png)) {
                            done.accept(null, "android: cannot encode the window's pixels");
                        } else if (png.size() > Protocol.MAX_MESSAGE_SIZE - 12) {
                            done.accept(null, "android: captured PNG is too large for the platform bridge");
                        } else {
                            done.accept(png.toByteArray(), "");
                        }
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
}
