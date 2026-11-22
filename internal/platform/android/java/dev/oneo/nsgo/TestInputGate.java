package dev.oneo.nsgo;

/**
 * Isolates Android test hosts ({@code nsgo test -target android}) from physical
 * user input: ordinary apps and live automation leave isolation off and accept
 * all input; isolated test hosts ignore physical touch, key, hover, scroll,
 * trackball and back input outside a scripted {@link TestAgent} step.
 * Accessed only on the UI thread.
 */
final class TestInputGate {
    private boolean isolated;
    private int scriptedDepth;

    void setIsolated(boolean isolated) {
        this.isolated = isolated;
    }

    boolean isIsolated() {
        return isolated;
    }

    void enterScripted() {
        scriptedDepth++;
    }

    void leaveScripted() {
        if (scriptedDepth > 0) scriptedDepth--;
    }

    /** Reports whether an input event may be delivered right now. */
    boolean allowInput() {
        return !isolated || scriptedDepth > 0;
    }
}
