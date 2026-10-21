#!/bin/sh
# Builds a Go main package for the iOS simulator, wraps it in an .app and
# runs it on the booted simulator (or boots the first iPhone).
# Usage: scripts/ios-sim.sh <package> <bundle id> [name]
set -eu
pkg=$1
bundle_id=$2
name=${3:-$(basename "$pkg")}
out=${OUT:-.nsgo/ios}
app="$out/$name.app"

sdk=$(xcrun --sdk iphonesimulator --show-sdk-path)
clang=$(xcrun --sdk iphonesimulator -f clang)
mkdir -p "$app"
CGO_ENABLED=1 GOOS=ios GOARCH=arm64 \
	CC="$clang -isysroot $sdk -target arm64-apple-ios15.0-simulator" \
	CGO_CFLAGS="-isysroot $sdk -target arm64-apple-ios15.0-simulator" \
	CGO_LDFLAGS="-isysroot $sdk -target arm64-apple-ios15.0-simulator" \
	go build -o "$app/$name" "$pkg"

cat > "$app/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key><string>$name</string>
	<key>CFBundleIdentifier</key><string>$bundle_id</string>
	<key>CFBundleName</key><string>$name</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleShortVersionString</key><string>0.1.0</string>
	<key>CFBundleVersion</key><string>1</string>
	<key>MinimumOSVersion</key><string>15.0</string>
	<key>UILaunchScreen</key><dict/>
	<key>UIApplicationSceneManifest</key>
	<dict>
		<key>UIApplicationSupportsMultipleScenes</key><false/>
		<key>UISceneConfigurations</key>
		<dict>
			<key>UIWindowSceneSessionRoleApplication</key>
			<array><dict>
				<key>UISceneConfigurationName</key><string>Default</string>
				<key>UISceneDelegateClassName</key><string>NsgoSceneDelegate</string>
			</dict></array>
		</dict>
	</dict>
	<key>UIRequiresFullScreen</key><true/>
	<key>UIDeviceFamily</key><array><integer>1</integer><integer>2</integer></array>
</dict>
</plist>
PLIST
codesign --force --sign - "$app" >/dev/null

if ! xcrun simctl list devices booted | grep -q Booted; then
	device=$(xcrun simctl list devices available | grep -m1 iPhone | sed -E 's/.*\(([0-9A-F-]{36})\).*/\1/')
	xcrun simctl boot "$device"
fi
xcrun simctl install booted "$app"
xcrun simctl launch booted "$bundle_id"
