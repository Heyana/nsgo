package dev.oneo.nsgo;

import android.content.Context;
import android.content.pm.PackageManager;
import android.media.AudioAttributes;
import android.os.VibrationEffect;
import android.os.Vibrator;
import java.io.DataInputStream;
import java.io.IOException;
import java.util.Arrays;

/** Timed haptic feedback, owned by the view that started the current pattern. */
final class HapticPatterns {
    private static HapticPatterns current;
    private static final AudioAttributes feedback = new AudioAttributes.Builder()
            .setUsage(AudioAttributes.USAGE_ASSISTANCE_SONIFICATION)
            .setContentType(AudioAttributes.CONTENT_TYPE_SONIFICATION)
            .build();
    private final Vibrator vibrator;
    final boolean available;

    HapticPatterns(Context context) {
        vibrator = context.getSystemService(Vibrator.class);
        available = vibrator != null && vibrator.hasVibrator()
                && context.checkSelfPermission(android.Manifest.permission.VIBRATE) == PackageManager.PERMISSION_GRANTED;
    }

    void apply(DataInputStream in, boolean visible) throws IOException {
        int count = in.readInt();
        long[] times = new long[count];
        int[] amplitudes = new int[count];
        for (int i = 0; i < count; i++) {
            times[i] = in.readLong();
            amplitudes[i] = in.readUnsignedByte();
        }
        if (count == 0 || !visible) {
            stop();
            return;
        }
        if (!available) return;
        if (current != null) current.stop();
        VibrationEffect effect = vibrator.hasAmplitudeControl()
                ? VibrationEffect.createWaveform(times, amplitudes, -1)
                : VibrationEffect.createWaveform(onOff(times, amplitudes), -1);
        vibrator.vibrate(effect, feedback);
        current = this;
    }

    void stop() {
        if (current != this) return;
        vibrator.cancel();
        current = null;
    }

    // Android's on/off waveforms begin with a pause. Adjacent amplitudes
    // greater than zero become a single interval on this motor.
    private static long[] onOff(long[] times, int[] amplitudes) {
        long[] pattern = new long[times.length + 1];
        int segment = 0;
        boolean on = false;
        for (int i = 0; i < times.length; i++) {
            boolean next = amplitudes[i] > 0;
            if (next != on) {
                segment++;
                on = next;
            }
            pattern[segment] += times[i];
        }
        return Arrays.copyOf(pattern, segment + 1);
    }
}
