package android.media;

import android.content.Context;
import android.content.res.AssetFileDescriptor;
import android.net.Uri;
import android.view.Surface;
import java.io.IOException;
import java.util.Map;

public class MediaPlayer {
    public static final int MEDIA_INFO_BUFFERING_START = 701;
    public static final int MEDIA_INFO_BUFFERING_END = 702;
    public static final int SEEK_CLOSEST = 3;

    public interface OnPreparedListener { void onPrepared(MediaPlayer mp); }
    public interface OnVideoSizeChangedListener { void onVideoSizeChanged(MediaPlayer mp, int w, int h); }
    public interface OnCompletionListener { void onCompletion(MediaPlayer mp); }
    public interface OnSeekCompleteListener { void onSeekComplete(MediaPlayer mp); }
    public interface OnInfoListener { boolean onInfo(MediaPlayer mp, int what, int extra); }
    public interface OnErrorListener { boolean onError(MediaPlayer mp, int what, int extra); }

    private final PlaybackParams params = new PlaybackParams();

    public void setOnPreparedListener(OnPreparedListener l) {}
    public void setOnVideoSizeChangedListener(OnVideoSizeChangedListener l) {}
    public void setOnCompletionListener(OnCompletionListener l) {}
    public void setOnSeekCompleteListener(OnSeekCompleteListener l) {}
    public void setOnInfoListener(OnInfoListener l) {}
    public void setOnErrorListener(OnErrorListener l) {}
    public void setSurface(Surface s) {}
    public void setAudioAttributes(AudioAttributes a) {}
    public void setDataSource(Context ctx, Uri uri) throws IOException {}
    public void setDataSource(Context ctx, Uri uri, Map<String, String> headers) throws IOException {}
    public void setDataSource(AssetFileDescriptor fd) throws IOException {}
    public void setDataSource(String path) throws IOException {}
    public void prepareAsync() {}
    public void start() {}
    public void pause() {}
    public void release() {}
    public void setVolume(float left, float right) {}
    public void seekTo(int msec) {}
    public void seekTo(long msec, int mode) {}
    public boolean isPlaying() { return false; }
    public int getDuration() { return 0; }
    public int getCurrentPosition() { return 0; }
    public int getVideoWidth() { return 0; }
    public int getVideoHeight() { return 0; }
    public int getAudioSessionId() { return 0; }
    public PlaybackParams getPlaybackParams() { return params; }
    public void setPlaybackParams(PlaybackParams p) {}
}
