package dev.oneo.nsgo;

import android.app.Activity;
import android.os.Bundle;

/** The launcher activity of a standalone NSGo app: one NsgoView filling the window. */
public class NsgoActivity extends Activity {
    private NsgoView view;

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        view = new NsgoView(this);
        setContentView(view);
    }

    @Override
    protected void onDestroy() {
        view.stop();
        super.onDestroy();
    }
}
