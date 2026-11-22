package driver

import (
	"reflect"
	"strconv"
	"time"
)

// Environment is what the platform reports about where the app runs. The
// surface size is not part of it: Tree.Resize reports that.
type Environment struct {
	// Network is connectivity when a platform or test supplies it with its environment.
	Network *Network
	// DeviceName is the name the user gave the device, empty where the platform keeps it private (core.Env.DeviceName).
	DeviceName string
	// DeviceModel is the device's hardware model, empty where it names none (core.Env.DeviceModel).
	DeviceModel string
	// Displays are the desktop's displays, in screen coordinates. They are
	// empty on targets without desktop displays.
	Displays []DisplayInfo
	// SafeArea is the part of each edge that system bars, notches and
	// rounded corners cover.
	SafeArea Edges
	// TitleBar is what a hidden desktop title bar still covers: its
	// height on top, its buttons on the left or right.
	TitleBar Edges
	// WindowMaterial is the material behind the window's content; None
	// when the window is opaque.
	WindowMaterial WindowMaterial
	// WindowTransparent is whether the surface's window or panel draws nothing of its own now: a panel opened with SurfaceConfig.Transparent, or a window opened so or made so by WindowSetTransparent; false where the platform could not make it so, and for any other surface. CanMakeWindowsTransparent is whether a window could be.
	WindowTransparent bool
	// Maximized is whether the window is maximized or zoomed, so an
	// app-drawn maximize button can show restore.
	Maximized bool
	// FullScreen is whether the window fills its screen in full screen.
	FullScreen bool
	// Minimized is whether the surface's window is minimized, an embedded surface's host window; false without desktop windows and where the compositor reports no minimized state.
	Minimized bool
	// WindowFrame is where the window is and how large, as WindowSetFrame
	// takes them; zero without a window, and a position only where the
	// platform knows one (Move).
	WindowFrame WindowFrame
	// KeyboardHeight is how much of the bottom the software keyboard
	// covers; KeyboardDuration is how long it takes to move.
	KeyboardHeight   Points
	KeyboardDuration time.Duration
	Dark             bool
	// FontScale multiplies text sizes, as the user's text size setting
	// asks; 0 means 1.
	FontScale    float32
	ReduceMotion bool
	// IncreaseContrast says the user asked the system for stronger contrast.
	IncreaseContrast bool
	ScreenReader     bool
	// ScreenColor says ItemPickScreenColor works here.
	ScreenColor bool
	// Speech says ItemSpeech recognizes speech here, Speak that ItemSpeak speaks text, Audio that ItemAudio plays sounds and players, Gamepads that package gamepad reads game controllers, GPU that KindGPU views can create a GPU device here.
	Speech   bool
	Speak    bool
	Audio    bool
	Gamepads bool
	GPU      bool
	// AudioSchedule says ItemAudio honors AudioPlay's StartAt.
	AudioSchedule bool
	// AudioPlaybackControls says sound playbacks can be addressed and report when they end.
	AudioPlaybackControls bool
	// HapticPatterns reports support for playing timed haptic events.
	HapticPatterns bool
	// Camera says ItemTakePhoto shows a camera here.
	Camera bool
	// ScanCode says ItemScanCode shows a camera that reads codes here.
	ScanCode bool
	// DiscoverServices says the target can advertise and browse DNS-SD services.
	DiscoverServices bool
	// CanSendInput says Ref.SendInput can deliver trusted input to web views.
	CanSendInput bool
	// CanLockPointer says ItemPointerLock can capture relative motion.
	CanLockPointer bool
	// CanLookUp says ItemLookUp can show the system dictionary panel.
	CanLookUp bool
	// CanQuickLook says ItemQuickLook can show the system's file preview panel.
	CanQuickLook bool
	// CanSetWindowLevel says WindowSetLevel reaches the window's stacking, and CanPlaceWindows that WindowSetFrame's position moves it; false without a desktop window of the surface's own (an embedded view, the app's own hidden surface, a tray icon), the first for an Android overlay, which has no level, and the second on Linux, whose window manager or compositor places windows.
	CanSetWindowLevel bool
	CanPlaceWindows   bool
	// CanSetWindowWorkspaces says WindowSetWorkspaces reaches the window's workspaces: on macOS, and on X11 where the window manager lists _NET_WM_DESKTOP in _NET_SUPPORTED; false without a desktop window of the surface's own (an embedded view, the app's own hidden surface, a tray icon, an unmanaged X11 panel), on Windows, which has no public API for it, on Wayland, phones and the web.
	CanSetWindowWorkspaces bool
	// HasWebView says this target provides a web view host.
	HasWebView bool
	// SignInWithApple says ItemSignInWithApple and ItemAppleCredentialState reach Sign in with Apple here.
	SignInWithApple bool
	// Purchases says ItemPurchase reaches the platform's store here.
	Purchases bool
	// CanRequestReview says ItemRequestReview can ask the platform's store to show an in-app rating prompt.
	CanRequestReview bool
	// CanUseGameServices says package gameservices reaches a platform game service here: Game Center on macOS, iOS and tvOS where GameKit loads, and Google Play Games Services on Android where available; false on watchOS, Windows, Linux, the web and mini programs.
	CanUseGameServices bool
	// ScheduledNotifications says ItemShowNotification schedules a notification for a later At, which the system shows with the app's process ended.
	ScheduledNotifications bool
	// CanMakeWindowsTransparent says SurfaceConfig.Transparent and WindowSetTransparent show what is behind the surface's window wherever the app draws nothing, and CanSkipTaskbar that SurfaceConfig.SkipTaskbar and WindowSetSkipTaskbar keep it off the taskbar; both false without a desktop window of the surface's own (an embedded view, the app's own hidden surface, a panel, a tray icon), and where the platform or session has no way (see desktop.CanMakeWindowsTransparent and desktop.CanSkipTaskbar).
	CanMakeWindowsTransparent bool
	CanSkipTaskbar            bool
	// VerticalBarEdge is the edge the system chose for a vertical bar where it may place one, as iOS 27.1 does; Unspecified elsewhere. It is not geometry: Tree.Resize reports that.
	VerticalBarEdge VerticalBarEdge
	// RTL lays the app out right to left.
	RTL bool
	// PixelRatio is device pixels per point; 0 means 1.
	PixelRatio float32
	// Locale is a BCP 47 language tag, such as "en-US".
	Locale string
	// LocaleSettings are how the user reads weeks, times and measures,
	// apart from the language, where the platform says.
	LocaleSettings LocaleSettings
	OS             OS
	// OSVersion is the system's version: major and minor on Apple
	// platforms, Windows and Linux (the kernel's), Android's API level
	// as Major, and zero on the web.
	OSVersion OSVersion
	Idiom     Idiom
	// Pointer is the precision of the primary pointing device when the platform reports it directly (such as CSS's pointer media query on the web); zero derives it from Idiom.
	Pointer Pointer
	// Simulated is whether OS and Idiom are ones the platform imitates rather than runs on, as a web build does for --nsgo-os, or whose native parts it lacks, as a mini program on its device's OS: the app's own views take that OS's paths, while its native components, system header, menus and window functions are absent.
	Simulated bool
	// AppleKeyboard says a browser or a mini program runs on an Apple platform, where "mod" is Command as on macOS and iOS.
	AppleKeyboard bool
	// KeyCodeNames maps physical positions to unmodified key names on the current keyboard layout, where the platform provides the map.
	KeyCodeNames map[string]string
	// HeaderHeight and LargeHeaderHeight are the heights of the system
	// header a platform draws over screens, without the status bar, and
	// with a large title. 0 where screens show their Header instead.
	HeaderHeight, LargeHeaderHeight Points
	// Lifecycle is whether the app is in use: the window's, an embedded
	// surface's the host window's.
	Lifecycle Lifecycle
	// ThermalState and LowPowerMode describe system power constraints.
	ThermalState ThermalState
	LowPowerMode LowPowerMode
	// AppClip says the app runs as an iOS App Clip, and OpenedURL is the
	// web link that last opened it: the clip's invocation, or a universal
	// link; "" when none did.
	AppClip   bool
	OpenedURL string
	// PageURL and PageBaseURL initialize the application address when the
	// surface owns its browser history. Empty on other surfaces and targets.
	PageURL, PageBaseURL string
	// LaunchedAtLogin says the system started this run as a login item. It stays the same throughout the run, including subsequent reopens.
	LaunchedAtLogin bool
	// FrontmostApp is the application in front on the desktop, the app itself included, and CanReadFrontmostApp says the platform follows it: on macOS, Windows and Linux on X11. FrontmostApp is zero where CanReadFrontmostApp is false.
	FrontmostApp        FrontmostAppInfo
	CanReadFrontmostApp bool
	// CustomCursor says the platform shows image-based custom cursors (Style.CursorImage): true on macOS, Windows, Linux and the web; false on touch platforms.
	CustomCursor bool
}

