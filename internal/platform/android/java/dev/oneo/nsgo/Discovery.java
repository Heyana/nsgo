package dev.oneo.nsgo;

import android.content.Context;
import android.net.Network;
import android.net.nsd.NsdManager;
import android.net.nsd.NsdServiceInfo;
import android.net.wifi.WifiManager;
import android.os.Build;
import android.os.Handler;
import android.os.ext.SdkExtensions;
import android.util.Log;

import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.lang.reflect.InvocationTargetException;
import java.net.Inet4Address;
import java.net.InetAddress;
import java.nio.ByteBuffer;
import java.nio.charset.CharacterCodingException;
import java.nio.charset.StandardCharsets;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.function.BiConsumer;

/** DNS-SD for the primary view and its overlays. All operation state lives on the UI thread. */
final class Discovery {
    private static final String TAG = "nsgo";
    private static final int T_EXTENSION = Build.VERSION.SDK_INT >= 33 ? SdkExtensions.getExtensionVersion(33) : 0;
    private static final boolean CAN_STOP_RESOLUTION = Build.VERSION.SDK_INT >= 34 || T_EXTENSION >= 7;

    // Older Android allows only one resolve per NsdManager, shared by the application's engines.
    private static final ArrayDeque<Resolution> pending = new ArrayDeque<>();
    private static Resolution resolving;

    private final Context app;
    private final Handler ui;
    private final BiConsumer<Integer, byte[]> send;
    private final Map<Key, Operation> operations = new HashMap<>();
    private NsdManager nsd;
    private WifiManager.MulticastLock multicast;

    private record Key(int surface, int count) {}
    private record Instance(String name, Network network) {}
    private record Endpoint(String name, String host, int port) {}

    Discovery(Context app, Handler ui, BiConsumer<Integer, byte[]> send) {
        this.app = app;
        this.ui = ui;
        this.send = send;
    }

    void request(int surface, DataInputStream in) {
        Key key;
        try {
            key = new Key(surface, in.readInt());
        } catch (IOException e) {
            Log.w(TAG, "invalid DNS-SD request", e);
            return;
        }
        Operation operation = null;
        try {
            int op = in.readUnsignedByte();
            stop(key);
            if (op == Protocol.DISCOVERY_STOP) return;
            switch (op) {
                case Protocol.DISCOVERY_ADVERTISE:
                    operation = new Advertisement(key, readAdvertisement(in));
                    break;
                case Protocol.DISCOVERY_BROWSE:
                    operation = new Browser(key, Protocol.readString(in));
                    break;
                default:
                    throw new IOException("unknown DNS-SD operation " + op);
            }
            if (nsd == null) nsd = app.getSystemService(NsdManager.class);
            if (nsd == null) throw new IllegalStateException("network service discovery unavailable");
            if (operations.isEmpty()) acquireMulticast();
            operations.put(key, operation);
            operation.start();
        } catch (IOException | RuntimeException e) {
            if (operation != null) operations.remove(key, operation);
            releaseMulticastIfIdle();
            reply(key, e.toString(), false, null, null, null);
        }
    }

    void closeSurface(int surface) {
        for (Key key : new ArrayList<>(operations.keySet())) {
            if (key.surface == surface) stop(key);
        }
    }

    private void stop(Key key) {
        Operation operation = operations.remove(key);
        if (operation == null) return;
        try {
            operation.stop();
        } catch (RuntimeException e) {
            Log.w(TAG, "could not stop Android DNS-SD", e);
        } finally {
            releaseMulticastIfIdle();
        }
    }

    private void acquireMulticast() {
        if (T_EXTENSION >= 7 || Build.VERSION.SDK_INT >= 34) return;
        WifiManager wifi = app.getSystemService(WifiManager.class);
        if (wifi == null) return;
        WifiManager.MulticastLock lock = wifi.createMulticastLock("nsgo:discovery");
        try {
            lock.acquire();
        } catch (SecurityException e) {
            throw new SecurityException("Android DNS-SD requires CHANGE_WIFI_MULTICAST_STATE on this device", e);
        }
        multicast = lock;
    }

    private void releaseMulticastIfIdle() {
        if (multicast != null && operations.isEmpty()) {
            multicast.release();
            multicast = null;
        }
    }

    private abstract class Operation {
        final Key key;

        Operation(Key key) {
            this.key = key;
        }

        abstract void start();
        abstract void stop();

        boolean live() {
            return operations.get(key) == this;
        }

        void failed(String error) {
            if (!operations.remove(key, this)) return;
            releaseMulticastIfIdle();
            reply(key, error, false, null, null, null);
        }
    }

    private final class Advertisement extends Operation implements NsdManager.RegistrationListener {
        final NsdServiceInfo service;

