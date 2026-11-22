package dev.oneo.nsgo;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.media.AudioAttributes;
import android.media.AudioFocusRequest;
import android.media.AudioManager;
import android.media.MediaPlayer;
import android.os.Build;
import android.os.Handler;

import java.io.FileNotFoundException;
import java.io.IOException;

/** One long audio source. All calls and callbacks belong to the engine's media looper. */
final class AudioPlayer {
    private static final long LOAD_TIMEOUT = 60000;
    private static final int PLAYBACK = 1;

    final int owner, id;
    private final Audio audio;
    private final Context context;
    private final Handler media;
    private final AudioAttributes attributes;
    private final int session;
    private final long interval;
    /** loopStart is where a loop starts over once a pass ends, in milliseconds. */
    private final long loopStart;
    private String source;
    private String sourceError = "", sourceMessage = "";
    private MediaPlayer player;
    private boolean prepareWanted, prepared, wanted, playing, loop, ended;
    private boolean pendingSeek, seeking, listeningForRoute;
    private float volume, rate;
    private long position, duration;
    // One seek is in flight. The newest request waits, so its completion
    // cannot accidentally acknowledge an earlier seek's position to Go.
    private int seeks, seekingNumber, appliedSeeks, laps;
    private AudioFocusRequest focus;
    private final Runnable progress = this::progress;
    private final Runnable timeout = () -> fail(
            AudioSource.remote(source) ? "network" : "failed", "audio preparation timed out");
    private final BroadcastReceiver route = new BroadcastReceiver() {
        @Override
        public void onReceive(Context ignored, Intent intent) {
            if (playing && AudioManager.ACTION_AUDIO_BECOMING_NOISY.equals(intent.getAction())) {
                fail("failed", "the audio output went away");
            }
        }
    };

    AudioPlayer(Audio audio, Context context, Handler media, int owner, int id, String source,
            float volume, float rate, boolean loop, long loopStart, int session, long interval, long position) {
        this.audio = audio;
        this.context = context;
        this.media = media;
        this.owner = owner;
        this.id = id;
        this.source = source;
        this.volume = volume;
        this.rate = rate;
        this.loop = loop;
        this.loopStart = Math.max(0, loopStart);
        this.session = session;
        this.interval = Math.max(16, interval);
        this.position = position;
        this.pendingSeek = position > 0;
        attributes = new AudioAttributes.Builder().setUsage(AudioAttributes.USAGE_MEDIA)
                .setContentType(session == 0 ? AudioAttributes.CONTENT_TYPE_MUSIC : AudioAttributes.CONTENT_TYPE_SPEECH)
                .build();
    }

    void file(String path, String code, String message) {
        source = path;
        sourceError = code;
        sourceMessage = message;
        if (prepareWanted) prepare();
    }

    void preload() {
        prepareWanted = true;
        prepare();
    }

    void play(boolean visible) {
        if (!visible) {
            if (wanted || playing) pause();
            else report(Audio.PAUSED, false);
            return;
        }
        wanted = true;
        preload();
        if (prepared) start();
    }

    private void prepare() {
        if (player != null || source == null) return;
        prepareWanted = false;
        if (!sourceError.isEmpty()) {
            fail(sourceError, sourceMessage);
            return;
        }
        try {
            MediaPlayer p = new MediaPlayer();
            player = p;
            p.setAudioAttributes(attributes);
            p.setOnPreparedListener(this::prepared);
            p.setOnCompletionListener(this::completed);
            p.setOnSeekCompleteListener(this::sought);
            p.setOnInfoListener((mp, what, extra) -> {
                if (mp != player) return false;
                if (what == MediaPlayer.MEDIA_INFO_BUFFERING_START || what == MediaPlayer.MEDIA_INFO_BUFFERING_END) {
                    report(Audio.BUFFER, what == MediaPlayer.MEDIA_INFO_BUFFERING_START);
                    return true;
                }
                return false;
            });
            p.setOnErrorListener((mp, what, extra) -> {
                if (mp == player) fail(mediaError(what, extra), "media error " + what + " (" + extra + ")");
                return true;
            });
            AudioSource.player(context, p, source);
            p.prepareAsync();
            media.postDelayed(timeout, LOAD_TIMEOUT);
        } catch (FileNotFoundException | SecurityException e) {
            fail(AudioSource.remote(source) ? "network" : "not-supported", e.toString());
        } catch (IOException | IllegalArgumentException e) {
            fail(AudioSource.remote(source) ? "network" : "decode", e.toString());
        } catch (RuntimeException e) {
            fail("failed", e.toString());
        }
    }

