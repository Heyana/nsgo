#!/bin/sh
# Rebuilds the example's DEX JAR. Building the example uses the committed JAR.
set -eu
: "${ANDROID_HOME:?Set ANDROID_HOME to the Android SDK directory}"
dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
android_jar="${ANDROID_PLATFORM_JAR:-$ANDROID_HOME/platforms/android-35/android.jar}"
d8="${ANDROID_D8:-$ANDROID_HOME/build-tools/35.0.0/d8}"
tmp=$(mktemp -d)
trap 'rm -r "$tmp"' EXIT
javac --release 8 -classpath "$android_jar" -d "$tmp" "$dir/BadgeView.java"
"$d8" --min-api 28 --lib "$android_jar" --output "$tmp/BadgeView.jar" "$tmp/dev/nsgo/example/BadgeView.class"
mv "$tmp/BadgeView.jar" "$dir/BadgeView.jar"
