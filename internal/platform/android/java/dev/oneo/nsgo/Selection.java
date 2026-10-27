package dev.oneo.nsgo;

import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.content.res.TypedArray;
import android.graphics.Rect;
import android.graphics.drawable.Drawable;
import android.text.Layout;
import android.view.ActionMode;
import android.view.Menu;
import android.view.MenuItem;
import android.view.MotionEvent;
import android.view.View;
import android.view.ViewGroup;
import android.widget.FrameLayout;
import android.widget.ImageView;
import java.text.BreakIterator;
import java.util.ArrayList;
import java.util.List;

/**
 * A selection across the texts of a selection area (a view with
 * SelectionArea, or a selectable text outside one), as Android's text
 * views select: a long press selects a word and shows the system's
 * handles and its floating toolbar with Copy and Select all; the handles
 * move the ends, and a tap elsewhere clears it. It follows the model of
 * internal/driver/selection.go, on the UI thread, with Android's word
 * boundaries (BreakIterator). Points are in the host view's pixels.
 */
final class Selection {
    /** A place in the area's texts: a text by its index, and a UTF-16 offset. */
    static final class Position {
        final int text, offset;

        Position(int text, int offset) {
            this.text = text;
            this.offset = offset;
        }

        boolean before(Position o) {
            return text < o.text || text == o.text && offset < o.offset;
        }
    }

    private final ViewGroup host;
    final View area;
    /** The area's texts in document order, and their content boxes in the host. */
    private final List<TextNode> texts = new ArrayList<>();
    private final List<Rect> boxes = new ArrayList<>();
    private Position start, end;
    /** The texts the ends are in, to find them again after a batch. */
    private TextNode startText, endText;
    private final ImageView[] handles = new ImageView[2];
    private ActionMode toolbar;
    private final int[] at = new int[2], hostAt = new int[2];

    Selection(ViewGroup host, View area) {
        this.host = host;
        this.area = area;
    }

    /** Lists the area's texts again, with their boxes in the host. */
    private void list() {
        texts.clear();
        boxes.clear();
        host.getLocationInWindow(hostAt);
        walk(area);
    }

    private void walk(View v) {
        if (v.getVisibility() != View.VISIBLE) return;
        if (v instanceof TextNode) {
            TextNode t = (TextNode) v;
            if (t.selectable != Protocol.SELECTABLE_OFF && t.layout() != null) {
                t.getLocationInWindow(at);
                Rect b = new Rect(at[0] - hostAt[0] + t.textX(), at[1] - hostAt[1] + t.textY(), 0, 0);
                b.right = b.left + t.layout().getWidth();
                b.bottom = b.top + t.layout().getHeight();
                texts.add(t);
                boxes.add(b);
            }
            return;
        }
        if (v instanceof ViewGroup) {
            ViewGroup g = (ViewGroup) v;
            for (int i = 0; i < g.getChildCount(); i++) walk(g.getChildAt(i));
        }
    }

    private CharSequence content(int i) {
        return texts.get(i).layout().getText();
    }

    /** The offset nearest the point x, y in text i's box, clamped into it. */
    private int offsetAt(int i, float x, float y) {
        Rect b = boxes.get(i);
        Layout l = texts.get(i).layout();
        float px = Math.min(Math.max(x, b.left), b.right) - b.left;
        float py = Math.min(Math.max(y, b.top), b.bottom - 1) - b.top;
        int offset = l.getOffsetForHorizontal(l.getLineForVertical((int) py), px);
        CharSequence c = l.getText();
        if (offset > 0 && offset < c.length() && Character.isLowSurrogate(c.charAt(offset))) offset--;
        return offset;
    }

    /** The position under x, y, by the hit-testing rule of selection.go; null without texts. */
    private Position hit(float x, float y) {
        int count = texts.size();
        if (count == 0) return null;
        for (int i = 0; i < count; i++) {
            if (boxes.get(i).contains((int) x, (int) y)) return new Position(i, offsetAt(i, x, y));
        }
        int nearest = -1;
        float best = 0;
        for (int i = 0; i < count; i++) {
            Rect f = boxes.get(i);
            if (y < f.top || y >= f.bottom) continue;
            float d = x < f.left ? f.left - x : Math.max(x - f.right, 0);
            if (nearest < 0 || d < best) {
                nearest = i;
                best = d;
            }
        }
        if (nearest >= 0) return new Position(nearest, offsetAt(nearest, x, y));
        int above = -1, below = -1;
        for (int i = count - 1; i >= 0; i--) {
            if (boxes.get(i).bottom <= y) {
                above = i;
                break;
            }
        }
        for (int i = 0; i < count; i++) {
            if (boxes.get(i).top > y) {
                below = i;
                break;
            }
        }
        if (above >= 0 && (below < 0 || y - boxes.get(above).bottom <= boxes.get(below).top - y)) {
            return new Position(above, content(above).length());
        }
        if (below >= 0) return new Position(below, 0);
        return new Position(count - 1, content(count - 1).length());
    }

