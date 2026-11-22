package android.widget;

import android.content.Context;
import android.text.TextUtils;
import android.view.View;

public class TextView extends View {
    public TextView(Context context) { super(context); }
    public void setTextSize(int unit, float size) {}
    public void setSingleLine(boolean singleLine) {}
    public void setEllipsize(TextUtils.TruncateAt where) {}
    public void setTextColor(int color) {}
    public void setText(CharSequence text) {}
    public android.graphics.Typeface getTypeface() { return null; }
    public void setTypeface(android.graphics.Typeface typeface) {}
    public int getLineHeight() { return 0; }
}
