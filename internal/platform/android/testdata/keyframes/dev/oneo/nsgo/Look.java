package dev.oneo.nsgo;

/** The five style fields Keyframes applies, without an Android view. */
final class Look {
    float opacity, scaleX, scaleY, rotation, translateY;

    Look copy() {
        Look result = new Look();
        result.opacity = opacity;
        result.scaleX = scaleX;
        result.scaleY = scaleY;
        result.rotation = rotation;
        result.translateY = translateY;
        return result;
    }
}
