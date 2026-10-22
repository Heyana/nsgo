package dev.oneo.nsgo;

import android.graphics.drawable.GradientDrawable;
import android.view.View;

/**
 * The animatable part of what an ItemViewStyle draws, in pixels and ARGB:
 * what a transition moves from and to.
 */
final class Look {
    int background, borderColor, shadowColor;
    float radius, border, opacity = 1, shadowRadius;
    float translateX, translateY, scaleX = 1, scaleY = 1, rotation;

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
    void apply(View view, GradientDrawable bg) {
        bg.setColor(background);
        bg.setCornerRadius(radius);
        // A stroke of width 0 draws nothing.
        bg.setStroke(border > 0 ? Math.max(1, Math.round(border)) : 0, borderColor);
        view.setAlpha(opacity);
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