// ThermalState is the system's current thermal pressure.
type ThermalState uint8

const (
	ThermalUnknown ThermalState = iota
	ThermalNominal
	ThermalFair
	ThermalSerious
	ThermalCritical
)

// LowPowerMode says whether the system's energy-saving mode is on.
type LowPowerMode uint8

const (
	LowPowerUnknown LowPowerMode = iota
	LowPowerOff
	LowPowerOn
)

// Network is the device's connectivity as its system reports it.
type Network struct {
	// Online says the system considers a network route usable.
	Online bool
	// Expensive says the system considers that route metered or expensive.
	Expensive bool
}

// NetworkWatcher supplies connectivity when an app reads its network state.
// Keeping its method behind an interface lets apps that never read that state
// leave the native watcher out of their binary.
type NetworkWatcher interface {
	// WatchNetwork returns the initial state and a stop function, or nil
	// when unsupported. Changes can arrive on any goroutine.
	WatchNetwork(changed func(Network)) (initial Network, stop func())
}

// FrontmostAppInfo is the application in front on the desktop.
type FrontmostAppInfo struct {
	// ID is the macOS bundle identifier, else the executable's path; the Windows executable's path; or on X11 the GTK application ID, else the class of WM_CLASS. "" where the platform will not say, as Windows for a process it refuses to open.
	ID string
	// Name is the app's display name: macOS's localized name, the executable's file description on Windows, else its file name, the class of WM_CLASS on X11.
	Name string
	// PID is its process ID, 0 when unknown.
	PID int
	// FullScreen is whether its frontmost window covers its display: in full screen, not merely maximized.
	FullScreen bool
}

