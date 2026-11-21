package dev.oneo.nsgo;

final class Text {
    static int argb(int rgba) {
        return (rgba >>> 8) | (rgba << 24);
    }
}