    /** Selects the word at x, y, as a long press does; false when nothing is there. */
    boolean selectWordAt(float x, float y) {
        list();
        Position p = hit(x, y);
        if (p == null) return false;
        CharSequence c = content(p.text);
        BreakIterator words = BreakIterator.getWordInstance();
        words.setText(c.toString());
        int s = p.offset, e = p.offset;
        if (c.length() > 0) {
            int at = Math.min(p.offset, c.length() - 1);
            e = words.following(at);
            s = words.preceding(e);
            if (s == BreakIterator.DONE) s = 0;
            if (e == BreakIterator.DONE) e = c.length();
        }
        set(new Position(p.text, s), new Position(p.text, e));
        return true;
    }

    /** Selects every text of the area. */
    void selectAll() {
        list();
        if (texts.isEmpty()) return;
        int last = texts.size() - 1;
        set(new Position(0, 0), new Position(last, content(last).length()));
    }

    /** Moves end h (0 the start, 1 the end) to x, y; the ends swap when one passes the other. */
    void moveHandle(int h, float x, float y) {
        if (start == null) return;
        list();
        Position p = hit(x, y);
        if (p == null) return;
        Position other = h == 0 ? end : start;
        if (p.before(other)) {
            set(p, other);
        } else {
            set(other, p);
        }
    }

    private void set(Position s, Position e) {
        start = s;
        end = e;
        startText = texts.get(s.text);
        endText = texts.get(e.text);
        show();
    }

    /** Has each text draw its part, and places the handles and the toolbar. */
    private void show() {
        for (int i = 0; i < texts.size(); i++) {
            int from = 0, to = content(i).length();
            if (start == null || i < start.text || i > end.text) {
                from = to = 0;
            } else {
                if (i == start.text) from = start.offset;
                if (i == end.text) to = end.offset;
            }
            texts.get(i).select(from, to);
        }
        placeHandles();
        if (toolbar != null) toolbar.invalidateContentRect();
    }

    /** Follows the texts after a batch, or as views scroll; false once the area is gone. */
    boolean refresh() {
        if (!area.isAttachedToWindow() || start == null) return false;
        List<TextNode> before = new ArrayList<>(texts);
        list();
        int s = texts.indexOf(startText), e = texts.indexOf(endText);
        for (TextNode t : before) {
            if (!texts.contains(t)) t.select(0, 0);
        }
        if (s < 0 || e < 0) return false;
        start = new Position(s, Math.min(start.offset, content(s).length()));
        end = new Position(e, Math.min(end.offset, content(e).length()));
        show();
        return true;
    }

    /** Clears the highlight, the handles and the toolbar. */
    void clear() {
        for (TextNode t : texts) t.select(0, 0);
        for (ImageView h : handles) {
            if (h != null) host.removeView(h);
        }
        if (toolbar != null) {
            ActionMode t = toolbar;
            toolbar = null;
            t.finish();
        }
        start = end = null;
    }

    /** The selected text: texts on one row joined by a tab, others by a line break; attachments left out. */
    String text() {
        if (start == null) return "";
        StringBuilder out = new StringBuilder();
        Rect previous = null;
        for (int i = start.text; i <= end.text; i++) {
            CharSequence c = content(i);
            int from = i == start.text ? start.offset : 0, to = i == end.text ? end.offset : c.length();
            if (from >= to) continue;
            Rect f = boxes.get(i);
            if (previous != null) out.append(previous.top < f.bottom && f.top < previous.bottom ? '\t' : '\n');
            previous = f;
            out.append(c.subSequence(from, to).toString().replace("￼", ""));
        }
        return out.toString();
    }

    /** The caret of position p: x at it, and its line's top and bottom, in the host. */
    private float[] caret(Position p) {
        Rect b = boxes.get(p.text);
        Layout l = texts.get(p.text).layout();
        int line = l.getLineForOffset(p.offset);
        return new float[] {b.left + l.getPrimaryHorizontal(p.offset), b.top + l.getLineTop(line), b.top + l.getLineBottom(line)};
    }

