package dev.oneo.nsgo;

import android.graphics.Canvas;
import android.graphics.Paint;
import android.view.View;

/**
 * Verifies Box fast paths, Look.apply incremental diffing on transform-only
 * frames (as DataTable scroll / stack slide), and Animator.changed stale tracking.
 */
public final class BoxPerfCheck {
    public static void main(String[] args) {
        checkEmptyBoxSkipsDraw();
        checkSquareRowFastPath();
        checkTransformOnlySkipsDecorateAndEffects();
        checkAnimatorViewsStaleAndUnbind();
    }

    private static void check(boolean ok, String message) {
        if (!ok) throw new AssertionError(message);
    }

    private static void checkEmptyBoxSkipsDraw() {
        Box box = new Box();
        box.setBounds(0, 0, 120, 32);
        box.decorate(new Look());
        Canvas canvas = new Canvas();
        box.draw(canvas);
        check(canvas.draws.isEmpty(), "transparent empty box drew " + canvas.draws.size() + " ops");
    }

    private static void checkSquareRowFastPath() {
        Look row = new Look();
        row.background = 0xfff8f9fa;
        row.sides = true;
        row.borders = new float[] {0, 0, 1, 0};
        row.borderColors = new int[] {0, 0, 0xffe0e0e0, 0};
        Box box = new Box();
        box.setBounds(0, 0, 400, 36);
        box.decorate(row);
        Canvas canvas = new Canvas();
        box.draw(canvas);
        check(canvas.draws.size() == 2, "square row: expected 2 rect draws, got " + canvas.draws.size());
        Canvas.Draw fill = canvas.draws.get(0);
        Canvas.Draw bottom = canvas.draws.get(1);
        check("rect".equals(fill.kind) && fill.style == Paint.Style.FILL && fill.color == 0xfff8f9fa
                        && fill.minX == 0 && fill.minY == 0 && fill.maxX == 400 && fill.maxY == 36,
                "square row fill mismatch");
        check("rect".equals(bottom.kind) && bottom.style == Paint.Style.FILL && bottom.color == 0xffe0e0e0
                        && bottom.minX == 0 && bottom.minY == 35 && bottom.maxX == 400 && bottom.maxY == 36,
                "square row bottom border mismatch");
    }

    private static void checkTransformOnlySkipsDecorateAndEffects() {
        View view = new View();
        Box box = new Box();
        Look base = new Look();
        base.background = 0xffffffff;
        base.radius = 8;

        Look scratch = new Look();
        Animator.Values vals = new Animator.Values();
        vals.set = 1 << Animator.PROP_TRANSLATE_Y;
        vals.translateY = 10f;

        base.animatedInto(scratch, vals, 2f);
        scratch.apply(view, box);
        check(box.invalidateCount == 1, "first apply must decorate once, got " + box.invalidateCount);
        check(view.translationY == 20f, "first translateY = " + view.translationY);
        int firstEffects = view.renderEffectCalls;
        int firstAlpha = view.alphaCalls;

        // Scroll 200 frames changing only translateY: Box must never invalidateSelf, and alpha/effects must not re-run.
        for (int i = 1; i <= 200; i++) {
            vals.translateY = 10f + i * 4f;
            base.animatedInto(scratch, vals, 2f);
            scratch.apply(view, box);
        }
        check(box.invalidateCount == 1, "transform-only scroll invalidated Box " + box.invalidateCount + " times");
        check(view.renderEffectCalls == firstEffects, "transform-only scroll re-ran RenderEffect");
        check(view.alphaCalls == firstAlpha, "transform-only scroll re-ran setAlpha");
        check(view.translationY == (10f + 200 * 4f) * 2f, "final translateY mismatch: " + view.translationY);
        check(view.translationYCalls == 201, "expected 201 translateY updates, got " + view.translationYCalls);

        // Changing background must re-decorate the box.
        vals.set |= 1 << Animator.PROP_BACKGROUND;
        vals.background = 0x112233ff;
        base.animatedInto(scratch, vals, 2f);
        scratch.apply(view, box);
        check(box.invalidateCount == 2, "background change must re-decorate, got " + box.invalidateCount);
    }

    private static void checkAnimatorViewsStaleAndUnbind() {
        Animator anim = new Animator((id, value, finished) -> {});
        Animator.Op create = new Animator.Op();
        create.kind = Animator.CREATE_VALUE;
        create.node = 1;
        create.value = 0;
        anim.apply(create);

        Animator.Op bind = new Animator.Op();
        bind.kind = Animator.BIND;
        bind.view = 10;
        bind.prop = Animator.PROP_TRANSLATE_Y;
        bind.node = 1;
        anim.apply(bind);

        int[] seen = new int[1];
        float[] lastY = new float[1];
        anim.changed((id, v) -> {
            seen[0]++;
            lastY[0] = v.translateY;
        });
        check(seen[0] == 1 && lastY[0] == 0f, "initial bind delivery failed");

        // No change -> zero deliveries.
        anim.changed((id, v) -> seen[0]++);
        check(seen[0] == 1, "unchanged animator delivered again");

        // Update value -> delivers once.
        Animator.Op set = new Animator.Op();
        set.kind = Animator.SET;
        set.node = 1;
        set.value = 42.5;
        anim.apply(set);
        anim.changed((id, v) -> {
            seen[0]++;
            lastY[0] = v.translateY;
        });
        check(seen[0] == 2 && lastY[0] == 42.5f, "updated value delivery failed");

        // Unbind -> delivers one empty Values to clear n.values.
        Animator.Op unbind = new Animator.Op();
        unbind.kind = Animator.BIND;
        unbind.view = 10;
        unbind.prop = Animator.PROP_TRANSLATE_Y;
        unbind.node = 0;
        anim.apply(unbind);
        int[] clearedSet = new int[] {-1};
        anim.changed((id, v) -> {
            seen[0]++;
            clearedSet[0] = v.set;
        });
        check(seen[0] == 3 && clearedSet[0] == 0, "unbind delivery failed: seen=" + seen[0] + " set=" + clearedSet[0]);
    }
}
