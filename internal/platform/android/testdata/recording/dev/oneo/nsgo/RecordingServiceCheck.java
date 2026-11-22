package dev.oneo.nsgo;

import android.app.Service;
import android.content.Context;
import android.media.AudioRecord;
import java.io.*;
import java.lang.reflect.*;
import java.nio.*;
import java.nio.file.*;
import java.util.ArrayDeque;
import java.util.concurrent.TimeUnit;
import java.util.function.BooleanSupplier;
import org.json.JSONObject;

/** Runs the production service, file writer and bounded sender on the JVM. */
public final class RecordingServiceCheck {
    private static Path root;
    private static int directory;

    public static void main(String[] args) throws Exception {
        root = Path.of(args[0]);
        if (args.length > 1 && args[1].equals("write-failure")) {
            writeFailure();
            return;
        }
        formatBusyAndStop();
        permissionAndServiceRefusal();
        starvationAndReadFailure();
        interruption(false);
        interruption(true);
        processDeathAndRepair();
        blockedReportsKeepTheirOwner();
    }

    private static NsgoEngine owner() throws Exception {
        AudioRecord.reset();
        Context.refuseService = Service.refuseForeground = false;
        return new NsgoEngine(Files.createDirectory(root.resolve("case-" + directory++)).toFile());
    }

    private static JSONObject start(long id, String name, boolean pause) throws Exception {
        return new JSONObject().put("id", id).put("name", name).put("rate", 44100)
                .put("channels", 2).put("level", 50).put("pause", pause);
    }

    private static void formatBusyAndStop() throws Exception {
        NsgoEngine owner = owner();
        RecordingService.start(new Context(), owner, start(1, "format.wav", false));
        await(() -> owner.report("started") != null, "start report");
        check(owner.report("started").contains("\"rate\":48000") &&
                owner.report("started").contains("\"channels\":1"), "actual format was not reported");
        check(Service.foreground && Service.type == 128, "microphone foreground service");
        RecordingService.start(new Context(), owner, start(2, "busy.wav", false));
        check(owner.report("ended").contains("\"error\":\"busy\""), "second start did not report busy");
        Path part = owner.recordings().toPath().resolve(".format.wav.part");
        await(() -> size(part) > 12000, "PCM writes");
        RecordingService.stop(owner, 1);
        drained(owner);
        Path file = owner.recordings().toPath().resolve("format.wav");
        wav(file, 48000, 1);
        check(AudioRecord.latest.released && !Service.foreground, "stop leaked capture or service");
        check(!Files.exists(part), "stop kept the temporary file");
    }

    private static void permissionAndServiceRefusal() throws Exception {
        NsgoEngine denied = owner();
        AudioRecord.denied = true;
        RecordingService.start(new Context(), denied, start(1, "denied.wav", false));
        drained(denied);
        check(denied.report("ended").contains("\"error\":\"denied\""), "capture permission failure");
        NsgoEngine unavailable = owner();
        Context.refuseService = true;
        RecordingService.start(new Context(), unavailable, start(1, "refused.wav", false));
        drained(unavailable);
        check(unavailable.report("ended").contains("\"error\":\"unavailable\""), "service refusal");
        NsgoEngine busy = owner();
        AudioRecord.busy = true;
        RecordingService.start(new Context(), busy, start(1, "busy.wav", false));
        drained(busy);
        check(busy.report("ended").contains("\"error\":\"busy\"") && AudioRecord.latest.released,
                "busy capture did not release its handle");
        check(busy.recordings().list().length == 0, "failed start kept an empty file");
    }

    private static void starvationAndReadFailure() throws Exception {
        NsgoEngine owner = owner();
        AudioRecord.starve = 5;
        AudioRecord.failAfter = 30;
        RecordingService.start(new Context(), owner, start(1, "read.wav", false));
        drained(owner);
        check(owner.report("started") != null && owner.report("level") != null, "starved capture did not resume");
        check(owner.report("ended").contains("\"error\":\"failed\""), "read error was not terminal");
        wav(owner.recordings().toPath().resolve("read.wav"), 48000, 1);
        check(AudioRecord.latest.released, "read error leaked the AudioRecord");
    }

    private static void writeFailure() throws Exception {
        NsgoEngine owner = owner();
        RecordingService.start(new Context(), owner, start(1, "full.wav", false));
        drained(owner);
        check(owner.report("ended").contains("\"error\":\"failed\""), "writer error was not terminal");
        check(AudioRecord.latest.released, "writer error leaked the microphone");
        wav(owner.recordings().toPath().resolve("full.wav"), 48000, 1);
    }

    private static void interruption(boolean pause) throws Exception {
        NsgoEngine owner = owner();
        RecordingService.start(new Context(), owner, start(1, "silenced.wav", pause));
        await(() -> owner.report("started") != null, "interruption start");
        Path part = owner.recordings().toPath().resolve(".silenced.wav.part");
        await(() -> size(part) > 12000, "interruption PCM");
        AudioRecord.latest.silenced(true);
        if (pause) {
            await(() -> owner.report("interrupted") != null, "silenced report");
            Thread.sleep(20);
            long before = size(part);
            Thread.sleep(40);
            check(size(part) == before, "silenced frames reached the file");
            AudioRecord.latest.silenced(false);
            await(() -> size(part) > before, "capture did not resume");
            RecordingService.stop(owner, 1);
        }
        drained(owner);
        String ended = owner.report("ended");
        check(ended != null && ended.contains(pause ? "\"error\":\"\"" : "\"error\":\"interrupted\""),
                "interruption policy");
        wav(owner.recordings().toPath().resolve("silenced.wav"), 48000, 1);
    }

