package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.Canvas;
import android.graphics.RectF;
import android.text.Spanned;
import android.text.StaticLayout;
import android.view.View;
import java.lang.reflect.Array;
import java.util.ArrayList;
import java.util.List;

public final class TextInkCheck {
    private static final class SimpleSpanned implements Spanned {
        private final String s;
        private final List<Object> spans = new ArrayList<>();
        private final List<Integer> starts = new ArrayList<>();
        private final List<Integer> ends = new ArrayList<>();

        SimpleSpanned(String s) {
            this.s = s;
        }

        void add(Object span, int start, int end) {
            spans.add(span);
            starts.add(start);
            ends.add(end);
        }

        @Override public int length() { return s.length(); }
        @Override public char charAt(int i) { return s.charAt(i); }
        @Override public CharSequence subSequence(int a, int b) { return s.subSequence(a, b); }
        @Override public String toString() { return s; }

        @Override
        @SuppressWarnings("unchecked")
        public <T> T[] getSpans(int start, int end, Class<T> type) {
            List<T> out = new ArrayList<>();
            for (int i = 0; i < spans.size(); i++) {
                if (type.isInstance(spans.get(i)) && ends.get(i) > start && starts.get(i) < end) {
                    out.add(type.cast(spans.get(i)));
                }
            }
            T[] arr = (T[]) Array.newInstance(type, out.size());
            return out.toArray(arr);
        }

        @Override
        public int getSpanStart(Object tag) {
            int i = spans.indexOf(tag);
            return i >= 0 ? starts.get(i) : -1;
        }

        @Override
        public int getSpanEnd(Object tag) {
            int i = spans.indexOf(tag);
            return i >= 0 ? ends.get(i) : -1;
        }
    }

    private static void check(boolean ok, String msg) {
        if (!ok) throw new AssertionError(msg);
    }

    public static void main(String[] args) {
        Context ctx = new Context();
        View parent = new View(ctx);
        TextNode node = new TextNode(ctx);
        node.parent = parent;
        node.size(80, 20);

        check(!node.willNotDraw, "TextNode must draw");
        check(!node.clipChildren, "TextNode must disable clipChildren");
        check(!node.clipToPadding, "TextNode must disable clipToPadding");

        // 1. Plain text inside box does not overflow and does not invalidate parent.
        StaticLayout plain = new StaticLayout("hello", 80, 20, 1, 15);
        node.setLayout(plain, 0, 0, 0, null);
        check(parent.invalidations == 0, "plain text should not invalidate parent, got " + parent.invalidations);
        RectF ink = new RectF();
        node.inkBounds(ink);
        check(ink.left == 0 && ink.top == 0 && ink.right == 80 && ink.bottom == 20,
                "plain inkBounds = " + ink.left + "," + ink.top + "," + ink.right + "," + ink.bottom);

        // 2. Offset solid underline past the 20px box draws unclipped and invalidates parent.
        SimpleSpanned underText = new SimpleSpanned("hello");
        underText.add(new Text.Decoration(true, false, Protocol.DECORATION_SOLID, 0xffff0000, 18f, 0f, 4f), 0, 5);
        StaticLayout underLayout = new StaticLayout(underText, 80, 20, 1, 15);
        node.setLayout(underLayout, 0, 0, 0, null);
        check(parent.invalidations == 1, "overflowing underline should invalidate parent");
        node.inkBounds(ink);
        // baseline(15) + under(18) + thickness/2(2) + AA(1) = 36
        check(ink.bottom >= 35, "underline inkBounds bottom = " + ink.bottom + ", want >= 35");

        Canvas c = new Canvas();
        node.onDraw(c);
        check(c.clips.isEmpty(), "unclipped TextNode should not push clipRect");
        check(c.drawnRects.size() == 1, "expected 1 underline rect, got " + c.drawnRects.size());
        RectF uRect = c.drawnRects.get(0);
        check(uRect.top == 31f && uRect.bottom == 35f, "underline rect y = " + uRect.top + ".." + uRect.bottom);
        check(!c.drawnShadows.get(0), "unshadowed decoration should have no shadow layer");

        // 3. Wavy underline + text shadow reaching past the box: decoration receives shadow layer,
        // and tinted saveLayer uses inkBounds covering both decoration and shadow.
        SimpleSpanned wavyText = new SimpleSpanned("hello");
        wavyText.add(new Text.Decoration(true, false, Protocol.DECORATION_WAVY, 0xff0000ff, 14f, 0f, 3f), 0, 5);
        StaticLayout wavyLayout = new StaticLayout(wavyText, 80, 20, 1, 15);
        Text.Shadow shadow = new Text.Shadow(0xff00aa00, 12f, 16f, 4f);
        node.setLayout(wavyLayout, 0, 0, 0, shadow);
        node.inkBounds(ink);
        // Shadow extends right past 80 + 12 + 8 = 100 and bottom past (15+14+5.5) + 16 + 9 = 59.5.
        check(ink.right >= 100 && ink.bottom >= 55,
                "shadow+wavy inkBounds = " + ink.left + "," + ink.top + "," + ink.right + "," + ink.bottom);

        int prevParentInv = parent.invalidations;
        node.setTint(0xff112233);
        check(parent.invalidations == prevParentInv + 1, "tinting overflowing text should invalidate parent");

        Canvas ct = new Canvas();
        node.onDraw(ct);
        check(ct.clips.isEmpty(), "tinted unclipped TextNode should not push clipRect");
        check(ct.saveLayers.size() == 1 && ct.saveLayers.get(0) != null,
                "tinted TextNode must pass explicit inkBounds to saveLayer");
        RectF layer = ct.saveLayers.get(0);
        check(layer.right >= 100 && layer.bottom >= 55,
                "tinted saveLayer bounds = " + layer.left + "," + layer.top + "," + layer.right + "," + layer.bottom);
        check(ct.drawnPaths.size() == 1, "expected 1 wavy path");
        check(ct.drawnPaths.get(0).bottom > 20f, "wavy path should draw past box height");
        check(ct.drawnShadows.get(0), "shadowed decoration should apply shadow layer");

        // 4. Clearing overflow back to plain text invalidates parent once to clear stale ink,
        // and subsequent plain updates do not invalidate parent.
        node.setTint(0);
        int beforeClear = parent.invalidations;
        node.setLayout(plain, 0, 0, 0, null);
        check(parent.invalidations == beforeClear + 1, "clearing overflow must invalidate parent");
        node.setLayout(plain, 0, 0, 0, null);
        check(parent.invalidations == beforeClear + 1, "subsequent plain layout should not invalidate parent");

        // 5. EllipsizeClip (clipLines > 0 with more lines) clips drawn lines.
        StaticLayout multi = new StaticLayout(underText, 80, 36, 2, 15);
        node.setLayout(multi, 0, 0, 1, null);
        Canvas cc = new Canvas();
        node.onDraw(cc);
        check(cc.clips.size() == 1 && cc.clips.get(0).bottom == 18f,
                "clipLines should clip to lineTop(clipLines)");
    }
}
