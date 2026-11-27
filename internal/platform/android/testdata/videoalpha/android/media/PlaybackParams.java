package android.media;

public class PlaybackParams {
    private float speed = 1.0f;
    public float getSpeed() { return speed; }
    public PlaybackParams setSpeed(float speed) { this.speed = speed; return this; }
}
