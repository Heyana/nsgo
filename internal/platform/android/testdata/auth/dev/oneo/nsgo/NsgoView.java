package dev.oneo.nsgo;
import android.app.Activity;
import java.util.ArrayList;
import java.util.List;
class NsgoView {
    final Activity owner = new Activity();
    record Reply(long id, int phase, String text, String data) {}
    final List<Reply> replies = new ArrayList<>();
    Activity activity() { return owner; }
    void authSessionEvent(long id, int phase, String text, String data) {
        replies.add(new Reply(id, phase, text, data));
    }
}
