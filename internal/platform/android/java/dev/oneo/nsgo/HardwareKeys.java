package dev.oneo.nsgo;

import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.LinkedHashMap;

/** Orders hardware keys and their native continuations around Go's asynchronous consume answer, and tracks the keys reported held. UI-thread only. */
final class HardwareKeys {
    static final class Event {
        /** The physical key, by device and scan code (or key code without one), which pairs a release with its down. */
        final long identity;
        final int scan, key, modifiers;
        final String name;
        final boolean down, repeat;
        final Object nativeEvent;
        int target;
        boolean observe;

        Event(long identity, int scan, int key, String name, int modifiers, boolean down, boolean repeat, Object nativeEvent) {
            this.identity = identity; this.scan = scan; this.key = key; this.name = name; this.modifiers = modifiers;
            this.down = down; this.repeat = repeat; this.nativeEvent = nativeEvent;
        }
    }

    interface Host {
        /** The focused view's node when it hears keys, -1 for a native editor, 0 for neither. */
        int target();
        /** Whether the app follows held keys (core.KeysDown) without a focused view hearing them. */
        boolean heldKeys();
        /** Reports event to Go at node target; request 0 is a report alone, any other a down whose consume answer reply takes. */
        void send(int request, int target, Event event);
        /** Runs the native handling of an event Go did not consume: the keymap, activation and Android's own dispatch. */
        void fallback(Event event);
        void schedule(Runnable timeout);
        void unschedule(Runnable timeout);
    }

    private static final class Held {
        final int target, scan, key;
        final String name;
        boolean passed;

        Held(int target, int scan, int key, String name) { this.target = target; this.scan = scan; this.key = key; this.name = name; }
    }

    private final Host host;
    private final ArrayDeque<Event> queue = new ArrayDeque<>();
    private final LinkedHashMap<Long, Held> held = new LinkedHashMap<>();
    private final ArrayList<Runnable> idle = new ArrayList<>();
    private Event pending;
    private Runnable timeout;
    private int serial, waiting;

    HardwareKeys(Host host) { this.host = host; }

    /** Takes a key, returning false when it has nothing to do with it, so its native handling goes on at once. */
    boolean dispatch(Event event) {
        int target = host.target();
        event.target = target;
        event.observe = target != 0 || host.heldKeys();
        if (pending == null && queue.isEmpty() && !held.containsKey(event.identity) &&
                (!event.down || target < 0 || (target == 0 && !host.heldKeys()))) return false;
        queue.add(event);
        drain();
        return true;
    }

    private Event release(long identity, Held key, int modifiers) {
        return new Event(identity, key.scan, key.key, key.name, modifiers, false, false, null);
    }

    private void drain() {
        while (pending == null && !queue.isEmpty()) {
            Event event = queue.remove();
            Held was = held.get(event.identity);
            if (!event.down) {
                if (was != null) {
                    held.remove(event.identity);
                    host.send(0, was.target, release(event.identity, was, modifiers(event.modifiers & ~modifier(was.name))));
                }
                if (was == null || was.passed) host.fallback(event);
                continue;
            }
            if (was != null && !event.repeat) {
                held.remove(event.identity);
                host.send(0, was.target, release(event.identity, was, modifiers(0)));
                was = null;
            }
            int current = event.target;
            if (current < 0) {
                if (was != null) was.passed = true;
                host.fallback(event);
                continue;
            }
            int target = was == null ? current : was.target;
            if (was == null && !event.observe) {
                host.fallback(event);
                continue;
            }
            if (was == null) {
                was = new Held(target, event.scan, event.key, event.name);
                held.put(event.identity, was);
            }
            Event report = new Event(event.identity, event.scan, event.key, event.name, modifiers(event.modifiers), true, event.repeat, event.nativeEvent);
            if (target == 0 || modifier(event.name) != 0) {
                // Held keys alone hear no repeats; a modifier alone is reported and never consumed.
                if (target != 0 || !event.repeat) host.send(0, target, report);
                was.passed = true;
                host.fallback(event);
                continue;
            }
            pending = event;
            waiting = ++serial;
            final int request = waiting;
            timeout = () -> { if (pending != null && waiting == request) cancel(); };
            host.schedule(timeout);
            host.send(request, target, report);
        }
        if (pending == null && queue.isEmpty()) notifyIdle();
    }

    /** Takes Go's answer to request: an unconsumed down goes on to its native handling, then the queued keys follow. */
    void reply(int request, boolean consumed) {
        if (pending == null || request != waiting) return;
        host.unschedule(timeout);
        Event event = pending;
        pending = null;
        Held was = held.get(event.identity);
        if (!consumed) {
            if (was != null) was.passed = true;
            host.fallback(event);
        }
        drain();
    }

    /** Focus loss, shutdown or a dropped reply releases every reported key and abandons native actions. */
    void cancel() {
        if (timeout != null) host.unschedule(timeout);
        pending = null;
        queue.clear();
        for (var entry : held.entrySet()) host.send(0, entry.getValue().target, release(entry.getKey(), entry.getValue(), 0));
        held.clear();
        notifyIdle();
    }

    void whenIdle(Runnable action) {
        if (pending == null && queue.isEmpty()) action.run();
        else idle.add(action);
    }

    private void notifyIdle() {
        var callbacks = new ArrayList<>(idle);
        idle.clear();
        for (Runnable callback : callbacks) callback.run();
    }

    private int modifiers(int nativeModifiers) {
        for (Held key : held.values()) nativeModifiers |= modifier(key.name);
        return nativeModifiers;
    }

    private static int modifier(String name) {
        switch (name) {
            case "ctrl": return 1;
            case "alt": return 2;
            case "shift": return 4;
            case "meta": return 8;
            default: return 0;
        }
    }
}
