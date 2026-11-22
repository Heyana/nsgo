package dev.oneo.nsgo;

public final class ViewAttachmentCheck {
    public static void main(String[] args) {
        ViewAttachment view = new ViewAttachment();
        long first = view.attach();
        require(view.permits(first), "first tree did not start");
        view.detach();
        require(!view.permits(first), "queued old work survived detach");
        long second = view.attach();
        require(view.waiting() && view.token() == -1, "reattach accepted a delta before its full tree");
        require(!view.reset(first) && view.waiting(), "old reset released the new attach");
        require(view.reset(second) && view.permits(second), "fresh reset did not release its full tree");
        require(!view.permits(first), "old tree became current again");
        require(!view.reset(second), "duplicate reset cleared a current tree");
        view.detach();
        long third = view.attach();
        require(!view.reset(second) && !view.permits(second), "late second attach crossed into the third");
        require(view.reset(third) && view.permits(third), "third attach never became ready");
    }

    private static void require(boolean condition, String message) {
        if (!condition) throw new AssertionError(message);
    }
}
