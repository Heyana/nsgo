package dev.oneo.nsgo;

import android.content.Context;
import android.text.StaticLayout;
import android.view.MotionEvent;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;

/**
 * Runs TextNode.onTouchEvent and TextNode.pressedIndex on the JVM against
 * multi-line LTR, RTL, BiDi, surrogate-pair and clipped layouts so Go tests
 * verify the exact UTF-8 byte offsets reported for span presses and long presses.
 */
public final class TextIndexCheck {
    private static final int OP_PRESS = 0;
    private static final int OP_TOUCHLESS = 1;
    private static final int OP_CANCEL = 2;
    private static final int OP_NULL_LAYOUT = 3;

    public static void main(String[] args) throws IOException {
        DataInputStream in = new DataInputStream(System.in);
        DataOutputStream out = new DataOutputStream(System.out);
        int cases = in.readInt();
        for (int c = 0; c < cases; c++) {
            String text = Protocol.readString(in);
            int layoutX = in.readInt();
            int layoutY = in.readInt();
            int clipLines = in.readInt();
            int width = in.readInt();
            int height = in.readInt();
            int lineCount = in.readInt();
            StaticLayout.Line[] lines = new StaticLayout.Line[lineCount];
            for (int i = 0; i < lineCount; i++) {
                int start = in.readInt();
                int end = in.readInt();
                float left = in.readFloat();
                int top = in.readInt();
                int bottom = in.readInt();
                boolean rtl = in.readByte() != 0;
                int runCount = in.readInt();
                StaticLayout.Run[] runs = new StaticLayout.Run[runCount];
                for (int j = 0; j < runCount; j++) {
                    runs[j] = new StaticLayout.Run(in.readInt(), in.readInt(), in.readFloat(), in.readByte() != 0);
                }
                lines[i] = new StaticLayout.Line(start, end, left, top, bottom, rtl, runs);
            }
            StaticLayout layout = new StaticLayout(text, width, height, lines);
            TextNode node = new TextNode(new Context());
            node.setLayout(layout, layoutX, layoutY, clipLines);
            int queries = in.readInt();
            for (int q = 0; q < queries; q++) {
                int op = in.readByte() & 0xff;
                float x = in.readFloat();
                float y = in.readFloat();
                switch (op) {
                    case OP_PRESS:
                        node.onTouchEvent(new MotionEvent(MotionEvent.ACTION_DOWN, x, y));
                        out.writeInt(node.pressedIndex());
                        break;
                    case OP_TOUCHLESS:
                        out.writeInt(node.pressedIndex());
                        break;
                    case OP_CANCEL:
                        node.onTouchEvent(new MotionEvent(MotionEvent.ACTION_DOWN, x, y));
                        node.onTouchEvent(new MotionEvent(MotionEvent.ACTION_CANCEL, x, y));
                        out.writeInt(node.pressedIndex());
                        break;
                    case OP_NULL_LAYOUT:
                        node.setLayout(null, 0, 0, 0);
                        node.onTouchEvent(new MotionEvent(MotionEvent.ACTION_DOWN, x, y));
                        out.writeInt(node.pressedIndex());
                        node.setLayout(layout, layoutX, layoutY, clipLines);
                        break;
                    default:
                        throw new IllegalArgumentException("unknown op " + op);
                }
            }
        }
        out.flush();
    }
}
