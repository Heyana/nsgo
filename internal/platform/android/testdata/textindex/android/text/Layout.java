package android.text;

import android.graphics.Canvas;
import android.graphics.Path;

public abstract class Layout {
    public static final int DIR_LEFT_TO_RIGHT = 1;
    public static final int DIR_RIGHT_TO_LEFT = -1;

    public abstract CharSequence getText();
    public abstract int getWidth();
    public abstract int getHeight();
    public abstract int getLineCount();
    public abstract int getLineForVertical(int y);
    public abstract int getLineForOffset(int offset);
    public abstract int getLineStart(int line);
    public abstract int getLineEnd(int line);
    public abstract int getLineVisibleEnd(int line);
    public abstract int getLineTop(int line);
    public abstract int getLineBaseline(int line);
    public abstract float getLineLeft(int line);
    public abstract float getLineRight(int line);
    public abstract int getParagraphDirection(int line);
    public abstract boolean isRtlCharAt(int offset);
    public abstract float getPrimaryHorizontal(int offset);
    public abstract int getOffsetForHorizontal(int line, float horiz);
    public int getEllipsisCount(int line) { return 0; }
    public int getEllipsisStart(int line) { return 0; }
    public void getSelectionPath(int start, int end, Path dest) {}
    public void draw(Canvas canvas) {}
}
