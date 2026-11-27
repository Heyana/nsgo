package com.google.android.gms.games;

import android.content.Intent;
import com.google.android.gms.tasks.Task;
import java.util.ArrayList;
import java.util.List;

public final class AchievementsClient {
    public final List<String> unlocked = new ArrayList<>();
    public final List<String> stepsCalls = new ArrayList<>();
    public Task<Void> unlockTask = Task.forResult(null);
    public Task<Boolean> setStepsTask = Task.forResult(Boolean.FALSE);
    public Task<Intent> intentTask = Task.forResult(new Intent("play.games.ACHIEVEMENTS"));

    public Task<Void> unlockImmediate(String id) {
        unlocked.add(id);
        return unlockTask;
    }

    public Task<Boolean> setStepsImmediate(String id, int numSteps) {
        stepsCalls.add(id + ":" + numSteps);
        return setStepsTask;
    }

    public Task<Intent> getAchievementsIntent() {
        return intentTask;
    }
}
