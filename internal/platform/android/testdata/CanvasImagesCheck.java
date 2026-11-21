package dev.oneo.nsgo;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/** Exercises the runtime's actual cache with resources that detect double frees. */
public final class CanvasImagesCheck {
    static final class Picture {
        boolean freed;
        void free() {
            check(!freed, "double free");
            freed = true;
        }
    }

    static void check(boolean ok, String message) {
        if (!ok) throw new AssertionError(message);
    }

    /** What the cache posted to run after the pass of drawing under way, as the UI thread's looper would. */
    static final List<Runnable> posted = new ArrayList<>();

    static void endPass() {
        List<Runnable> run = new ArrayList<>(posted);
        posted.clear();
        for (Runnable r : run) r.run();
    }

    /** Acquires key in a pass of drawing of its own. */
    static CanvasImages<Picture>.Lease draw(CanvasImages<Picture> cache, String key) {
        CanvasImages<Picture>.Lease lease = cache.acquire(key);
        endPass();
        return lease;
    }

    public static void main(String[] args) throws Exception {
        Map<String, Integer> requests = new HashMap<>();
        Map<String, Integer> cancellations = new HashMap<>();
        CanvasImages<Picture> cache = new CanvasImages<>((key, generation, cancel) -> {
            (cancel ? cancellations : requests).put(key, generation);
        }, Picture::free, posted::add);
        Picture first = null;
        for (int i = 0; i < 1000; i++) {
            String key = "image-" + i;
            check(draw(cache, key) == null, "new source was already loaded");
            Picture pic = new Picture();
            check(cache.complete(key, requests.get(key), pic), "current request rejected");
            if (i == 0) first = pic;
            check(cache.size() <= 64, "unbounded decoded cache");
        }
        check(first.freed, "evicted bitmap was not recycled");
        int oldGeneration = requests.get("image-0");
        check(draw(cache, "image-0") == null, "eviction did not reload");
        int generation = requests.get("image-0");
        check(generation != oldGeneration, "request generation reused");
        Picture stale = new Picture();
        check(!cache.complete("image-0", oldGeneration, stale) && stale.freed, "stale response survived");
        Picture reloaded = new Picture();
        cache.complete("image-0", generation, reloaded);
        CanvasImages<Picture>.Lease lease = draw(cache, "image-0");
        check(lease.value == reloaded, "reloaded image differs");
        // Loads under way are never evicted, so the cache exceeds its limit only by them: a canvas or shader waiting for one would go without it until it draws again, and canvases drawn in passes of their own would cancel each other's loads forever.
        for (int i = 0; i < 100; i++) draw(cache, "pending-" + i);
        check(cache.size() == 100, "a load under way was evicted");
        check(cancellations.isEmpty(), "a load under way was cancelled");
        for (int i = 0; i < 100; i++) {
            String key = "pending-" + i;
            check(cache.complete(key, requests.get(key), new Picture()), "a load under way was dropped");
        }
        draw(cache, "after-pending");
        check(cache.size() == 64, "cache did not shrink once its loads under way loaded");
        cache.complete("after-pending", requests.get("after-pending"), new Picture());
        check(!reloaded.freed, "GL frame's leased bitmap freed early");
        Thread gl = new Thread(lease::close);
        gl.start(); gl.join();
        lease.close();
        check(reloaded.freed, "bitmap leaked after GL frame");
        for (int i = 0; i < 1000; i++) {
            String key = "failed-" + i;
            draw(cache, key);
            cache.complete(key, requests.get(key), null);
            draw(cache, key);
            check(cache.size() <= 64, "unbounded failed cache");
        }
        draw(cache, "recent");
        int recent = requests.get("recent");
        cache.complete("recent", recent, new Picture());
        for (int i = 0; i < 100; i++) {
            cache.acquire("recent").close();
            cache.acquire("other-" + i);
            cache.complete("other-" + i, requests.get("other-" + i), new Picture());
            endPass();
            check(cache.size() <= 64, "unbounded loaded cache");
        }
        check(requests.get("recent") == recent, "recently drawn image evicted");

        // One pass drawing more sources than the limit keeps them all: evicting one would request it again at every pass, forever.
        Map<String, Picture> wide = new HashMap<>();
        for (int i = 0; i < 100; i++) cache.acquire("wide-" + i);
        check(cache.size() == 100, "a pass's own sources were evicted");
        for (int i = 0; i < 100; i++) {
            String key = "wide-" + i;
            check(!cancellations.containsKey(key), "a pass cancelled its own load");
            Picture pic = new Picture();
            wide.put(key, pic);
            check(cache.complete(key, requests.get(key), pic), "a pass's load was dropped");
        }
        endPass();
        // Drawing them all again requests none and frees none.
        Map<String, Integer> before = new HashMap<>(requests);
        for (int i = 0; i < 100; i++) {
            CanvasImages<Picture>.Lease l = cache.acquire("wide-" + i);
            check(l != null && l.value == wide.get("wide-" + i), "a wide pass's image did not draw");
            l.close();
        }
        endPass();
        check(requests.equals(before), "a wide pass requested again");
        // Later passes of others shrink the cache back to the limit.
        for (int i = 0; i < 100; i++) {
            draw(cache, "after-" + i);
            cache.complete("after-" + i, requests.get("after-" + i), new Picture());
        }
        check(cache.size() <= 64, "cache did not shrink after a wide pass");
        check(wide.get("wide-0").freed, "a wide pass's bitmap leaked");

        draw(cache, "removed-source");
        int removed = requests.get("removed-source");
        check(cache.unavailable("removed-source", removed), "obsolete source response lost");
        draw(cache, "removed-source");
        check(requests.get("removed-source") != removed, "remounted source stuck pending");

        // Kept images stay out of the 64-source eviction budget, are refcounted, and become evictable on release.
        cache.keep("atlas");
        cache.keep("atlas");
        check(draw(cache, "atlas") == null, "kept atlas should load on first draw");
        Picture atlas = new Picture();
        check(cache.complete("atlas", requests.get("atlas"), atlas), "kept atlas failed to complete");
        for (int i = 0; i < 100; i++) {
            String key = "flood-" + i;
            draw(cache, key);
            cache.complete(key, requests.get(key), new Picture());
            check(cache.size() <= 65, "unkept cache exceeded budget with one kept atlas");
        }
        check(!atlas.freed && cache.ready("atlas"), "kept atlas was evicted");
        cache.release("atlas");
        draw(cache, "flood-extra");
        cache.complete("flood-extra", requests.get("flood-extra"), new Picture());
        check(!atlas.freed && cache.ready("atlas"), "atlas evicted while second pin remained");
        cache.release("atlas");
        cache.release("atlas");
        check(atlas.freed && !cache.ready("atlas"), "released old atlas was not evicted");
        check(cache.size() == 64, "cache did not trim to 64 after release");

        // Keeping more than 64 sources does not starve or crash eviction.
        List<Picture> manyKept = new ArrayList<>();
        for (int i = 0; i < 74; i++) {
            String key = "many-kept-" + i;
            cache.keep(key);
            draw(cache, key);
            Picture pic = new Picture();
            manyKept.add(pic);
            cache.complete(key, requests.get(key), pic);
        }
        for (int i = 0; i < 70; i++) {
            String key = "after-many-" + i;
            draw(cache, key);
            cache.complete(key, requests.get(key), new Picture());
        }
        for (int i = 0; i < 74; i++) {
            check(!manyKept.get(i).freed && cache.ready("many-kept-" + i), "kept source " + i + " evicted");
        }
        for (int i = 0; i < 74; i++) cache.release("many-kept-" + i);
        check(cache.size() == 64, "cache did not trim after releasing 74 kept images");

        CanvasImages<Picture> other = new CanvasImages<>((k, g, c) -> {}, Picture::free, posted::add);
        draw(other, "same");
        Picture isolated = new Picture();
        other.complete("same", 1, isolated);
        cache.close(); cache.close();
        check(cache.size() == 0 && cache.acquire("closed") == null, "closed surface resurrected");
        check(!isolated.freed, "closing one surface freed another's image");
        Picture late = new Picture();
        check(!cache.complete("removed-source", requests.get("removed-source"), late) && late.freed, "closed surface accepted a late decode");
        other.close();
        check(isolated.freed, "surface close leaked decoded bitmap");
    }
}
