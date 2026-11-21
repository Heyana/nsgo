package dev.oneo.nsgo;

import android.graphics.Canvas;
import android.graphics.Paint;

import java.io.BufferedInputStream;
import java.io.BufferedOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.util.List;

/**
 * Checks Look's rim calculation against Go's driver.ViewStyle.Rim and verifies
 * that Box.draw stops the fill inset under the border with the rimmed color.
 */
public final class BoxCheck {
    public static void main(String[] args) throws IOException {
        checkDraw();
        DataInputStream in = new DataInputStream(new BufferedInputStream(System.in));
        try (DataOutputStream out = new DataOutputStream(new BufferedOutputStream(System.out))) {
            int cases = in.readInt();
            for (int i = 0; i < cases; i++) {
                float density = in.readFloat();
                Look look = readLook(in, density);
                boolean ok = look.hasRim();
                out.writeBoolean(ok);
                out.writeInt(ok ? look.rimColor() : 0);
                out.writeFloat(ok ? look.rimInset() : 0);
            }
        }
    }

    /** Reads a Look as NsgoView.style and readDecor populate it, scaled by density. */
    private static Look readLook(DataInputStream in, float density) throws IOException {
        Look look = new Look();
        look.background = Text.argb(in.readInt());
        look.radius = in.readFloat() * density;
        look.border = in.readFloat() * density;
        look.borderColor = Text.argb(in.readInt());
        for (int i = 0; i < 4; i++) look.borders[i] = in.readFloat() * density;
        for (int i = 0; i < 4; i++) look.borderColors[i] = Text.argb(in.readInt());
        look.sides = look.borders[0] > 0 || look.borders[1] > 0 || look.borders[2] > 0 || look.borders[3] > 0;
        float[] corners = new float[4];
        boolean any = false;
        for (int i = 0; i < 4; i++) {
            corners[i] = in.readFloat() * density;
            any |= corners[i] > 0;
        }
        look.corners = any ? corners : null;
        look.borderStyle = in.readUnsignedByte();
        int gradients = in.readUnsignedByte();
        look.gradients = new Box.Gradient[gradients];
        for (int i = 0; i < gradients; i++) look.gradients[i] = new Box.Gradient();
        look.continuous = in.readBoolean();
        return look;
    }

    private static void check(boolean ok, String message) {
        if (!ok) throw new AssertionError(message);
    }

    private static void checkBounds(String label, Canvas.Draw d, float x0, float y0, float x1, float y1) {
        float eps = 1e-3f;
        check(Math.abs(d.minX - x0) <= eps && Math.abs(d.minY - y0) <= eps
                        && Math.abs(d.maxX - x1) <= eps && Math.abs(d.maxY - y1) <= eps,
                label + ": bounds [" + d.minX + ", " + d.minY + ", " + d.maxX + ", " + d.maxY
                        + "], want [" + x0 + ", " + y0 + ", " + x1 + ", " + y1 + "]");
    }

    private static List<Canvas.Draw> draw(Look look, int w, int h) {
        Box box = new Box();
        box.setBounds(0, 0, w, h);
        box.decorate(look);
        Canvas canvas = new Canvas();
        box.draw(canvas);
        return canvas.draws;
    }

    private static Look baseRimmed() {
        Look l = new Look();
        l.radius = 12;
        l.border = 4;
        l.background = 0xffffffff;
        l.borderColor = 0x80000000;
        return l;
    }

