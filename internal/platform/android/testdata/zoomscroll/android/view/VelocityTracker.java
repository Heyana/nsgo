package android.view;

public final class VelocityTracker {
    private float prevX, prevY, vx, vy;
    private long prevTime = -1;

    public static VelocityTracker obtain() {
        return new VelocityTracker();
    }

    public void addMovement(MotionEvent e) {
        if (e.getActionMasked() == MotionEvent.ACTION_UP || e.getActionMasked() == MotionEvent.ACTION_CANCEL) {
            return;
        }
        if (prevTime >= 0 && e.getEventTime() > prevTime) {
            float dt = (e.getEventTime() - prevTime) / 1000f;
            vx = (e.getX() - prevX) / dt;
            vy = (e.getY() - prevY) / dt;
        }
        prevX = e.getX();
        prevY = e.getY();
        prevTime = e.getEventTime();
    }

    public void computeCurrentVelocity(int units) {}

    public float getXVelocity() {
        return vx;
    }

    public float getYVelocity() {
        return vy;
    }

    public void recycle() {}
}
