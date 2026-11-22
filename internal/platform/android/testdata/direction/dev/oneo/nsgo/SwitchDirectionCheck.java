package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.Canvas;
import android.view.View;

public final class SwitchDirectionCheck {
    public static void main(String[] args) {
        Switch sw = new Switch(new Context());
        Canvas c = new Canvas();

        // LTR unchecked: thumb sits on left (minX = 0 + 8 + 8 = 16).
        sw.setLayoutDirection(View.LAYOUT_DIRECTION_LTR);
        sw.setChecked(false);
        sw.drawForTest(c);
        check("LTR off", c.lastCircleX, 16f);

        // LTR checked: thumb sits on right (maxX = 0 + 52 - 8 - 12 = 32).
        sw.setChecked(true);
        sw.drawForTest(c);
        check("LTR on", c.lastCircleX, 32f);

        // Switching to RTL invalidates and mirrors checked thumb to left (minX = 0 + 8 + 12 = 20).
        int before = sw.invalidations;
        sw.setLayoutDirection(View.LAYOUT_DIRECTION_RTL);
        if (sw.invalidations <= before) {
            throw new AssertionError("setLayoutDirection did not invalidate Switch");
        }
        sw.drawForTest(c);
        check("RTL on", c.lastCircleX, 20f);

        // RTL unchecked: thumb sits on right (maxX = 0 + 52 - 8 - 8 = 36).
        sw.setChecked(false);
        sw.drawForTest(c);
        check("RTL off", c.lastCircleX, 36f);
    }

    private static void check(String name, float got, float want) {
        if (Math.abs(got - want) > 0.01f) {
            throw new AssertionError(name + ": thumb cx=" + got + ", want " + want);
        }
    }
}
