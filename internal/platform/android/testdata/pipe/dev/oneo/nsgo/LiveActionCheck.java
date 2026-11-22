package dev.oneo.nsgo;

import android.content.Context;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.File;
import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.TimeUnit;

/** The shipped engine routes actions to their owner and rejects ended lifetimes. */
public final class LiveActionCheck {
    public static void main(String[] args) throws Exception {
        if (args.length != 0) {
            DataInputStream in = new DataInputStream(System.in);
            DataOutputStream out = new DataOutputStream(System.out);
            while (true) {
                int kind = in.readUnsignedByte();
                byte[] payload = new byte[in.readInt()];
                in.readFully(payload);
                if (kind != Protocol.LIVE_REPORT) continue;
                out.writeByte(Protocol.BATCH);
                out.writeInt(payload.length);
                out.write(payload);
                out.flush();
            }
        }
        NsgoEngine first = new NsgoEngine(new Context());
        NsgoEngine second = new NsgoEngine(new Context());
        LinkedBlockingQueue<byte[]> a = start(first), b = start(second);
        byte[] press = "press:delivery:pause".getBytes(StandardCharsets.UTF_8);
        try {
            check(!NsgoEngine.deliverLiveAction(null, press), "missing owner accepted");
            check(!NsgoEngine.deliverLiveAction("foreign", press), "foreign owner accepted");
            check(!NsgoEngine.deliverLiveAction(first.liveActionToken(), null), "missing press accepted");
            check(NsgoEngine.deliverLiveAction(first.liveActionToken(), press), "first owner not reached");
            check(Arrays.equals(press, a.poll(5, TimeUnit.SECONDS)), "first engine did not read action frame");
            check(b.poll(100, TimeUnit.MILLISECONDS) == null, "first engine's action crossed to second");
            first.destroy();
            check(!NsgoEngine.deliverLiveAction(first.liveActionToken(), press), "ended owner still receives presses");
            check(NsgoEngine.deliverLiveAction(second.liveActionToken(), press), "destroying first removed second");
            check(Arrays.equals(press, b.poll(5, TimeUnit.SECONDS)), "second engine did not read action frame");
        } finally {
            first.destroy();
            second.destroy();
        }
    }

    private static LinkedBlockingQueue<byte[]> start(NsgoEngine engine) throws Exception {
        String java = new File(System.getProperty("java.home"), "bin/java").getPath();
        engine.start(new ProcessBuilder(java, "-cp", System.getProperty("java.class.path"), LiveActionCheck.class.getName(), "child"));
        LinkedBlockingQueue<byte[]> messages = new LinkedBlockingQueue<>();
        engine.attach(new NsgoEngine.Listener() {
            public void onMessage(int kind, byte[] payload) { messages.add(payload); }
            public void onProcessEnded(java.io.IOException error) {}
            public void onLog(String line) {}
        });
        return messages;
    }

    private static void check(boolean ok, String why) {
        if (!ok) throw new AssertionError(why);
    }
}
