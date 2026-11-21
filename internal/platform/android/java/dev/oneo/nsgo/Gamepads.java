package dev.oneo.nsgo;

import android.content.Context;
import android.hardware.input.InputManager;
import android.os.Build;
import android.os.CombinedVibration;
import android.os.Handler;
import android.os.VibrationEffect;
import android.os.Vibrator;
import android.os.VibratorManager;
import android.util.SparseArray;
import android.view.InputDevice;
import android.view.KeyEvent;
import android.view.MotionEvent;

import java.io.ByteArrayOutputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;

/**
 * Tracks connected game controllers through InputManager and forwards KeyEvent
 * and MotionEvent readings in the W3C Standard Gamepad layout to Go over
 * Protocol.GAMEPAD. Also drives controller vibration on Protocol.GAMEPAD_RUMBLE.
 */
final class Gamepads implements InputManager.InputDeviceListener {
    private static final int BUTTON_COUNT = 17;
    private static final int AXIS_COUNT = 4;

    private static final int BUTTON_SOUTH = 0;
    private static final int BUTTON_EAST = 1;
    private static final int BUTTON_WEST = 2;
    private static final int BUTTON_NORTH = 3;
    private static final int BUTTON_LB = 4;
    private static final int BUTTON_RB = 5;
    private static final int BUTTON_LT = 6;
    private static final int BUTTON_RT = 7;
    private static final int BUTTON_BACK = 8;
    private static final int BUTTON_START = 9;
    private static final int BUTTON_LEFT_STICK = 10;
    private static final int BUTTON_RIGHT_STICK = 11;
    private static final int BUTTON_DPAD_UP = 12;
    private static final int BUTTON_DPAD_DOWN = 13;
    private static final int BUTTON_DPAD_LEFT = 14;
    private static final int BUTTON_DPAD_RIGHT = 15;
    private static final int BUTTON_HOME = 16;

    private static final int AXIS_LEFT_X = 0;
    private static final int AXIS_LEFT_Y = 1;
    private static final int AXIS_RIGHT_X = 2;
    private static final int AXIS_RIGHT_Y = 3;

    interface Sink {
        void send(byte[] payload);
    }

    private static final class Pad {
        final int id;
        final String name;
        final int vendorId;
        final int productId;
        final boolean canRumble;
        final float[] buttons = new float[BUTTON_COUNT];
        final float[] axes = new float[AXIS_COUNT];

        Pad(int id, String name, int vendorId, int productId, boolean canRumble) {
            this.id = id;
            this.name = name != null && !name.isEmpty() ? name : "Gamepad";
            this.vendorId = vendorId & 0xFFFF;
            this.productId = productId & 0xFFFF;
            this.canRumble = canRumble;
        }
    }

    private final InputManager manager;
    private final Sink sink;
    private final SparseArray<Pad> pads = new SparseArray<>();
    private boolean started;

    Gamepads(Context context, Sink sink) {
        this.manager = (InputManager) context.getSystemService(Context.INPUT_SERVICE);
        this.sink = sink;
    }

    void start(Handler handler) {
        if (started || manager == null) return;
        started = true;
        manager.registerInputDeviceListener(this, handler);
        for (int deviceId : manager.getInputDeviceIds()) {
            maybeConnect(deviceId);
        }
    }

    void stop() {
        if (!started || manager == null) return;
        started = false;
        manager.unregisterInputDeviceListener(this);
        for (int i = 0; i < pads.size(); i++) {
            InputDevice dev = manager.getInputDevice(pads.keyAt(i));
            if (dev != null) stopRumble(dev);
        }
        pads.clear();
    }

    @Override
    public void onInputDeviceAdded(int deviceId) {
        maybeConnect(deviceId);
    }

    @Override
    public void onInputDeviceChanged(int deviceId) {
        maybeConnect(deviceId);
    }

    @Override
    public void onInputDeviceRemoved(int deviceId) {
        if (pads.indexOfKey(deviceId) < 0) return;
        pads.remove(deviceId);
        sendDisconnect(deviceId);
    }

    void rumble(int deviceId, float lowFreq, float highFreq, int durationMs) {
        if (manager == null) return;
        InputDevice dev = manager.getInputDevice(deviceId);
        if (dev == null) return;
        lowFreq = clamp01(lowFreq);
        highFreq = clamp01(highFreq);
        if (durationMs <= 0 || (lowFreq <= 0f && highFreq <= 0f)) {
            stopRumble(dev);
            return;
        }
        if (Build.VERSION.SDK_INT >= 31) {
            VibratorManager vm = dev.getVibratorManager();
            if (vm != null) {
                int[] ids = vm.getVibratorIds();
                if (ids.length >= 2) {
                    CombinedVibration.ParallelCombination combo = CombinedVibration.startParallel();
                    boolean any = false;
                    if (lowFreq > 0f) {
                        combo.addVibrator(ids[0], VibrationEffect.createOneShot(durationMs, toAmplitude(lowFreq)));
                        any = true;
                    }
                    if (highFreq > 0f) {
                        combo.addVibrator(ids[1], VibrationEffect.createOneShot(durationMs, toAmplitude(highFreq)));
                        any = true;
                    }
                    if (any) {
                        vm.vibrate(combo.combine());
                    }
                    return;
                }
                if (ids.length == 1) {
                    int amp = toAmplitude(Math.max(lowFreq, highFreq));
                    vm.vibrate(CombinedVibration.createParallel(VibrationEffect.createOneShot(durationMs, amp)));
                    return;
                }
            }
        }
        vibrateLegacy(dev, Math.max(lowFreq, highFreq), durationMs);
    }

