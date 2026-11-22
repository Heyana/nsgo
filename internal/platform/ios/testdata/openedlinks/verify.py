#!/usr/bin/env python3
"""Verify cold/warm system URL delivery to this owned full-app fixture."""

import argparse
import json
import pathlib
import plistlib
import subprocess
import sys
import time
import uuid


def simctl(*args, check=True):
    return subprocess.run(["xcrun", "simctl", *args], check=check,
                          capture_output=True, text=True, timeout=90)


def await_proof(path, predicate):
    deadline = time.monotonic() + 60
    latest = None
    while time.monotonic() < deadline:
        try:
            latest = json.loads(path.read_text())
            if predicate(latest):
                return latest
        except (FileNotFoundError, json.JSONDecodeError):
            pass
        time.sleep(0.1)
    raise AssertionError(f"fixture did not reach expected invocation state: {latest}")


def approve_fixture(opts):
    command = [str(opts.idb), "--companion", opts.companion]
    state = subprocess.run(command + ["ui", "describe-all", "--format", "complete", "--json"],
                           check=True, capture_output=True, text=True, timeout=15)
    elements = []

    def walk(items):
        for item in items:
            elements.append(item)
            walk(item.get("children", []))

    walk(json.loads(state.stdout)["elements"])
    if not any("OpenedLinks1474" in (item.get("label") or "") for item in elements):
        return False
    buttons = [item for item in elements if item.get("type") == "Button"]
    opens = [item for item in buttons if item.get("label") in ("Open", "打开")]
    if len(buttons) != 2 or len(opens) != 1:
        return False
    subprocess.run(command + ["ui", "tap", opens[0]["label"], "--api", "ax", "--match-key", "AXLabel", "--reason",
                              "Approve the owned OpenedLinks1474 fixture's system URL handoff"],
                   check=True, timeout=15)
    return True


def send_link(opts, device, link):
    # A separate native app calls UIApplication.openURL, letting the
    # system create the receiver's real scene contexts. Current iOS asks
    # for confirmation, which idb can tap on this owned simulator only.
    repo = pathlib.Path(__file__).resolve().parents[5]
    command = ["go", "tool", "nsgo", "test", "-target", "ios_simulator",
               "-device", device, "-env", "NSGO_IOS_LINK1474_URL=" + link,
               "-run", "^TestSystemFullAppLinkDelivery$",
               "./internal/platform/ios", "--", "-count=1"]
    with subprocess.Popen(command, cwd=repo) as sender:
        deadline = time.monotonic() + 240
        approved = False
        if not opts.idb:
            print("Approve the OpenedLinks1474 confirmation on the owned simulator.", file=sys.stderr)
        try:
            while sender.poll() is None:
                if time.monotonic() >= deadline:
                    raise TimeoutError("native link sender timed out")
                if opts.idb and not approved:
                    approved = approve_fixture(opts)
                time.sleep(0.1)
            if sender.returncode:
                raise subprocess.CalledProcessError(sender.returncode, command)
        finally:
            if sender.poll() is None:
                sender.terminate()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--device", required=True, help="owned simulator UUID")
    parser.add_argument("--app", type=pathlib.Path, required=True)
    parser.add_argument("--idb", type=pathlib.Path, help="optional idb client to tap the owned fixture's confirmation")
    parser.add_argument("--companion", help="idb companion for this same owned simulator")
    opts = parser.parse_args()
    device = str(uuid.UUID(opts.device)).upper()
    assert bool(opts.idb) == bool(opts.companion), "pass both --idb and --companion, or approve manually"
    if opts.idb:
        target = subprocess.run([str(opts.idb), "--companion", opts.companion, "describe", "--json"],
                                check=True, capture_output=True, text=True, timeout=15)
        target = json.loads(target.stdout)
        assert target["target_type"] == "simulator" and target["udid"].upper() == device, \
            "idb must target this same owned simulator"
    with (opts.app / "Info.plist").open("rb") as source:
        info = plistlib.load(source)
    bundle = info["CFBundleIdentifier"]
    assert bundle == "dev.nsgo.openedlinks1474" and "NSAppClip" not in info
    assert any("nsgo1474" in item.get("CFBundleURLSchemes", [])
               for item in info.get("CFBundleURLTypes", []))
    assert simctl("get_app_container", device, bundle, "data", check=False).returncode != 0, \
        "refusing to replace a previously installed fixture"
    installed = False
    try:
        simctl("install", device, str(opts.app))
        installed = True
        cold = "nsgo1474://order?table=12"
        send_link(opts, device, cold)
        container = pathlib.Path(simctl("get_app_container", device, bundle, "data").stdout.strip())
        path = container / "tmp" / "nsgo1474-invocations.json"
        first = await_proof(path, lambda p: p["Initial"] == cold and p["Links"] == [cold]
                            and p["HandlerURLs"] == [cold] and p["URLs"] == [cold])
        pid, started = first["PID"], first["Started"]
        expected = [cold]
        for link in ("nsgo1474://order?table=21#detail", "nsgo1474://order?table=31&label=%E4%B8%96"):
            expected.append(link)
            send_link(opts, device, link)
            current = await_proof(path, lambda p: p["Links"] == expected and p["HandlerURLs"] == expected
                                  and p["URLs"] == expected)
            assert current["PID"] == pid and current["Started"] == started, "warm URL restarted the app"
        simctl("terminate", device, bundle)
        restart = "nsgo1474://order?table=41"
        send_link(opts, device, restart)
        final = await_proof(path, lambda p: p["Started"] != started and p["Initial"] == restart
                            and p["Links"] == [restart] and p["HandlerURLs"] == [restart]
                            and p["URLs"] == [restart])
        print(json.dumps({"cold": cold, "warm": expected[1:], "cold_restart": restart,
                          "same_warm_process": True, "full_app": True,
                          "restart_process_changed": final["Started"] != started}))
    finally:
        if installed:
            simctl("terminate", device, bundle, check=False)
            simctl("uninstall", device, bundle)


if __name__ == "__main__":
    main()
