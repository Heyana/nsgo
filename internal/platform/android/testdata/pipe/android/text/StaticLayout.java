package android.text;

public final class StaticLayout {
    public final String text;
    public final float width, fontSize;

    public StaticLayout(String text, float width, float fontSize) {
        this.text = text;
        this.width = width;
        this.fontSize = fontSize;
    }
}