    private String mediaError(int what, int extra) {
        if (what == MediaPlayer.MEDIA_ERROR_SERVER_DIED) return "failed";
        if (extra == MediaPlayer.MEDIA_ERROR_MALFORMED || extra == MediaPlayer.MEDIA_ERROR_UNSUPPORTED) return "decode";
        if (AudioSource.remote(source)
                && (extra == MediaPlayer.MEDIA_ERROR_IO || extra == MediaPlayer.MEDIA_ERROR_TIMED_OUT)) return "network";
        return prepared ? "failed" : "decode";
    }

    private void prepared(MediaPlayer p) {
        if (p != player) return;
        media.removeCallbacks(timeout);
        try {
            boolean hasAudio = false;
            for (MediaPlayer.TrackInfo track : p.getTrackInfo()) {
                if (track.getTrackType() == MediaPlayer.TrackInfo.MEDIA_TRACK_TYPE_AUDIO) hasAudio = true;
            }
            if (!hasAudio) {
                fail("decode", "the source has no audio track");
                return;
            }
            prepared = true;
            duration = Math.max(0, p.getDuration());
            p.setVolume(volume, volume);
            report(Audio.OPENED, false);
            if (pendingSeek) applySeek();
            else start();
        } catch (RuntimeException e) {
            fail("failed", e.toString());
        }
    }

    private void start() {
        if (!wanted || !prepared || seeking || pendingSeek) return;
        try {
            if (!holdFocus()) {
                fail("not-allowed", "Android refused audio focus");
                return;
            }
            if (ended) position = 0;
            ended = false;
            if (!player.isPlaying()) player.start();
            applyRate();
            if (!playing) {
                playing = true;
                watchRoute();
                report(Audio.PLAYING, false);
                scheduleProgress();
            }
        } catch (RuntimeException e) {
            fail("failed", e.toString());
        }
    }

    void pause() {
        if (!wanted && !playing) return;
        wanted = false;
        if (playing) {
            try {
                player.pause();
                if (!seeking && !pendingSeek) position = Math.max(0, player.getCurrentPosition());
            } catch (RuntimeException e) {
                fail("failed", e.toString());
                return;
            }
        }
        playing = false;
        releaseOutput();
        report(Audio.PAUSED, false);
    }

    void seek(long target) {
        position = Math.max(0, target);
        pendingSeek = true;
        seeks++;
        applySeek();
    }

    private void applySeek() {
        if (!prepared || seeking) return;
        try {
            if (ended) player.pause(); // Start from the seek after PlaybackCompleted.
            ended = false;
            pendingSeek = false;
            seeking = true;
            seekingNumber = seeks;
            player.seekTo(duration > 0 ? Math.min(position, duration) : position, MediaPlayer.SEEK_CLOSEST);
        } catch (RuntimeException e) {
            fail("failed", e.toString());
        }
    }

    private void sought(MediaPlayer p) {
        if (p != player || !seeking) return;
        seeking = false;
        appliedSeeks = seekingNumber;
        if (pendingSeek) {
            applySeek();
            return;
        }
        try {
            position = Math.max(0, p.getCurrentPosition());
            report(Audio.PROGRESS, false);
            start();
        } catch (RuntimeException e) {
            fail("failed", e.toString());
        }
    }

