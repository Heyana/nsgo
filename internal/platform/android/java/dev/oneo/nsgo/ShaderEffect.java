package dev.oneo.nsgo;

import android.annotation.TargetApi;
import android.graphics.Canvas;
import android.graphics.Matrix;
import android.graphics.drawable.Drawable;
import android.util.Log;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.widget.FrameLayout.LayoutParams;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.function.Supplier;

import dev.oneo.nsgo.NsgoView.Group;

/** Takes the pictures an effect reads and swaps its output in for the host's children. UI thread only. */
@TargetApi(29)
final class ShaderEffect {
    final ShaderNode shader;
    Group host;
    boolean stale = true;

    private final View owner;
    private final Supplier<View> root;
    private final ShaderPicture[] inputs = new ShaderPicture[4];
    private ShaderPicture before;
    private int margin = -1;
    private int width, height;
    private boolean needsDraw;

    ShaderEffect(View owner, ShaderNode shader, Supplier<View> root) {
        this.owner = owner;
        this.shader = shader;
        this.root = root;
    }

    private Group target() {
        View view = shader.role == ShaderNode.SCREEN || shader.role == ShaderNode.SCREEN_TRANSITION ? root.get() : owner;
        return view instanceof Group ? (Group) view : null;
    }

    private boolean transition() {
        return shader.role == ShaderNode.TRANSITION || shader.role == ShaderNode.SCREEN_TRANSITION
                || shader.role == ShaderNode.NAV;
    }

    private int frozenSide() {
        return shader.backwards ? ShaderNode.TO : ShaderNode.FROM;
    }

    private int required() {
        int pictures = transition()
                ? (1 << ShaderNode.FROM) | (1 << ShaderNode.TO)
                : (1 << ShaderNode.CONTENT) | (1 << ShaderNode.BACKDROP);
        return shader.reads & pictures;
    }

    void start() {
        Group view = target();
        if (view == null || !view.isAttachedToWindow() || view.getWidth() <= 0 || view.getHeight() <= 0) return;
        if (before != null) before.close();
        before = take(null, view, shader.margin, ShaderNode.CONTENT, true);
    }

    void configure() {
        if (transition() && !shader.running) {
            detach();
            return;
        }
        if (before != null) {
            // The picture taken before the batch belongs to the side its progress starts on.
            closeInputs();
            inputs[frozenSide()] = before;
            before = null;
            shader.resetEffectFrame();
        }
        attach();
        needsDraw = true;
        dirty();
    }

    void dirty() {
        stale = true;
        if (host != null) shader.postInvalidateOnAnimation();
    }

