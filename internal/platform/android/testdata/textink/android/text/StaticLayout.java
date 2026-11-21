package android.text;

import android.graphics.Canvas;
import android.graphics.Path;

public class StaticLayout extends Layout {
    public CharSequence text;
    public int width;
    public int height;
    public int lineCount = 1;
    public int baseline = 14;
    public int lineHeight = 18;
    public boolean drew;

    public StaticLayout(CharSequence text, int width, int height, int lineCount, int baseline) {
        this.text = text;
        this.width = width;
        this.height = height;
        this.lineCount = lineCount;
        this.baseline = baseline;
    }

    public CharSequence getText() { return text; }
    public int getWidth() { return width; }
    public int getHeight() { return height; }
    public int getLineCount() { return lineCount; }
    public int getLineTop(int line) { return line * lineHeight; }
    public int getLineStart(int line) { return 0; }
    public int getLineEnd(int line) { return text.length(); }
    public int getLineVisibleEnd(int line) { return text.length(); }
    public int getLineBaseline(int line) { return line * lineHeight + baseline; }
    public float getLineLeft(int line) { return 0; }
    public float getLineRight(int line) { return width; }
    public int getLineForOffset(int offset) { return 0; }
    public int getLineForVertical(int vertical) { return 0; }
    public int getEllipsisCount(int line) { return 0; }
    public int getEllipsisStart(int line) { return 0; }
    public int getParagraphDirection(int line) { return DIR_LEFT_TO_RIGHT; }
    public float getPrimaryHorizontal(int offset) { return offset * (width / (float) Math.max(text.length(), 1)); }
    public int getOffsetForHorizontal(int line, float horiz) { return 0; }
    public boolean isRtlCharAt(int offset) { return false; }
    public void getSelectionPath(int start, int end, Path dest) {}

    public void draw(Canvas canvas) {
        drew = true;
    }
}