    void set(float volume, float rate, boolean loop) {
        this.volume = volume;
        this.rate = rate;
        this.loop = loop;
        if (!prepared) return;
        try {
            player.setVolume(volume, volume);
            if (playing) applyRate();
        } catch (RuntimeException e) {
            fail("failed", e.toString());
        }
    }

    private void applyRate() {
        android.media.PlaybackParams params = player.getPlaybackParams();
        if (params.getSpeed() != rate) player.setPlaybackParams(params.setSpeed(rate));
    }

    private void completed(MediaPlayer p) {
        if (p != player || !playing || seeking || pendingSeek) return;
        if (loop) {
            try {
                // A loop starts over at its LoopStart, within the track (driver.AudioLoopStart), with the short gap of a seek.
                position = loopStart < duration ? loopStart : 0;
                laps++;
                if (position > 0) p.seekTo((int) position);
                p.start();
                applyRate();
                report(Audio.PROGRESS, false);
                scheduleProgress();
            } catch (RuntimeException e) {
                fail("failed", e.toString());
            }
            return;
        }
        ended = true;
        wanted = playing = false;
        position = duration;
        releaseOutput();
        report(Audio.ENDED, false);
    }

    private void scheduleProgress() {
        media.removeCallbacks(progress);
        media.postDelayed(progress, interval);
    }

    private void progress() {
        if (!playing) return;
        try {
            if (!seeking && !pendingSeek) {
                position = Math.max(0, player.getCurrentPosition());
                report(Audio.PROGRESS, false);
            }
            scheduleProgress();
        } catch (RuntimeException e) {
            fail("failed", e.toString());
        }
    }

    private boolean holdFocus() {
        if (session != PLAYBACK || focus != null) return true;
        AudioManager manager = context.getSystemService(AudioManager.class);
        if (manager == null) return false;
        AudioFocusRequest request = new AudioFocusRequest.Builder(AudioManager.AUDIOFOCUS_GAIN_TRANSIENT)
                .setAudioAttributes(attributes)
                .setWillPauseWhenDucked(true)
                .setOnAudioFocusChangeListener(change -> {
                    if (change < 0 && focus != null) pause();
                }, media)
                .build();
        if (manager.requestAudioFocus(request) != AudioManager.AUDIOFOCUS_REQUEST_GRANTED) return false;
        focus = request;
        return true;
    }

    private void watchRoute() {
        if (listeningForRoute) return;
        IntentFilter filter = new IntentFilter(AudioManager.ACTION_AUDIO_BECOMING_NOISY);
        if (Build.VERSION.SDK_INT >= 33) {
            context.registerReceiver(route, filter, null, media, Context.RECEIVER_NOT_EXPORTED);
        } else {
            context.registerReceiver(route, filter, null, media);
        }
        listeningForRoute = true;
    }

    private void releaseOutput() {
        media.removeCallbacks(progress);
        if (focus != null) {
            AudioManager manager = context.getSystemService(AudioManager.class);
            if (manager != null) manager.abandonAudioFocusRequest(focus);
            focus = null;
        }
        if (listeningForRoute) {
            context.unregisterReceiver(route);
            listeningForRoute = false;
        }
    }

    void close() {
        media.removeCallbacks(timeout);
        releaseOutput();
        if (player != null) {
            MediaPlayer p = player;
            player = null;
            p.release();
        }
        wanted = playing = prepared = seeking = prepareWanted = false;
    }

    private void fail(String code, String message) {
        if (ended) position = 0;
        ended = false;
        pendingSeek = pendingSeek || seeking || position > 0;
        close();
        audio.failure(owner, id, code, message);
    }

    private void report(int phase, boolean buffering) {
        audio.report(owner, id, phase, duration * 1000, position * 1000, 0,
                appliedSeeks, laps, buffering, "", "");
    }
}
