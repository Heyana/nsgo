package dev.oneo.nsgo;

import android.content.Context;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.io.PipedInputStream;
import java.io.PipedOutputStream;
import java.nio.charset.StandardCharsets;
import java.lang.reflect.Field;
import java.util.Arrays;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;

/** Exercises the real Protocol and NsgoEngine readers, without Android. */
public final class PipeCheck {
    public static void main(String[] args) throws Exception {
        if (args.length != 0) {
            child(args[0]);
            return;
        }
        messages();
        for (String scenario : new String[]{"stdout", "length", "negative", "header", "payload", "valid"}) {
            engine(scenario, true);
        }
        engine("stdout", false);
        logsAfterProtocolEOF();
    }

    /** Hold stderr until stdout's EOF has already cleared the engine's listener. */
    private static void logsAfterProtocolEOF() throws Exception {
        PipedInputStream stderr = new PipedInputStream();
        PipedOutputStream tail = new PipedOutputStream(stderr);
        Process process = new Process() {
            public InputStream getInputStream() { return new ByteArrayInputStream(new byte[0]); }
            public InputStream getErrorStream() { return stderr; }
            public OutputStream getOutputStream() { return new ByteArrayOutputStream(); }
            public int waitFor() { return 0; }
            public int exitValue() { return 0; }
            public void destroy() {}
        };
        NsgoEngine engine = new NsgoEngine(new Context());
        Field field = NsgoEngine.class.getDeclaredField("process");
        field.setAccessible(true);
        field.set(engine, process);
        CountDownLatch logged = new CountDownLatch(1);
        engine.attach(new NsgoEngine.Listener() {
            public void onMessage(int kind, byte[] payload) { throw new AssertionError("unexpected message"); }
            public void onProcessEnded(IOException error) {
                check(error == null, "clean EOF failed");
                try {
                    tail.write("nsgo-test-exit final-run 0\n".getBytes(StandardCharsets.UTF_8));
                    tail.close();
                } catch (IOException e) { throw new AssertionError(e); }
            }
            public void onLog(String line) {
                check(line.equals("nsgo-test-exit final-run 0"), "changed final log");
                logged.countDown();
            }
        });
        check(logged.await(5, TimeUnit.SECONDS), "lost stderr after the protocol ended");
        engine.destroy();
    }

    private static byte[] frame(int kind, int length, byte[] payload) throws IOException {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream out = new DataOutputStream(bytes);
        out.writeByte(kind);
        out.writeInt(length);
        out.write(payload);
        return bytes.toByteArray();
    }

    private static void messages() throws Exception {
        byte[] payload = new byte[]{0, 1, -1, 42};
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        // Consecutive frames preserve their boundaries and every current kind.
        for (int kind = 1; kind <= Protocol.DISK_SPACE; kind++) bytes.write(frame(kind, payload.length, payload));
        bytes.write(frame(13, 0, new byte[0]));
        DataInputStream in = new DataInputStream(new ByteArrayInputStream(bytes.toByteArray()));
        for (int kind = 1; kind <= Protocol.DISK_SPACE; kind++) {
            Protocol.Message message = Protocol.readMessage(in);
            check(message != null && message.kind == kind && Arrays.equals(message.payload, payload), "changed valid frame " + kind);
        }
        check(Protocol.readMessage(in).payload.length == 0, "empty payload refused");
        check(Protocol.readMessage(in) == null, "clean EOF refused");
        for (String text : new String[]{"=== RUN   TestA\n", "--- FAIL: TestA\n", "    target.go:151: TestMain\n", "PASS\n", "FAIL\n"}) {
            refused(text.getBytes(StandardCharsets.UTF_8), "unexpected message kind");
        }
        for (int kind : new int[]{0, Protocol.DISK_SPACE + 1, 255}) refused(frame(kind, 0, new byte[0]), "unexpected message kind");
        for (int length : new int[]{64 * 1024 * 1024 + 1, Integer.MAX_VALUE, -1, Integer.MIN_VALUE}) {
            refused(frame(1, length, new byte[0]), "message length");
        }
        for (int length = 1; length < 5; length++) {
            refused(Arrays.copyOf(frame(1, 4, payload), length), "incomplete message header");
        }
        refused(frame(1, 4, new byte[]{1, 2}), "incomplete message payload");
    }

