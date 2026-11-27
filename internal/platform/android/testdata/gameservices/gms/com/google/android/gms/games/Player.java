package com.google.android.gms.games;

public final class Player {
    private final String playerId;
    private final String displayName;

    public Player(String playerId, String displayName) {
        this.playerId = playerId;
        this.displayName = displayName;
    }

    public String getPlayerId() {
        return playerId;
    }

    public String getDisplayName() {
        return displayName;
    }
}
