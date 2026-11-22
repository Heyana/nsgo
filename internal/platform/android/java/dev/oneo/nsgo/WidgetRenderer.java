package dev.oneo.nsgo;

import android.content.Context;
import android.content.res.Configuration;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.Canvas;
import android.graphics.Color;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.Rect;
import android.graphics.RectF;
import android.graphics.Typeface;
import android.icu.text.MeasureFormat;
import android.icu.util.Measure;
import android.icu.util.MeasureUnit;
import android.text.Layout;
import android.text.StaticLayout;
import android.text.TextPaint;
import android.text.TextUtils;
import android.text.format.DateUtils;
import android.util.TypedValue;
import android.view.Gravity;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.File;
import java.text.DateFormat;
import java.util.ArrayList;
import java.util.Date;
import java.util.HashMap;
import java.util.Locale;

/** Lays out the same JSON nodes WidgetKit reads, then draws a bounded bitmap for RemoteViews. */
final class WidgetRenderer {
    static final class Link {
        final RectF bounds; final String url, label;
        Link(RectF bounds, String url, String label) { this.bounds = bounds; this.url = url; this.label = label; }
    }
    static final class Timer {
        RectF bounds; long date; float size; int color, gravity;
    }
    static final class Result {
        Bitmap bitmap;
        String description;
        final ArrayList<Link> links = new ArrayList<>();
        final ArrayList<Timer> timers = new ArrayList<>();
        long nextRefresh;
    }
    private static final class Box {
        final JSONObject node;
        final ArrayList<Box> children = new ArrayList<>();
        float width, height;
        Box(JSONObject node) { this.node = node; }
        String type() { return node.optString("t", ""); }
        boolean flexible(boolean horizontal) {
            return "spacer".equals(type()) || node.optDouble(horizontal ? "width" : "height", 0) <= 0 && node.optBoolean(horizontal ? "fillW" : "fillH");
        }
    }

    private final Context context;
    private final File directory;
    private final boolean dark;
    private final long now;
    private final float fontScale, maximumWidth, maximumHeight;
    private final HashMap<String, Bitmap> images = new HashMap<>();
    private final StringBuilder description = new StringBuilder();
    private final Result result = new Result();
    private Canvas canvas;
    private int nodes;

    private WidgetRenderer(Context context, File directory, int width, int height, long now) {
        this.context = context; this.directory = directory; this.now = now;
        dark = (context.getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK) == Configuration.UI_MODE_NIGHT_YES;
        fontScale = context.getResources().getConfiguration().fontScale;
        maximumWidth = width; maximumHeight = height;
    }

    static Result render(Context context, File directory, JSONObject view, JSONObject background, int width, int height, long now) {
        WidgetRenderer renderer = new WidgetRenderer(context, directory, width, height, now);
        float scale = Math.min(context.getResources().getDisplayMetrics().density, 1024f / Math.max(width, height));
        renderer.result.bitmap = Bitmap.createBitmap(Math.max(1, Math.round(width * scale)), Math.max(1, Math.round(height * scale)), Bitmap.Config.ARGB_8888);
        renderer.canvas = new Canvas(renderer.result.bitmap);
        renderer.canvas.scale(scale, scale);
        renderer.canvas.clipRect(0, 0, width, height);
        renderer.canvas.drawColor(renderer.dark ? 0xff1e1e1e : Color.WHITE);
        int foreground = renderer.dark ? Color.WHITE : Color.BLACK;
        if (background != null) renderer.draw(renderer.measure(background, width, height, 0), 0, 0, width, height, foreground);
        float margin = Math.min(16, Math.min(width, height) / 4f);
        Box box = renderer.measure(view, width - 2 * margin, height - 2 * margin, 0);
        float w = box.node.optBoolean("fillW") || expands(box, true) ? width - 2 * margin : Math.min(box.width, width - 2 * margin);
        float h = box.node.optBoolean("fillH") || expands(box, false) ? height - 2 * margin : Math.min(box.height, height - 2 * margin);
        renderer.draw(box, (width - w) / 2, (height - h) / 2, Math.max(0, w), Math.max(0, h), foreground);
        renderer.result.description = renderer.description.toString();
        return renderer.result;
    }

    private static boolean expands(Box box, boolean horizontal) {
        if (!(horizontal ? "row" : "column").equals(box.type())) return false;
        for (Box child : box.children) if (child.flexible(horizontal)) return true;
        return false;
    }

