package dev.oneo.nsgo;

import android.app.Activity;
import android.content.Context;
import java.util.ArrayList;
import java.util.List;

final class NsgoView {
    Activity act;
    Context ctx;
    final List<String> events = new ArrayList<>();

    NsgoView(Activity act) {
        this.act = act;
        this.ctx = act;
    }

    Context getContext() { return ctx; }
    Activity activity() { return act; }

    void answerSpeech(long count, int phase, String text, boolean finalResult, String error) {
        events.add(count + ":" + phase + ":" + text + ":" + finalResult + ":" + error);
    }
}
