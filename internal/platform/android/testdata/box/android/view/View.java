package android.view;

import android.graphics.Paint;
import android.graphics.RenderEffect;
import android.graphics.drawable.Drawable;

public class View {
    public static final int LAYER_TYPE_NONE = 0;
    public static final int LAYER_TYPE_HARDWARE = 2;

    public float translationX, translationY, alpha = 1f;
    public int renderEffectCalls, translationYCalls, alphaCalls;

    public Drawable getBackground() { return null; }
    public boolean getClipToOutline() { return false; }
    public int getWidth() { return 0; }
    public int getHeight() { return 0; }
    public void setAlpha(float alpha) { this.alpha = alpha; alphaCalls++; }
    public void setElevation(float elevation) {}
    public void setOutlineAmbientShadowColor(int color) {}
    public void setOutlineSpotShadowColor(int color) {}
    public void setTranslationX(float x) { this.translationX = x; }
    public void setTranslationY(float y) { this.translationY = y; translationYCalls++; }
    public void setScaleX(float x) {}
    public void setScaleY(float y) {}
    public void setRotation(float r) {}
    public void setRotationX(float r) {}
    public void setRotationY(float r) {}
    public void setCameraDistance(float d) {}
    public void setPivotX(float x) {}
    public void setPivotY(float y) {}
    public boolean isPivotSet() { return false; }
    public void resetPivot() {}
    public void setRenderEffect(RenderEffect effect) { renderEffectCalls++; }
    public int getLayerType() { return LAYER_TYPE_NONE; }
    public void setLayerType(int layerType, Paint paint) {}
}