    private float[] padding(JSONObject n) {
        float[] out = new float[4]; JSONArray values = n.optJSONArray("padding");
        if (values != null) for (int i = 0; i < 4; i++) out[i] = Math.max(0, Math.min(2048, (float) values.optDouble(i, 0)));
        return out;
    }

    private Box measure(JSONObject node, float width, float height, int depth) {
        if (node == null || depth > 64 || ++nodes > 4096) node = new JSONObject();
        Box box = new Box(node);
        float[] p = padding(node);
        float availableW = Math.max(1, (float) node.optDouble("width", width) - p[1] - p[3]);
        float availableH = Math.max(1, (float) node.optDouble("height", height) - p[0] - p[2]);
        JSONArray children = node.optJSONArray("children");
        if (children != null) for (int i = 0; i < Math.min(children.length(), 4096); i++) box.children.add(measure(children.optJSONObject(i), availableW, availableH, depth + 1));
        float w = 0, h = 0;
        switch (box.type()) {
            case "text": case "date": case "symbol": {
                TextPaint paint = textPaint(node, dark ? Color.WHITE : Color.BLACK);
                String text = text(node);
                w = Math.min(availableW, Math.max(1, (float) Math.ceil(Layout.getDesiredWidth(text, paint))));
                StaticLayout layout = textLayout(node, text, paint, Math.max(1, w));
                h = layout.getHeight();
                break;
            }
            case "image": {
                Bitmap image = image(node.optString("image"));
                if (image != null) {
                    float scale = Math.min(availableW / image.getWidth(), availableH / image.getHeight());
                    w = image.getWidth() * scale; h = image.getHeight() * scale;
                }
                break;
            }
            case "rectangle": w = availableW; h = availableH; break;
            case "progress": w = Math.min(120, availableW); h = 4; break;
            case "gauge": w = h = Math.min(44, Math.min(availableW, availableH)); break;
            case "divider": w = availableW; h = 1; break;
            case "row": case "column": case "stack": case "button": {
                boolean row = "row".equals(box.type()), column = "column".equals(box.type());
                float gap = (float) node.optDouble("gap", 0);
                for (Box child : box.children) {
                    if (row) { w += child.width; h = Math.max(h, child.height); }
                    else if (column) { w = Math.max(w, child.width); h += child.height; }
                    else { w = Math.max(w, child.width); h = Math.max(h, child.height); }
                }
                if (row) w += Math.max(0, box.children.size() - 1) * gap;
                if (column) h += Math.max(0, box.children.size() - 1) * gap;
                break;
            }
        }
        box.width = Math.max(0, Math.min(2048, (float) node.optDouble("width", w + p[1] + p[3])));
        box.height = Math.max(0, Math.min(2048, (float) node.optDouble("height", h + p[0] + p[2])));
        return box;
    }

    private void draw(Box box, float x, float y, float width, float height, int inherited) {
        if (width <= 0 || height <= 0) return;
        JSONObject n = box.node;
        int color = color(n.optJSONObject("color"), inherited);
        float radius = Math.max(0, (float) n.optDouble("radius", 0));
        canvas.save();
        RectF bounds = new RectF(x, y, x + width, y + height);
        String link = n.optString("link", "");
        if (!link.isEmpty() && result.links.size() < 64) {
            RectF visible = new RectF(bounds);
            if (visible.intersect(0, 0, maximumWidth, maximumHeight)) result.links.add(new Link(visible, link, collectText(n).isEmpty() ? link : collectText(n)));
        }
        if (radius > 0) { Path clip = new Path(); clip.addRoundRect(bounds, radius, radius, Path.Direction.CW); canvas.clipPath(clip); }
        if (n.optJSONObject("bg") != null) { Paint fill = paint(color(n.optJSONObject("bg"), Color.TRANSPARENT)); canvas.drawRoundRect(bounds, radius, radius, fill); }
        float[] p = padding(n);
        float left = x + p[3], top = y + p[0], w = Math.max(0, width - p[1] - p[3]), h = Math.max(0, height - p[0] - p[2]);
        switch (box.type()) {
            case "text": case "date": case "symbol": drawText(n, left, top, w, h, color); break;
            case "image": drawImage(n, left, top, w, h); break;
            case "rectangle": canvas.drawRoundRect(bounds, radius, radius, paint(color)); break;
            case "divider": canvas.drawRect(left, top + h / 2, left + w, top + h / 2 + 1, paint(withAlpha(color, 0.25f))); break;
            case "progress": {
                float bar = Math.min(4, h), cy = top + (h - bar) / 2;
                canvas.drawRoundRect(new RectF(left, cy, left + w, cy + bar), bar / 2, bar / 2, paint(withAlpha(color, 0.2f)));
                canvas.drawRoundRect(new RectF(left, cy, left + w * fraction(n), cy + bar), bar / 2, bar / 2, paint(color));
                break;
            }
            case "gauge": {
                float side = Math.min(w, h), stroke = Math.max(2, side / 10);
                RectF ring = new RectF(left + (w - side + stroke) / 2, top + (h - side + stroke) / 2, left + (w + side - stroke) / 2, top + (h + side - stroke) / 2);
                Paint line = paint(withAlpha(color, 0.2f)); line.setStyle(Paint.Style.STROKE); line.setStrokeWidth(stroke); line.setStrokeCap(Paint.Cap.ROUND);
                canvas.drawOval(ring, line); line.setColor(color); canvas.drawArc(ring, -90, fraction(n) * 360, false, line);
                break;
            }
            default: drawChildren(box, left, top, w, h, color);
        }
        canvas.restore();
    }

