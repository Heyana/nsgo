package dev.oneo.nsgo;

import android.content.Context;
import android.view.ViewGroup;
import java.io.BufferedInputStream;
import java.io.BufferedOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.util.Locale;

/** Feeds the Android runtime's Selection the same boxes and gestures as the Go driver. */
public final class SelectionCheck {
    public static void main(String[] args) throws IOException {
        Locale.setDefault(Locale.ROOT);
        DataInputStream in = new DataInputStream(new BufferedInputStream(System.in));
        Context context = new Context();
        ViewGroup host = new ViewGroup(context), area = new ViewGroup(context);
        TextNode[] texts = new TextNode[in.readInt()];
        for (int i = 0; i < texts.length; i++) {
            String text = Protocol.readString(in);
            int x = (int) in.readFloat(), y = (int) in.readFloat();
            int width = (int) in.readFloat(), height = (int) in.readFloat();
            texts[i] = new TextNode(context, text, x, y, width, height);
            area.addView(texts[i]);
        }
        Selection selection = new Selection(host, area);
        try (DataOutputStream out = new DataOutputStream(new BufferedOutputStream(System.out))) {
            int steps = in.readInt();
            for (int i = 0; i < steps; i++) {
                int op = in.readUnsignedByte();
                float x = in.readFloat(), y = in.readFloat();
                switch (op) {
                    case 0: selection.selectWordAt(x, y); break;
                    case 1: selection.moveHandle(0, x, y); break;
                    case 2: selection.moveHandle(1, x, y); break;
                    case 3: selection.selectAll(); break;
                    case 4: selection.clear(); break;
                    default: throw new IOException("unknown selection step " + op);
                }
                out.writeBoolean(selection.isEmpty());
                Protocol.writeString(out, selection.text());
                for (TextNode text : texts) {
                    out.writeInt(text.from);
                    out.writeInt(text.to);
                }
            }
            if (in.read() != -1) throw new IOException("unread selection fixture data");
        }
    }
}
