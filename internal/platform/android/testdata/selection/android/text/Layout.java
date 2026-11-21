package android.text;

/** A single line with ten points per UTF-16 code unit. */
public final class Layout {
    private final String text;
    private final int width, height;

    public Layout(String text, int width, int height) {
        this.text = text;
        this.width = width;
        this.height = height;
    }

    public CharSequence getText() { return text; }

    public int getWidth() { return width; }

    public int getHeight() { return height; }

    public int getLineForVertical(int y) { return 0; }

    public int getLineForOffset(int offset) { return 0; }

    public int getLineTop(int line) { return 0; }

    public int getLineBottom(int line) { return height; }

    public float getPrimaryHorizontal(int offset) { return offset * 10; }

    public int getOffsetForHorizontal(int line, float x) {
        return Math.min(text.length(), Math.max(0, Math.round(x / 10)));
    }
}