// Equal reports whether two platform environments have the same values.
// Environment includes a slice of displays, so it cannot use ==.
func (e Environment) Equal(other Environment) bool { return reflect.DeepEqual(e, other) }

// VerticalBarEdge is the edge where the system places a vertical bar.
type VerticalBarEdge uint8

const (
	// VerticalBarUnspecified is a system without a preferred edge, or one that places no vertical bar.
	VerticalBarUnspecified VerticalBarEdge = iota
	VerticalBarLeading
	VerticalBarTrailing
)

// String returns the edge's name: "unspecified", "leading" or "trailing".
func (e VerticalBarEdge) String() string {
	switch e {
	case VerticalBarLeading:
		return "leading"
	case VerticalBarTrailing:
		return "trailing"
	}
	return "unspecified"
}

// LocaleSettings are the regional preferences the user set, in Intl's
// names: FirstDayOfWeek 1 for Monday to 7 for Sunday (ISO 8601),
// HourCycle "h11", "h12", "h23" or "h24", MeasurementSystem "metric",
// "us" or "uk", TemperatureUnit "celsius" or "fahrenheit"; zero where
// the platform does not say, which Region decides, else the locale's
// region (i18n.Regional). Region is the region whose formats the user
// chose apart from the language, such as Linux's LC_TIME, an ISO 3166
// code.
type LocaleSettings struct {
	FirstDayOfWeek    int
	HourCycle         string
	MeasurementSystem string
	TemperatureUnit   string
	Region            string
}

// Lifecycle is whether the app is in use, as NSH's lifecycle() reads it.
type Lifecycle uint8

const (
	// LifecycleActive is an app that shows and has the focus: on desktops
	// one of its windows is the active one.
	LifecycleActive Lifecycle = iota
	// LifecycleInactive is an app that shows without the focus, as behind
	// another app's window, an incoming call or the app switcher.
	LifecycleInactive
	// LifecycleBackground is an app none of which shows: hidden,
	// minimized, covered where the system reports it, or in the
	// background on phones.
	LifecycleBackground
)

// String returns the state's name in TSX: active, inactive or background.
func (l Lifecycle) String() string {
	switch l {
	case LifecycleInactive:
		return "inactive"
	case LifecycleBackground:
		return "background"
	}
	return "active"
}

// OS names the operating system the app runs on.
type OS string

const (
	OSMacOS   OS = "macos"
	OSIOS     OS = "ios"
	OSAndroid OS = "android"
	OSWindows OS = "windows"
	OSLinux   OS = "linux"
	OSWeb     OS = "web"
)

// Idiom is the kind of device, which decides how components present.
type Idiom uint8

const (
	IdiomPhone Idiom = iota
	IdiomTablet
	// IdiomDesktop is a desktop window, or a browser with a mouse and a
	// wide page.
	IdiomDesktop
	// IdiomMac is an iPhone or iPad app running on a Mac.
	IdiomMac
	// IdiomTV is a television, such as Android TV.
	IdiomTV
	// IdiomWatch is a watch: Apple Watch.
	IdiomWatch
)

// Pointer is the precision of the device's primary pointing device, matching
// CSS's pointer media feature (fine, coarse or none). The zero value derives
// it from Idiom.
type Pointer uint8

const (
	// PointerUnspecified derives the pointer precision from Idiom.
	PointerUnspecified Pointer = iota
	// PointerFine is an accurate pointing device, such as a mouse or trackpad.
	PointerFine
	// PointerCoarse is a limited-accuracy pointing device, such as a finger on a touchscreen.
	PointerCoarse
	// PointerNone is a device with no primary pointing device, such as a TV remote.
	PointerNone
)

// OSVersion is an operating system's version.
type OSVersion struct {
	Major, Minor int
}

// AtLeast reports whether v is major.minor or later.
func (v OSVersion) AtLeast(major, minor int) bool {
	return v.Major > major || v.Major == major && v.Minor >= minor
}

// String returns v as major.minor.
func (v OSVersion) String() string { return strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor) }
