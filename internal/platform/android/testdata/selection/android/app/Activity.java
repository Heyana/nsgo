package android.app;

import android.content.Context;
import android.content.ContextWrapper;

public class Activity extends ContextWrapper {
    public Activity(Context base) {
        super(base);
    }
}
