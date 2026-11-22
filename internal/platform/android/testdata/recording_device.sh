#!/bin/sh
# Opt-in acceptance on a dedicated test device: serial, then seconds (1800 by default). Records only in the test APK; cleanup uninstalls that APK and restores runtime files.
set -eu

if [ "$#" -lt 1 ]; then echo "pass an adb serial" >&2; exit 2; fi
device=$1
seconds=1800
if [ "$#" -ge 2 ]; then seconds=$2; fi
case "$seconds" in ''|*[!0-9]*) echo "duration must be seconds" >&2; exit 2;; esac
if [ "$seconds" -lt 15 ] || [ "$seconds" -gt 3600 ]; then
    echo "duration must be between 15 and 3600 seconds" >&2
    exit 2
fi
test=TestNativeAndroidRecordingAcceptance
if [ "$#" -ge 3 ]; then
    case "$3" in
        acceptance) ;;
        background-refusal) test=TestNativeAndroidRecordingBackgroundRefusal;;
        *) echo "mode must be acceptance or background-refusal" >&2; exit 2;;
    esac
fi
cd "$(CDPATH= cd -- "$(dirname -- "$0")/../../../.." && pwd)"
app=dev.nsgo.tests.android.nsgotest
temporary=$(mktemp -d)
log=$temporary/test.log
runtime=internal/platform/android/runtime
cp "$runtime/classes.dex" "$temporary/classes.dex"
cp "$runtime/classes.jar" "$temporary/classes.jar"
runner=
stop_tree() {
    for child in $(pgrep -P "$1" 2>/dev/null || true); do stop_tree "$child"; done
    kill "$1" 2>/dev/null || true
}
cleanup() {
    if [ -n "$runner" ] && kill -0 "$runner" 2>/dev/null; then stop_tree "$runner"; fi
    adb -s "$device" shell input keyevent 224 >/dev/null 2>&1 || true
    adb -s "$device" uninstall "$app" >/dev/null 2>&1 || true
    cp "$temporary/classes.dex" "$runtime/classes.dex"
    cp "$temporary/classes.jar" "$runtime/classes.jar"
    find "$temporary" -type f -delete
    rmdir "$temporary"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

wait_for() {
    phase=$1
    attempts=$2
    while ! grep -q "$phase" "$log" 2>/dev/null; do
        if grep -Eq '✗|error .*failed|tests did not build' "$log" 2>/dev/null; then cat "$log"; exit 1; fi
        attempts=$((attempts-1))
        if [ "$attempts" -le 0 ]; then echo "timed out waiting for $phase" >&2; cat "$log"; exit 1; fi
        sleep 1
    done
}

sh "$runtime/build.sh"
go tool nsgo test -target android -device "$device" -run "^$test$" \
    -env "NSGO_ANDROID_RECORDING_ACCEPTANCE=$seconds"s -timeout "$((seconds+180))s" \
    -trace ./internal/platform/android >"$log" 2>&1 &
runner=$!
wait_for 'recording acceptance waiting-for-permission' 600
adb -s "$device" shell pm grant "$app" android.permission.RECORD_AUDIO
api=$(adb -s "$device" shell getprop ro.build.version.sdk | tr -d '\r')
if [ "$api" -ge 33 ]; then
    adb -s "$device" shell pm grant "$app" android.permission.POST_NOTIFICATIONS
fi
adb -s "$device" shell run-as "$app" touch cache/nsgo-recording-test-ready
wait_for 'recording acceptance ready-for-screen-off' 90
adb -s "$device" shell input keyevent 3
adb -s "$device" shell input keyevent 223
wait_for 'recording acceptance ready-to-stop' "$((seconds+120))"
adb -s "$device" shell input keyevent 224
adb -s "$device" shell wm dismiss-keyguard
# The test launcher has no MAIN action. Reuse its Activity by component.
adb -s "$device" shell am start -f 0x34000000 -n "$app/dev.oneo.nsgo.NsgoActivity" >/dev/null
wait_for 'recording acceptance complete' 90
code=0
wait "$runner" || code=$?
runner=
cat "$log"
exit "$code"
