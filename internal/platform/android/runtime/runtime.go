// Package runtime holds the prebuilt parts of every APK: the Java
// runtime in ../java, compiled to dex, and the manifest nsgo build fills
// in for each app; and the same runtime as a jar, for the libraries
// nsgo build -embed makes. The nsgo tool embeds them, so packaging needs
// neither a JDK nor the Android SDK.
//
// The Java runtime cannot be built with Go, so classes.dex and
// classes.jar are committed. Rebuild them with build.sh after changing
// ../java.
package runtime

import _ "embed"

// ClassesDex is the Java runtime compiled for Android, built by d8.
//
//go:embed classes.dex
var ClassesDex []byte

// ClassesJar is the Java runtime's classes, for a host app's build.
//
//go:embed classes.jar
var ClassesJar []byte

// Manifest describes an NSGo app before packaging fills in its package
// name, version and label.
//
//go:embed AndroidManifest.xml
var Manifest []byte
