package android.view;

import android.content.Context;

public class ScaleGestureDetector {
    public static class SimpleOnScaleGestureListener {
        public boolean onScaleBegin(ScaleGestureDetector detector) {
            return true;
        }

        public boolean onScale(ScaleGestureDetector detector) {
            return false;
        }
    }

    public ScaleGestureDetector(Context context, SimpleOnScaleGestureListener listener) {}

    public boolean onTouchEvent(MotionEvent e) {
        return true;
    }

    public float getScaleFactor() {
        return 1f;
    }

    public float getFocusX() {
        return 0f;
    }

    public float getFocusY() {
        return 0f;
    }
}
