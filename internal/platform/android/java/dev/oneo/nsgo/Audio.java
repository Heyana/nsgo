package dev.oneo.nsgo;

import android.content.Context;
import android.media.AudioAttributes;
import android.media.MediaExtractor;
import android.media.MediaFormat;
import android.media.SoundPool;
import android.os.Handler;
import android.os.HandlerThread;
import android.os.Looper;
import android.os.SystemClock;
import android.util.SparseArray;

import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.File;
import java.io.IOException;
import java.util.ArrayList;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.function.Consumer;

/** One engine's SoundPool and players. Sound commands use the UI thread and players own a media looper. */
final class Audio {
    // The driver's AudioCommand.
    private static final int LOAD = 0, PLAY = 1, STOP = 2, UNLOAD = 3, OPEN = 4,
            PRELOAD = 5, RESUME = 6, PAUSE = 7, SEEK = 8, SET = 9, CLOSE = 10,
            PLAYBACK_SET = 12, PLAYBACK_STOP = 13;
    // The driver's AudioPhase.
    static final int LOADED = 0, OPENED = 1, PLAYING = 2, PAUSED = 3, PROGRESS = 4,
            BUFFER = 5, ENDED = 6, FAILURE = 7, PLAYBACK_ENDED = 9;
    private static final int MAX_VOICES = 32;
    /** SoundPool truncates decoded 16-bit PCM after 1 MiB. */
    private static final long MAX_SAMPLE_BYTES = 1L << 20;

    private static final class Sound {
        final int owner, id;
        final long limit, maxDuration;
        Future<?> inspecting;
        int sample;
        long duration, bytes;
        boolean loaded;

        Sound(int owner, int id, long limit, long maxDuration) {
            this.owner = owner;
            this.id = id;
            this.limit = limit;
            this.maxDuration = maxDuration;
        }
    }

    private static final class Voice {
        final Sound sound;
        final int stream, playback;
        final float left, right;
        /** A looping voice repeats until stopped: it has no remaining time and no completion. */
        final boolean loop;
        long changed;
        double remaining;
        float rate;
        Runnable completion;

        Voice(Sound sound, int playback, int stream, float left, float right, float rate, boolean loop, long now) {
            this.sound = sound;
            this.loop = loop;
            this.playback = playback;
            this.stream = stream;
            this.left = left;
            this.right = right;
            this.rate = rate;
            this.changed = now;
            this.remaining = sound.duration * 1000.0;
        }

        void advance(long now) {
            if (!loop) remaining = Math.max(0, remaining - Math.max(0, now - changed) * (double) rate);
            changed = now;
        }
    }

    private final Context context;
    private final Downloads.Writer out;
    private final Handler ui = new Handler(Looper.getMainLooper());
    private final SparseArray<Sound> sounds = new SparseArray<>();
    private final SparseArray<Sound> loading = new SparseArray<>();
    private final ArrayList<Voice> voices = new ArrayList<>();
    private final SparseArray<AudioPlayer> players = new SparseArray<>(); // media looper only
    private SoundPool pool;
    private ExecutorService inspectors;
    private HandlerThread playerThread;
    private Handler media;

    Audio(Context context, Downloads.Writer out) {
        this.context = context.getApplicationContext();
        this.out = out;
    }

