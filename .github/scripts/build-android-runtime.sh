#!/bin/sh
# Builds internal/platform/android/runtime's classes.dex and classes.jar with its build.sh on a CI runner, after installing the Android platform 36 and build-tools 36 the runner image lacks. test.yml's android job, release.yml's runtime check and android-runtime.yml share it, so all three build the runtime the same way; each sets up JDK 21 first.
set -eu

: "${ANDROID_HOME:?set ANDROID_HOME to the Android SDK of the runner}"
# build.sh needs platform 36 and a d8 from cmdline-tools or build-tools 36+.
if [ ! -f "$ANDROID_HOME/platforms/android-36/android.jar" ] || { [ ! -x "$ANDROID_HOME/cmdline-tools/latest/bin/d8" ] && ! ls "$ANDROID_HOME"/build-tools/3[6-9]*/d8 >/dev/null 2>&1; }; then
	(yes || true) | "$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager" --install "platforms;android-36" "build-tools;36.0.0" >/dev/null
fi
exec internal/platform/android/runtime/build.sh
