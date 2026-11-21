package dev.oneo.nsgo;

final class Text {
    static final class Shadow {
        int color;
        float dx, dy, radius;
    }

    static final class Decoration {
        int color, style;
        boolean underline, strike;
        float thickness, under, through;
    }

    static final class Background {
        int color;
        float top, bottom, radius;
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
}
