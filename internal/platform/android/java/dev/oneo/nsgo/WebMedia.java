package dev.oneo.nsgo;

import android.webkit.PermissionRequest;
import android.webkit.WebChromeClient;
import java.util.ArrayList;
import java.util.IdentityHashMap;
import java.util.List;
import java.util.Map;
import java.util.function.BiConsumer;
import java.util.function.Consumer;
import java.util.function.Predicate;

/** A web view's capture requests and the system permission prompts they await. UI thread. */
final class WebMedia extends WebChromeClient {
    static final int MICROPHONE = 1, CAMERA = 2;
    private final Predicate<String> granted;
    private final BiConsumer<String[], Consumer<Boolean>> ask;
    private final Map<PermissionRequest, Integer> pending = new IdentityHashMap<>();
    private int media;
    private boolean closed;

    WebMedia(Predicate<String> granted, BiConsumer<String[], Consumer<Boolean>> ask) {
        this.granted = granted;
        this.ask = ask;
    }

    void configure(int media) {
        this.media = media;
        for (PermissionRequest request : new ArrayList<>(pending.keySet())) {
            Integer devices = pending.get(request);
            if (devices != null && (media & devices) != devices) {
                pending.remove(request);
                request.deny();
            }
        }
    }

    /** A page leaving retires its requests before Android answers its dialogs. */
    void leave() {
        List<PermissionRequest> requests = new ArrayList<>(pending.keySet());
        pending.clear();
        for (PermissionRequest request : requests) request.deny();
    }

    void close() {
        closed = true;
        leave();
    }

    @Override
    public void onPermissionRequestCanceled(PermissionRequest request) {
        // WebView already canceled it: never grant or deny it after this.
        pending.remove(request);
    }

    @Override
    public void onPermissionRequest(PermissionRequest request) {
        int want = 0;
        List<String> resources = new ArrayList<>(), permissions = new ArrayList<>();
        for (String resource : request.getResources()) {
            if (PermissionRequest.RESOURCE_AUDIO_CAPTURE.equals(resource)) {
                want |= MICROPHONE;
                permissions.add(android.Manifest.permission.RECORD_AUDIO);
            } else if (PermissionRequest.RESOURCE_VIDEO_CAPTURE.equals(resource)) {
                want |= CAMERA;
                permissions.add(android.Manifest.permission.CAMERA);
            } else {
                continue;
            }
            resources.add(resource);
        }
        if (want == 0 || (media & want) != want || closed) {
            request.deny();
            return;
        }
        String[] allowedResources = resources.toArray(new String[0]);
        List<String> missing = new ArrayList<>();
        for (String p : permissions) if (!granted.test(p)) missing.add(p);
        if (missing.isEmpty()) {
            request.grant(allowedResources);
            return;
        }
        pending.put(request, want);
        ask.accept(missing.toArray(new String[0]), allowed -> {
            Integer devices = pending.remove(request);
            if (devices == null) return;
            if (allowed && !closed && (media & devices) == devices
                    && permissions.stream().allMatch(granted)) request.grant(allowedResources);
            else request.deny();
        });
    }
}
