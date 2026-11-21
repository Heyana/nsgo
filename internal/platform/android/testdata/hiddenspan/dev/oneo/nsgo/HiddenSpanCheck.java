package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.Canvas;
import android.text.StaticLayout;
import java.io.BufferedInputStream;
import java.io.BufferedOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.util.ArrayList;
import java.util.List;

/** Exercises Text.Prepared and TextNode.onDraw with hidden spans on the JVM. */
public final class HiddenSpanCheck {
    public static void main(String[] args) throws IOException {
        DataInputStream in = new DataInputStream(new BufferedInputStream(System.in));
        Context context = new Context();
        int cases = in.readInt();
        try (DataOutputStream out = new DataOutputStream(new BufferedOutputStream(System.out))) {
            for (int c = 0; c < cases; c++) {
                float density = 1f;
                float layoutWidth = in.readFloat();
                // Read the exact wire format NsgoView.textOperation(TEXT_PREPARE) reads (minus id).
                String text = Protocol.readString(in);
                Text.Style style = new Text.Style(in, density);
                int maxLines = in.readInt();
                List<Text.Span> spans = new ArrayList<>();
                for (int i = 0, n = in.readInt(); i < n; i++) spans.add(new Text.Span(in, density));
                List<Text.Attachment> attachments = new ArrayList<>();
                for (int i = 0, n = in.readInt(); i < n; i++) attachments.add(new Text.Attachment(in, density));
                int ellipsize = in.readUnsignedByte();
                Text.Shadow shadow = new Text.Shadow(in, density);
                style.readDecoration(in, density);
                for (Text.Span span : spans) span.style.readDecoration(in, density);
                for (Text.Span span : spans) span.hidden = Protocol.flag(in.readUnsignedByte(), 0);

                Text.Prepared prepared = new Text.Prepared(text, style, maxLines, ellipsize, shadow, spans, attachments);
                float[] size = prepared.measure(Float.NaN);
                StaticLayout layout = prepared.layout(layoutWidth);
                TextNode node = new TextNode(context);
                node.setLayout(layout, 0, 0, prepared.clipLines(), shadow);
                Canvas canvas = new Canvas();
                node.onDraw(canvas);

                out.writeFloat(size[0]);
                out.writeFloat(size[1]);
                out.writeBoolean(Text.hasHidden(layout));
                out.writeInt(canvas.ops.size());
                for (String op : canvas.ops) Protocol.writeString(out, op);
            }
            if (in.read() != -1) throw new IOException("unread fixture data");
        }
    }
}
