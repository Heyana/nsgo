package dev.oneo.nsgo;

final class Text {
    static final class Shadow {
        final int color;
        final float dx, dy, radius;

        Shadow(int color, float dx, float dy, float radius) {
            this.color = color;
            this.dx = dx;
            this.dy = dy;
            this.radius = radius;
        }
    }

    static final class Decoration {
        final boolean underline, strike;
        final int style, color;
        final float under, through, thickness;

        Decoration(boolean underline, boolean strike, int style, int color, float under, float through, float thickness) {
            this.underline = underline;
            this.strike = strike;
            this.style = style;
            this.color = color;
            this.under = under;
            this.through = through;
            this.thickness = thickness;
        }
    }

    static final class Background {
        final int color;
        final float radius, top, bottom;

        Background(int color, float radius, float top, float bottom) {
            this.color = color;
            this.radius = radius;
            this.top = top;
            this.bottom = bottom;
        }
    }

    static final class DrawnLine {
        final android.text.StaticLayout layout;
        final float dx, dy;

        DrawnLine(android.text.StaticLayout layout, float dx, float dy) {
            this.layout = layout;
            this.dx = dx;
            this.dy = dy;
        }
    }

    static DrawnLine[] drawLines(android.text.StaticLayout layout, int clipLines) {
        return null;
    }

    static int utf8Length(CharSequence s, int end) {
        return end;
    }
}
