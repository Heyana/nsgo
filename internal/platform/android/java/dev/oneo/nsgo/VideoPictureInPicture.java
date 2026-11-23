package dev.oneo.nsgo;

import android.app.Activity;
import android.app.PictureInPictureParams;
import android.content.Intent;
import android.content.pm.ActivityInfo;
import android.content.pm.PackageManager;
import android.graphics.Color;
import android.graphics.Rect;
import android.media.MediaMetadata;
import android.media.session.MediaSession;
import android.media.session.PlaybackState;
import android.os.Build;
import android.util.Rational;
import android.view.View;
import android.view.ViewGroup;
import android.widget.FrameLayout;
import java.util.LinkedHashMap;
import java.util.IdentityHashMap;
import org.json.JSONException;
import org.json.JSONObject;

/** One Activity's video PiP session. The player stays with its view; only
 * its TextureView moves into a full-window host while Android shows PiP.
 * All methods and MediaSession callbacks run on the UI thread. */
final class VideoPictureInPicture {
    static final String EVENT = "nsgo:picture-in-picture";
    private final NsgoView host;
    private final LinkedHashMap<VideoNode, String> videos = new LinkedHashMap<>();
    private static final class Failure {
        final long generation;
        final String message;
        Failure(VideoNode video, String message) { this.generation = video.pictureInPictureGeneration(); this.message = message; }
    }
    private final IdentityHashMap<VideoNode, Failure> failures = new IdentityHashMap<>();
    private VideoNode owner;
    private boolean active, starting, stopping;
    private FrameLayout presentation;
    private ViewGroup inline;
    private ViewGroup.LayoutParams inlineParams;
    private int inlineIndex;
    private Rect sourceRect;
    private MediaSession session;

    VideoPictureInPicture(NsgoView host) { this.host = host; }

    private boolean supported() {
        Activity activity = host.activity();
        if (activity == null || activity.isFinishing() || activity.isDestroyed()
                || !activity.getPackageManager().hasSystemFeature(PackageManager.FEATURE_PICTURE_IN_PICTURE)) return false;
        try {
            ActivityInfo info = activity.getPackageManager().getActivityInfo(activity.getComponentName(), 0);
            return (info.flags & 0x400000 /* ActivityInfo’s manifest PiP bit, retained in flags but hidden from the SDK. */) != 0;
        } catch (PackageManager.NameNotFoundException e) {
            return false;
        }
    }

    private boolean possible(VideoNode video) {
        return supported() && video.pictureInPictureEnabled() && video.pictureInPictureReady()
                && (owner == video || owner == null && host.isAttachedToWindow() && host.isShown()
                        && host.activity().hasWindowFocus() && !host.activity().isInPictureInPictureMode());
    }

    void changed(VideoNode video) {
        if (video.pictureInPictureEnabled()) {
            if (!videos.containsKey(video)) videos.put(video, "");
        } else {
            videos.remove(video);
            failures.remove(video);
            if (owner == video) stop(video);
        }
        updateAutomatic();
        updateSession();
        publish(video, "");
    }

    private VideoNode automatic() {
        VideoNode next = null;
        for (VideoNode video : videos.keySet()) {
            if (video.automaticPictureInPicture() && video.isPlaying() && video.pictureInPictureReady()
                    && supported() && (owner == null || owner == video)) next = video;
        }
        return next;
    }

    private PictureInPictureParams params(VideoNode video, boolean automatic) {
        PictureInPictureParams.Builder builder = new PictureInPictureParams.Builder();
        if (video != null && video.pictureWidth() > 0 && video.pictureHeight() > 0) {
            double ratio = (double) video.pictureWidth() / video.pictureHeight();
            builder.setAspectRatio(new Rational((int) Math.round(Math.max(1 / 2.39, Math.min(2.39, ratio)) * 10000), 10000));
            Rect rect = owner == video && sourceRect != null ? sourceRect : new Rect();
            if (rect == sourceRect || video.getGlobalVisibleRect(rect)) builder.setSourceRectHint(rect);
        }
        if (Build.VERSION.SDK_INT >= 31) {
            builder.setAutoEnterEnabled(automatic);
            builder.setSeamlessResizeEnabled(true);
        }
        return builder.build();
    }

    private void updateAutomatic() {
        if (!supported()) return;
        VideoNode next = owner != null ? owner : automatic();
        try {
            host.activity().setPictureInPictureParams(params(next, !stopping && next != null
                    && next.automaticPictureInPicture() && next.isPlaying()));
        } catch (IllegalArgumentException | IllegalStateException ignored) {
            // A state change while finishing cannot enter PiP. A requested
            // start has its own explicit error path below.
        }
    }

    void command(VideoNode video, Object value) {
        if (!(value instanceof JSONObject)) return;
        int command = ((JSONObject) value).optInt("command");
        if (command == 1) start(video);
        else if (command == 2) stop(video);
        // Restoration acknowledgement has no native completion block on Android.
    }

    private void start(VideoNode video) {
        if (owner == video && (active || starting)) return;
        failures.remove(video);
        if (!supported()) { publish(video, ""); return; }
        if (!possible(video)) {
            publish(video, owner != null ? "another video owns Picture in Picture" : "the video is not ready for Picture in Picture");
            return;
        }
        prepare(video);
        try {
            if (!host.activity().enterPictureInPictureMode(params(video, video.automaticPictureInPicture() && video.isPlaying()))) {
                finish(false);
                publish(video, "Android refused to enter Picture in Picture");
            }
        } catch (IllegalArgumentException | IllegalStateException | SecurityException e) {
            finish(false);
            publish(video, "could not enter Picture in Picture: " + e.getMessage());
        }
    }

