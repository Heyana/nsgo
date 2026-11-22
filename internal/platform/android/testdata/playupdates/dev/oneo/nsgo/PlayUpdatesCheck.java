package dev.oneo.nsgo;

import android.app.Activity;
import java.util.function.Consumer;

public final class PlayUpdatesCheck {
    static final class Client implements PlayUpdates.Client {
        Consumer<PlayUpdates.Info> success;
        Consumer<String> failure;
        int creates, launches;
        boolean launch = true;
        public Object create(Activity activity) { creates++; return this; }
        public void info(Object manager, Consumer<PlayUpdates.Info> success, Consumer<String> failure) {
            this.success = success; this.failure = failure;
        }
        public boolean launch(Object manager, PlayUpdates.Info info, Activity activity) { launches++; return launch; }
        void answer(int status, boolean allowed) { success.accept(new PlayUpdates.Info(this, status, allowed)); }
    }
    static final class Reply implements PlayUpdates.Reply {
        int count; boolean shown; String error;
        public void done(boolean shown, String error) { count++; this.shown = shown; this.error = error; }
    }
    static void require(boolean condition, String message) { if (!condition) throw new AssertionError(message); }

    public static void main(String[] args) {
        Activity activity = new Activity();
        Client client = new Client();
        PlayUpdates updates = new PlayUpdates(client);
        Reply available = new Reply();
        updates.check(activity, available);
        Reply concurrent = new Reply();
        updates.check(activity, concurrent);
        require(concurrent.count == 1 && concurrent.shown && client.creates == 1, "duplicate request started a second query");
        client.answer(PlayUpdates.AVAILABLE, true);
        require(client.launches == 1 && available.shown && available.error.isEmpty(), "available update did not enter Play UI");
        client.answer(PlayUpdates.AVAILABLE, true);
        require(available.count == 1 && client.launches == 1, "duplicate SDK callback replayed consent");

        updates.resume(activity);
        client.answer(PlayUpdates.IN_PROGRESS, true);
        require(client.launches == 2, "consented update did not resume");
        updates.activityResult(PlayUpdates.REQUEST);
        updates.resume(activity);
        require(client.launches == 2, "cancellation reopened consent");

        Reply none = new Reply();
        updates.check(activity, none); client.answer(1, true);
        require(!none.shown && none.error.isEmpty() && client.launches == 2, "absent update launched UI");
        Reply forbidden = new Reply();
        updates.check(activity, forbidden); client.answer(PlayUpdates.AVAILABLE, false);
        require(!forbidden.shown && forbidden.error.isEmpty() && client.launches == 2, "disallowed immediate update launched UI");
        Reply failed = new Reply();
        updates.check(activity, failed); client.failure.accept("not owned");
        require(failed.count == 1 && failed.error.equals("not owned"), "SDK failure was hidden");
        Reply background = new Reply();
        updates.check(activity, background); activity.focused = false; client.answer(PlayUpdates.AVAILABLE, true);
        require(!background.shown && !background.error.isEmpty() && client.launches == 2, "background activity launched consent");
        activity.focused = true;
        Reply launchFailure = new Reply();
        client.launch = false; updates.check(activity, launchFailure); client.answer(PlayUpdates.AVAILABLE, true);
        require(!launchFailure.shown && !launchFailure.error.isEmpty(), "declined launch was reported shown");
        Reply missing = new Reply();
        new PlayUpdates(null).check(activity, missing);
        require(missing.count == 1 && !missing.error.isEmpty(), "missing host SDK was ignored");
        Reply dead = new Reply();
        activity.destroyed = true; updates.check(activity, dead);
        require(dead.count == 1 && !dead.error.isEmpty(), "destroyed activity accepted a query");
        require(client.creates == 1, "manager was not retained for the process view");
        Client late = new Client();
        PlayUpdates racing = new PlayUpdates(late);
        racing.check(new Activity(), new Reply()); late.answer(PlayUpdates.AVAILABLE, true);
        racing.resume(new Activity());
        racing.activityResult(PlayUpdates.REQUEST); // Cancel while the resume query is still pending.
        late.answer(PlayUpdates.IN_PROGRESS, true);
        require(late.launches == 1, "a stale resume callback reopened cancelled consent");
    }
}
