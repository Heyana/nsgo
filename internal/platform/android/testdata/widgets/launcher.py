#!/usr/bin/env python3
"""Drive the opt-in native widget test through an Android launcher's pin UI.

Start go tool nsgo test with NSGO_ANDROID_WIDGET_ACCEPTANCE=1 separately, then
run this controller with --serial. It only writes its installed test package's
private acknowledgement files and saves screenshots under --out.
"""
import argparse
import pathlib
import re
import struct
import subprocess
import time
import xml.etree.ElementTree as ET
import zlib


def command(*args, check=True):
    return subprocess.run([options.adb, "-s", options.serial, *args], check=check,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE).stdout


def private(*args, check=True):
    return command("shell", "run-as", options.package, *args, check=check)


def rgba(data):
    """Decode the RGB/RGBA 8-bit screenshots screencap produces, using stdlib."""
    if data[:8] != b"\x89PNG\r\n\x1a\n":
        raise ValueError("not a PNG")
    offset, compressed = 8, bytearray()
    while offset < len(data):
        length = struct.unpack_from(">I", data, offset)[0]
        kind = data[offset + 4:offset + 8]
        payload = data[offset + 8:offset + 8 + length]
        if kind == b"IHDR":
            width, height, depth, color, _, _, interlace = struct.unpack(">IIBBBBB", payload)
            if depth != 8 or color not in (2, 6) or interlace:
                raise ValueError("unsupported screenshot format")
            channels = 4 if color == 6 else 3
        elif kind == b"IDAT":
            compressed.extend(payload)
        offset += 12 + length
    raw = zlib.decompress(compressed)
    stride = width * channels
    previous = bytearray(stride)
    rows, offset = [], 0
    for _ in range(height):
        filtering = raw[offset]
        current = bytearray(raw[offset + 1:offset + stride + 1])
        offset += stride + 1
        for x in range(stride):
            left = current[x - channels] if x >= channels else 0
            up = previous[x]
            upper_left = previous[x - channels] if x >= channels else 0
            if filtering == 1:
                prediction = left
            elif filtering == 2:
                prediction = up
            elif filtering == 3:
                prediction = (left + up) // 2
            elif filtering == 4:
                p = left + up - upper_left
                distances = (abs(p - left), abs(p - up), abs(p - upper_left))
                prediction = (left, up, upper_left)[distances.index(min(distances))]
            elif filtering == 0:
                prediction = 0
            else:
                raise ValueError("unknown PNG filter")
            current[x] = (current[x] + prediction) & 255
        rows.append(current)
        previous = current
    return width, height, channels, rows


def visible(color, filename):
    data = command("exec-out", "screencap", "-p")
    width, height, channels, rows = rgba(data)
    columns = (width + 1) // 2
    mask = set()
    for y in range(0, height, 2):
        row = rows[y]
        for x in range(0, width, 2):
            at = x * channels
            if all(abs(row[at + c] - color[c]) < 14 for c in range(3)):
                mask.add((y // 2) * columns + x // 2)
    largest = []
    while mask:
        first = mask.pop()
        component, stack = [first], [first]
        while stack:
            at = stack.pop()
            neighbors = [at - columns, at + columns]
            if at % columns:
                neighbors.append(at - 1)
            if at % columns < columns - 1:
                neighbors.append(at + 1)
            for neighbor in neighbors:
                if neighbor in mask:
                    mask.remove(neighbor)
                    component.append(neighbor)
                    stack.append(neighbor)
        if len(component) > len(largest):
            largest = component
    if len(largest) < 1000:
        return None
    x0 = min(at % columns for at in largest) * 2
    x1 = max(at % columns for at in largest) * 2
    y0 = min(at // columns for at in largest) * 2
    y1 = max(at // columns for at in largest) * 2
    if x1 - x0 < 80 or y1 - y0 < 80:
        return None
    (options.out / filename).write_bytes(data)
    return (x0 + x1) // 2, (y0 + y1) // 2, x0, x1


def home_if_app_foreground():
    state = command("shell", "dumpsys", "activity", "activities").decode()
    resumed = [line for line in state.splitlines() if "topResumedActivity=" in line or "mResumedActivity=" in line]
    if any(options.package in line for line in resumed):
        command("shell", "input", "keyevent", "KEYCODE_HOME")


def confirm_pin():
    command("shell", "uiautomator", "dump", "/sdcard/nsgo1469-window.xml", check=False)
    data = command("shell", "cat", "/sdcard/nsgo1469-window.xml", check=False)
    try:
        root = ET.fromstring(data)
    except ET.ParseError:
        return False
    candidates = []
    for node in root.iter("node"):
        text = node.attrib.get("text", "").lower()
        resource = node.attrib.get("resource-id", "")
        if (resource.endswith("/button1") or text in ("add", "add automatically", "add to home screen", "add widget")) and node.attrib.get("clickable") == "true":
            bounds = list(map(int, re.findall(r"\d+", node.attrib.get("bounds", ""))))
            if len(bounds) == 4:
                candidates.append(bounds)
    if not candidates:
        return False
    x0, y0, x1, y1 = candidates[-1]
    command("shell", "input", "tap", str((x0 + x1) // 2), str((y0 + y1) // 2))
    time.sleep(0.5)
    home_if_app_foreground()
    return True


parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--adb", default="adb")
parser.add_argument("--serial", required=True)
parser.add_argument("--package", default="dev.nsgo.tests.android.nsgotest")
parser.add_argument("--out", type=pathlib.Path, required=True)
options = parser.parse_args()
options.out.mkdir(parents=True, exist_ok=True)
acknowledged, pinned, tapped = set(), False, set()
last_phase = ""
deadline = time.monotonic() + 900
started = False
try:
    while time.monotonic() < deadline:
        phase = private("cat", "files/NSGoWidgets/accept-phase", check=False).decode().strip()
        if phase != last_phase and phase:
            print("widget controller:", phase, flush=True)
            last_phase = phase
            if not started:
                started = True
                deadline = time.monotonic() + 240
        if phase == "pin-requested" and not pinned:
            pinned = confirm_pin()
        for name, color in (("red-visible", (192, 32, 32)), ("green-visible", (32, 128, 64)), ("blue-visible", (32, 64, 176))):
            if phase != name or name in acknowledged:
                continue
            if name == "blue-visible":
                home_if_app_foreground()
            center = visible(color, name + ".png")
            if center:
                private("touch", "files/NSGoWidgets/accept-" + name)
                acknowledged.add(name)
                print("widget controller: captured", name, flush=True)
        if phase in ("tap-green-link", "tap-green-entry") and phase not in tapped:
            home_if_app_foreground()
            center = visible((32, 128, 64), "green-before-tap.png")
            if center:
                command("shell", "input", "tap", str(center[0] if phase == "tap-green-link" else center[2] + 4), str(center[1]))
                tapped.add(phase)
        if phase == "complete" or ("blue-visible" in acknowledged and len(tapped) == 2):
            print("widget controller: complete", flush=True)
            break
        time.sleep(0.4)
    else:
        raise SystemExit("widget controller timed out at " + last_phase)
finally:
    command("shell", "rm", "-f", "/sdcard/nsgo1469-window.xml", check=False)
