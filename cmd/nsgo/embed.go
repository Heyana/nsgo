package main

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	androidruntime "github.com/oneo-me/nsgo/internal/platform/android/runtime"
)

// nsgo build -embed packages the app for a host app of its own instead of
// as an app: for iOS an XCFramework of a static library per slice with
// NSGo.h, for Android a library (embedAAR). Desktop hosts are Go
// programs, which embed apps with nsgo.NewView.

// embedTargets are the targets -embed builds for.
var embedTargets = []string{"ios", "ios_simulator", "android_arm64", "android_x64"}

// embedExport is the file the tool adds to the app's main package for an
// iOS framework: the one C function the host calls. It is cgo's because
// only cgo exports Go functions; it waits for the Go runtime to start,
// which a static library does on a thread of its own.
const embedExport = `// Added by nsgo build -embed: the framework's NSGoStart.
package main

import "C"

import nsgoembed "github.com/oneo-me/nsgo"

//export NSGoStart
func NSGoStart() { nsgoembed.StartEmbedded(main) }
`

// embedHeader declares what Go registers with the Objective-C runtime:
// objc_runtime_visible classes have no symbols to link, so the compiler
// looks them up by name.
const embedHeader = `// NSGo.h: the NSGo app {{name}} in an iOS app's own views, made by nsgo build -embed.
#import <UIKit/UIKit.h>

NS_ASSUME_NONNULL_BEGIN

/// Starts the app's Go code. Call it once on the main thread, as the app
/// launches and before the first NSGoView: it returns once the app is
/// ready.
void NSGoStart(void);

/// A view of the host's own that the app shows with a Native element of
/// the name it was registered under (NSGoView.register(component:factory:)).
@protocol NSGoComponent <NSObject>
/// The view, which fills the element's box.
@property (nonatomic, readonly) UIView *view;
/// Sets one of the element's props: JSON values as Foundation's objects,
/// nil when the prop goes.
- (void)setProp:(NSString *)name value:(nullable id)value;
@optional
/// Sets a prop the app animates, on the frames it changes; without it
/// setProp receives the number.
- (void)setAnimatedNumber:(NSString *)name value:(double)value;
/// Sets a color prop the app animates, as 0xAARRGGBB, on the frames it
/// changes; without it setProp receives the number.
- (void)setAnimatedColor:(NSString *)name argb:(uint32_t)argb;
/// Runs a command the app sent (core.Ref.Command).
- (void)command:(NSString *)name args:(nullable id)args;
/// Releases what the component holds, once its element or app is gone.
- (void)destroy;
@end

/// How a component talks back to its element: events reach its OnEvent,
/// and a size it asks for is the box's where the app's style leaves it
/// open.
__attribute__((objc_runtime_visible))
@interface NSGoComponentContext : NSObject
- (void)emit:(NSString *)name data:(nullable id)data NS_SWIFT_NAME(emit(_:_:));
- (void)setSize:(CGSize)size;
@end

/// The app in a view. Its app starts once the view is in a window, from
/// the settings made by then, and stops when the view is deallocated or
/// stopped. Several views run at once, each an app of its own. Use it on
/// the main thread; its handlers and components run there.
__attribute__((objc_runtime_visible))
@interface NSGoView : UIView
/// The launch parameters the app reads (core.Scope.HostParams), set
/// before it starts.
@property (nonatomic, copy, nullable) NSDictionary<NSString *, id> *params;
/// Where the app keeps its data, its storage included, set before it
/// starts; Application Support/<the app's ID> when nil.
@property (nonatomic, copy, nullable) NSString *dataDirectory;
/// A BCP 47 locale the app formats for in place of the system's; nil
/// follows the system. overrideUserInterfaceStyle sets the appearance.
@property (nonatomic, copy, nullable) NSString *localeIdentifier;
/// Sets a text size multiplier in place of the system's; 0 follows the
/// system.
- (void)setFontScale:(CGFloat)scale;
/// The values the host themes the app with (core.Env.Token), CSS colors
/// and other strings: tokens in both appearances, darkTokens over them
/// while the app shows dark.
@property (nonatomic, copy, nullable) NSDictionary<NSString *, NSString *> *tokens;
@property (nonatomic, copy, nullable) NSDictionary<NSString *, NSString *> *darkTokens;
/// Hears each failure the app does not catch.
@property (nonatomic, copy, nullable) void (^onError)(NSString *message);
/// Sees the URLs the app opens before the system: YES takes one.
@property (nonatomic, copy, nullable) BOOL (^onOpenURL)(NSURL *url);
/// The status bar the app asks for, for the host's view controller to
/// return from its own.
@property (nonatomic, readonly) UIStatusBarStyle preferredStatusBarStyle;
@property (nonatomic, readonly) BOOL prefersStatusBarHidden;

/// Sends the app an event (core.Scope.OnHostEvent), kept until it has a
/// listener.
- (void)send:(NSString *)name data:(nullable id)data NS_SWIFT_NAME(send(_:_:));
/// Answers the app's calls and posts of name (core.Scope.CallHost):
/// reply once, from any thread, with a result or an error message.
- (void)handle:(NSString *)name
          with:(void (^)(id _Nullable data, void (^reply)(id _Nullable result, NSString *_Nullable error)))handler
    NS_SWIFT_NAME(handle(_:_:));
- (void)removeHandler:(NSString *)name;
/// Registers a component of the host's for this view's app, which shows
/// it with a Native element of name.
- (void)registerComponent:(NSString *)name
                  factory:(id<NSGoComponent> (^)(NSGoComponentContext *context))factory
    NS_SWIFT_NAME(register(component:factory:));
/// Stops the app now.
- (void)stop;
@end

NS_ASSUME_NONNULL_END
`

