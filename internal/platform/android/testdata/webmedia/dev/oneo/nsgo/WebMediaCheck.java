package dev.oneo.nsgo;

import android.Manifest;
import android.webkit.PermissionRequest;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import java.util.function.Consumer;

public final class WebMediaCheck {
    private static final String MIC = PermissionRequest.RESOURCE_AUDIO_CAPTURE;
    private static final String CAM = PermissionRequest.RESOURCE_VIDEO_CAPTURE;

    private static final class Request extends PermissionRequest {
        final String[] resources;
        String[] allowed;
        int grants, denials;
        Request(String... resources) { this.resources = resources; }
        @Override public String[] getResources() { return resources; }
        @Override public void grant(String[] resources) { grants++; allowed = resources; }
        @Override public void deny() { denials++; }
        void answered(int grants, int denials) {
            check(this.grants == grants && this.denials == denials,
                    "answers: " + this.grants + " grants, " + this.denials + " denials; expected " + grants + ", " + denials);
        }
    }

    private static final class Fixture {
        final Set<String> permissions = new HashSet<>();
        final List<String[]> prompts = new ArrayList<>();
        final List<Consumer<Boolean>> answers = new ArrayList<>();
        final WebMedia media = new WebMedia(permissions::contains, (wanted, answer) -> {
            prompts.add(wanted);
            answers.add(answer);
        });
        Fixture(int devices) { media.configure(devices); }
        Request request(String... resources) {
            Request r = new Request(resources);
            media.onPermissionRequest(r);
            return r;
        }
        void answer(int i, boolean allowed) {
            if (allowed) permissions.addAll(Arrays.asList(prompts.get(i)));
            answers.get(i).accept(allowed);
        }
    }

    private static void check(boolean ok, String why) { if (!ok) throw new AssertionError(why); }

    public static void main(String[] args) {
        // Each device and the pair need all requested flags, even if the app
        // already holds the system permission. Unknown WebView resources never
        // inherit a media grant, including when mixed with a known device.
        for (int devices = 0; devices <= 3; devices++) {
            Fixture f = new Fixture(devices);
            f.permissions.add(Manifest.permission.RECORD_AUDIO);
            f.permissions.add(Manifest.permission.CAMERA);
            f.request(MIC).answered((devices & 1) != 0 ? 1 : 0, (devices & 1) != 0 ? 0 : 1);
            f.request(CAM).answered((devices & 2) != 0 ? 1 : 0, (devices & 2) != 0 ? 0 : 1);
            f.request(MIC, CAM).answered(devices == 3 ? 1 : 0, devices == 3 ? 0 : 1);
            f.request("future.resource").answered(0, 1);
            check(f.prompts.isEmpty(), "unnecessary system prompt");
        }
        Fixture f = new Fixture(3);
        f.permissions.add(Manifest.permission.RECORD_AUDIO);
        Request mixed = f.request(MIC, "future.resource");
        mixed.answered(1, 0);
        check(Arrays.equals(mixed.allowed, new String[]{MIC}), "unknown resource granted");
        Request both = f.request(MIC, CAM);
        both.answered(0, 0);
        check(Arrays.equals(f.prompts.get(0), new String[]{Manifest.permission.CAMERA}), "prompt did not name only the missing device");
        f.answer(0, true);
        both.answered(1, 0);
        f.answer(0, true);
        both.answered(1, 0);

        f = new Fixture(1);
        Request refused = f.request(MIC);
        f.answer(0, false);
        refused.answered(0, 1);

        // Revoking Media while Android's dialog is open must refuse the page,
        // even if the system later reports success, or Media is enabled again.
        f = new Fixture(1);
        Request revoked = f.request(MIC);
        f.media.configure(0);
        f.media.configure(1);
        f.answer(0, true);
        revoked.answered(0, 1);

        // Canceling one request must not invalidate another view/frame's request.
        f = new Fixture(3);
        Request canceled = f.request(MIC), current = f.request(CAM);
        f.media.onPermissionRequestCanceled(canceled);
        f.answer(0, true);
        f.answer(1, true);
        canceled.answered(0, 0);
        current.answered(1, 0);

        // Removing only the camera leaves an independent microphone request
        // alive, while retiring a request for both devices in its entirety.
        f = new Fixture(3);
        Request pair = f.request(MIC, CAM), micOnly = f.request(MIC);
        f.media.configure(1);
        pair.answered(0, 1);
        micOnly.answered(0, 0);
        f.answer(0, true);
        f.answer(1, true);
        pair.answered(0, 1);
        micOnly.answered(1, 0);

        f = new Fixture(3);
        f.permissions.add(Manifest.permission.RECORD_AUDIO);
        Request lost = f.request(MIC, CAM);
        f.permissions.clear();
        f.answer(0, true);
        lost.answered(0, 1);

        // A navigation or destruction retires all still-live requests. Delayed
        // callbacks must neither grant access nor answer a native request twice.
        for (boolean close : new boolean[]{false, true}) {
            f = new Fixture(3);
            Request mic = f.request(MIC), cam = f.request(CAM);
            if (close) f.media.close(); else f.media.leave();
            f.answer(0, true);
            f.answer(1, false);
            mic.answered(0, 1);
            cam.answered(0, 1);
            if (close) f.request(MIC).answered(0, 1);
            else f.request(MIC).answered(1, 0);
        }
        System.out.println("web media devices, delayed permissions, revocation, cancellation, navigation and destruction passed");
    }
}
