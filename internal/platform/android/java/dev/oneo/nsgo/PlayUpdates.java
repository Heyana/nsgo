package dev.oneo.nsgo;

import android.app.Activity;
import android.content.Context;

import java.lang.reflect.Method;
import java.lang.reflect.Proxy;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.function.Consumer;

/** Play owns downloading, consent, installation and restart; no APK leaves its flow. */
final class PlayUpdates {
    static final int REQUEST = 0x4e55;
    static final int AVAILABLE = 2;
    static final int IN_PROGRESS = 3;
    static final int IMMEDIATE = 1;

    interface Reply { void done(boolean shown, String error); }
    interface Client {
        Object create(Activity activity) throws Exception;
        void info(Object manager, Consumer<Info> success, Consumer<String> failure) throws Exception;
        boolean launch(Object manager, Info info, Activity activity) throws Exception;
    }
    static final class Info {
        final Object value;
        final int availability;
        final boolean allowed;
        Info(Object value, int availability, boolean allowed) {
            this.value = value;
            this.availability = availability;
            this.allowed = allowed;
        }
    }

    private final Client client;
    private Object manager;
    private boolean checking, started;

    PlayUpdates() { this(SDK.load()); }
    PlayUpdates(Client client) { this.client = client; }

    void check(Activity activity, Reply reply) { request(activity, false, reply); }

    /** A consented immediate update interrupted by another activity resumes on return. */
    void resume(Activity activity) {
        if (started) request(activity, true, (shown, error) -> {});
    }

    void activityResult(int request) {
        if (request == REQUEST) started = false; // Cancellation never immediately reopens consent.
    }

    private void request(Activity activity, boolean resume, Reply reply) {
        if (checking) { reply.done(true, ""); return; }
        if (client == null) { reply.done(false, "the Android host has no Play In-App Updates SDK"); return; }
        if (activity == null || activity.isFinishing() || activity.isDestroyed()) {
            reply.done(false, "the app has no active activity for a Play update");
            return;
        }
        AtomicBoolean answered = new AtomicBoolean();
        Consumer<String> fail = error -> {
            if (answered.compareAndSet(false, true)) {
                checking = false;
                reply.done(false, error);
            }
        };
        try {
            if (manager == null) manager = client.create(activity);
            checking = true;
            client.info(manager, info -> {
                if (!answered.compareAndSet(false, true)) return;
                checking = false;
                boolean available = (!resume || started) && (info.availability == IN_PROGRESS || (!resume && info.availability == AVAILABLE));
                if (!available || !info.allowed) { reply.done(false, ""); return; }
                if (activity.isFinishing() || activity.isDestroyed() || !activity.hasWindowFocus()) {
                    reply.done(false, "the app left the foreground before Play could offer its update");
                    return;
                }
                try {
                    started = client.launch(manager, info, activity);
                    reply.done(started, started ? "" : "Play declined to start its update flow");
                } catch (Exception | LinkageError error) {
                    reply.done(false, failure(error));
                }
            }, fail);
        } catch (Exception | LinkageError error) { fail.accept(failure(error)); }
    }

    private static String failure(Throwable error) {
        Throwable cause = error.getCause();
        return "Play update: " + (cause == null ? error : cause).toString();
    }

    /** Standalone runtimes bundle this SDK; embedded hosts supply it through Gradle. */
    private static final class SDK implements Client {
        final Method create, request, addListener, successful, result, exception;
        final Method availability, allowed, options, build, launch;
        final Class<?> listener;

        SDK() throws ReflectiveOperationException {
            Class<?> factory = Class.forName("com.google.android.play.core.appupdate.AppUpdateManagerFactory");
            Class<?> manager = Class.forName("com.google.android.play.core.appupdate.AppUpdateManager");
            Class<?> info = Class.forName("com.google.android.play.core.appupdate.AppUpdateInfo");
            Class<?> option = Class.forName("com.google.android.play.core.appupdate.AppUpdateOptions");
            Class<?> builder = Class.forName("com.google.android.play.core.appupdate.AppUpdateOptions$Builder");
            Class<?> task = Class.forName("com.google.android.gms.tasks.Task");
            listener = Class.forName("com.google.android.gms.tasks.OnCompleteListener");
            create = factory.getMethod("create", Context.class);
            request = manager.getMethod("getAppUpdateInfo");
            addListener = task.getMethod("addOnCompleteListener", listener);
            successful = task.getMethod("isSuccessful");
            result = task.getMethod("getResult");
            exception = task.getMethod("getException");
            availability = info.getMethod("updateAvailability");
            allowed = info.getMethod("isUpdateTypeAllowed", int.class);
            options = option.getMethod("newBuilder", int.class);
            build = builder.getMethod("build");
            launch = manager.getMethod("startUpdateFlowForResult", info, Activity.class, option, int.class);
        }

        static Client load() {
            try { return new SDK(); }
            catch (ReflectiveOperationException | LinkageError error) { return null; }
        }
        public Object create(Activity activity) throws Exception {
            return create.invoke(null, activity.getApplicationContext());
        }
        public void info(Object manager, Consumer<Info> success, Consumer<String> failure) throws Exception {
            Object task = request.invoke(manager);
            Object callback = Proxy.newProxyInstance(listener.getClassLoader(), new Class<?>[]{listener}, (proxy, method, args) -> {
                switch (method.getName()) {
                    case "hashCode": return System.identityHashCode(proxy);
                    case "equals": return proxy == args[0];
                    case "toString": return "NsgoPlayUpdateListener";
                    case "onComplete":
                        try {
                            Object answer = args[0];
                            if (!Boolean.TRUE.equals(successful.invoke(answer))) {
                                failure.accept("Play update: " + String.valueOf(exception.invoke(answer)));
                            } else {
                                Object value = result.invoke(answer);
                                success.accept(new Info(value, (Integer)availability.invoke(value),
                                        Boolean.TRUE.equals(allowed.invoke(value, IMMEDIATE))));
                            }
                        } catch (ReflectiveOperationException | LinkageError error) { failure.accept(failure(error)); }
                        return null;
                    default: return null;
                }
            });
            addListener.invoke(task, callback);
        }
        public boolean launch(Object manager, Info info, Activity activity) throws Exception {
            Object option = build.invoke(options.invoke(null, IMMEDIATE));
            return Boolean.TRUE.equals(launch.invoke(manager, info.value, activity, option, REQUEST));
        }
    }
}
