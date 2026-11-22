package dev.oneo.nsgo;

import android.app.Activity;
import android.os.Handler;
import android.view.MotionEvent;
import android.view.View;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.nio.charset.StandardCharsets;

public final class TestAgentCheck {
    private static void send(TestAgent agent, int id, String json) throws Exception {
        ByteArrayOutputStream buf = new ByteArrayOutputStream();
        DataOutputStream out = new DataOutputStream(buf);
        out.writeInt(id);
        byte[] bytes = json.getBytes(StandardCharsets.UTF_8);
        out.writeInt(bytes.length);
        out.write(bytes);
        agent.handle(new DataInputStream(new ByteArrayInputStream(buf.toByteArray())));
        Handler.drain();
    }

    public static void main(String[] args) throws Exception {
        Activity activity = new Activity();
        NsgoView view = new NsgoView(activity);
        TestAgent agent = new TestAgent(view);

        // 1. A press must let runnables posted during ACTION_UP (such as View.PerformClick)
        //    execute before testReply is sent back to Go.
        activity.onTouch = e -> {
            if (e.action == MotionEvent.ACTION_DOWN) {
                view.log.add("down");
            } else if (e.action == MotionEvent.ACTION_UP) {
                view.log.add("up");
                view.post(() -> view.log.add("click"));
            }
        };
        send(agent, 1, "{\"op\":\"press\",\"x\":10,\"y\":20,\"hold\":0}");
        String expectedOrder = "[down, up, click, reply:1::]";
        if (!view.log.toString().equals(expectedOrder)) {
            throw new AssertionError("press event order = " + view.log + ", want " + expectedOrder);
        }

        // 2. signature() must reflect native focus changes even when no view geometry changed.
        View fieldA = new View(activity);
        View fieldB = new View(activity);
        activity.window.decor.addChild(fieldA);
        activity.window.decor.addChild(fieldB);

        send(agent, 2, "{\"op\":\"signature\"}");
        String sigNone1 = view.lastReplyData;
        send(agent, 3, "{\"op\":\"signature\"}");
        String sigNone2 = view.lastReplyData;
        if (!sigNone1.equals(sigNone2)) {
            throw new AssertionError("unchanged state produced different signatures: " + sigNone1 + " vs " + sigNone2);
        }

        activity.currentFocus = fieldA;
        send(agent, 4, "{\"op\":\"signature\"}");
        String sigA = view.lastReplyData;
        if (sigA.equals(sigNone1)) {
            throw new AssertionError("focusing fieldA did not change signature");
        }

        activity.currentFocus = fieldB;
        send(agent, 5, "{\"op\":\"signature\"}");
        String sigB = view.lastReplyData;
        if (sigB.equals(sigA) || sigB.equals(sigNone1)) {
            throw new AssertionError("moving focus to fieldB did not change signature");
        }

        activity.currentFocus = null;
        send(agent, 6, "{\"op\":\"signature\"}");
        String sigBlur = view.lastReplyData;
        if (!sigBlur.equals(sigNone1)) {
            throw new AssertionError("blurring did not return to unfocused signature: " + sigBlur + " vs " + sigNone1);
        }
    }
}