        Advertisement(Key key, NsdServiceInfo service) {
            super(key);
            this.service = service;
        }

        @Override void start() { nsd.registerService(service, NsdManager.PROTOCOL_DNS_SD, this); }
        @Override void stop() { nsd.unregisterService(this); }

        @Override public void onServiceRegistered(NsdServiceInfo info) {}

        @Override public void onRegistrationFailed(NsdServiceInfo info, int code) {
            ui.post(() -> failed("Android DNS-SD registration failed (" + code + ")"));
        }

        @Override public void onServiceUnregistered(NsdServiceInfo info) {
            ui.post(() -> failed("Android DNS-SD registration ended"));
        }

        @Override public void onUnregistrationFailed(NsdServiceInfo info, int code) {
            Log.w(TAG, "Android DNS-SD unregistration failed (" + code + ")");
        }
    }

    private final class Browser extends Operation implements NsdManager.DiscoveryListener {
        final String type;
        final Map<Instance, Resolution> instances = new HashMap<>();
        final Map<Endpoint, Published> endpoints = new HashMap<>();

        Browser(Key key, String type) {
            super(key);
            this.type = type;
        }

        @Override void start() { nsd.discoverServices(type, NsdManager.PROTOCOL_DNS_SD, this); }

        @Override void stop() {
            clearInstances();
            nsd.stopServiceDiscovery(this);
        }

        private void clearInstances() {
            instances.clear();
            endpoints.clear();
            pending.removeIf(request -> request.browser == this);
            if (resolving != null && resolving.browser == this) resolving.cancel();
        }

        @Override void failed(String error) {
            if (!live()) return;
            super.failed(error);
            clearInstances();
        }

        @Override public void onDiscoveryStarted(String type) {}

        @Override public void onStartDiscoveryFailed(String type, int code) {
            ui.post(() -> failed("Android DNS-SD browsing failed (" + code + ")"));
        }

        @Override public void onDiscoveryStopped(String type) {
            ui.post(() -> failed("Android DNS-SD browsing ended"));
        }

        @Override public void onStopDiscoveryFailed(String type, int code) {
            Log.w(TAG, "Android DNS-SD stop browsing failed (" + code + ")");
        }

        @Override public void onServiceFound(NsdServiceInfo service) {
            ui.post(() -> {
                Instance instance = instanceOf(service);
                if (!live() || instance.name == null || instances.containsKey(instance)) return;
                Resolution request = new Resolution(this, instance, service);
                instances.put(instance, request);
                pending.addLast(request);
                resolveNext();
            });
        }

        @Override public void onServiceLost(NsdServiceInfo service) {
            ui.post(() -> {
                Resolution request = instances.remove(instanceOf(service));
                if (request == null) return;
                if (request == resolving) request.cancel();
                else pending.remove(request);
                if (request.endpoint != null) {
                    Published published = endpoints.get(request.endpoint);
                    if (--published.instances == 0) {
                        endpoints.remove(request.endpoint);
                        reply(key, "", true, request.endpoint, type, published.txt);
                    }
                }
            });
        }

        void resolved(Resolution request, NsdServiceInfo service) {
            if (instances.get(request.instance) != request) return;
            InetAddress address = service == null ? null : addressOf(service);
            if (address == null || service.getPort() == 0) {
                instances.remove(request.instance);
                return;
            }
            Endpoint endpoint = new Endpoint(request.instance.name, address.getHostAddress(), service.getPort());
            request.endpoint = endpoint;
            Published published = endpoints.get(endpoint);
            if (published == null) {
                published = new Published(txtBytes(service));
                endpoints.put(endpoint, published);
                reply(key, "", false, endpoint, type, published.txt);
            }
            published.instances++;
        }
    }

    private static final class Published {
        final byte[] txt;
        int instances;

        Published(byte[] txt) {
            this.txt = txt;
        }
    }

    private final class Resolution implements NsdManager.ResolveListener {
        final Browser browser;
        final Instance instance;
        final NsdServiceInfo service;
        Endpoint endpoint;

        Resolution(Browser browser, Instance instance, NsdServiceInfo service) {
            this.browser = browser;
            this.instance = instance;
            this.service = service;
        }

        void start() {
            nsd.resolveService(service, this);
        }

        void cancel() {
            if (!CAN_STOP_RESOLUTION) return;
            try {
                nsd.stopServiceResolution(this);
            } catch (IllegalArgumentException alreadyFinished) {
                // Its completion has already been queued.
            } catch (RuntimeException e) {
                Log.w(TAG, "could not stop Android DNS-SD resolution", e);
            }
        }

        @Override public void onResolveFailed(NsdServiceInfo service, int code) {
            ui.post(() -> finish(null));
        }

