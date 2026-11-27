package com.google.android.gms.games;

import com.google.android.gms.tasks.Task;

public final class PlayersClient {
    public Task<Player> currentPlayerTask = Task.forResult(new Player("p-ada", "Ada"));

    public Task<Player> getCurrentPlayer() {
        return currentPlayerTask;
    }
}
