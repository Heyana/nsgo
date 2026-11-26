// Package runtime holds the prebuilt parts of every APK: the Java
// runtime in ../java, compiled to dex, and the manifest nsgo build fills
// in for each app; and the same runtime as a jar, for the libraries
// nsgo build -embed makes. The nsgo tool embeds them, so packaging needs
// neither a JDK nor the Android SDK.
//
// The Java runtime cannot be built with Go, so classes.dex and classes.jar are committed. Pull requests change only ../java or the runtime's build inputs: CI builds them with build.sh for its tests, and a release commits the rebuilt files to main in its changelog pull request (go run ./tools/changelog release X.Y.Z builds them with android-runtime.yml when the Java changed), as maintainers otherwise do in a bot/android-runtime pull request. Run build.sh to try a change on a device, without committing what it writes.
package runtime

import _ "embed"

// ClassesDex is the Java runtime and Play Review/In-App Updates SDKs compiled for Android, built by d8.
//
//go:embed classes.dex
var ClassesDex []byte

// ClassesJar holds NSGo's Java classes for a host app's build and the standalone SDK's license metadata; a Gradle host supplies its own SDK.
//
//go:embed classes.jar
var ClassesJar []byte

// Manifest describes an NSGo app before packaging fills in its package
// name, version and label.
//
//go:embed AndroidManifest.xml
var Manifest []byte