    void command(int owner, boolean visible, DataInputStream in) throws IOException {
        int id = in.readInt(), command = in.readUnsignedByte();
        int playback = in.readInt();
        switch (command) {
            case LOAD: {
                String source = source(in);
                Sound sound = new Sound(owner, id, in.readLong(), in.readLong());
                sounds.put(id, sound);
                if (source != null) load(sound, source);
                break;
            }
            case PLAY: {
                float volume = in.readFloat(), left = in.readFloat(), right = in.readFloat(), rate = in.readFloat();
                boolean loop = in.readBoolean();
                play(sounds.get(id), playback, volume, left, right, rate, loop);
                break;
            }
            case PLAYBACK_SET:
                setPlayback(id, playback, in.readFloat(), in.readFloat());
                break;
            case PLAYBACK_STOP:
                stopPlayback(id, playback);
                break;
            case STOP:
                stop(sounds.get(id));
                break;
            case UNLOAD:
                unload(id);
                break;
            case OPEN: {
                String source = source(in);
                float volume = in.readFloat(), rate = in.readFloat();
                boolean loop = in.readBoolean();
                int session = in.readUnsignedByte();
                long interval = in.readLong(), position = in.readLong(), loopStart = in.readLong();
                if (media == null) {
                    playerThread = new HandlerThread("nsgo-audio-player");
                    playerThread.start();
                    media = new Handler(playerThread.getLooper());
                }
                Handler handler = media;
                handler.post(() -> players.put(id, new AudioPlayer(this, context, handler, owner, id,
                        source, volume, rate, loop, loopStart, session, interval, position)));
                break;
            }
            case PRELOAD:
                player(id, AudioPlayer::preload);
                break;
            case RESUME:
                player(id, p -> p.play(visible));
                break;
            case PAUSE:
                player(id, AudioPlayer::pause);
                break;
            case SEEK: {
                long position = in.readLong();
                player(id, p -> p.seek(position));
                break;
            }
            case SET: {
                float volume = in.readFloat(), rate = in.readFloat();
                boolean loop = in.readBoolean();
                player(id, p -> p.set(volume, rate, loop));
                break;
            }
            case CLOSE:
                player(id, p -> { players.remove(id); p.close(); });
                break;
            default:
                throw new IOException("unknown audio command " + command);
        }
    }

    private static String source(DataInputStream in) throws IOException {
        return in.readBoolean() ? null : Protocol.readString(in);
    }

    private void player(int id, Consumer<AudioPlayer> command) {
        if (media != null) media.post(() -> {
            AudioPlayer p = players.get(id);
            if (p != null) command.accept(p);
        });
    }

    void file(int owner, int id, String path, String code, String message) {
        Sound sound = sounds.get(id);
        if (sound != null) {
            if (sound.owner != owner) return;
            if (code.isEmpty()) load(sound, path);
            else soundFailed(sound, code, message);
            return;
        }
        player(id, p -> {
            if (p.owner == owner) p.file(path, code, message);
        });
    }

    private void load(Sound sound, String source) {
        if (inspectors == null) inspectors = Executors.newFixedThreadPool(2, work ->
                new Thread(work, "nsgo-sound-load"));
        sound.inspecting = inspectors.submit(() -> inspect(sound, source));
    }

    private void inspect(Sound sound, String source) {
        File file;
        try {
            file = AudioSource.soundFile(context, source, sound.limit);
        } catch (AudioSource.Limit e) {
            reportSoundFailure(sound, "quota-exceeded", e.getMessage());
            return;
        } catch (IOException | RuntimeException e) {
            reportSoundFailure(sound, AudioSource.remote(source) ? "network" : "not-supported", e.toString());
            return;
        }
        MediaExtractor extractor = new MediaExtractor();
        boolean handedOff = false;
        try {
            extractor.setDataSource(file.getPath());
            for (int i = 0; i < extractor.getTrackCount(); i++) {
                MediaFormat format = extractor.getTrackFormat(i);
                String mime = format.getString(MediaFormat.KEY_MIME);
                if (mime == null || !mime.startsWith("audio/")) continue;
                if (!format.containsKey(MediaFormat.KEY_DURATION) || !format.containsKey(MediaFormat.KEY_SAMPLE_RATE)
                        || !format.containsKey(MediaFormat.KEY_CHANNEL_COUNT)) break;
                long duration = format.getLong(MediaFormat.KEY_DURATION);
                int rate = format.getInteger(MediaFormat.KEY_SAMPLE_RATE);
                int channels = format.getInteger(MediaFormat.KEY_CHANNEL_COUNT);
                if (duration < 0 || rate <= 0 || channels <= 0) break;
                if (duration > sound.maxDuration) {
                    reportSoundFailure(sound, "quota-exceeded", "the sound is longer than its limit of " + sound.maxDuration / 1000 + " ms");
                    return;
                }
                double bytes = Math.ceil(duration * (double) rate / 1000000) * channels * 2;
                if (bytes > MAX_SAMPLE_BYTES) {
                    reportSoundFailure(sound, "quota-exceeded", "decoded audio exceeds Android's 1 MiB SoundPool limit");
                    return;
                }
                handedOff = ui.post(() -> loadSample(sound, file, duration, (long) bytes));
                return;
            }
            reportSoundFailure(sound, "decode", "the source has no audio track with a duration");
        } catch (IOException | RuntimeException e) {
            reportSoundFailure(sound, "decode", e.toString());
        } finally {
            extractor.release();
            if (!handedOff) file.delete();
        }
    }

