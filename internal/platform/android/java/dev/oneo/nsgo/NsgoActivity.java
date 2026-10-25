package dev.oneo.nsgo;

import android.app.Activity;
import android.content.res.Configuration;
import android.graphics.Color;
import android.os.Build;
import android.os.Bundle;
import android.view.Window;
import android.view.WindowManager;

/**
 * The launcher activity of a standalone NSGo app: one NsgoView filling the
 * window edge to edge, behind the system bars it reports as the safe area.
 * Its manifest entry, as in AndroidManifest.xml, declares the
 * configuration changes it handles, or the system restarts the app on each.
 */
public class NsgoActivity extends Activity {
    private NsgoView view;

    @Override
    protected void onCreate(Bundle state) {
        // The manifest's theme is dark; web views take prefers-color-scheme
        // from the theme, so it follows the system's mode at launch.
        int night = getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK;
        if (night != Configuration.UI_MODE_NIGHT_YES) setTheme(android.R.style.Theme_DeviceDefault_Light_NoActionBar);
        super.onCreate(state);
        Window window = getWindow();
        WindowManager.LayoutParams lp = window.getAttributes();
        lp.layoutInDisplayCutoutMode = WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES;
        window.setAttributes(lp);
        // Edge to edge, this reports the keyboard in the insets rather
        // than resizing the view.
        window.setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE);
        window.setStatusBarColor(Color.TRANSPARENT);
        window.setNavigationBarColor(Color.TRANSPARENT);
        if (Build.VERSION.SDK_INT >= 30) window.setDecorFitsSystemWindows(false);
        // The view lays itself out behind the system bars and styles them.
        view = new NsgoView(this);
        view.setTest(getIntent().getStringExtra(TestAgent.EXTRA));
        setContentView(view);
    }

    @Override
    public void onBackPressed() {
        // Back arrives here before API 33, or without predictive back.
        if (!view.back()) super.onBackPressed();
    }

    @Override
    protected void onActivityResult(int request, int result, android.content.Intent data) {
        super.onActivityResult(request, result, data);
        view.activityResult(request, result, data);
    }

    @Override
    protected void onDestroy() {
        view.stop();
        super.onDestroy();
    }
}