    private static void refused(byte[] bytes, String reason) throws Exception {
        try {
            Protocol.readMessage(new DataInputStream(new ByteArrayInputStream(bytes)));
            throw new AssertionError("accepted " + reason);
        } catch (IOException e) {
            check(e.getMessage().contains(reason), e.toString());
            check(e.getMessage().contains("stdout") && e.getMessage().contains("TestMain") &&
                    e.getMessage().contains("nsgotest.Main"), "missing diagnostic: " + e);
        }
    }

    private static void child(String scenario) throws Exception {
        byte[] bytes;
        switch (scenario) {
            case "stdout": bytes = "=== RUN   TestWithoutMain\n".getBytes(StandardCharsets.UTF_8); break;
            case "length": bytes = frame(1, 64 * 1024 * 1024 + 1, new byte[0]); break;
            case "negative": bytes = frame(1, -1, new byte[0]); break;
            case "header": bytes = new byte[]{1, 0}; break;
            case "payload": bytes = frame(1, 4, new byte[]{1, 2}); break;
            case "valid": bytes = frame(13, 0, new byte[0]); break;
            default: throw new AssertionError(scenario);
        }
        System.out.write(bytes);
        System.out.flush();
        // Broken complete headers are rejected while the child is still alive. Partial frames close their pipe, as a child exiting mid-write would.
        if (scenario.equals("header") || scenario.equals("payload") || scenario.equals("valid")) System.out.close();
        Thread.sleep(60_000);
    }

    private static void engine(String scenario, boolean testing) throws Exception {
        String java = new File(System.getProperty("java.home"), "bin/java").getPath();
        ProcessBuilder builder = new ProcessBuilder(java, "-Xmx32m", "-cp", System.getProperty("java.class.path"),
                PipeCheck.class.getName(), scenario);
        builder.environment().remove("NSGO_TEST_RUN");
        if (testing) builder.environment().put("NSGO_TEST_RUN", "current-test");
        NsgoEngine engine = new NsgoEngine(new Context());
        CountDownLatch ended = new CountDownLatch(1);
        class Listener implements NsgoEngine.Listener {
            IOException failure;
            String log = "";
            int messages, ends;
            public void onMessage(int kind, byte[] payload) { messages++; }
            public void onLog(String line) { log += line + "\n"; }
            public void onProcessEnded(IOException error) { failure = error; ends++; ended.countDown(); }
        }
        Listener listener = new Listener();
        try {
            engine.start(builder);
            Field processField = NsgoEngine.class.getDeclaredField("process");
            processField.setAccessible(true);
            Process process = (Process) processField.get(engine);
            engine.attach(listener);
            check(ended.await(5, TimeUnit.SECONDS), "reader hung: " + scenario);
            if (scenario.equals("valid")) {
                check(listener.failure == null && listener.messages == 1 && listener.log.isEmpty(), "valid stream failed");
            } else {
                check(listener.failure != null && listener.messages == 0, "corrupt stream delivered: " + scenario);
                check(listener.failure.getMessage().contains("stdout"), "missing stdout diagnostic");
                check(!engine.isAlive(), "corrupt engine stayed alive");
                check(process.waitFor(3, TimeUnit.SECONDS), "corrupt pipe left its child process running");
                if (testing) {
                    check(listener.log.startsWith("nsgo-test-error current-test NSGo pipe:"), "missing run error: " + listener.log);
                    check(listener.log.contains("nsgotest.Main"), "missing TestMain hint");
                } else {
                    check(listener.log.isEmpty(), "a normal app wrote a test marker");
                }
            }
            engine.destroy();
            check(process.waitFor(3, TimeUnit.SECONDS), "destroy left its child process running");
            check(listener.ends == 1, "notified twice: " + scenario);
        } finally {
            engine.destroy();
        }
    }

    private static void check(boolean ok, String why) {
        if (!ok) throw new AssertionError(why);
    }
}
