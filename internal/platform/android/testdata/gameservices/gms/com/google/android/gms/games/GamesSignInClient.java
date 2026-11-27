package com.google.android.gms.games;

import com.google.android.gms.tasks.Task;

public final class GamesSignInClient {
    public Task<AuthenticationResult> isAuthenticatedTask = Task.forResult(new AuthenticationResult(false));
    public Task<AuthenticationResult> signInTask = Task.forResult(new AuthenticationResult(true));
    public int isAuthenticatedCalls;
    public int signInCalls;

    public Task<AuthenticationResult> isAuthenticated() {
        isAuthenticatedCalls++;
        return isAuthenticatedTask;
    }

    public Task<AuthenticationResult> signIn() {
        signInCalls++;
        return signInTask;
    }
}
