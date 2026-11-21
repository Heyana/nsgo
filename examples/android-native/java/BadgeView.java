package dev.nsgo.example;

import android.content.Context;
import android.view.Gravity;
import android.widget.TextView;

public final class BadgeView extends TextView {
    public BadgeView(Context context) {
        super(context);
        setGravity(Gravity.CENTER);
        setTextSize(20);
    }

    public void setCaption(String caption) {
        setText("Android view: " + caption);
    }
}
