package dev.oneo.nsgo;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;

/** Delivers a Live Update action to the engine that posted it, while it runs. */
public final class LiveUpdateReceiver extends BroadcastReceiver {
    @Override
    public void onReceive(Context context, Intent intent) {
        if (intent == null) return;
        NsgoEngine.deliverLiveAction(intent.getStringExtra(LiveUpdates.EXTRA_ENGINE), LiveUpdates.press(intent));
        // A receiver must not launch an Activity as a notification trampoline. Stale presses are dropped when the posting engine has ended.
    }
}
