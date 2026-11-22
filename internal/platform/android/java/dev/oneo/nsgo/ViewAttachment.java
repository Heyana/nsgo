package dev.oneo.nsgo;

/** Orders a surface's native work across detach and the next full tree. */
final class ViewAttachment {
    private long generation;
    private boolean attached, waiting, used;

    synchronized long attach() {
        attached = true;
        waiting = used;
        used = true;
        return ++generation;
    }

    synchronized void detach() {
        attached = false;
        waiting = true;
        generation++;
    }

    synchronized boolean waiting() { return waiting; }

    synchronized long token() { return attached && !waiting ? generation : -1; }

    synchronized boolean permits(long token) {
        return attached && !waiting && token == generation;
    }

    synchronized boolean reset(long token) {
        if (!attached || !waiting || token != generation) return false;
        waiting = false;
        return true;
    }
}
