package dev.oneo.nsgo;

import android.content.Intent;
import android.os.Handler;
import java.io.DataInputStream;
import java.net.URI;

/** Runs the production session/reader against controlled lifecycle and browser callbacks. */
public class AuthSessionCheck {
    private static void require(boolean value, String message) {
        if (!value) throw new AssertionError(message);
    }

    private static void begin(NsgoView view, long id) {
        AuthSession.handle(view, id, Protocol.AUTH_START, "dev.nsgo.fixture", "", "/oauth", false, "");
    }

    private static void open(NsgoView view, long id) {
        AuthSession.handle(view, id, Protocol.AUTH_OPEN, "", "", "", false, "https://provider.invalid/auth");
    }

    private static NsgoView.Reply last(NsgoView view) {
        return view.replies.get(view.replies.size() - 1);
    }

    public static void main(String[] args) throws Exception {
        NsgoView wire = new NsgoView();
        DataInputStream input = new DataInputStream(System.in);
        require(input.readUnsignedByte() == Protocol.AUTH_REDIRECT && input.readUnsignedByte() == Protocol.AUTH_ENDED,
                "Java reply phases differ from the Go driver");
        AuthSession.read(wire, input);
        require(last(wire).id() == 0x80000010L && last(wire).phase() == Protocol.AUTH_REDIRECT,
                "the Go request lost its unsigned ID or phase");
        require(last(wire).text().equals("dev.nsgo.fixture:/%E5%9B%9E%E8%B0%83"),
                "the Go callback path was not escaped and normalized");
        AuthSession.read(wire, input);
        Intent tab = wire.owner.launched.get(0);
        require(tab.extras.containsKey("android.support.customtabs.extra.SESSION"), "not a Custom Tab intent");
        require(Boolean.TRUE.equals(tab.extras.get("androidx.browser.customtabs.extra.ENABLE_EPHEMERAL_BROWSING")),
                "the ephemeral request was lost");
        String callback = "dev.nsgo.fixture:/%E5%9B%9E%E8%B0%83?code=a%2Bb&state=owned#original";
        require(!AuthSession.redirect(wire, "dev.nsgo.fixture://foreign/%E5%9B%9E%E8%B0%83?code=bad"), "foreign authority consumed");
        require(!AuthSession.redirect(wire, "dev.nsgo.fixture:/different?code=bad"), "wrong path consumed");
        require(AuthSession.redirect(wire, callback), "matching callback not consumed");
        require(last(wire).data().equals(callback) && wire.replies.size() == 2, "callback changed or completed twice");
        require(wire.owner.application.callbacks.isEmpty(), "successful callback kept its Activity");
        AuthSession.read(wire, input);
        require(wire.owner.application.callbacks.size() == 1, "the second Go start was not prepared");
        AuthSession.read(wire, input);
        require(wire.owner.application.callbacks.isEmpty() && wire.replies.size() == 3,
                "the Go cancellation command was ignored or answered twice");
        require(input.read() == -1, "the production reader did not consume all Go fields");

        NsgoView literal = new NsgoView();
        AuthSession.handle(literal, 17, Protocol.AUTH_START, "DEV.NSGO.FIXTURE", "", "/percent%/old/%2e/callback", false, "");
        require(last(literal).text().equals("dev.nsgo.fixture:/percent%25/old/%252e/callback"),
                "literal percent or encoded-dot text was interpreted as a path escape");
        AuthSession.dispose(literal);

        URI destination = new URI("dev.app:/a%2Fb/%C3%A9");
        require(!AuthSession.matches(destination, null), "null link matched");
        require(AuthSession.matches(destination, "DEV.APP:/a%2fb/%c3%a9?state=x#fragment"), "URI escape/scheme case mismatch");
        require(!AuthSession.matches(destination, "dev.app:/a/b/%C3%A9"), "encoded slash matched another path");
        require(!AuthSession.matches(destination, "other:/a%2Fb/%C3%A9"), "other scheme matched");
        require(!AuthSession.matches(destination, "dev.app:opaque"), "opaque URI matched");
        require(!AuthSession.matches(destination, "dev.app:/broken%"), "invalid URI matched");

        NsgoView race = new NsgoView();
        begin(race, 1); open(race, 1);
        race.owner.application.pause(race.owner);
        race.owner.application.resume(race.owner);
        require(AuthSession.redirect(race, "dev.nsgo.fixture:/oauth?code=winner"), "resume won before its callback");
        Handler.drain();
        require(race.replies.size() == 2 && last(race).text().isEmpty(), "late resume cancelled a successful callback");

        NsgoView closed = new NsgoView();
        begin(closed, 2); open(closed, 2);
        closed.owner.application.pause(closed.owner);
        closed.owner.application.resume(closed.owner);
        Handler.drain();
        closed.owner.application.resume(closed.owner);
        Handler.drain();
        require(closed.replies.size() == 2 && last(closed).text().equals("cancelled"), "browser return did not cancel exactly once");
        require(closed.owner.application.callbacks.isEmpty(), "browser return retained lifecycle callbacks");

        NsgoView cancelled = new NsgoView();
        begin(cancelled, 3);
        AuthSession.handle(cancelled, 3, Protocol.AUTH_CANCEL, "", "", "", false, "");
        require(cancelled.replies.size() == 1 && cancelled.owner.launched.isEmpty(), "cancel before open launched or answered twice");
        require(cancelled.owner.application.callbacks.isEmpty(), "prepared cancellation retained its Activity");

        begin(cancelled, 4); open(cancelled, 4);
        cancelled.owner.application.pause(cancelled.owner);
        AuthSession.handle(cancelled, 4, Protocol.AUTH_CANCEL, "", "", "", false, "");
        Intent back = cancelled.owner.launched.get(cancelled.owner.launched.size() - 1);
        require(back.target == cancelled.owner.getClass() && back.flags == (Intent.FLAG_ACTIVITY_CLEAR_TOP | Intent.FLAG_ACTIVITY_SINGLE_TOP),
                "explicit cancellation did not bring only its Activity back");
        require(cancelled.replies.size() == 2, "native cancel duplicated the core answer");

        NsgoView first = new NsgoView(), second = new NsgoView();
        begin(first, 7); begin(second, 7);
        require(last(first).text().equals("cancelled") && first.owner.application.callbacks.isEmpty(), "replacement left the previous waiter alive");
        AuthSession.handle(first, 7, Protocol.AUTH_CANCEL, "", "", "", false, "");
        AuthSession.dispose(first);
        open(second, 7);
        require(!AuthSession.redirect(first, "dev.nsgo.fixture:/oauth?code=wrong-activity"), "another Activity stole a callback");
        require(AuthSession.redirect(second, "dev.nsgo.fixture:/oauth?code=second"), "stale cancellation stopped another view");

        NsgoView disposed = new NsgoView();
        begin(disposed, 8); open(disposed, 8);
        disposed.owner.application.pause(disposed.owner);
        disposed.owner.application.resume(disposed.owner);
        AuthSession.dispose(disposed);
        Handler.drain();
        require(disposed.replies.size() == 1 && disposed.owner.application.callbacks.isEmpty(), "disposed engine received a late cancellation");

        NsgoView unsupported = new NsgoView();
        for (String scheme : new String[] {"", "http", "https"}) {
            AuthSession.handle(unsupported, 9, Protocol.AUTH_START, scheme, "", "/oauth", false, "");
            require(last(unsupported).text().equals("unavailable"), "unsupported redirect did not report unavailable");
        }
        unsupported.owner.packages.claimed = false;
        begin(unsupported, 10);
        require(last(unsupported).text().equals("unavailable") && unsupported.owner.application.callbacks.isEmpty(), "unclaimed callback was prepared");

        NsgoView fallback = new NsgoView();
        fallback.owner.noCustomTabs = true;
        begin(fallback, 11); open(fallback, 11);
        require(fallback.owner.launched.size() == 1 && fallback.owner.launched.get(0).extras.isEmpty(), "plain browser fallback failed");
        AuthSession.dispose(fallback);

        NsgoView unavailable = new NsgoView();
        unavailable.owner.noBrowser = true;
        begin(unavailable, 12); open(unavailable, 12);
        require(last(unavailable).text().equals("unavailable") && unavailable.owner.application.callbacks.isEmpty(), "no-browser path hung");
        NsgoView denied = new NsgoView();
        denied.owner.denied = true;
        begin(denied, 13); open(denied, 13);
        require(last(denied).text().equals("failed") && denied.owner.application.callbacks.isEmpty(), "browser refusal leaked the session");

        NsgoView destroyed = new NsgoView();
        begin(destroyed, 14); open(destroyed, 14);
        destroyed.owner.application.destroy(destroyed.owner);
        require(last(destroyed).text().equals("cancelled") && destroyed.owner.application.callbacks.isEmpty(), "destroyed Activity kept its session");
        destroyed.owner.destroyed = true;
        begin(destroyed, 15);
        require(last(destroyed).text().equals("unavailable"), "destroyed Activity prepared a session");
        System.out.println("ok");
    }
}
