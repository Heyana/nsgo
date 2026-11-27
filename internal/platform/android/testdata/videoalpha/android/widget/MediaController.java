package android.widget;

import android.content.Context;
import android.view.View;

public class MediaController {
    public interface MediaPlayerControl {
        void start();
        void pause();
        int getDuration();
        int getCurrentPosition();
        void seekTo(int pos);
        boolean isPlaying();
        int getBufferPercentage();
        boolean canPause();
        boolean canSeekBackward();
        boolean canSeekForward();
        int getAudioSessionId();
    }

    private boolean showing;

    public MediaController(Context context) {}
    public void setMediaPlayer(MediaPlayerControl player) {}
    public void setAnchorView(View view) {}
    public boolean isShowing() { return showing; }
    public void show() { showing = true; }
    public void hide() { showing = false; }
}
