package android.app;

import android.content.Context;
import android.content.Intent;
import java.util.ArrayList;
import java.util.List;

public class Activity extends Context {
    public boolean finishing;
    public boolean destroyed;
    public final List<String> launchedActions = new ArrayList<>();
    public final List<Integer> launchedCodes = new ArrayList<>();

    public boolean isFinishing() {
        return finishing;
    }

    public boolean isDestroyed() {
        return destroyed;
    }

    public void runOnUiThread(Runnable action) {
        action.run();
    }

    public void startActivityForResult(Intent intent, int requestCode) {
        launchedActions.add(intent.action);
        launchedCodes.add(requestCode);
    }
}
