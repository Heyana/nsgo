package android.view;

import android.content.Context;
import android.graphics.Matrix;
import android.graphics.SurfaceTexture;

public class TextureView extends View {
    public interface SurfaceTextureListener {
        void onSurfaceTextureAvailable(SurfaceTexture surface, int width, int height);
        void onSurfaceTextureSizeChanged(SurfaceTexture surface, int width, int height);
        boolean onSurfaceTextureDestroyed(SurfaceTexture surface);
        void onSurfaceTextureUpdated(SurfaceTexture surface);
    }

    // Android's real TextureView defaults mOpaque to true.
    private boolean opaque = true;
    private int setOpaqueCalls;

    public TextureView(Context context) { super(context); }
    public void setOpaque(boolean opaque) {
        this.opaque = opaque;
        this.setOpaqueCalls++;
    }
    public boolean isOpaque() { return opaque; }
    public int setOpaqueCalls() { return setOpaqueCalls; }
    public void setSurfaceTextureListener(SurfaceTextureListener l) {}
    public void setTransform(Matrix transform) {}
}
