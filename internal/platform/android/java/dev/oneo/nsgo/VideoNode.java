package dev.oneo.nsgo;

import android.annotation.SuppressLint;
import android.content.Context;
import android.content.res.AssetFileDescriptor;
import android.graphics.Matrix;
import android.graphics.SurfaceTexture;
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

        void report(int kind, String text, long time, long duration, int width, int height, boolean buffering);
    }

    private final Reporter reporter;
    private MediaPlayer player;
    private Surface surface;
    private String source;
    private boolean prepared;
    private boolean paused, muted, repeat;
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
    private final Runnable progress = this::progress;

    VideoNode(Context context, Reporter reporter) {
        super(context);
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
        paused = Protocol.flag(flags, 0);
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
        if (!Objects.equals(source, this.source) || reopen) {
            this.source = source;
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
            report(Protocol.EVENT_END, "", 0, 0, 0, 0, false);
            if (repeat) {
                mp.seekTo(0);
                mp.start();
            }
            updateFocus();
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
        // A stream without a duration, such as a live one, has -1.
        if (reporter.wants(kind)) {
            reporter.report(kind, text, Math.max(0, time), Math.max(0, duration), width, height, buffering);
        }
    }

    /**
     * Points a player at a URL, a file or an "asset:" path in the APK's
     * assets, where packaging puts the app's resources.
     */
    private void setMediaSource(MediaPlayer player, String uri) throws IOException {
        Context context = getContext();
        if (!uri.startsWith("asset:")) {
            player.setDataSource(context, Uri.parse(uri));
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
        if (paused) {
            if (p.isPlaying()) p.pause();
        } else {
            // Setting a speed starts playback, so only while playing.
            if (!p.isPlaying()) p.start();
            if (rate > 0 && p.getPlaybackParams().getSpeed() != rate) {
                p.setPlaybackParams(p.getPlaybackParams().setSpeed(rate));
            }
        }
        removeCallbacks(progress);
        if (!paused) postDelayed(progress, progressInterval);
        updateFocus();
    }

    /** The sound of a playback or mix-with-others video is speech; a default one keeps the player's own. */
    private AudioAttributes audioAttributes() {
        if (audioSession == 0) return null;
        return new AudioAttributes.Builder()
                .setUsage(AudioAttributes.USAGE_MEDIA)
                .setContentType(AudioAttributes.CONTENT_TYPE_SPEECH)
                .build();
    }

    /**
     * Holds audio focus while a playback video plays, and only then: other apps pause while it plays and
     * resume once it pauses, ends or goes. Speech pauses rather than ducks when another app takes the focus,
     * and plays on when it comes back.
     */
    private void updateFocus() {
        MediaPlayer p = player;
        boolean wanted = audioSession == 1 && p != null && prepared && p.isPlaying();
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
        MediaPlayer p = player;
        if (change == AudioManager.AUDIOFOCUS_GAIN) {
            hasFocus = true;
            if (p != null && prepared && !paused && !p.isPlaying()) p.start();
        } else if (change == AudioManager.AUDIOFOCUS_LOSS) {
            // Gone for good: the app plays it again by its props.
            hasFocus = false;
            if (p != null && prepared && p.isPlaying()) p.pause();
        } else if (p != null && prepared && p.isPlaying()) {
            // Lost for a while (a call, a navigation prompt): back on gain.
            p.pause();
        }
    }

    private void abandonFocus() {
        if (focusRequest == null || !hasFocus) return;
        hasFocus = false;
        AudioManager manager = (AudioManager) getContext().getSystemService(Context.AUDIO_SERVICE);
        if (manager != null) manager.abandonAudioFocusRequest(focusRequest);
    }

    private void progress() {
        MediaPlayer p = player;
        if (p == null || !prepared || paused) return;
        report(Protocol.EVENT_PROGRESS, "", p.getCurrentPosition(), p.getDuration(), 0, 0, false);
        postDelayed(progress, Math.max(16, progressInterval));
    }

    void seek(long millis) {
        if (player != null && prepared) player.seekTo(millis, MediaPlayer.SEEK_CLOSEST);
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
        if (player != null) {
            player.release();
            player = null;
        }
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

    // MediaController.MediaPlayerControl: the controller plays and pauses
    // the player directly; the app learns of it only through progress, as
    // with the native controls on Apple platforms.

    @Override
    public void start() {
        if (player != null && prepared) {
            player.start();
            paused = false;
            removeCallbacks(progress);
            post(progress);
            updateFocus();
        }
    }

    @Override
    public void pause() {
        if (player != null && prepared) player.pause();
        paused = true;
        updateFocus();
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
        if (player != null && prepared) player.seekTo(pos);
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
