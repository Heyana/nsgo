package dev.oneo.nsgo;

import android.graphics.BlendMode;
import android.graphics.ColorMatrix;
import android.graphics.ColorMatrixColorFilter;
import android.graphics.Paint;
import android.graphics.PorterDuff;
import android.graphics.PorterDuffColorFilter;
import android.graphics.RenderEffect;
import android.graphics.Shader;
import android.os.Build;
import android.view.View;

/**
 * The animatable part of what an ItemViewStyle draws, in pixels and ARGB:
 * what a transition moves from and to.
 */
final class Look {
    int background, borderColor, shadowColor;
    float radius, border, opacity = 1, shadowRadius;
    float translateX, translateY, scaleX = 1, scaleY = 1, rotation;
    /**
     * The style's own Scale, ScaleX and ScaleY, 0 when unset, which
     * scaleX and scaleY combine as core.Transform.Scales does: animated
     * scales combine with them again.
     */
    float rawScale, rawScaleX, rawScaleY;
    /**
     * Each side's border width and color (top, right, bottom, left), set
     * when they differ; otherwise every side is border and borderColor.
     */
    boolean sides;
    float[] borders = new float[4];
    int[] borderColors = new int[4];
    /** Each corner's radius (top left, top right, bottom right, bottom left), null when radius rounds them all. */
    float[] corners;
    boolean continuous;
    int borderStyle;
    Outline outline;
    Shadow[] shadows = {};
    Box.Gradient[] gradients = {};
    /** Turns out of the plane, the depth they are seen from and where they act, from the center. */
    float rotationX, rotationY, cameraDistance, originX, originY;
    boolean backfaceHidden;
    /** The depth the children's turns are seen from, 0 for none. */
    float childPerspective;
    /** A color matrix of the filters (4×5, offsets from 0 to 1), null for none, and their blur. */
    float[] colorMatrix;
    float blur;
    /** The filters' drop shadow, in pixels, null for none: drawn from Android 12. */
    Shadow dropShadow;
    /** The blend mode, as driver.BlendMode numbers them. */
    int blend;

    /** One box shadow, in pixels. */
    static final class Shadow {
        float dx, dy, blur, spread;
        int color;
        boolean inset;
    }

    /** An outline, in pixels. */
    static final class Outline {
        float width, offset;
        int color, style;
    }

    Look copy() {
        Look l = new Look();
        l.background = background;
        l.borderColor = borderColor;
        l.shadowColor = shadowColor;
        l.radius = radius;
        l.border = border;
        l.opacity = opacity;
        l.shadowRadius = shadowRadius;
        l.translateX = translateX;
        l.translateY = translateY;
        l.scaleX = scaleX;
        l.scaleY = scaleY;
        l.rotation = rotation;
        l.rawScale = rawScale;
        l.rawScaleX = rawScaleX;
        l.rawScaleY = rawScaleY;
        l.sides = sides;
        l.borders = borders.clone();
        l.borderColors = borderColors.clone();
        l.corners = corners;
        l.continuous = continuous;
        l.borderStyle = borderStyle;
        l.outline = outline;
        l.shadows = shadows;
        l.gradients = gradients;
        l.rotationX = rotationX;
        l.rotationY = rotationY;
        l.cameraDistance = cameraDistance;
        l.originX = originX;
        l.originY = originY;
        l.backfaceHidden = backfaceHidden;
        l.childPerspective = childPerspective;
        l.colorMatrix = colorMatrix;
        l.blur = blur;
        l.dropShadow = dropShadow;
        l.blend = blend;
        return l;
    }

    /**
     * The look t of the way from a to b for the properties in mask (all
     * when 0), and b's for the rest. Corner radius moves with the border.
     */
    static Look mix(Look a, Look b, float t, int mask) {
        Look l = b.copy();
        if (has(mask, Protocol.TRANSITION_OPACITY)) l.opacity = Math.max(0, Math.min(1, mix(a.opacity, b.opacity, t)));
        if (has(mask, Protocol.TRANSITION_BACKGROUND)) l.background = mix(a.background, b.background, t);
        if (has(mask, Protocol.TRANSITION_BORDER)) {
            l.borderColor = mix(a.borderColor, b.borderColor, t);
            l.border = Math.max(0, mix(a.border, b.border, t));
            l.radius = Math.max(0, mix(a.radius, b.radius, t));
            // A look with one border moves as one with it on every side.
            l.sides = a.sides || b.sides;
            for (int i = 0; i < 4; i++) {
                l.borders[i] = Math.max(0, mix(a.side(i), b.side(i), t));
                l.borderColors[i] = mix(a.sideColor(i), b.sideColor(i), t);
            }
        }
        if (has(mask, Protocol.TRANSITION_SHADOW)) {
            l.shadowColor = mix(a.shadowColor, b.shadowColor, t);
            l.shadowRadius = Math.max(0, mix(a.shadowRadius, b.shadowRadius, t));
        }
        if (has(mask, Protocol.TRANSITION_TRANSFORM)) {
            l.translateX = mix(a.translateX, b.translateX, t);
            l.translateY = mix(a.translateY, b.translateY, t);
            l.scaleX = mix(a.scaleX, b.scaleX, t);
            l.scaleY = mix(a.scaleY, b.scaleY, t);
            l.rotation = mix(a.rotation, b.rotation, t);
        }
        return l;
    }