    private void attach() {
        Group next = target();
        if (next != host) {
            removeOutput();
            host = next;
        }
        if (host == null || host.effect != null && host.effect != this) return;
        if (margin != shader.margin || shader.getParent() != host) {
            margin = shader.margin;
            shader.resetEffectFrame();
            LayoutParams params = new LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT,
                    Gravity.LEFT | Gravity.TOP);
            params.setMargins(-margin, -margin, -margin, -margin);
            if (shader.getParent() != host) {
                NsgoView.detach(shader);
                host.addView(shader, params);
            } else {
                shader.setLayoutParams(params);
            }
            host.effect = this;
            shader.pictureShown = () -> {
                if (host != null) host.invalidate();
            };
            stale = true;
        }
    }

    void capture() {
        if (!shader.available() || transition() && !shader.running) return;
        attach();
        if (host == null || host.effect != this || !host.isAttachedToWindow() || !host.isHardwareAccelerated()
                || host.getWidth() <= 0 || host.getHeight() <= 0) return;
        if (width != host.getWidth() || height != host.getHeight()) {
            width = host.getWidth();
            height = host.getHeight();
            shader.resetEffectFrame();
            stale = true;
        }
        int required = required();
        if (stale) {
            int fixed = transition() ? frozenSide() : -1;
            stale = false;
            for (int i = 0; i < inputs.length; i++) {
                if (i == fixed || (required & (1 << i)) == 0) continue;
                ShaderPicture picture = inputs[i];
                if (picture != null && (picture.width != width + 2 * margin || picture.height != height + 2 * margin)) {
                    picture.close();
                    picture = null;
                }
                ShaderPicture captured = take(picture, host, margin, i, false);
                if (inputs[i] != captured) needsDraw = true;
                inputs[i] = captured;
            }
        }
        if (needsDraw) {
            needsDraw = false;
            shader.inputs(inputs, required);
        }
    }

    private ShaderPicture take(ShaderPicture picture, Group view, int padding, int slot, boolean previous) {
        try {
            if (picture == null) {
                picture = new ShaderPicture(view.getWidth() + 2 * padding, view.getHeight() + 2 * padding, () -> {
                    shader.draw();
                    if (stale && host != null) shader.postInvalidateOnAnimation();
                });
            }
            boolean recorded = picture.take(canvas -> {
                canvas.translate(padding, padding);
                if (slot == ShaderNode.BACKDROP) drawBehind(canvas, view);
                else view.picture(canvas, previous);
            });
            if (previous && !recorded) {
                picture.close();
                return null;
            }
            if (!recorded) stale = true;
            return picture;
        } catch (RuntimeException e) {
            Log.w("nsgo", "shader picture capture", e);
            if (picture != null) picture.close();
            return null;
        }
    }

    private void closeInputs() {
        for (ShaderPicture picture : inputs) {
            if (picture != null) picture.close();
        }
        Arrays.fill(inputs, null);
    }

    private void removeOutput() {
        if (host != null && host.effect == this) {
            host.effect = null;
            host.invalidate();
        }
        shader.pictureShown = null;
        shader.resetEffectFrame();
        shader.inputs(null, 0);
        NsgoView.detach(shader);
        host = null;
    }

    void detach() {
        removeOutput();
        closeInputs();
        if (before != null) {
            before.close();
            before = null;
        }
        width = height = 0;
    }

    /** Records the surface up to view, walking its ancestors and earlier siblings in drawing order. */
    private void drawBehind(Canvas canvas, View view) {
        View surface = root.get();
        ArrayList<View> chain = new ArrayList<>();
        for (View v = view; v != null; v = v.getParent() instanceof View ? (View) v.getParent() : null) {
            chain.add(v);
            if (v == surface) break;
        }
        int last = chain.size() - 1;
        if (last <= 0 || chain.get(last) != surface) return;
        Matrix place = new Matrix(), origin = new Matrix();
        view.transformMatrixToGlobal(place);
        if (!place.invert(origin)) return;
        place.reset();
        surface.transformMatrixToGlobal(place);
        origin.preConcat(place);
        int saved = canvas.save();
        canvas.concat(origin);
        drawBehind(canvas, chain, last);
        canvas.restoreToCount(saved);
    }

    private static void drawBehind(Canvas canvas, List<View> chain, int depth) {
        ViewGroup group = (ViewGroup) chain.get(depth);
        View next = chain.get(depth - 1);
        Drawable background = group.getBackground();
        if (background != null) background.draw(canvas);
        for (int i = 0; i < group.getChildCount(); i++) {
            View child = group.getChildAt(i);
            if (child == next && depth == 1) break;
            if (child.getVisibility() != View.VISIBLE || child instanceof ShaderNode) continue;
            int saved = canvas.save();
            canvas.translate(child.getLeft() - group.getScrollX(), child.getTop() - group.getScrollY());
            canvas.concat(child.getMatrix());
            if (child.getAlpha() < 1) canvas.saveLayerAlpha(null, Math.round(child.getAlpha() * 255));
            if (child == next) drawBehind(canvas, chain, depth - 1);
            else child.draw(canvas);
            canvas.restoreToCount(saved);
            if (child == next) break;
        }
    }
}
