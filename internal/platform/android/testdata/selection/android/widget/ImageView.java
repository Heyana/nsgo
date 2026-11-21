package android.widget;

import android.content.Context;
import android.graphics.drawable.Drawable;
import android.view.View;

public final class ImageView extends View {
    private Drawable drawable;

    public ImageView(Context context) {
        super(context, 0, 0);
    }

    public Drawable getDrawable() { return drawable; }

    public void setImageDrawable(Drawable drawable) { this.drawable = drawable; }
}
