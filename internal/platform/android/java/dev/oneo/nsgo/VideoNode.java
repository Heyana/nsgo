package dev.oneo.nsgo;

import android.annotation.SuppressLint;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.res.AssetFileDescriptor;
import android.graphics.Matrix;
import android.graphics.SurfaceTexture;
import android.content.Intent;
import android.content.IntentFilter;
import android.os.Build;
import android.media.AudioAttributes;
import android.media.AudioFocusRequest;
import android.media.AudioManager;
import android.media.MediaPlayer;
import android.net.Uri;
import android.view.MotionEvent;
import android.view.Surface;
import android.view.TextureView;
import android.widget.MediaController;
import java.io.DataInputStream;
import java.io.File;
import java.io.FileNotFoundException;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.util.Collections;
import java.util.HashMap;
import java.util.Map;
import java.util.Objects;

/**
 * A video node's player, as NSH's VideoBox: a MediaPlayer drawing into a
 * TextureView, which unlike a SurfaceView transforms, clips and animates
 * like any view, filling the node's view. Controls attaches the
 * platform's MediaController.
 */
@SuppressLint("ViewConstructor")
final class VideoNode extends TextureView implements TextureView.SurfaceTextureListener,
        MediaController.MediaPlayerControl {
    /** Receives a video's events, times in milliseconds and sizes in pixels. UI thread. */
    interface Reporter {
        boolean wants(int kind);

        void report(int kind, String text, long time, long duration, int width, int height, int index,
                boolean buffering);
    }

    private final Context context;
    private final Reporter reporter;
    private MediaPlayer player;
    private Surface surface;
    private String source;
    private Map<String, String> headers = Collections.emptyMap();
    private boolean prepared;
    /** A non-repeating end stays stopped until an explicit replay, seek or source change. */
    private boolean completed;
    /** A seek that moved a completed player back has not landed yet: it plays, if paused is false, once it does. */
    private boolean replaying;
    private boolean paused, muted, repeat;
    /** Whether the player plays or not is the controls' or the system's, not paused, until paused changes: each such play or pause is reported once. resumable is a pause for a transient loss of the audio focus, which plays on when the focus comes back. */
    private boolean held, resumable;
    private float volume = 1, rate = 1;
    private int resizeMode = Protocol.RESIZE_CONTAIN;
    private long progressInterval = 250;
    /** Protocol's audio session: 0 default, 1 playback, 2 mix with others. */
    private int audioSession;
    /** Audio focus while a playback video plays (updateFocus). */
    private AudioFocusRequest focusRequest;
    private boolean hasFocus;
    private int videoWidth, videoHeight;
    private MediaController controller;
    /** Hears the sound's output go away, such as headphones unplugged, while the video plays and is heard. */
    private final BroadcastReceiver noisy = new BroadcastReceiver() {
        @Override
        public void onReceive(Context ignored, Intent intent) {
            if (AudioManager.ACTION_AUDIO_BECOMING_NOISY.equals(intent.getAction())) {
                platformPause(Protocol.VIDEO_PAUSE_ROUTE_LOST, false);
            }
        }
    };
    private boolean hearsNoisy;
    private final Runnable progress = this::progress;

    VideoNode(Context context, Reporter reporter) {
        super(context);
        this.context = context.getApplicationContext();
        this.reporter = reporter;
        setSurfaceTextureListener(this);
    }

    /** Applies an ItemVideo's fields, in the order android.go writes them. */
    void configure(DataInputStream in) throws IOException {
        String source = Protocol.readString(in);
        int flags = in.readUnsignedByte();
        float volume = in.readFloat(), rate = in.readFloat();
        int resizeMode = in.readUnsignedByte();
        long interval = in.readInt() & 0xffffffffL;
        int audioSession = in.readUnsignedByte();
        int headerCount = in.readInt();
        Map<String, String> headers = headerCount == 0 ? Collections.emptyMap() : new HashMap<>();
        for (int i = 0; i < headerCount; i++) {
            headers.put(Protocol.readString(in), Protocol.readString(in));
        }
        boolean wasPaused = paused;
        paused = Protocol.flag(flags, 0);
        if (paused != wasPaused) {
            // The app decides again.
            held = false;
            resumable = false;
            if (!paused) completed = false;
        }
        muted = Protocol.flag(flags, 1);
        repeat = Protocol.flag(flags, 2);
        boolean controls = Protocol.flag(flags, 3);
        this.volume = volume;
        this.rate = rate;
        progressInterval = interval;
        // A player takes its audio attributes before it prepares: another
        // session opens the source again.
        boolean reopen = audioSession != this.audioSession && player != null;
        if (audioSession != this.audioSession) {
            this.audioSession = audioSession;
            if (focusRequest != null) {
                abandonFocus();
                focusRequest = null;
            }
        }
        if (resizeMode != this.resizeMode) {
            this.resizeMode = resizeMode;
            fit();
        }
        if (controls && controller == null) {
            controller = new MediaController(getContext());
            controller.setMediaPlayer(this);
            controller.setAnchorView(this);
        } else if (!controls && controller != null) {
            controller.hide();
            controller = null;
        }
        // Go sends no source as an empty one.
        if (source.isEmpty()) source = null;
        if (!Objects.equals(source, this.source) || !headers.equals(this.headers) || reopen) {
            this.source = source;
            this.headers = headers;
            open();
        } else {
            apply();
        }
    }

    private void open() {
        release();
        if (source == null) return;
        MediaPlayer p = new MediaPlayer();
        player = p;
        p.setOnPreparedListener(mp -> {
            if (mp != player) return;
            prepared = true;
            // A paused player draws nothing until it seeks: show the first frame.
            if (paused) mp.seekTo(0);
            report(Protocol.EVENT_LOAD, "", 0, mp.getDuration(), mp.getVideoWidth(), mp.getVideoHeight(), false);
            apply();
        });
        p.setOnVideoSizeChangedListener((mp, w, h) -> {
            videoWidth = w;
            videoHeight = h;
            fit();
        });
        p.setOnCompletionListener(mp -> {
            if (mp != player) return;
            // One that repeats plays on from its start, and whoever held its play state keeps it, so a play from the controls while paused goes on; one that does not stops, the play state is paused's again, and it stays at its end through unrelated configuration until paused changes, a seek or the controls play it.
            if (!repeat) {
                completed = true;
                held = false;
                resumable = false;
            }
            report(Protocol.EVENT_END, "", 0, 0, 0, 0, false);
            if (repeat) {
                mp.seekTo(0);
                mp.start();
            }
            followPlaying();
        });
        p.setOnSeekCompleteListener(mp -> {
            if (mp != player || !replaying) return;
            replaying = false;
            apply();
        });
        p.setOnInfoListener((mp, what, extra) -> {
            if (what == MediaPlayer.MEDIA_INFO_BUFFERING_START || what == MediaPlayer.MEDIA_INFO_BUFFERING_END) {
                report(Protocol.EVENT_BUFFER, "", 0, 0, 0, 0, what == MediaPlayer.MEDIA_INFO_BUFFERING_START);
            }
            return false;
        });
        p.setOnErrorListener((mp, what, extra) -> {
            if (mp == player) report(Protocol.EVENT_ERROR, "media error " + what + " (" + extra + ")", 0, 0, 0, 0, false);
            return true;
        });
        if (surface != null) p.setSurface(surface);
        AudioAttributes attributes = audioAttributes();
        if (attributes != null) p.setAudioAttributes(attributes);
        report(Protocol.EVENT_LOAD_START, "", 0, 0, 0, 0, false);
        try {
            setMediaSource(p, source);
            p.prepareAsync();
        } catch (Exception e) {
            report(Protocol.EVENT_ERROR, String.valueOf(e.getMessage()), 0, 0, 0, 0, false);
        }
    }

    private void report(int kind, String text, long time, long duration, int width, int height, boolean buffering) {
        report(kind, text, time, duration, width, height, 0, buffering);
    }

    private void report(int kind, String text, long time, long duration, int width, int height, int index,
            boolean buffering) {
        // A stream without a duration, such as a live one, has -1.
        if (reporter.wants(kind)) {
            reporter.report(kind, text, Math.max(0, time), Math.max(0, duration), width, height, index, buffering);
        }
    }

    /**
     * Points a player at a URL, a file or an "asset:" path in the APK's
     * assets, where packaging puts the app's resources.
     */
    private void setMediaSource(MediaPlayer player, String uri) throws IOException {
        Context context = getContext();
        if (!uri.startsWith("asset:")) {
            Uri location = Uri.parse(uri);
            String scheme = location.getScheme();
            if (!headers.isEmpty() && ("http".equalsIgnoreCase(scheme) || "https".equalsIgnoreCase(scheme))) {
                player.setDataSource(context, location, headers);
            } else {
                player.setDataSource(context, location);
            }
            return;
        }
        String path = uri.substring("asset:".length()).replaceFirst("^/+", "");
        try (AssetFileDescriptor fd = context.getAssets().openFd(path)) {
            player.setDataSource(fd);
        } catch (FileNotFoundException compressed) {
            // Compressed in the APK: players need a seekable file.
            File copy = new File(context.getCacheDir(), "nsgo-media/" + path);
            if (!copy.exists()) {
                copy.getParentFile().mkdirs();
                File tmp = new File(copy.getPath() + ".tmp");
                try (InputStream in = context.getAssets().open(path); OutputStream out = new FileOutputStream(tmp)) {
                    byte[] buf = new byte[64 * 1024];
                    for (int n; (n = in.read(buf)) > 0; ) out.write(buf, 0, n);
                }
                tmp.renameTo(copy);
            }
            player.setDataSource(copy.getPath());
        }
    }

    /** Applies the playback properties to a prepared player. */
    private void apply() {
        MediaPlayer p = player;
        if (p == null || !prepared) return;
        float v = muted ? 0 : Math.max(0, Math.min(1, volume));
        p.setVolume(v, v);
        if (!held) {
            if (paused) {
                if (p.isPlaying()) p.pause();
            } else if (!completed && !replaying && !p.isPlaying()) {
                p.start();
            }
        }
        // Setting a speed starts playback, so only while playing.
        if (p.isPlaying() && rate > 0 && p.getPlaybackParams().getSpeed() != rate) {
            p.setPlaybackParams(p.getPlaybackParams().setSpeed(rate));
        }
        removeCallbacks(progress);
        if (p.isPlaying()) postDelayed(progress, progressInterval);
        followPlaying();
    }

    /** The sound of a playback or mix-with-others video is speech; a default one keeps the player's own. */
    private AudioAttributes audioAttributes() {
        if (audioSession == 0) return null;
        return new AudioAttributes.Builder()
                .setUsage(AudioAttributes.USAGE_MEDIA)
                .setContentType(AudioAttributes.CONTENT_TYPE_SPEECH)
                .build();
    }

    /** Holds the audio focus while the video plays, and hears the output go away while it plays and is heard: a muted video has nothing to stop. */
    private void followPlaying() {
        MediaPlayer p = player;
        boolean heard = p != null && prepared && p.isPlaying() && !muted;
        if (heard != hearsNoisy) {
            hearsNoisy = heard;
            if (heard) {
                IntentFilter filter = new IntentFilter(AudioManager.ACTION_AUDIO_BECOMING_NOISY);
                // A system broadcast reaches a receiver that is not exported.
                if (Build.VERSION.SDK_INT >= 33) {
                    context.registerReceiver(noisy, filter, Context.RECEIVER_NOT_EXPORTED);
                } else {
                    context.registerReceiver(noisy, filter);
                }
            } else {
                context.unregisterReceiver(noisy);
            }
        }
        updateFocus();
    }

    /** Holds audio focus while a playback video plays, and only then: other apps pause while it plays and resume once it pauses, ends or goes. Speech pauses rather than ducks when another app takes the focus, and plays on when it comes back unless the app changed paused meanwhile; the focus is kept for that. */
    private void updateFocus() {
        MediaPlayer p = player;
        boolean wanted = audioSession == 1 && p != null && prepared && (p.isPlaying() || resumable);
        if (wanted == hasFocus) return;
        if (!wanted) {
            abandonFocus();
            return;
        }
        AudioManager manager = (AudioManager) getContext().getSystemService(Context.AUDIO_SERVICE);
        if (manager == null) return;
        if (focusRequest == null) {
            focusRequest = new AudioFocusRequest.Builder(AudioManager.AUDIOFOCUS_GAIN_TRANSIENT)
                    .setAudioAttributes(audioAttributes())
                    .setWillPauseWhenDucked(true)
                    .setOnAudioFocusChangeListener(this::focusChanged)
                    .build();
        }
        hasFocus = manager.requestAudioFocus(focusRequest) == AudioManager.AUDIOFOCUS_REQUEST_GRANTED;
    }

    private void focusChanged(int change) {
        if (change == AudioManager.AUDIOFOCUS_GAIN) {
            hasFocus = true;
            boolean resume = resumable && held;
            resumable = false;
            if (resume) platformPlay();
            updateFocus();
        } else if (change == AudioManager.AUDIOFOCUS_LOSS) {
            // Gone for good: it stays paused.
            hasFocus = false;
            platformPause(Protocol.VIDEO_PAUSE_INTERRUPTION, false);
        } else {
            // Lost for a while (a call, a navigation prompt): it plays on when the focus is back.
            platformPause(Protocol.VIDEO_PAUSE_INTERRUPTION, true);
        }
    }

    /** Pauses a playing player that the app did not pause, and reports it. */
    private void platformPause(int reason, boolean transientLoss) {
        MediaPlayer p = player;
        if (p == null || !prepared || !p.isPlaying()) return;
        p.pause();
        held = !paused;
        resumable = held && transientLoss;
        removeCallbacks(progress);
        report(Protocol.EVENT_VIDEO_PAUSE, "", 0, 0, 0, 0, reason, false);
        followPlaying();
    }

    /** Plays a paused player that the app did not play, and reports it. */
    private void platformPlay() {
        MediaPlayer p = player;
        if (p == null || !prepared || p.isPlaying()) return;
        completed = replaying = false;
        p.start();
        held = paused;
        resumable = false;
        if (rate > 0 && p.getPlaybackParams().getSpeed() != rate) {
            p.setPlaybackParams(p.getPlaybackParams().setSpeed(rate));
        }
        removeCallbacks(progress);
        post(progress);
        report(Protocol.EVENT_VIDEO_PLAY, "", 0, 0, 0, 0, false);
        followPlaying();
    }

    private void abandonFocus() {
        if (focusRequest == null || !hasFocus) return;
        hasFocus = false;
        AudioManager manager = (AudioManager) getContext().getSystemService(Context.AUDIO_SERVICE);
        if (manager != null) manager.abandonAudioFocusRequest(focusRequest);
    }

    private void progress() {
        MediaPlayer p = player;
        if (p == null || !prepared || !p.isPlaying()) return;
        report(Protocol.EVENT_PROGRESS, "", p.getCurrentPosition(), p.getDuration(), 0, 0, false);
        postDelayed(progress, Math.max(16, progressInterval));
    }

    void seek(long millis) {
        MediaPlayer p = player;
        if (p == null || !prepared) return;
        millis = Math.max(0, millis);
        boolean replay = completed && millis < p.getDuration();
        if (replay) {
            // Leave PlaybackCompleted before seeking: start() in that state restarts at zero instead of resuming at the requested time.
            p.pause();
            completed = false;
            // The seek is asynchronous, and a start before it lands plays on from the end, which ends again at once: the seek's completion applies paused.
            replaying = true;
        }
        p.seekTo(millis, MediaPlayer.SEEK_CLOSEST);
    }

    /** Scales the picture inside the view by its resize mode. */
    private void fit() {
        int w = getWidth(), h = getHeight();
        if (w == 0 || h == 0 || videoWidth == 0 || videoHeight == 0) return;
        Matrix m = new Matrix();
        if (resizeMode != Protocol.RESIZE_STRETCH) {
            // The texture stretches the picture to the view: undo, then fit.
            float sx = (float) videoWidth / w, sy = (float) videoHeight / h;
            float s = resizeMode == Protocol.RESIZE_COVER
                    ? Math.max((float) w / videoWidth, (float) h / videoHeight)
                    : Math.min((float) w / videoWidth, (float) h / videoHeight);
            m.setScale(sx * s, sy * s, w / 2f, h / 2f);
        }
        setTransform(m);
        invalidate();
    }

    @Override
    protected void onSizeChanged(int w, int h, int oldw, int oldh) {
        super.onSizeChanged(w, h, oldw, oldh);
        fit();
    }

    @SuppressLint("ClickableViewAccessibility")
    @Override
    public boolean onTouchEvent(MotionEvent e) {
        if (controller != null && e.getActionMasked() == MotionEvent.ACTION_UP) {
            if (controller.isShowing()) controller.hide(); else controller.show();
            return true;
        }
        // Without controls the node's view takes the touch, for its presses.
        return controller != null && e.getActionMasked() == MotionEvent.ACTION_DOWN;
    }

    private void release() {
        removeCallbacks(progress);
        prepared = false;
        completed = replaying = false;
        if (player != null) {
            player.release();
            player = null;
        }
        held = false;
        resumable = false;
        followPlaying();
        abandonFocus();
    }

    /** Stops the player and frees it and the surface. */
    void destroy() {
        if (controller != null) controller.hide();
        release();
        if (surface != null) {
            surface.release();
            surface = null;
        }
    }

    // TextureView.SurfaceTextureListener

    @Override
    public void onSurfaceTextureAvailable(SurfaceTexture st, int width, int height) {
        surface = new Surface(st);
        if (player != null) player.setSurface(surface);
        fit();
    }

    @Override
    public void onSurfaceTextureSizeChanged(SurfaceTexture st, int width, int height) {
        fit();
    }

    @Override
    public boolean onSurfaceTextureDestroyed(SurfaceTexture st) {
        if (player != null) player.setSurface(null);
        if (surface != null) {
            surface.release();
            surface = null;
        }
        return true;
    }

    @Override
    public void onSurfaceTextureUpdated(SurfaceTexture st) {}

    // MediaController.MediaPlayerControl: the controller plays and pauses the player directly, which holds until the app changes paused and is reported as the native controls are on Apple platforms.

    @Override
    public void start() {
        platformPlay();
    }

    @Override
    public void pause() {
        platformPause(Protocol.VIDEO_PAUSE_CONTROLS, false);
    }

    @Override
    public int getDuration() {
        return player != null && prepared ? player.getDuration() : 0;
    }

    @Override
    public int getCurrentPosition() {
        return player != null && prepared ? player.getCurrentPosition() : 0;
    }

    @Override
    public void seekTo(int pos) {
        seek(pos);
    }

    @Override
    public boolean isPlaying() {
        return player != null && prepared && player.isPlaying();
    }

    @Override
    public int getBufferPercentage() {
        return 0;
    }

    @Override
    public boolean canPause() {
        return true;
    }

    @Override
    public boolean canSeekBackward() {
        return true;
    }

    @Override
    public boolean canSeekForward() {
        return true;
    }

    @Override
    public int getAudioSessionId() {
        return player != null ? player.getAudioSessionId() : 0;
    }
}