        @Override public void onServiceResolved(NsdServiceInfo service) {
            ui.post(() -> finish(service));
        }

        @Override public void onResolutionStopped(NsdServiceInfo service) {
            ui.post(() -> finish(null));
        }

        @Override public void onStopResolutionFailed(NsdServiceInfo service, int code) {
            ui.post(() -> finish(null));
        }

        private void finish(NsdServiceInfo service) {
            if (resolving != this) return;
            resolving = null;
            browser.resolved(this, service);
            resolveNext();
        }
    }

    private static void resolveNext() {
        while (resolving == null && !pending.isEmpty()) {
            Resolution request = pending.removeFirst();
            try {
                request.start();
                resolving = request;
            } catch (RuntimeException e) {
                request.browser.resolved(request, null);
                Log.w(TAG, "Android DNS-SD could not resolve an instance", e);
            }
        }
    }

    private static Instance instanceOf(NsdServiceInfo service) {
        return new Instance(service.getServiceName(), Build.VERSION.SDK_INT >= 33 ? service.getNetwork() : null);
    }

    @SuppressWarnings("deprecation")
    private static InetAddress addressOf(NsdServiceInfo service) {
        if (!CAN_STOP_RESOLUTION) return service.getHost();
        List<InetAddress> addresses = service.getHostAddresses();
        for (InetAddress address : addresses) {
            if (address instanceof Inet4Address) return address;
        }
        return addresses.isEmpty() ? null : addresses.get(0);
    }

    private static NsdServiceInfo readAdvertisement(DataInputStream in) throws IOException {
        NsdServiceInfo service = new NsdServiceInfo();
        service.setServiceType(Protocol.readString(in));
        service.setServiceName(Protocol.readString(in));
        service.setPort(in.readInt());
        byte[] txt = Protocol.readBytes(in);
        for (int start = 0; start < txt.length; ) {
            int length = Byte.toUnsignedInt(txt[start++]);
            int end = start + length, equals = start;
            if (end > txt.length) throw new IOException("truncated DNS-SD attribute");
            while (equals < end && txt[equals] != '=') equals++;
            if (equals == start || equals == end) throw new IOException("invalid DNS-SD attribute");
            String key = new String(txt, start, equals - start, StandardCharsets.US_ASCII);
            setAttribute(service, key, txt, equals + 1, end);
            start = end;
        }
        return service;
    }

    private static void setAttribute(NsdServiceInfo service, String key, byte[] txt, int start, int end) throws IOException {
        ByteBuffer value = ByteBuffer.wrap(txt, start, end - start);
        String text;
        try {
            text = StandardCharsets.UTF_8.newDecoder().decode(value).toString();
        } catch (CharacterCodingException nonUTF8) {
            if (Build.VERSION.SDK_INT < 37 && T_EXTENSION < 22) {
                throw new IOException("Android DNS-SD requires UTF-8 TXT values on this device");
            }
            // The raw overload is public in API 37 and not in the runtime's compile SDK.
            try {
                NsdServiceInfo.class.getMethod("setAttribute", String.class, byte[].class)
                        .invoke(service, key, Arrays.copyOfRange(txt, start, end));
            } catch (InvocationTargetException e) {
                throw new IOException("could not set Android DNS-SD TXT attribute", e.getCause());
            } catch (ReflectiveOperationException e) {
                throw new IOException("could not set Android DNS-SD TXT attribute", e);
            }
            return;
        }
        service.setAttribute(key, text);
    }

    private static byte[] txtBytes(NsdServiceInfo service) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        for (Map.Entry<String, byte[]> attribute : service.getAttributes().entrySet()) {
            byte[] key = attribute.getKey().getBytes(StandardCharsets.UTF_8), value = attribute.getValue();
            int length = key.length + (value == null ? 0 : 1 + value.length);
            if (length > 255) continue;
            bytes.write(length);
            bytes.write(key, 0, key.length);
            if (value != null) {
                bytes.write('=');
                bytes.write(value, 0, value.length);
            }
        }
        return bytes.toByteArray();
    }

    private void reply(Key key, String error, boolean removed, Endpoint endpoint, String type, byte[] txt) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream out = new DataOutputStream(bytes);
        try {
            out.writeInt(key.surface);
            out.writeInt(key.count);
            Protocol.writeString(out, error);
            if (endpoint != null) {
                out.writeBoolean(removed);
                Protocol.writeString(out, endpoint.name);
                Protocol.writeString(out, type);
                Protocol.writeString(out, endpoint.host);
                out.writeInt(endpoint.port);
                out.writeInt(txt.length);
                out.write(txt);
            }
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        send.accept(Protocol.DISCOVERY_REPLY, bytes.toByteArray());
    }
}
