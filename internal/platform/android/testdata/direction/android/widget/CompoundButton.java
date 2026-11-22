package android.widget;

import android.content.Context;
import android.view.View;

public class CompoundButton extends View {
    public interface OnCheckedChangeListener {
        void onCheckedChanged(CompoundButton buttonView, boolean isChecked);
    }
    private boolean checked;
    private OnCheckedChangeListener listener;

    public CompoundButton(Context context) { super(context); }
    public void setGravity(int gravity) {}
    public void setButtonDrawable(Object d) {}
    public void setOnCheckedChangeListener(OnCheckedChangeListener l) { listener = l; }
    public boolean isChecked() { return checked; }
    public void setChecked(boolean checked) {
        this.checked = checked;
        if (listener != null) listener.onCheckedChanged(this, checked);
    }
    public void toggle() { setChecked(!checked); }
}
