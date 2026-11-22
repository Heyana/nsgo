package dev.oneo.nsgo;

import java.io.File;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Map;

public final class BackgroundTransferCheck {
    public static void main(String[] args) throws Exception {
        File partial = new File(args[1]);
        String certificate = args.length < 3 ? "" : Files.readString(Path.of(args[2]));
        BackgroundTransfer transfer = new BackgroundTransfer(partial, args[0], Map.of("X-Test", "download"), certificate);
        long[] progress = {0, -1};
        BackgroundTransfer.Progress report = (loaded, total) -> {
            progress[0] = loaded;
            progress[1] = total;
        };
        if (args[0].startsWith("http:")) {
            String failure = transfer.run((loaded, total) -> {
                report.update(loaded, total);
                if (loaded > 0) transfer.stopped = true;
            });
            if (failure != null || progress[0] <= 0 || progress[0] >= progress[1] || partial.length() != progress[0]) {
                throw new AssertionError("pause at " + progress[0] + "/" + progress[1] + ": " + failure);
            }
            System.out.println("paused " + partial.length());
            transfer.stopped = false;
        }
        String failure = transfer.run(report);
        if (failure != null) {
            System.out.println(failure);
            System.exit(1);
        }
        if (partial.length() != progress[0] || progress[0] != progress[1]) {
            throw new AssertionError("staged " + partial.length() + " for " + progress[0] + "/" + progress[1]);
        }
    }
}