    private static void checkDraw() {
        // Opaque fill + translucent border: fill inset by 3, stroke uses opaque rim color 0xff7f7f7f.
        List<Canvas.Draw> draws = draw(baseRimmed(), 100, 40);
        check(draws.size() == 2, "rimmed: expected 2 draws, got " + draws.size());
        Canvas.Draw fill = draws.get(0), stroke = draws.get(1);
        check(fill.style == Paint.Style.FILL && fill.color == 0xffffffff, "rimmed fill mismatch");
        checkBounds("rimmed fill", fill, 3, 3, 97, 37);
        check(stroke.style == Paint.Style.STROKE && stroke.color == 0xff7f7f7f && stroke.strokeWidth == 4,
                "rimmed stroke mismatch: " + Integer.toHexString(stroke.color));
        checkBounds("rimmed stroke", stroke, 2, 2, 98, 38);

        // Translucent fill + opaque border: fill inset by 3, stroke keeps opaque border color.
        Look translucentFill = baseRimmed();
        translucentFill.background = 0xebffffff;
        translucentFill.borderColor = 0xff808080;
        draws = draw(translucentFill, 100, 40);
        check(draws.size() == 2, "translucent fill rim: expected 2 draws");
        check(draws.get(0).color == 0xebffffff, "translucent fill color");
        checkBounds("translucent fill", draws.get(0), 3, 3, 97, 37);
        check(draws.get(1).color == 0xff808080, "translucent fill stroke color");
        checkBounds("translucent fill stroke", draws.get(1), 2, 2, 98, 38);

        // Per-corner radii + continuous corners + explicit BorderSolid (1).
        Look perCorner = baseRimmed();
        perCorner.radius = 0;
        perCorner.corners = new float[] {12, 0, 0, 12};
        perCorner.continuous = true;
        perCorner.borderStyle = 1;
        draws = draw(perCorner, 100, 40);
        check(draws.size() == 2, "per-corner rim: expected 2 draws");
        checkBounds("per-corner fill", draws.get(0), 3, 3, 97, 37);
        check(draws.get(1).color == 0xff7f7f7f, "per-corner stroke color");

        // Sub-pixel hairline border: border() widens stroke to 1 px, so fill insets by 0.75 px.
        Look hairline = baseRimmed();
        hairline.border = 0.5f;
        draws = draw(hairline, 100, 40);
        check(draws.size() == 2, "hairline rim: expected 2 draws");
        checkBounds("hairline fill", draws.get(0), 0.75f, 0.75f, 99.25f, 39.25f);
        check(draws.get(1).strokeWidth == 1f && draws.get(1).color == 0xff7f7f7f, "hairline stroke");
        checkBounds("hairline stroke", draws.get(1), 0.5f, 0.5f, 99.5f, 39.5f);

        // Box narrower than twice the rim inset skips the fill and draws only the rimmed border.
        draws = draw(baseRimmed(), 6, 40);
        check(draws.size() == 1, "tiny rimmed box: expected only stroke, got " + draws.size());
        check(draws.get(0).style == Paint.Style.STROKE && draws.get(0).color == 0xff7f7f7f, "tiny rimmed stroke");

        // Outer shadow still clips out the full outer box, not the inset fill box.
        Look shadowed = baseRimmed();
        Look.Shadow sh = new Look.Shadow();
        sh.dy = 4;
        sh.blur = 8;
        sh.color = 0x40000000;
        shadowed.shadows = new Look.Shadow[] {sh};
        draws = draw(shadowed, 100, 40);
        check(draws.size() == 4, "shadowed rim: expected clipOut + shadow + fill + stroke, got " + draws.size());
        check("clipOut".equals(draws.get(0).kind), "expected clipOut first");
        checkBounds("outer shadow clipOut", draws.get(0), 0, 0, 100, 40);
        checkBounds("shadowed rimmed fill", draws.get(2), 3, 3, 97, 37);
        check(draws.get(3).color == 0xff7f7f7f, "shadowed rimmed stroke");

        // Square corners: no rim, full fill and raw translucent border color.
        Look square = baseRimmed();
        square.radius = 0;
        draws = draw(square, 100, 40);
        check(draws.size() == 2, "square: expected 2 draws");
        checkBounds("square fill", draws.get(0), 0, 0, 100, 40);
        check(draws.get(1).color == 0x80000000, "square stroke keeps raw border color");

        // Translucent fill + translucent border: no rim, full fill and raw border color.
        Look bothSeeThrough = baseRimmed();
        bothSeeThrough.background = 0xebffffff;
        bothSeeThrough.borderColor = 0x80000000;
        draws = draw(bothSeeThrough, 100, 40);
        check(draws.size() == 2, "translucent+translucent: expected 2 draws");
        checkBounds("translucent+translucent fill", draws.get(0), 0, 0, 100, 40);
        check(draws.get(1).color == 0x80000000, "translucent+translucent stroke keeps raw border color");

        // Dashed and dotted borders: no rim, full fill and raw border color.
        for (int style : new int[] {Protocol.BORDER_DASHED, Protocol.BORDER_DOTTED}) {
            Look patterned = baseRimmed();
            patterned.borderStyle = style;
            draws = draw(patterned, 100, 40);
            check(draws.size() == 2, "patterned border: expected 2 draws");
            checkBounds("patterned fill", draws.get(0), 0, 0, 100, 40);
            check(draws.get(1).color == 0x80000000 && draws.get(1).hasPathEffect, "patterned stroke");
        }

        // Gradient fill: no rim, full fill + gradient + raw border color.
        Look grad = baseRimmed();
        Box.Gradient g = new Box.Gradient();
        g.kind = Protocol.GRADIENT_LINEAR;
        g.x1 = 1;
        g.offsets = new float[] {0, 1};
        g.colors = new int[] {0xffff0000, 0xff0000ff};
        grad.gradients = new Box.Gradient[] {g};
        draws = draw(grad, 100, 40);
        check(draws.size() == 3, "gradient: expected fill + gradient + stroke, got " + draws.size());
        checkBounds("gradient base fill", draws.get(0), 0, 0, 100, 40);
        check(draws.get(1).hasShader, "gradient shader");
        check(draws.get(2).color == 0x80000000, "gradient stroke keeps raw border color");

        // Per-side borders: no rim, full fill.
        Look sides = baseRimmed();
        sides.sides = true;
        sides.borders = new float[] {4, 2, 4, 2};
        sides.borderColors = new int[] {0x80000000, 0x80000000, 0x80000000, 0x80000000};
        draws = draw(sides, 100, 40);
        checkBounds("per-side fill", draws.get(0), 0, 0, 100, 40);

        // Transition Look.mix: background transitioning white -> black under 50% black border updates rim mid-flight.
        Look from = baseRimmed();
        Look to = baseRimmed();
        to.background = 0xff000000;
        Look mid = Look.mix(from, to, 0.5f, Protocol.TRANSITION_BACKGROUND);
        draws = draw(mid, 100, 40);
        checkBounds("transition mid fill", draws.get(0), 3, 3, 97, 37);
        int expectedMidRim = Look.over(0x80000000, mid.background);
        check(draws.get(1).color == expectedMidRim && draws.get(1).color != 0xff7f7f7f && draws.get(1).color != 0xff000000,
                "transition mid rim color: " + Integer.toHexString(draws.get(1).color));

        // Transition into translucent background under translucent border drops the rim mid-flight.
        Look clearTo = baseRimmed();
        clearTo.background = 0x00ffffff;
        Look fadeMid = Look.mix(from, clearTo, 0.5f, Protocol.TRANSITION_BACKGROUND);
        draws = draw(fadeMid, 100, 40);
        checkBounds("fading fill drops rim", draws.get(0), 0, 0, 100, 40);
        check(draws.get(1).color == 0x80000000, "fading stroke keeps raw border color");

        // Animator values: animating background or borderColor updates the rim live.
        Animator.Values vals = new Animator.Values();
        vals.set = 1 << Animator.PROP_BACKGROUND;
        vals.background = 0xff0000ff; // opaque red in RGBA
        Look anim = baseRimmed().animated(vals, 1f);
        draws = draw(anim, 100, 40);
        checkBounds("animated fill", draws.get(0), 3, 3, 97, 37);
        check(draws.get(0).color == 0xffff0000, "animated fill color");
        check(draws.get(1).color == 0xff7f0000, "animated rim color: " + Integer.toHexString(draws.get(1).color));

        Animator.Values borderVals = new Animator.Values();
        borderVals.set = 1 << Animator.PROP_BORDER_COLOR;
        borderVals.borderColor = 0x0000ff80; // 50% blue in RGBA
        Look animBorder = baseRimmed().animated(borderVals, 1f);
        draws = draw(animBorder, 100, 40);
        checkBounds("animated border fill", draws.get(0), 3, 3, 97, 37);
        check(draws.get(1).color == 0xff7f7fff, "animated border rim color: " + Integer.toHexString(draws.get(1).color));
    }
}
