package dev.oneo.nsgo;

import android.content.Context;
import android.view.ViewGroup;
import java.io.BufferedInputStream;
import java.io.BufferedOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;

/** Feeds the Android runtime's Selection the same boxes and gestures as the Go driver. */
public final class SelectionCheck {
    private static final class Report {
        final String text;
        final int x, y, width, height;

        Report(String text, int x, int y, int width, int height) {
            this.text = text;
            this.x = x;
            this.y = y;
            this.width = width;
            this.height = height;
        }
    }

    public static void main(String[] args) throws IOException {
        Locale.setDefault(Locale.ROOT);
        DataInputStream in = new DataInputStream(new BufferedInputStream(System.in));
        Context context = new Context();
        int hostX = (int) in.readFloat(), hostY = (int) in.readFloat();
        int areaX = (int) in.readFloat(), areaY = (int) in.readFloat();
        ViewGroup host = new ViewGroup(context, hostX, hostY);
        ViewGroup area = new ViewGroup(context, hostX + areaX, hostY + areaY);
        TextNode[] texts = new TextNode[in.readInt()];
        int[] textX = new int[texts.length], textY = new int[texts.length];
        for (int i = 0; i < texts.length; i++) {
            String text = Protocol.readString(in);
            int x = (int) in.readFloat(), y = (int) in.readFloat();
            int width = (int) in.readFloat(), height = (int) in.readFloat();
            textX[i] = x;
            textY[i] = y;
            texts[i] = new TextNode(context, text, hostX + areaX + x, hostY + areaY + y, width, height);
            area.addView(texts[i]);
        }
        List<Report> reports = new ArrayList<>();
        Selection selection = new Selection(host, area,
                (text, x, y, width, height) -> reports.add(new Report(text, x, y, width, height)));
        try (DataOutputStream out = new DataOutputStream(new BufferedOutputStream(System.out))) {
            int steps = in.readInt();
            for (int i = 0; i < steps; i++) {
                reports.clear();
                int op = in.readUnsignedByte();
                float x = in.readFloat(), y = in.readFloat();
                switch (op) {
                    case 0: selection.selectWordAt(areaX + x, areaY + y); break;
                    case 1: selection.moveHandle(0, areaX + x, areaY + y); break;
                    case 2: selection.moveHandle(1, areaX + x, areaY + y); break;
                    case 3: selection.selectAll(); break;
                    case 4: selection.clear(); break;
                    case 5: {
                        // Outer scroll: area and every text move together in the window.
                        areaX += (int) x;
                        areaY += (int) y;
                        area.setLocation(hostX + areaX, hostY + areaY);
                        for (int j = 0; j < texts.length; j++) {
                            texts[j].setLocation(hostX + areaX + textX[j], hostY + areaY + textY[j]);
                        }
                        if (!selection.refresh()) selection.clear();
                        break;
                    }
                    case 6: {
                        // Inner layout/scroll shift: texts move relative to the area.
                        for (int j = 0; j < texts.length; j++) {
                            textX[j] += (int) x;
                            textY[j] += (int) y;
                            texts[j].setLocation(hostX + areaX + textX[j], hostY + areaY + textY[j]);
                        }
                        if (!selection.refresh()) selection.clear();
                        break;
                    }
                    case 7: {
                        // Detach the area and refresh, as NsgoView.refreshSelection does.
                        area.setAttached(false);
                        if (!selection.refresh()) selection.clear();
                        break;
                    }
                    default: throw new IOException("unknown selection step " + op);
                }
                out.writeBoolean(selection.isEmpty());
                Protocol.writeString(out, selection.text());
                for (TextNode text : texts) {
                    out.writeInt(text.from);
                    out.writeInt(text.to);
                }
                out.writeInt(reports.size());
                for (Report r : reports) {
                    Protocol.writeString(out, r.text);
                    out.writeInt(r.x);
                    out.writeInt(r.y);
                    out.writeInt(r.width);
                    out.writeInt(r.height);
                }
            }
            if (in.read() != -1) throw new IOException("unread selection fixture data");
        }
    }
}
