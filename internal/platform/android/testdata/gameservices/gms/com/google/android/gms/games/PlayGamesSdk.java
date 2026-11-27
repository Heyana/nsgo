package com.google.android.gms.games;

import android.content.Context;

public final class PlayGamesSdk {
    public static int initCount;

    public static void initialize(Context context) {
        initCount++;
    }
}
