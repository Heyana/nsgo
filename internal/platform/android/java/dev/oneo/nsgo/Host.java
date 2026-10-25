package dev.oneo.nsgo;

import android.os.Handler;
import android.os.Looper;
import android.util.Log;
import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.Iterator;
import java.util.List;
import java.util.Map;
import org.json.JSONArray;
import org.json.JSONObject;

/**
 * An NsgoView's end of the channel between the embedding app and the NSGo
 * app (core.Host), across the pipe as host.go has it: what the host sets
 * goes as HOST messages, queued until the app's process starts; the app's
 * calls, URLs and failures come as HOST_CALL and reach the host's
 * handlers on the UI thread. Values cross as JSON, which the handlers see
 * as maps, lists, strings, numbers, booleans and null.
 */
final class Host {
    private static final String TAG = "nsgo";

    private final NsgoView view;
    private final Handler ui = new Handler(Looper.getMainLooper());
    /** HOST messages written before the process started. */
    private final List<byte[]> queued = new ArrayList<>();
    private boolean started;

    final Map<String, NsgoView.HostHandler> handlers = new HashMap<>();
    final Map<String, NsgoComponent.Factory> components = new HashMap<>();
    NsgoView.OpenUrlHandler openUrl;
    NsgoView.ErrorListener errors;
    String params = "";

    Host(NsgoView view) {
        this.view = view;
    }

    /** Sends what was queued, as the process starts. */
    synchronized void start() {
        started = true;
        for (byte[] m : queued) view.write(Protocol.HOST, m);
        queued.clear();
    }

    /** Writes a HOST message of part, now or once the process starts. */
    private void send(int part, Body body) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeByte(part);
            body.write(d);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        synchronized (this) {
            if (!started) {
                queued.add(bytes.toByteArray());
                return;
            }
        }
        view.write(Protocol.HOST, bytes.toByteArray());
    }

    private interface Body {
        void write(DataOutputStream d) throws IOException;
    }

    void event(String name, Object data) {
        send(Protocol.HOST_EVENT, d -> {
            Protocol.writeString(d, name);
            Protocol.writeString(d, json(data));
        });
    }

    void handle(String name, NsgoView.HostHandler handler) {
        if (handler == null) handlers.remove(name);
        else handlers.put(name, handler);
        send(Protocol.HOST_HANDLE, d -> {
            Protocol.writeString(d, name);
            d.writeByte(handler != null ? 1 : 0);
        });
    }

    void tokens(Map<String, String> values, Map<String, String> dark) {
        send(Protocol.HOST_TOKENS, d -> {
            Protocol.writeString(d, json(values));
            Protocol.writeString(d, json(dark));
        });
    }

    void openUrl(NsgoView.OpenUrlHandler handler) {
        openUrl = handler;
        send(Protocol.HOST_URL_HANDLER, d -> d.writeByte(handler != null ? 1 : 0));
    }

    /** Handles a HOST_CALL, on the reader thread. */
    void message(DataInputStream in) throws IOException {
        switch (in.readUnsignedByte()) {
            case Protocol.CALL_CALL: {
                int id = in.readInt();
                String name = Protocol.readString(in);
                Object data = plain(Embedded.parse(Protocol.readString(in)));
                boolean post = in.readUnsignedByte() != 0;
                ui.post(() -> call(id, name, data, post));
                break;
            }
            case Protocol.CALL_OPEN_URL: {
                int id = in.readInt();
                String url = Protocol.readString(in);
                ui.post(() -> {
                    boolean handled = false;
                    try {
                        handled = openUrl != null && openUrl.openUrl(url);
                    } catch (RuntimeException e) {
                        Log.w(TAG, "the host's URL handler failed", e);
                    }
                    boolean answer = handled;
                    send(Protocol.HOST_URL_ANSWER, d -> {
                        d.writeInt(id);
                        d.writeByte(answer ? 1 : 0);
                    });
                });
                break;
            }
            case Protocol.CALL_ERROR: {
                String message = Protocol.readString(in);
                ui.post(() -> {
                    if (errors != null) errors.onError(message);
                });
                break;
            }
            default:
        }
    }

    /** Runs the host's handler of a call; a post's answer goes nowhere. */
    private void call(int id, String name, Object data, boolean post) {
        NsgoView.HostReply reply = new NsgoView.HostReply() {
            private boolean answered;

            @Override
            public void resolve(Object result) {
                answer(true, json(result));
            }

            @Override
            public void reject(String error) {
                answer(false, error == null ? "the host rejected the call" : error);
            }

            private void answer(boolean ok, String value) {
                synchronized (this) {
                    if (answered || post) return;
                    answered = true;
                }
                send(Protocol.HOST_REPLY, d -> {
                    d.writeInt(id);
                    d.writeByte(ok ? 1 : 0);
                    Protocol.writeString(d, value);
                });
            }
        };
        NsgoView.HostHandler handler = handlers.get(name);
        if (handler == null) {
            reply.reject("the host has no handler of " + name);
            return;
        }
        try {
            handler.call(data, reply);
        } catch (RuntimeException e) {
            reply.reject("the host's handler of " + name + " failed: " + e);
        }
    }

    /** A JSON value as maps, lists and plain values. */
    static Object plain(Object v) {
        if (v == null || v == JSONObject.NULL) return null;
        if (v instanceof JSONObject) {
            JSONObject o = (JSONObject) v;
            Map<String, Object> m = new HashMap<>();
            for (Iterator<String> keys = o.keys(); keys.hasNext(); ) {
                String k = keys.next();
                m.put(k, plain(o.opt(k)));
            }
            return m;
        }
        if (v instanceof JSONArray) {
            JSONArray a = (JSONArray) v;
            List<Object> l = new ArrayList<>(a.length());
            for (int i = 0; i < a.length(); i++) l.add(plain(a.opt(i)));
            return l;
        }
        return v;
    }

    /** Plain values, maps and lists as JSON; null for what JSON cannot hold. */
    static String json(Object v) {
        if (v == null) return "null";
        Object w = JSONObject.wrap(v);
        if (w == null) return "null";
        if (w instanceof String) return JSONObject.quote((String) w);
        return w.toString();
    }
}
