package android.text.style;

import android.text.TextPaint;

public abstract class MetricAffectingSpan {
    public abstract void updateMeasureState(TextPaint paint);
    public abstract void updateDrawState(TextPaint paint);
}