// embedOnly keeps the targets -embed builds for, the iOS framework once.
func embedOnly(targets []string) ([]string, error) {
	var out []string
	for _, t := range targets {
		if t == "ios_simulator" {
			t = "ios"
		}
		if slices.Contains(embedTargets, t) && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("-embed builds for %s; desktop hosts embed apps with nsgo.NewView", strings.Join(embedTargets, ", "))
	}
	return out, nil
}

// embedModule names the framework's module, which Swift imports: the
// app's name in letters and digits.
func embedModule(p *project) string {
	name := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, p.Name)
	if name == "" || unicode.IsDigit(rune(name[0])) {
		name = "NSGoApp" + name
	}
	return name
}

// buildIOSFramework writes <dir>/<module>.xcframework, the app as a
// static library for devices and for the simulator with NSGo.h, and
// <dir>/<name>Resources, the app's resources, which the host adds to its
// bundle as a folder.
func buildIOSFramework(ctx context.Context, p *project, dir string) error {
	tmp, err := os.MkdirTemp("", "nsgo-embed-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	export := filepath.Join(tmp, "nsgo_embed.go")
	if err := os.WriteFile(export, []byte(embedExport), 0o644); err != nil {
		return err
	}
	overlay := filepath.Join(tmp, "overlay.json")
	added := filepath.Join(p.Dir, "nsgo_embed_export.go")
	if err := os.WriteFile(overlay, fmt.Appendf(nil, `{"Replace":{%q:%q}}`, added, export), 0o644); err != nil {
		return err
	}
	module := embedModule(p)
	args := []string{"-create-xcframework"}
	for _, simulator := range []bool{false, true} {
		sdkName, triple := "iphoneos", "arm64-apple-ios15.0"
		if simulator {
			sdkName, triple = "iphonesimulator", "arm64-apple-ios15.0-simulator"
		}
		sdk, err := output(ctx, "xcrun", "--sdk", sdkName, "--show-sdk-path")
		if err != nil {
			return fmt.Errorf("the iOS SDK needs Xcode: %w", err)
		}
		clang, err := output(ctx, "xcrun", "--sdk", sdkName, "-f", "clang")
		if err != nil {
			return err
		}
		flags := "-isysroot " + sdk + " -target " + triple
		slice := filepath.Join(tmp, sdkName)
		lib := filepath.Join(slice, "lib"+module+".a")
		env := []string{"GOOS=ios", "GOARCH=arm64", "CGO_ENABLED=1",
			"CC=" + clang + " " + flags, "CGO_CFLAGS=" + flags, "CGO_LDFLAGS=" + flags}
		if err := goBuildWith(ctx, p, lib, env, []string{"-buildmode=c-archive", "-overlay", overlay, "-tags", "nsgoembed"}); err != nil {
			return err
		}
		headers := filepath.Join(slice, "Headers")
		if err := os.MkdirAll(headers, 0o755); err != nil {
			return err
		}
		// cgo's header declares Go's types too; the host needs only ours.
		os.Remove(strings.TrimSuffix(lib, ".a") + ".h")
		header := strings.ReplaceAll(embedHeader, "{{name}}", p.Name)
		if err := os.WriteFile(filepath.Join(headers, "NSGo.h"), []byte(header), 0o644); err != nil {
			return err
		}
		// What Go's runtime and net link, autolinked by the module.
		modulemap := fmt.Sprintf("module %s {\n\theader \"NSGo.h\"\n\tlink framework \"UIKit\"\n"+
			"\tlink framework \"Security\"\n\tlink \"resolv\"\n\texport *\n}\n", module)
		if err := os.WriteFile(filepath.Join(headers, "module.modulemap"), []byte(modulemap), 0o644); err != nil {
			return err
		}
		args = append(args, "-library", lib, "-headers", headers)
	}
	framework := filepath.Join(dir, module+".xcframework")
	if err := run(ctx, "xcodebuild", append(args, "-output", framework)...); err != nil {
		return err
	}
	resources := filepath.Join(dir, p.Name+"Resources")
	if err := os.MkdirAll(resources, 0o755); err != nil {
		return err
	}
	return copyResources(p, resources)
}

// aarManifest is the library's manifest, which a host's build merges into
// its own: the runtime starts the app from the native library directory,
// so native libraries must be extracted, and the clipboard's provider is
// named after the host's package.
const aarManifest = `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="dev.oneo.nsgo">
    <uses-sdk android:minSdkVersion="28" />
    <uses-permission android:name="android.permission.INTERNET" />
    <uses-permission android:name="android.permission.ACCESS_NETWORK_STATE" />
    <application android:extractNativeLibs="true">
        <provider
            android:name="dev.oneo.nsgo.ClipProvider"
            android:authorities="${applicationId}.nsgo.clip"
            android:exported="false"
            android:grantUriPermissions="true" />
    </application>
</manifest>
`

// buildAndroidAAR writes <dir>/<name>.aar for a host app's Gradle build:
// the Java runtime's classes as a jar, the app's executable for goarch as
// libnsgo.so and its resources as assets.
func buildAndroidAAR(ctx context.Context, p *project, dir, goarch string) error {
	tmp, err := os.MkdirTemp("", "nsgo-aar-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	app, abi, err := buildAndroidBinary(ctx, p, tmp, goarch)
	if err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, p.Name+".aar"))
	if err != nil {
		return err
	}
	defer f.Close()
	z := zip.NewWriter(f)
	add := func(name string, data []byte) error {
		w, err := z.Create(name)
		if err == nil {
			_, err = w.Write(data)
		}
		return err
	}
	for _, e := range []struct {
		name string
		data []byte
	}{
		{"AndroidManifest.xml", []byte(aarManifest)},
		{"classes.jar", androidruntime.ClassesJar},
		{"R.txt", nil},
		{"jni/" + abi + "/libnsgo.so", app},
	} {
		if err := add(e.name, e.data); err != nil {
			return err
		}
	}
	err = walkResources(p, func(rel string, data []byte) error { return add("assets/"+rel, data) })
	if err != nil {
		return err
	}
	return z.Close()
}