    private void drawChildren(Box parent, float x, float y, float width, float height, int color) {
        boolean row = "row".equals(parent.type()), column = "column".equals(parent.type());
        String items = parent.node.optString("items", ""), justify = parent.node.optString("justify", "");
        float gap = (float) parent.node.optDouble("gap", 0);
        if (!row && !column) {
            for (Box child : parent.children) {
                float cw = child.node.optBoolean("fillW") ? width : Math.min(width, child.width);
                float ch = child.node.optBoolean("fillH") ? height : Math.min(height, child.height);
                draw(child, x + align(width - cw, items), y + align(height - ch, items), cw, ch, color);
            }
            return;
        }
        float fixed = Math.max(0, parent.children.size() - 1) * gap;
        int flexible = 0;
        for (Box child : parent.children) { if (child.flexible(row)) flexible++; else fixed += row ? child.width : child.height; }
        float available = row ? width : height;
        float flex = flexible == 0 ? 0 : Math.max(0, available - fixed) / flexible;
        float cursor = flexible > 0 ? 0 : align(Math.max(0, available - fixed), justify);
        for (Box child : parent.children) {
            float main = child.flexible(row) ? flex : row ? child.width : child.height;
            float crossSpace = row ? height : width;
            float cross = child.node.optBoolean(row ? "fillH" : "fillW") ? crossSpace : Math.min(crossSpace, row ? child.height : child.width);
            float offset = align(crossSpace - cross, items);
            draw(child, x + (row ? cursor : offset), y + (row ? offset : cursor), row ? main : cross, row ? cross : main, color);
            cursor += main + gap;
        }
    }

    private static float align(float extra, String alignment) { return "start".equals(alignment) ? 0 : "end".equals(alignment) ? Math.max(0, extra) : Math.max(0, extra) / 2; }
    private static float fraction(JSONObject n) { return Math.max(0, Math.min(1, (float) n.optDouble("value", 0))); }
    private static Paint paint(int color) { Paint p = new Paint(Paint.ANTI_ALIAS_FLAG); p.setColor(color); return p; }
    private static int withAlpha(int color, float alpha) { return (color & 0xffffff) | (Math.round(Color.alpha(color) * alpha) << 24); }

    private int color(JSONObject value, int inherited) {
        if (value == null) return inherited;
        if (dark && value.optJSONObject("dark") != null) value = value.optJSONObject("dark");
        switch (value.optString("name", "")) {
            case "primary": return dark ? Color.WHITE : Color.BLACK;
            case "secondary": return dark ? 0xff999999 : 0xff737373;
            case "clear": return Color.TRANSPARENT;
            case "accent": {
                TypedValue tint = new TypedValue();
                return context.getTheme().resolveAttribute(android.R.attr.colorAccent, tint, true) ? tint.data : 0xff007aff;
            }
            default: return Color.argb(channel(value.optDouble("a", 1)), channel(value.optDouble("r", 0)), channel(value.optDouble("g", 0)), channel(value.optDouble("b", 0)));
        }
    }
    private static int channel(double value) { return (int) Math.round(Math.max(0, Math.min(1, value)) * 255); }

