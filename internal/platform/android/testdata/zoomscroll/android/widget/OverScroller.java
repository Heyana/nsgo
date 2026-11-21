package android.widget;

import android.content.Context;

public class OverScroller {
    private boolean finished = true;
    private int currX, currY, stepX, stepY, minX, maxX, minY, maxY, steps;

    public OverScroller(Context context) {}

    public void setFriction(float friction) {}

    public void forceFinished(boolean finished) {
        this.finished = finished;
        if (finished) steps = 0;
    }

    public boolean isFinished() {
        return finished;
    }

    public void fling(int startX, int startY, int velocityX, int velocityY, int minX, int maxX, int minY, int maxY) {
        this.currX = startX;
        this.currY = startY;
        this.minX = minX;
        this.maxX = maxX;
        this.minY = minY;
        this.maxY = maxY;
        this.stepX = velocityX / 60;
        this.stepY = velocityY / 60;
        boolean canX = (stepX > 0 && startX < maxX) || (stepX < 0 && startX > minX);
        boolean canY = (stepY > 0 && startY < maxY) || (stepY < 0 && startY > minY);
        if (canX || canY) {
            this.steps = 10;
            this.finished = false;
        } else {
            this.steps = 0;
            this.finished = true;
        }
    }

    public boolean computeScrollOffset() {
        if (finished || steps <= 0) {
            finished = true;
            return false;
        }
        steps--;
        currX = Math.min(Math.max(currX + stepX, minX), maxX);
        currY = Math.min(Math.max(currY + stepY, minY), maxY);
        if (steps == 0) finished = true;
        return true;
    }

    public int getCurrX() {
        return currX;
    }

    public int getCurrY() {
        return currY;
    }
}
