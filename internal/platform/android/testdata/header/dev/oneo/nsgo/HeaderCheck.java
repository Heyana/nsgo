package dev.oneo.nsgo;

import android.content.Context;
import android.view.View;
import android.widget.Toolbar;
import java.io.DataInputStream;
import java.util.HashMap;
import java.util.Map;

public final class HeaderCheck {
    public static void main(String[] args) throws Exception {
        final float density = 2.0f;
        Context ctx = new Context();
        Header header = new Header(ctx, density, () -> {});
        Toolbar toolbar = (Toolbar) header.view.getChildAt(0);
        // Standard 56dp navigation button width = 112px at 2x density.
        toolbar.setNavButtonWidth(Math.round(56 * density));

        Map<Integer, Header.Holder> holders = new HashMap<>();
        Map<View, Integer> ids = new HashMap<>();
        Header.Holder left = new Header.Holder(ctx);
        Header.size(left, Math.round(40 * density), Math.round(24 * density));
        holders.put(1, left);
        ids.put(left, 1);

        Header.Holder right = new Header.Holder(ctx);
        Header.size(right, Math.round(50 * density), Math.round(24 * density));
        holders.put(2, right);
        ids.put(right, 2);

        Header.Holder title = new Header.Holder(ctx);
        Header.size(title, Math.round(120 * density), Math.round(32 * density));
        holders.put(3, title);
        ids.put(title, 3);

        DataInputStream in = new DataInputStream(System.in);

        // Scenario 1: filling titleView with back arrow (56dp) + left (40dp) + right (50dp + 12dp end margin) in a 354dp (708px) toolbar => 354 - 56 - 40 - 50 - 12 = 196dp.
        header.configure(in, holders::get);
        StringBuilder order = new StringBuilder();
        for (int i = 0; i < toolbar.getChildCount(); i++) {
            if (i > 0) order.append(',');
            order.append(ids.get(toolbar.getChildAt(i)));
        }
        header.measure(Math.round(354 * density));
        int fillLpWidth = title.getLayoutParams().width;
        float measuredDp = title.getMeasuredWidth() / density;
        float tw1 = header.titleWidth();
        // Resizing the filling holder from core preserves MATCH_PARENT width.
        Header.size(title, Math.round(120 * density), Math.round(36 * density));
        int afterSizeFillLpWidth = title.getLayoutParams().width;
        System.out.printf("fill-with-ends:order=%s lpWidth=%d measured=%.1f titleWidth=%.1f afterSizeLpWidth=%d%n",
                order, fillLpWidth, measuredDp, tw1, afterSizeFillLpWidth);

        // Scenario 2: toggling titleFills back to false restores the saved 120dp (240px) core width.
        header.configure(in, holders::get);
        header.measure(Math.round(354 * density));
        int intrinsicLpWidth = title.getLayoutParams().width;
        float tw2 = header.titleWidth();
        Header.size(title, Math.round(100 * density), Math.round(32 * density));
        int afterSizeIntrinsicLpWidth = title.getLayoutParams().width;
        System.out.printf("intrinsic-after-toggle:lpWidth=%d titleWidth=%.1f afterSizeLpWidth=%d%n",
                intrinsicLpWidth, tw2, afterSizeIntrinsicLpWidth);

        // Scenario 3: filling titleView alone (no back arrow, no left/right) spans the full 400dp bar.
        header.configure(in, holders::get);
        header.measure(Math.round(400 * density));
        System.out.printf("fill-alone:lpWidth=%d measured=%.1f titleWidth=%.1f%n",
                title.getLayoutParams().width, title.getMeasuredWidth() / density, header.titleWidth());
    }
}