    private void loadSample(Sound sound, File file, long duration, long bytes) {
        try {
            if (sounds.get(sound.id) != sound) return;
            sound.inspecting = null;
            if (pool == null) {
                AudioAttributes attrs = new AudioAttributes.Builder().setUsage(AudioAttributes.USAGE_GAME)
                        .setContentType(AudioAttributes.CONTENT_TYPE_SONIFICATION).build();
                pool = new SoundPool.Builder().setMaxStreams(MAX_VOICES).setAudioAttributes(attrs).build();
                pool.setOnLoadCompleteListener((current, sample, status) -> {
                    Sound ready = loading.get(sample);
                    loading.remove(sample);
                    if (current != pool || ready == null) return;
                    if (status != 0) {
                        soundFailed(ready, "decode", "SoundPool load failed (" + status + ")");
                        return;
                    }
                    ready.loaded = true;
                    report(ready.owner, ready.id, LOADED, ready.duration, 0, ready.bytes, 0, 0, false, "", "", 0);
                });
            }
            sound.sample = pool.load(file.getPath(), 1);
            if (sound.sample == 0) {
                soundFailed(sound, "decode", "SoundPool could not load the source");
                return;
            }
            sound.duration = duration;
            sound.bytes = bytes;
            loading.put(sound.sample, sound);
        } catch (RuntimeException e) {
            soundFailed(sound, "failed", e.toString());
        } finally {
            file.delete();
        }
    }

    private void reportSoundFailure(Sound sound, String code, String message) {
        ui.post(() -> soundFailed(sound, code, message));
    }

    private void soundFailed(Sound sound, String code, String message) {
        if (sounds.get(sound.id) != sound) return;
        unload(sound.id);
        failure(sound.owner, sound.id, code, message);
    }

    private void play(Sound sound, int playback, float volume, float left, float right, float rate, boolean loop) {
        if (sound == null || !sound.loaded) return;
        long now = SystemClock.elapsedRealtimeNanos();
        for (int i = voices.size() - 1; i >= 0; i--) {
            Voice voice = voices.get(i);
            voice.advance(now);
            if (voice.remaining == 0) finishVoice(voice, false);
        }
        if (voices.size() == MAX_VOICES) finishVoice(voices.get(0), true);
        // SoundPool's loop of -1 repeats the sample until the stream stops.
        int stream = pool.play(sound.sample, volume * left, volume * right, 1, loop ? -1 : 0, rate);
        if (stream != 0) {
            Voice voice = new Voice(sound, playback, stream, left, right, rate, loop, now);
            voice.completion = () -> finishVoice(voice, false);
            voices.add(voice);
            scheduleVoice(voice);
        } else if (playback != 0) {
            report(sound.owner, sound.id, PLAYBACK_ENDED, 0, 0, 0, 0, 0, false, "", "", playback);
        }
    }

    // SoundPool has no stream completion callback. Schedule one from the
    // decoded duration, adjusting the remaining source time after rate changes.
    private void scheduleVoice(Voice voice) {
        ui.removeCallbacks(voice.completion);
        if (voice.loop) return;
        long delay = (long) Math.ceil(voice.remaining / voice.rate / 1000000);
        ui.postDelayed(voice.completion, delay);
    }

