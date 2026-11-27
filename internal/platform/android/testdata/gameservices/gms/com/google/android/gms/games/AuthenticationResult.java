package com.google.android.gms.games;

public final class AuthenticationResult {
    private final boolean authenticated;

    public AuthenticationResult(boolean authenticated) {
        this.authenticated = authenticated;
    }

    public boolean isAuthenticated() {
        return authenticated;
    }
}
