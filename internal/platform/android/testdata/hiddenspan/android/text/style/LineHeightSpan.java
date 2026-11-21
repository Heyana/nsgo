package android.text.style;

import android.graphics.Paint;

public interface LineHeightSpan {
    void chooseHeight(CharSequence text, int start, int end, int spanstartv, int lineHeight, Paint.FontMetricsInt fm);
}
