package dev.oneo.nsgo;

/**
 * Host JVM checks for {@link TestInputGate}.
 */
public final class TestInputGateCheck {
    public static void main(String[] args) {
        TestInputGate gate = new TestInputGate();

        // Ordinary apps and live automation leave isolation off and accept all input.
        check(!gate.isIsolated(), "default gate must not be isolated");
        check(gate.allowInput(), "normal mode must accept input");

        // Test mode drops physical input outside scripted steps.
        gate.setIsolated(true);
        check(gate.isIsolated(), "gate must report isolated once enabled");
        check(!gate.allowInput(), "test mode must reject physical input");

        // Nested scripted steps allow input and restore isolation when unwound.
        gate.enterScripted();
        check(gate.allowInput(), "scripted step must accept input");
        gate.enterScripted();
        check(gate.allowInput(), "nested scripted step must accept input");
        gate.leaveScripted();
        check(gate.allowInput(), "inner leave must not end outer scripted scope");
        gate.leaveScripted();
        check(!gate.allowInput(), "test mode must reject physical input after scripted step");

        // Extra leaveScripted does not underflow.
        gate.leaveScripted();
        gate.enterScripted();
        check(gate.allowInput(), "extra leave must not corrupt scripted depth");
        gate.leaveScripted();
        check(!gate.allowInput(), "test mode must reject input after leave");

        // Disabling isolation restores normal acceptance.
        gate.setIsolated(false);
        check(gate.allowInput(), "disabling isolation must accept input again");
    }

    private static void check(boolean condition, String message) {
        if (!condition) throw new AssertionError(message);
    }
}