    /**
     * Draws the look on view through its background. View transforms
     * scale, then rotate, then translate around the center, as core's do,
     * and touches follow them.
     */
    void apply(View view, Box bg) {
        bg.decorate(this);
        boolean away = backfaceHidden && Math.cos(Math.toRadians(rotationX)) * Math.cos(Math.toRadians(rotationY)) < 0;
        view.setAlpha(away ? 0 : opacity);
        boolean shadowed = (shadowColor >>> 24) != 0 && shadowRadius > 0;
        view.setElevation(shadowed ? shadowRadius : 0);
        if (shadowed) {
            view.setOutlineAmbientShadowColor(shadowColor);
            view.setOutlineSpotShadowColor(shadowColor);
        }
        view.setTranslationX(translateX);
        view.setTranslationY(translateY);
        view.setScaleX(scaleX);
        view.setScaleY(scaleY);
        view.setRotation(rotation);
        view.setRotationX(rotationX);
        view.setRotationY(rotationY);
        if (cameraDistance > 0) view.setCameraDistance(cameraDistance);
        if (originX != 0 || originY != 0) {
            view.setPivotX(view.getWidth() / 2f + originX);
            view.setPivotY(view.getHeight() / 2f + originY);
        } else if (view.isPivotSet()) {
            view.resetPivot();
        }
        effects(view);
    }

    /**
     * Gives the view its filters and blend mode: RenderEffects from
     * Android 12, else a layer whose paint takes the color matrix and the
     * blend mode, without the blur.
     */
    private void effects(View view) {
        ColorMatrixColorFilter filter = null;
        if (colorMatrix != null) {
            float[] m = colorMatrix.clone();
            // Android's offsets are from 0 to 255.
            for (int i = 4; i < 20; i += 5) m[i] *= 255;
            filter = new ColorMatrixColorFilter(new ColorMatrix(m));
        }
        if (Build.VERSION.SDK_INT >= 31) {
            RenderEffect effect = null;
            if (filter != null) effect = RenderEffect.createColorFilterEffect(filter);
            if (blur > 0) {
                RenderEffect b = RenderEffect.createBlurEffect(blur, blur, Shader.TileMode.DECAL);
                effect = effect == null ? b : RenderEffect.createChainEffect(b, effect);
            }
            if (dropShadow != null && (dropShadow.color >>> 24) != 0) effect = dropShadow(effect, dropShadow);
            view.setRenderEffect(effect);
            filter = null;
        }
        if (filter == null && blend == 0) {
            if (view.getLayerType() != View.LAYER_TYPE_NONE) view.setLayerType(View.LAYER_TYPE_NONE, null);
            return;
        }
        Paint paint = new Paint();
        paint.setColorFilter(filter);
        if (blend != 0 && Build.VERSION.SDK_INT >= 29) paint.setBlendMode(BLENDS[Math.min(blend, BLENDS.length - 1)]);
        view.setLayerType(View.LAYER_TYPE_HARDWARE, paint);
    }

    /**
     * The view's drawing through effect, then under it its shadow s: what
     * it draws in s's color, blurred as a box shadow's blur, offset.
     */
    private static RenderEffect dropShadow(RenderEffect effect, Shadow s) {
        RenderEffect drawn = effect != null ? effect : RenderEffect.createOffsetEffect(0, 0);
        RenderEffect shadow = RenderEffect.createColorFilterEffect(
                new PorterDuffColorFilter(s.color, PorterDuff.Mode.SRC_IN), drawn);
        // A box shadow's blur is twice its sigma; RenderEffect's radius
        // has a sigma of 0.577 radius + 0.5.
        float radius = Math.max(0, (s.blur / 2 - 0.5f) / 0.57735f);
        if (radius > 0) shadow = RenderEffect.createBlurEffect(radius, radius, shadow, Shader.TileMode.DECAL);
        shadow = RenderEffect.createOffsetEffect(s.dx, s.dy, shadow);
        return RenderEffect.createBlendModeEffect(shadow, drawn, BlendMode.SRC_OVER);
    }

