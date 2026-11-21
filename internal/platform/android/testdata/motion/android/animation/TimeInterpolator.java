package android.animation;

/** The IME interpolator interface LayoutTransition needs to compile on the JVM. */
public interface TimeInterpolator {
    float getInterpolation(float input);
}