    @SuppressWarnings("deprecation")
    private static void vibrateLegacy(InputDevice dev, float intensity, int durationMs) {
        Vibrator v = dev.getVibrator();
        if (v != null && v.hasVibrator()) {
            v.vibrate(VibrationEffect.createOneShot(durationMs, toAmplitude(intensity)));
        }
    }

    @SuppressWarnings("deprecation")
    private static void stopRumble(InputDevice dev) {
        if (Build.VERSION.SDK_INT >= 31) {
            VibratorManager vm = dev.getVibratorManager();
            if (vm != null && vm.getVibratorIds().length > 0) {
                vm.cancel();
                return;
            }
        }
        Vibrator v = dev.getVibrator();
        if (v != null && v.hasVibrator()) {
            v.cancel();
        }
    }

    @SuppressWarnings("deprecation")
    private static boolean hasVibrator(InputDevice dev) {
        if (Build.VERSION.SDK_INT >= 31) {
            VibratorManager vm = dev.getVibratorManager();
            if (vm != null) {
                int[] ids = vm.getVibratorIds();
                for (int id : ids) {
                    Vibrator v = vm.getVibrator(id);
                    if (v != null && v.hasVibrator()) return true;
                }
            }
        }
        Vibrator v = dev.getVibrator();
        return v != null && v.hasVibrator();
    }

    private static int toAmplitude(float intensity) {
        return Math.max(1, Math.min(255, Math.round(intensity * 255f)));
    }

    boolean onKeyEvent(KeyEvent e) {
        if (!isGamepadSource(e.getSource())) return false;
        int button = mapKeyCode(e.getKeyCode());
        if (button < 0) return false;
        Pad pad = ensurePad(e.getDeviceId());
        if (pad == null) return false;
        if (e.getAction() == KeyEvent.ACTION_DOWN) {
            pad.buttons[button] = 1f;
        } else if (e.getAction() == KeyEvent.ACTION_UP) {
            pad.buttons[button] = 0f;
        } else {
            return false;
        }
        sendState(pad);
        return true;
    }

    boolean onGenericMotionEvent(MotionEvent e) {
        if ((e.getSource() & InputDevice.SOURCE_JOYSTICK) != InputDevice.SOURCE_JOYSTICK) return false;
        if (e.getAction() != MotionEvent.ACTION_MOVE) return false;
        InputDevice dev = e.getDevice();
        Pad pad = ensurePad(e.getDeviceId());
        if (pad == null) return false;

        pad.axes[AXIS_LEFT_X] = readAxis(e, dev, MotionEvent.AXIS_X);
        pad.axes[AXIS_LEFT_Y] = readAxis(e, dev, MotionEvent.AXIS_Y);
        pad.axes[AXIS_RIGHT_X] = readAxis(e, dev, MotionEvent.AXIS_Z);
        pad.axes[AXIS_RIGHT_Y] = readAxis(e, dev, MotionEvent.AXIS_RZ);

        float lt = Math.max(e.getAxisValue(MotionEvent.AXIS_LTRIGGER), e.getAxisValue(MotionEvent.AXIS_BRAKE));
        float rt = Math.max(e.getAxisValue(MotionEvent.AXIS_RTRIGGER), e.getAxisValue(MotionEvent.AXIS_GAS));
        pad.buttons[BUTTON_LT] = clamp01(lt);
        pad.buttons[BUTTON_RT] = clamp01(rt);

        float hatX = e.getAxisValue(MotionEvent.AXIS_HAT_X);
        float hatY = e.getAxisValue(MotionEvent.AXIS_HAT_Y);
        pad.buttons[BUTTON_DPAD_LEFT] = hatX < -0.5f ? 1f : 0f;
        pad.buttons[BUTTON_DPAD_RIGHT] = hatX > 0.5f ? 1f : 0f;
        pad.buttons[BUTTON_DPAD_UP] = hatY < -0.5f ? 1f : 0f;
        pad.buttons[BUTTON_DPAD_DOWN] = hatY > 0.5f ? 1f : 0f;

        sendState(pad);
        return true;
    }

