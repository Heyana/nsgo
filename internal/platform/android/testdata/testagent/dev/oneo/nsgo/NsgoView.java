package dev.oneo.nsgo;

import android.content.Context;
import android.util.SparseArray;
import android.view.ViewGroup;
import java.util.ArrayList;
import java.util.List;

final class NsgoView extends ViewGroup {
    final SparseArray<TestAgent.Alert> alerts = new SparseArray<>();
    final CanvasImages canvasImages = new CanvasImages();
    final List<String> log = new ArrayList<>();
    String lastReplyData = "";
    String lastReplyErr = "";

    NsgoView(Context c) {
        super(c);
    }

    float density() { return 1f; }
    void back() {}
    void selectTestTab(int id, int index) {}

    void testReply(int id, String err, String data) {
        lastReplyErr = err;
        lastReplyData = data;
        log.add("reply:" + id + ":" + err + ":" + data);
    }
}