    private void prepare(VideoNode video) {
        if (owner != null) return;
        owner = video;
        starting = true;
        sourceRect = new Rect();
        video.getGlobalVisibleRect(sourceRect);
        inline = (ViewGroup) video.getParent();
        inlineParams = video.getLayoutParams();
        inlineIndex = inline.indexOfChild(video);
        presentation = new FrameLayout(host.getContext());
        presentation.setBackgroundColor(Color.BLACK);
        host.addView(presentation, new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
        video.hideControls();
        inline.removeView(video);
        presentation.addView(video, new FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
        updateSession();
        publishAll();
        host.postDelayed(() -> {
            Activity activity = host.activity();
            if (owner == video && starting && (activity == null || !activity.isInPictureInPictureMode())) {
                finish(false);
                publish(video, "Android did not enter Picture in Picture");
            }
        }, 3000);
    }

    private void stop(VideoNode video) {
        if (owner != video || stopping) return;
        Activity activity = host.activity();
        if (activity == null || !activity.isInPictureInPictureMode()) { finish(false); return; }
        stopping = true;
        updateAutomatic();
        // Android has no public exit-PiP method. Bringing the same Activity
        // to the front expands its existing task and preserves the player.
        try {
            activity.startActivity(new Intent(activity, activity.getClass())
                    .addFlags(Intent.FLAG_ACTIVITY_REORDER_TO_FRONT | Intent.FLAG_ACTIVITY_SINGLE_TOP));
            host.postDelayed(() -> {
                if (owner == video && stopping && activity.isInPictureInPictureMode()) {
                    stopping = false;
                    publish(video, "Android did not leave Picture in Picture");
                }
            }, 3000);
        } catch (RuntimeException e) {
            stopping = false;
            publish(video, "could not leave Picture in Picture: " + e.getMessage());
        }
    }

    boolean isActive() { return active || starting; }

    void windowChanged() {
        updateAutomatic();
        publishAll();
    }

    void mounted() {
        if (presentation != null) presentation.bringToFront();
    }

    void userLeaving() {
        if (Build.VERSION.SDK_INT < 31 && owner == null) {
            VideoNode video = automatic();
            if (video != null) start(video);
        }
    }

    void transitioning() {
        if (owner == null) {
            VideoNode video = automatic();
            if (video != null) prepare(video);
        }
    }

    void modeChanged(boolean nowActive) {
        if (nowActive) {
            if (owner == null) {
                VideoNode video = automatic();
                if (video != null) prepare(video);
            }
            if (owner != null) failures.remove(owner);
            active = owner != null;
            starting = false;
            updateSession();
            publishAll();
        } else {
            finish(true);
        }
    }

    private void finish(boolean restoring) {
        VideoNode video = owner;
        owner = null;
        active = starting = stopping = false;
        if (presentation != null) {
            if (video != null) {
                presentation.removeView(video);
                if (!video.disposed() && inline != null) inline.addView(video, Math.min(inlineIndex, inline.getChildCount()), inlineParams);
            }
            host.removeView(presentation);
        }
        presentation = null;
        inline = null;
        inlineParams = null;
        sourceRect = null;
        if (session != null) { session.release(); session = null; }
        updateAutomatic();
        publishAll();
        if (video != null && !videos.containsKey(video)) publish(video, "");
        if (restoring && video != null && !video.disposed()) host.post(() -> {
            Activity activity = host.activity();
            if (activity != null && !activity.isFinishing() && host.getWindowVisibility() == View.VISIBLE) {
                video.pictureInPictureState("{\"action\":\"restore\"}");
            }
        });
    }

    private void updateSession() {
        if (owner == null) return;
        if (session == null) {
            session = new MediaSession(host.getContext(), "NSGoVideo");
            session.setCallback(new MediaSession.Callback() {
                @Override public void onPlay() { if (owner != null) owner.start(); }
                @Override public void onPause() { if (owner != null) owner.pause(); }
                @Override public void onSeekTo(long position) { if (owner != null) owner.seek(position); }
            });
            session.setActive(true);
        }
        session.setMetadata(new MediaMetadata.Builder().putLong(MediaMetadata.METADATA_KEY_DURATION, owner.getDuration()).build());
        session.setPlaybackState(new PlaybackState.Builder()
                .setActions(PlaybackState.ACTION_PLAY | PlaybackState.ACTION_PAUSE | PlaybackState.ACTION_PLAY_PAUSE | PlaybackState.ACTION_SEEK_TO)
                .setState(owner.isPlaying() ? PlaybackState.STATE_PLAYING : PlaybackState.STATE_PAUSED, owner.getCurrentPosition(), 1).build());
    }

    private void publishAll() {
        for (VideoNode video : videos.keySet()) publish(video, "");
    }

    private void publish(VideoNode video, String error) {
        if (!error.isEmpty()) failures.put(video, new Failure(video, error));
        Failure failure = failures.get(video);
        if (failure != null && failure.generation != video.pictureInPictureGeneration()) {
            failures.remove(video);
            failure = null;
        }
        error = failure == null ? "" : failure.message;
        try {
            JSONObject state = new JSONObject().put("supported", supported()).put("possible", possible(video))
                    .put("active", owner == video && active).put("error", error);
            String json = new JSONObject().put("state", state).toString();
            if (json.equals(videos.get(video))) return;
            if (videos.containsKey(video)) videos.put(video, json);
            video.pictureInPictureState(json);
        } catch (JSONException impossible) {
            throw new AssertionError(impossible);
        }
    }
}