    private static void processDeathAndRepair() throws Exception {
        NsgoEngine dead = owner();
        RecordingService.start(new Context(), dead, start(1, "abandoned.wav", false));
        Path part = dead.recordings().toPath().resolve(".abandoned.wav.part");
        await(() -> size(part) > 12000, "abandoned PCM");
        dead.alive = false;
        RecordingService.ownerEnded(dead);
        drained(dead);
        NsgoEngine heir = new NsgoEngine(dead.recordings());
        RecordingService.connected(heir);
        check(heir.report("earlier").contains("\"error\":\"abandoned\""), "dead process lost its terminal record");
        Path file = dead.recordings().toPath().resolve("abandoned.wav");
        wav(file, 48000, 1);
        RecordingService.taken(file.toString());

        Method header = RecordingService.class.getDeclaredMethod("header", int.class, int.class, long.class);
        header.setAccessible(true);
        Path partial = dead.recordings().toPath().resolve(".partial.wav.part");
        try (OutputStream out = Files.newOutputStream(partial)) {
            out.write((byte[]) header.invoke(null, 48000, 1, 0L));
            out.write(new byte[1001]);
        }
        RecordingService.start(new Context(), heir, start(2, "repair-trigger.wav", false));
        await(() -> heir.report("started") != null, "repair trigger");
        RecordingService.stop(heir, 2);
        drained(heir);
        Path repaired = dead.recordings().toPath().resolve("partial.wav");
        wav(repaired, 48000, 1);
        check(Files.size(repaired) == 1044 && !Files.exists(partial), "partial frame was not repaired");
    }

    private static void blockedReportsKeepTheirOwner() throws Exception {
        NsgoEngine owner = owner();
        owner.blocked = true;
        RecordingService.start(new Context(), owner, start(1, "blocked.wav", false));
        check(owner.writing.await(2, TimeUnit.SECONDS), "sender never reached the blocked pipe");
        Field current = RecordingService.class.getDeclaredField("current");
        current.setAccessible(true);
        Object recorder = current.get(null);
        Field senderField = recorder.getClass().getDeclaredField("sender");
        senderField.setAccessible(true);
        Object sender = senderField.get(recorder);
        Path part = owner.recordings().toPath().resolve(".blocked.wav.part");
        await(() -> size(part) > 24000, "blocked Go pipe stalled the PCM writer");
        Field reports = sender.getClass().getDeclaredField("reports");
        reports.setAccessible(true);
        synchronized (sender) {
            check(((ArrayDeque<?>) reports.get(sender)).size() <= 1, "level reports grew an unbounded queue");
        }
        RecordingService.stop(owner, 1);
        Path file = owner.recordings().toPath().resolve("blocked.wav");
        await(() -> Files.exists(file), "blocked terminal report stalled file completion");
        check(RecordingService.active(owner), "finishing sender stopped retaining its engine before the terminal report");
        // A new capture can begin while the previous terminal report waits.
        RecordingService.start(new Context(), owner, start(2, "second.wav", false));
        check(owner.secondWriting.await(2, TimeUnit.SECONDS), "second sender did not reach its pipe");
        Path secondPart = owner.recordings().toPath().resolve(".second.wav.part");
        await(() -> size(secondPart) > 12000, "second capture did not write");
        RecordingService.stop(owner, 2);
        Path second = owner.recordings().toPath().resolve("second.wav");
        await(() -> Files.exists(second), "second capture did not finish");
        owner.writable.countDown();
        drained(owner);
        check(RecordingService.active(owner), "first sender released the second sender's engine hold");
        owner.secondWritable.countDown();
        await(() -> !RecordingService.active(owner), "second sender did not release its engine hold");
        check(!RecordingService.active(owner), "drained sender kept its engine");
        check(owner.report("ended") != null, "terminal report was lost behind levels");
        wav(file, 48000, 1);
    }

    private static void drained(NsgoEngine owner) throws Exception {
        check(owner.drained.await(3, TimeUnit.SECONDS), "recording sender did not finish");
    }

    private static long size(Path path) {
        try { return Files.size(path); } catch (IOException e) { return 0; }
    }

    private static void wav(Path path, int rate, int channels) throws Exception {
        try (DataInputStream in = new DataInputStream(Files.newInputStream(path))) {
            byte[] header = new byte[44];
            in.readFully(header);
            ByteBuffer b = ByteBuffer.wrap(header).order(ByteOrder.LITTLE_ENDIAN);
            check(new String(header, 0, 4, java.nio.charset.StandardCharsets.US_ASCII).equals("RIFF") &&
                    b.getInt(24) == rate && b.getShort(22) == channels &&
                    Integer.toUnsignedLong(b.getInt(40)) == Files.size(path) - 44, "invalid WAV format or sizes");
        }
    }

    private static void await(BooleanSupplier predicate, String message) throws Exception {
        long deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(3);
        while (!predicate.getAsBoolean()) {
            check(System.nanoTime() < deadline, message);
            Thread.sleep(2);
        }
    }

    private static void check(boolean value, String message) {
        if (!value) throw new AssertionError(message);
    }
}