    private Pad ensurePad(int deviceId) {
        Pad pad = pads.get(deviceId);
        if (pad != null) return pad;
        return maybeConnect(deviceId);
    }

    private Pad maybeConnect(int deviceId) {
        if (manager == null) return null;
        InputDevice dev = manager.getInputDevice(deviceId);
        if (dev == null || !isGamepadSource(dev.getSources())) return null;
        Pad existing = pads.get(deviceId);
        if (existing != null) return existing;
        Pad pad = new Pad(deviceId, dev.getName(), dev.getVendorId(), dev.getProductId(), hasVibrator(dev));
        pads.put(deviceId, pad);
        sendConnect(pad);
        return pad;
    }

    private static boolean isGamepadSource(int sources) {
        return (sources & InputDevice.SOURCE_GAMEPAD) == InputDevice.SOURCE_GAMEPAD
                || (sources & InputDevice.SOURCE_JOYSTICK) == InputDevice.SOURCE_JOYSTICK;
    }

    private static float readAxis(MotionEvent e, InputDevice dev, int axis) {
        float v = e.getAxisValue(axis);
        if (dev != null) {
            InputDevice.MotionRange range = dev.getMotionRange(axis, e.getSource());
            if (range != null && Math.abs(v) <= range.getFlat()) return 0f;
        }
        return clampAxis(v);
    }

    private static int mapKeyCode(int keyCode) {
        switch (keyCode) {
            case KeyEvent.KEYCODE_BUTTON_A: return BUTTON_SOUTH;
            case KeyEvent.KEYCODE_BUTTON_B: return BUTTON_EAST;
            case KeyEvent.KEYCODE_BUTTON_X: return BUTTON_WEST;
            case KeyEvent.KEYCODE_BUTTON_Y: return BUTTON_NORTH;
            case KeyEvent.KEYCODE_BUTTON_L1: return BUTTON_LB;
            case KeyEvent.KEYCODE_BUTTON_R1: return BUTTON_RB;
            case KeyEvent.KEYCODE_BUTTON_L2: return BUTTON_LT;
            case KeyEvent.KEYCODE_BUTTON_R2: return BUTTON_RT;
            case KeyEvent.KEYCODE_BUTTON_SELECT: return BUTTON_BACK;
            case KeyEvent.KEYCODE_BUTTON_START: return BUTTON_START;
            case KeyEvent.KEYCODE_BUTTON_THUMBL: return BUTTON_LEFT_STICK;
            case KeyEvent.KEYCODE_BUTTON_THUMBR: return BUTTON_RIGHT_STICK;
            case KeyEvent.KEYCODE_DPAD_UP: return BUTTON_DPAD_UP;
            case KeyEvent.KEYCODE_DPAD_DOWN: return BUTTON_DPAD_DOWN;
            case KeyEvent.KEYCODE_DPAD_LEFT: return BUTTON_DPAD_LEFT;
            case KeyEvent.KEYCODE_DPAD_RIGHT: return BUTTON_DPAD_RIGHT;
            case KeyEvent.KEYCODE_BUTTON_MODE: return BUTTON_HOME;
            default: return -1;
        }
    }

    private static float clamp01(float v) {
        if (Float.isNaN(v) || v <= 0f) return 0f;
        return Math.min(v, 1f);
    }

    private static float clampAxis(float v) {
        if (Float.isNaN(v)) return 0f;
        return Math.max(-1f, Math.min(1f, v));
    }

    private void sendConnect(Pad pad) {
        byte[] name = pad.name.getBytes(StandardCharsets.UTF_8);
        ByteArrayOutputStream bytes = new ByteArrayOutputStream(18 + name.length);
        DataOutputStream out = new DataOutputStream(bytes);
        try {
            out.writeByte(Protocol.GAMEPAD_CONNECT);
            out.writeInt(pad.id);
            out.writeInt(pad.vendorId);
            out.writeInt(pad.productId);
            out.writeByte(pad.canRumble ? 1 : 0);
            out.writeInt(name.length);
            out.write(name);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        sink.send(bytes.toByteArray());
    }

    private void sendDisconnect(int id) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream(5);
        DataOutputStream out = new DataOutputStream(bytes);
        try {
            out.writeByte(Protocol.GAMEPAD_DISCONNECT);
            out.writeInt(id);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        sink.send(bytes.toByteArray());
    }

    private void sendState(Pad pad) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream(5 + 4 * (BUTTON_COUNT + AXIS_COUNT));
        DataOutputStream out = new DataOutputStream(bytes);
        try {
            out.writeByte(Protocol.GAMEPAD_STATE);
            out.writeInt(pad.id);
            for (float b : pad.buttons) out.writeFloat(b);
            for (float a : pad.axes) out.writeFloat(a);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        sink.send(bytes.toByteArray());
    }
}
