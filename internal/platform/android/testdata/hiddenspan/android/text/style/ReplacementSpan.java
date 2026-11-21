package android.text.style;

import android.graphics.Canvas;
import android.graphics.Paint;
import android.text.TextPaint;

public abstract class ReplacementSpan extends MetricAffectingSpan {
    public abstract int getSize(Paint paint, CharSequence text, int start, int end, Paint.FontMetricsInt fm);
    public abstract void draw(Canvas canvas, CharSequence text, int start, int end, float x, int top, int y, int bottom, Paint paint);

    @Override public void updateMeasureState(TextPaint paint) {}
    @Override public void updateDrawState(TextPaint paint) {}
}