    /** Android's blend modes in driver.BlendMode's order, normal first. */
    private static final BlendMode[] BLENDS = Build.VERSION.SDK_INT >= 29 ? new BlendMode[] {
        BlendMode.SRC_OVER, BlendMode.MULTIPLY, BlendMode.SCREEN, BlendMode.OVERLAY, BlendMode.DARKEN,
        BlendMode.LIGHTEN, BlendMode.COLOR_DODGE, BlendMode.COLOR_BURN, BlendMode.HARD_LIGHT, BlendMode.SOFT_LIGHT,
        BlendMode.DIFFERENCE, BlendMode.EXCLUSION, BlendMode.HUE, BlendMode.SATURATION, BlendMode.COLOR,
        BlendMode.LUMINOSITY, BlendMode.PLUS,
    } : null;

    /** Sets scaleX and scaleY from the raw scales, as core.Transform.Scales. */
    void combineScales() {
        float[] xy = scales(rawScale, rawScaleX, rawScaleY);
        scaleX = xy[0];
        scaleY = xy[1];
    }

    private static float[] scales(float s, float sx, float sy) {
        float x = 1, y = 1;
        if (s != 0) x = y = s;
        if (sx != 0) x = sx;
        if (sy != 0) y = sy;
        return new float[] {x, y};
    }

    /**
     * The look with an animator's bound values in place of its own, as
     * core.AnimatedValues.Apply: points become pixels at density.
     */
    Look animated(Animator.Values v, float density) {
        Look l = copy();
        if (v.has(Animator.PROP_OPACITY)) l.opacity = v.opacity;
        if (v.has(Animator.PROP_TRANSLATE_X)) l.translateX = v.translateX * density;
        if (v.has(Animator.PROP_TRANSLATE_Y)) l.translateY = v.translateY * density;
        boolean s = v.has(Animator.PROP_SCALE), sx = v.has(Animator.PROP_SCALE_X), sy = v.has(Animator.PROP_SCALE_Y);
        if (s || sx || sy) {
            float[] xy = scales(s ? v.scale : rawScale, sx ? v.scaleX : rawScaleX, sy ? v.scaleY : rawScaleY);
            l.scaleX = xy[0];
            l.scaleY = xy[1];
        }
        if (v.has(Animator.PROP_ROTATE)) l.rotation = v.rotate;
        if (v.has(Animator.PROP_BACKGROUND)) l.background = Text.argb(v.background);
        if (v.has(Animator.PROP_BORDER_COLOR)) {
            l.borderColor = Text.argb(v.borderColor);
            if (l.sides) java.util.Arrays.fill(l.borderColors, l.borderColor);
        }
        if (v.has(Animator.PROP_SHADOW_COLOR)) l.shadowColor = Text.argb(v.shadowColor);
        if (v.has(Animator.PROP_SHADOW_RADIUS)) l.shadowRadius = v.shadowRadius * density;
        if (v.has(Animator.PROP_SHADOW_OPACITY)) {
            int a = Math.round(((l.shadowColor >>> 24) & 0xff) * Math.max(0, Math.min(1, v.shadowOpacity)));
            l.shadowColor = (l.shadowColor & 0x00ffffff) | (a << 24);
        }
        if (v.has(Animator.PROP_ROTATE_X)) l.rotationX = v.rotateX;
        if (v.has(Animator.PROP_ROTATE_Y)) l.rotationY = v.rotateY;
        return l;
    }

    /** Side i's border width and color, whether or not the sides differ. */
    float side(int i) {
        return sides ? borders[i] : border;
    }

    int sideColor(int i) {
        return sides ? borderColors[i] : borderColor;
    }

    private static boolean has(int mask, int property) {
        return mask == 0 || (mask & property) != 0;
    }

    private static float mix(float a, float b, float t) {
        return a + (b - a) * t;
    }

    /** Mixes two ARGB colors channel by channel, clamped for curves that overshoot. */
    private static int mix(int a, int b, float t) {
        int c = 0;
        for (int shift = 0; shift < 32; shift += 8) {
            int x = (a >>> shift) & 0xff, y = (b >>> shift) & 0xff;
            c |= Math.max(0, Math.min(255, Math.round(x + (y - x) * t))) << shift;
        }
        return c;
    }
}
