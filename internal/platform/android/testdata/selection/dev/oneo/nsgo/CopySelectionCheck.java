package dev.oneo.nsgo;

import android.content.Context;
import android.view.ViewGroup;
import java.io.DataInputStream;
import java.io.DataOutputStream;

/** Decodes Go's copy metadata and exercises the actual selection model. */
public final class CopySelectionCheck {
    public static void main(String[] args) throws Exception {
        DataInputStream in = new DataInputStream(System.in);
        DataOutputStream out = new DataOutputStream(System.out);
        for (int n = in.readInt(); n > 0; n--) {
            String text = Protocol.readString(in);
            Context context = new Context();
            ViewGroup host = new ViewGroup(context);
            TextNode node = new TextNode(context, text, 0, 0, text.length() * 10, 20);
            node.copy = TextCopy.read(in);
            host.addView(node);
            String[] report = {""};
            Selection selection = new Selection(host, host, (s, x, y, width, height) -> report[0] = s);
            for (int cases = in.readInt(); cases > 0; cases--) {
                int from = in.readInt(), to = in.readInt();
                selection.selectAll();
                selection.moveHandle(0, from * 10, 5);
                selection.moveHandle(1, to * 10, 5);
                Protocol.writeString(out, selection.text());
                Protocol.writeString(out, report[0]);
            }
        }
        out.flush();
    }
}