    private TextPaint textPaint(JSONObject n, int color) {
        JSONObject font = n.optJSONObject("font");
        TextPaint paint = new TextPaint(Paint.ANTI_ALIAS_FLAG | Paint.SUBPIXEL_TEXT_FLAG);
        float size = 17;
        String design = "", style = "";
        int weight = 400; boolean italic = false;
        if (font != null) {
            style = font.optString("style", ""); design = font.optString("design", ""); weight = font.optInt("weight", 400); italic = font.optBoolean("italic");
            switch (style) {
                case "largeTitle": size = 34; break; case "title": size = 28; break; case "title2": size = 22; break;
                case "title3": size = 20; break; case "headline": weight = font.optInt("weight", 600); break;
                case "subheadline": size = 15; break; case "callout": size = 16; break; case "footnote": size = 13; break;
                case "caption": size = 12; break; case "caption2": size = 11; break;
            }
            if (font.has("size")) size = (float) font.optDouble("size", size); else size *= fontScale;
            String features = font.optBoolean("digits") ? "tnum" : "";
            if (font.optBoolean("smallCaps")) features += features.isEmpty() ? "smcp" : ",smcp";
            if (!features.isEmpty()) paint.setFontFeatureSettings(features);
        } else size *= fontScale;
        String family = "serif".equals(design) ? "serif" : "monospaced".equals(design) ? "monospace" : "rounded".equals(design) ? "sans-serif-rounded" : "sans-serif";
        paint.setTypeface(Typeface.create(Typeface.create(family, Typeface.NORMAL), Math.max(100, Math.min(weight, 900)), italic));
        paint.setTextSize(Math.max(1, Math.min(size, 512))); paint.setColor(color);
        return paint;
    }

    private StaticLayout textLayout(JSONObject n, String text, TextPaint paint, float width) {
        Layout.Alignment alignment = "end".equals(n.optString("textAlign")) ? Layout.Alignment.ALIGN_OPPOSITE : "center".equals(n.optString("textAlign")) ? Layout.Alignment.ALIGN_CENTER : Layout.Alignment.ALIGN_NORMAL;
        int max = Math.max(1, n.optInt("maxLines", 128));
        float minimum = (float) n.optDouble("minScale", 1);
        if (max == 1 && minimum > 0 && minimum < 1) {
            float desired = Layout.getDesiredWidth(text, paint);
            if (desired > width) paint.setTextSize(paint.getTextSize() * Math.max(minimum, width / desired));
        }
        return StaticLayout.Builder.obtain(text, 0, text.length(), paint, Math.max(1, (int) Math.ceil(width)))
                .setAlignment(alignment).setIncludePad(false).setMaxLines(Math.min(128, max)).setEllipsize(TextUtils.TruncateAt.END).build();
    }

    private void drawText(JSONObject n, float x, float y, float width, float height, int color) {
        if (width <= 0 || height <= 0) return;
        TextPaint paint = textPaint(n, color);
        String text = text(n);
        if (description.length() > 0) description.append(", "); description.append(text);
        if ("date".equals(n.optString("t")) && "timer".equals(n.optString("dateStyle")) && result.timers.size() < 16) {
            Timer timer = new Timer(); timer.bounds = new RectF(x, y, x + width, y + height); timer.date = (long) (n.optDouble("date", 0) * 1000);
            timer.size = paint.getTextSize(); timer.color = color;
            timer.gravity = "end".equals(n.optString("textAlign")) ? Gravity.END : "center".equals(n.optString("textAlign")) ? Gravity.CENTER_HORIZONTAL : Gravity.START;
            result.timers.add(timer);
            return;
        }
        StaticLayout layout = textLayout(n, text, paint, width);
        canvas.save(); canvas.clipRect(x, y, x + width, y + height); canvas.translate(x, y + Math.max(0, (height - layout.getHeight()) / 2)); layout.draw(canvas); canvas.restore();
    }

