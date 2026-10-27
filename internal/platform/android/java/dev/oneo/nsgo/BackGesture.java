package dev.oneo.nsgo;

import android.window.BackEvent;
import android.window.OnBackAnimationCallback;

/**
 * Back on Android 14 and later, loaded only there: the screen back pops
 * follows the predictive back gesture (NsgoView.backStarted).
 */
final class BackGesture implements OnBackAnimationCallback {
    private final NsgoView view;

    BackGesture(NsgoView view) {
        this.view = view;
    }

    @Override
    public void onBackStarted(BackEvent event) {
        view.backStarted(event.getProgress());
    }

    @Override
    public void onBackProgressed(BackEvent event) {
        view.backProgressed(event.getProgress());
    }

    @Override
    public void onBackCancelled() {
        view.backCancelled();
    }

    @Override
    public void onBackInvoked() {
        view.backInvoked();
    }
}