    /** Puts the system's handles under the ends, their points at the carets. */
    private void placeHandles() {
        if (start == null) return;
        for (int h = 0; h < 2; h++) {
            if (handles[h] == null) {
                handles[h] = handle(h);
                host.addView(handles[h]);
            }
            Drawable d = handles[h].getDrawable();
            float[] c = caret(h == 0 ? start : end);
            // As TextView places them: the start's point three quarters
            // across, the end's a quarter.
            int w = d.getIntrinsicWidth(), x = Math.round(c[0] - (h == 0 ? w * 3 / 4f : w / 4f));
            FrameLayout.LayoutParams lp = new FrameLayout.LayoutParams(w, d.getIntrinsicHeight());
            lp.leftMargin = x;
            lp.topMargin = Math.round(c[2]);
            handles[h].setLayoutParams(lp);
            handles[h].bringToFront();
        }
    }

    private ImageView handle(int h) {
        Context context = host.getContext();
        TypedArray a = context.obtainStyledAttributes(new int[] {
            h == 0 ? android.R.attr.textSelectHandleLeft : android.R.attr.textSelectHandleRight});
        Drawable d = a.getDrawable(0);
        a.recycle();
        ImageView v = new ImageView(context);
        v.setImageDrawable(d);
        float[] grab = new float[2];
        v.setOnTouchListener((view, e) -> {
            float x = e.getRawX(), y = e.getRawY();
            host.getLocationOnScreen(hostAt);
            x -= hostAt[0];
            y -= hostAt[1];
            switch (e.getActionMasked()) {
                case MotionEvent.ACTION_DOWN: {
                    // Where the finger is from the caret's middle, kept as it moves.
                    float[] c = caret(h == 0 ? start : end);
                    grab[0] = x - c[0];
                    grab[1] = y - (c[1] + c[2]) / 2;
                    hideToolbar();
                    break;
                }
                case MotionEvent.ACTION_MOVE:
                    moveHandle(h, x - grab[0], y - grab[1]);
                    break;
                case MotionEvent.ACTION_UP:
                case MotionEvent.ACTION_CANCEL:
                    showToolbar();
                    break;
            }
            return true;
        });
        return v;
    }

    /** Whether x, y is on a handle. */
    boolean onHandle(float x, float y) {
        for (ImageView h : handles) {
            if (h != null && h.getParent() != null && x >= h.getLeft() && x < h.getRight() && y >= h.getTop() && y < h.getBottom()) {
                return true;
            }
        }
        return false;
    }

    /** Shows the floating toolbar over the selection: Copy and Select all, in the system's words. */
    void showToolbar() {
        if (start == null || toolbar != null) return;
        toolbar = host.startActionMode(new ActionMode.Callback2() {
            @Override
            public boolean onCreateActionMode(ActionMode mode, Menu menu) {
                menu.add(Menu.NONE, android.R.id.copy, 0, android.R.string.copy);
                menu.add(Menu.NONE, android.R.id.selectAll, 1, android.R.string.selectAll);
                return true;
            }

            @Override
            public boolean onPrepareActionMode(ActionMode mode, Menu menu) {
                return false;
            }

            @Override
            public boolean onActionItemClicked(ActionMode mode, MenuItem item) {
                if (item.getItemId() == android.R.id.copy) {
                    ClipboardManager clipboard = host.getContext().getSystemService(ClipboardManager.class);
                    clipboard.setPrimaryClip(ClipData.newPlainText(null, text()));
                    mode.finish();
                    return true;
                }
                if (item.getItemId() == android.R.id.selectAll) {
                    selectAll();
                    return true;
                }
                return false;
            }

            @Override
            public void onDestroyActionMode(ActionMode mode) {
                if (toolbar == mode) toolbar = null;
            }

            @Override
            public void onGetContentRect(ActionMode mode, View view, Rect out) {
                if (start == null) return;
                float[] a = caret(start), b = caret(end);
                out.set((int) Math.min(a[0], b[0]), (int) a[1], (int) Math.max(a[0], b[0]) + 1, (int) b[2]);
                if (start.text != end.text || a[1] != b[1]) {
                    Rect area = boxes.get(start.text);
                    out.left = Math.min(out.left, area.left);
                    out.right = Math.max(out.right, boxes.get(end.text).right);
                }
            }
        }, ActionMode.TYPE_FLOATING);
    }

    private void hideToolbar() {
        if (toolbar != null) {
            ActionMode t = toolbar;
            toolbar = null;
            t.finish();
        }
    }

    boolean isEmpty() {
        return start == null || start.text == end.text && start.offset == end.offset;
    }
}
