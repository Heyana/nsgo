// Package runtime holds the prebuilt parts of every APK: the Java
// runtime in ../java, compiled to dex, and the manifest nsgo build fills
// in for each app. The nsgo tool embeds both, so packaging needs neither
// a JDK nor the Android SDK.
//
// The Java runtime cannot be built with Go, so classes.dex is committed.
// Rebuild it with build.sh after changing ../java.
package runtime

import _ "embed"

// ClassesDex is the Java runtime compiled for Android, built by d8.
//
//go:embed classes.dex
var ClassesDex []byte

// Manifest describes an NSGo app before packaging fills in its package
// name, version and label.
//
//go:embed AndroidManifest.xml
var Manifest []byte
