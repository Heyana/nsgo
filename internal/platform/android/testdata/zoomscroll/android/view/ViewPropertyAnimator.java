package android.view;

import android.animation.ValueAnimator;

public class ViewPropertyAnimator {
    public ViewPropertyAnimator translationY(float value) {
        return this;
    }

    public ViewPropertyAnimator setDuration(long duration) {
        return this;
    }

    public ViewPropertyAnimator setUpdateListener(ValueAnimator.AnimatorUpdateListener listener) {
        return this;
    }

    public ViewPropertyAnimator withEndAction(Runnable runnable) {
        return this;
    }

    public void start() {}
}
