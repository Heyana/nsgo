package dev.oneo.nsgo;

import java.util.ArrayList;
import java.util.List;

public final class HardwareKeysCheck {
    static final class Host implements HardwareKeys.Host {
        int target = 7, request;
        boolean held;
        Runnable timeout;
        final List<String> reports = new ArrayList<>(), passed = new ArrayList<>();
        public int target() { return target; }
        public boolean heldKeys() { return held; }
        public void send(int id, int node, HardwareKeys.Event e) {
            if (id != 0) request = id;
            reports.add(node + ":" + e.name + ":" + (e.down ? "down" : "up") + ":" + e.modifiers + ":" + e.repeat);
        }
        public void fallback(HardwareKeys.Event e) { passed.add(e.name + (e.down ? "+" : "-")); }
        public void schedule(Runnable action) { timeout = action; }
        public void unschedule(Runnable action) { if (timeout == action) timeout = null; }
    }

    static HardwareKeys.Event key(int code, String name, boolean down, boolean repeat) {
        return new HardwareKeys.Event(code, 0, code, name, 0, down, repeat, null);
    }

    static void check(boolean value, String why) { if (!value) throw new AssertionError(why); }

    public static void main(String[] args) {
        Host host = new Host();
        HardwareKeys keys = new HardwareKeys(host);
        check(keys.dispatch(key(1, "k", true, false)), "focused down was not captured");
        keys.dispatch(key(1, "k", false, false));
        check(host.passed.isEmpty() && host.reports.size() == 1, "release overtook consume answer");
        keys.reply(host.request, true);
        check(host.reports.equals(List.of("7:k:down:0:false", "7:k:up:0:false")), "consumed release did not follow down");
        check(host.passed.isEmpty(), "consumed key reached native fallback");

        host.reports.clear();
        keys.dispatch(key(2, "x", true, false));
        int first = host.request;
        keys.dispatch(key(2, "x", true, true));
        host.target = 9;
        keys.dispatch(key(2, "x", false, false));
        keys.reply(first, false);
        check(host.request != first, "repeat did not wait for initial answer");
        keys.reply(host.request, false);
        check(host.reports.equals(List.of("7:x:down:0:false", "7:x:down:0:true", "7:x:up:0:false")), "repeat or original release target changed");
        check(host.passed.equals(List.of("x+", "x+", "x-")), "unconsumed keys lost native actions");

        host.reports.clear(); host.passed.clear();
        keys.dispatch(key(3, "shift", true, false));
        keys.dispatch(key(4, "a", true, false));
        keys.reply(host.request, true);
        keys.dispatch(key(4, "a", false, false));
        keys.dispatch(key(3, "shift", false, false));
        check(host.reports.equals(List.of("9:shift:down:4:false", "9:a:down:4:false", "9:a:up:4:false", "9:shift:up:0:false")), "modifier tracking disagrees with native held keys");

        host.reports.clear(); host.passed.clear();
        keys.dispatch(key(5, "b", true, false));
        int expired = host.request;
        keys.dispatch(key(5, "b", false, false));
        final boolean[] idle = {false};
        keys.whenIdle(() -> idle[0] = true);
        host.timeout.run();
        keys.reply(expired, false);
        check(idle[0] && host.reports.equals(List.of("9:b:down:0:false", "9:b:up:0:false")), "dropped reply left keys held or test waiting");
        check(host.passed.isEmpty(), "expired native continuation was replayed");

        host.reports.clear();
        keys.dispatch(key(6, "c", true, false));
        keys.cancel();
        keys.reply(host.request, false);
        check(host.reports.equals(List.of("9:c:down:0:false", "9:c:up:0:false")), "teardown did not release pending down");

        host.reports.clear(); host.target = 7;
        keys.dispatch(key(9, "k", true, false));
        int old = host.request;
        keys.dispatch(key(10, "y", true, false));
        host.target = 9;
        keys.reply(old, true);
        keys.reply(host.request, true);
        keys.dispatch(key(9, "k", false, false));
        keys.dispatch(key(10, "y", false, false));
        check(host.reports.equals(List.of("7:k:down:0:false", "7:y:down:0:false", "7:k:up:0:false", "7:y:up:0:false")), "queued down forgot the focus at arrival");

        host.target = -1; host.held = true;
        check(!keys.dispatch(key(7, "d", true, false)), "native editor key was observed");
        host.target = 0; host.reports.clear();
        check(keys.dispatch(key(8, "e", true, false)), "surface held-key observer missed a down");
        keys.dispatch(key(8, "e", true, true));
        keys.dispatch(key(8, "e", false, false));
        check(host.reports.equals(List.of("0:e:down:0:false", "0:e:up:0:false")), "surface held keys did not receive both halves alone, without the repeat");
        System.out.println("hardware key continuations passed");
    }
}
