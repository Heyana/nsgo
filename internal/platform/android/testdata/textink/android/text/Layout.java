package android.text;

public abstract class Layout {
    public static final int DIR_LEFT_TO_RIGHT = 1;
    public static final int DIR_RIGHT_TO_LEFT = -1;

    private final TextPaint paint = new TextPaint();

    public TextPaint getPaint() { return paint; }

    /** The text this layout drew, as Android's Layout.getText. */
    public abstract CharSequence getText();
    public abstract int getHeight();
    public abstract int getLineCount();
    public abstract int getLineForVertical(int vertical);
    public abstract float getLineLeft(int line);
    public abstract float getLineRight(int line);
    public abstract int getLineStart(int line);
    public abstract int getLineEnd(int line);
    public abstract int getOffsetForHorizontal(int line, float horiz);
    public abstract boolean isRtlCharAt(int offset);
    public abstract float getPrimaryHorizontal(int offset);
}