    private String text(JSONObject n) {
        if ("symbol".equals(n.optString("t"))) return symbol(n.optString("symbol", ""));
        if (!"date".equals(n.optString("t"))) return n.optString("text", "");
        long date = (long) (n.optDouble("date", 0) * 1000);
        String style = n.optString("dateStyle", "time");
        switch (style) {
            case "date": return DateFormat.getDateInstance(DateFormat.LONG).format(new Date(date));
            case "relative":
                refreshAt((now / 60000 + 1) * 60000);
                return DateUtils.getRelativeTimeSpanString(date, now, DateUtils.MINUTE_IN_MILLIS, DateUtils.FORMAT_ABBREV_RELATIVE).toString();
            case "offset": {
                refreshAt((now / 60000 + 1) * 60000);
                long seconds = Math.abs(date - now) / 1000;
                long unit = seconds >= 86400 ? 86400 : seconds >= 3600 ? 3600 : seconds >= 60 ? 60 : 1;
                MeasureUnit measure = unit == 86400 ? MeasureUnit.DAY : unit == 3600 ? MeasureUnit.HOUR : unit == 60 ? MeasureUnit.MINUTE : MeasureUnit.SECOND;
                return (date >= now ? "+" : "−") + MeasureFormat.getInstance(Locale.getDefault(), MeasureFormat.FormatWidth.WIDE).format(new Measure(seconds / unit, measure));
            }
            case "timer": {
                if (date > now) refreshAt(date);
                long seconds = Math.abs(date - now) / 1000;
                return seconds >= 3600 ? String.format(Locale.getDefault(), "%d:%02d:%02d", seconds / 3600, seconds / 60 % 60, seconds % 60) : String.format(Locale.getDefault(), "%02d:%02d", seconds / 60, seconds % 60);
            }
            default: return android.text.format.DateFormat.getTimeFormat(context).format(new Date(date));
        }
    }

    private void refreshAt(long date) {
        if (result.nextRefresh == 0 || date < result.nextRefresh) result.nextRefresh = date;
    }

    private static String symbol(String name) {
        switch (name) {
            case "number": return "#"; case "plus": return "+"; case "minus": return "−";
            case "checkmark": return "✓"; case "xmark": return "×"; case "star": return "☆"; case "star.fill": return "★";
            case "heart": return "♡"; case "heart.fill": return "♥"; case "circle": return "○"; case "circle.fill": return "●";
            case "square": return "□"; case "square.fill": return "■"; case "chevron.left": return "‹"; case "chevron.right": return "›";
            case "arrow.left": return "←"; case "arrow.right": return "→"; case "arrow.up": return "↑"; case "arrow.down": return "↓";
            default: return "";
        }
    }

    private Bitmap image(String name) {
        if (!name.matches("img-[a-f0-9]{32}")) return null;
        if (images.containsKey(name)) return images.get(name);
        File file = new File(directory, name);
        BitmapFactory.Options options = new BitmapFactory.Options(); options.inJustDecodeBounds = true;
        BitmapFactory.decodeFile(file.getAbsolutePath(), options);
        if (options.outWidth <= 0 || options.outHeight <= 0) return null;
        options.inSampleSize = 1;
        while (Math.max(options.outWidth, options.outHeight) / options.inSampleSize > 1024) options.inSampleSize *= 2;
        options.inJustDecodeBounds = false;
        Bitmap image = BitmapFactory.decodeFile(file.getAbsolutePath(), options);
        images.put(name, image); return image;
    }

    private void drawImage(JSONObject n, float x, float y, float width, float height) {
        Bitmap image = image(n.optString("image", "")); if (image == null || width <= 0 || height <= 0) return;
        float scale = n.optBoolean("cover") ? Math.max(width / image.getWidth(), height / image.getHeight()) : Math.min(width / image.getWidth(), height / image.getHeight());
        float w = image.getWidth() * scale, h = image.getHeight() * scale;
        canvas.save(); canvas.clipRect(x, y, x + width, y + height);
        Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG | Paint.FILTER_BITMAP_FLAG);
        canvas.drawBitmap(image, null, new RectF(x + (width - w) / 2, y + (height - h) / 2, x + (width + w) / 2, y + (height + h) / 2), paint); canvas.restore();
    }

    private static String collectText(JSONObject node) {
        StringBuilder text = new StringBuilder(node.optString("text", ""));
        JSONArray children = node.optJSONArray("children");
        if (children != null) for (int i = 0; i < children.length(); i++) { JSONObject child = children.optJSONObject(i); if (child != null) { if (text.length() > 0) text.append(' '); text.append(collectText(child)); } }
        return text.toString();
    }
}