    private void finishVoice(Voice voice, boolean stop) {
        if (!voices.remove(voice)) return;
        ui.removeCallbacks(voice.completion);
        if (stop) pool.stop(voice.stream);
        if (voice.playback != 0) {
            report(voice.sound.owner, voice.sound.id, PLAYBACK_ENDED,
                    voice.sound.duration, 0, 0, 0, 0, false, "", "", voice.playback);
        }
    }

    private void setPlayback(int id, int playback, float volume, float rate) {
        for (Voice voice : voices) if (voice.sound.id == id && voice.playback == playback) {
            voice.advance(SystemClock.elapsedRealtimeNanos());
            if (voice.remaining == 0) {
                finishVoice(voice, false);
                return;
            }
            pool.setVolume(voice.stream, volume * voice.left, volume * voice.right);
            pool.setRate(voice.stream, rate);
            voice.rate = rate;
            scheduleVoice(voice);
            return;
        }
    }

    private void stopPlayback(int id, int playback) {
        for (Voice voice : voices) if (voice.sound.id == id && voice.playback == playback) {
            finishVoice(voice, true);
            return;
        }
    }

    private void stop(Sound sound) {
        if (sound == null) return;
        for (int i = voices.size() - 1; i >= 0; i--) {
            Voice voice = voices.get(i);
            if (voice.sound != sound) continue;
            finishVoice(voice, true);
        }
    }

    private void unload(int id) {
        Sound sound = sounds.get(id);
        if (sound == null) return;
        sounds.remove(id);
        if (sound.inspecting != null) sound.inspecting.cancel(true);
        stop(sound);
        if (sound.sample != 0) {
            loading.remove(sound.sample);
            pool.unload(sound.sample);
        }
    }

    void pauseSurface(int owner) {
        eachPlayer(owner, AudioPlayer::pause);
    }

    private void eachPlayer(int owner, Consumer<AudioPlayer> action) {
        if (media != null) media.post(() -> {
            for (int i = players.size() - 1; i >= 0; i--) {
                AudioPlayer p = players.valueAt(i);
                if (p.owner == owner) action.accept(p);
            }
        });
    }

    void closeSurface(int owner) {
        for (int i = sounds.size() - 1; i >= 0; i--) {
            Sound sound = sounds.valueAt(i);
            if (sound.owner == owner) unload(sound.id);
        }
        eachPlayer(owner, p -> { players.remove(p.id); p.close(); });
    }

    void close() {
        for (int i = sounds.size() - 1; i >= 0; i--) unload(sounds.keyAt(i));
        if (pool != null) { pool.release(); pool = null; }
        if (inspectors != null) inspectors.shutdownNow();
        if (media != null) {
            media.post(() -> {
                for (int i = 0; i < players.size(); i++) players.valueAt(i).close();
                players.clear();
            });
            playerThread.quitSafely();
            media = null;
        }
    }

    void failure(int owner, int id, String code, String message) {
        report(owner, id, FAILURE, 0, 0, 0, 0, 0, false, code, message, 0);
    }

    void report(int owner, int id, int phase, long duration, long position, long bytes,
            int seeks, int laps, boolean buffering, String code, String message) {
        report(owner, id, phase, duration, position, bytes, seeks, laps, buffering, code, message, 0);
    }

    void report(int owner, int id, int phase, long duration, long position, long bytes,
            int seeks, int laps, boolean buffering, String code, String message, int playback) {
        ByteArrayOutputStream data = new ByteArrayOutputStream(64);
        DataOutputStream d = new DataOutputStream(data);
        try {
            d.writeInt(owner);
            d.writeInt(id);
            d.writeInt(playback);
            d.writeByte(phase);
            d.writeLong(duration);
            d.writeLong(position);
            d.writeLong(bytes);
            d.writeInt(seeks);
            d.writeInt(laps);
            d.writeBoolean(buffering);
            Protocol.writeString(d, code);
            Protocol.writeString(d, message);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        out.write(Protocol.AUDIO_REPORT, data.toByteArray());
    }
}
