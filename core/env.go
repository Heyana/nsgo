package core

import (
	"maps"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Env is the environment as signals, one per part, so a binding runs
// again only when a part it read changes.
type Env struct {
	network                 *networkState
	deviceName, deviceModel *Signal[string]
	window                  *Signal[Size]
	safe, titleBar          *Signal[Edges]
	maximized               *Signal[bool]
	fullScreen              *Signal[bool]
	minimized               *Signal[bool]
	windowFrame             *Signal[WindowFrame]
	keyboard                *Signal[Points]
	keyboardDuration        *Signal[time.Duration]
	dark, reduce, reader    *Signal[bool]
	contrast                *Signal[bool]
	screenColor, speech     *Signal[bool]
	audio, speak, gamepads  *Signal[bool]
	gpu                     *Signal[bool]
	audioSchedule           *Signal[bool]
	hapticPatterns          *Signal[bool]
	clipboardItems          *Signal[bool]
	nowPlaying              *Signal[bool]
	camera                  *Signal[bool]
	scanCode                *Signal[bool]
	discoverServices        *Signal[bool]
	canSendInput            *Signal[bool]
	canLockPointer          *Signal[bool]
	canLookUp               *Signal[bool]
	canQuickLook            *Signal[bool]
	canDockPanels           *Signal[bool]
	level, place            *Signal[bool]
	workspaces              *Signal[bool]
	hasWebView              *Signal[bool]
	appleSignIn             *Signal[bool]
	miniSignIn              *Signal[bool]
	purchases               *Signal[bool]
	canRequestReview        *Signal[bool]
	gameServices            *Signal[bool]
	cloudStorage            *Signal[bool]
	scheduled               *Signal[bool]
	transparent, taskbar    *Signal[bool]
	fontScale, ratio        *Signal[float32]
	// locale and systemRTL are the platform's or the host's; forced is
	// the locale Scope.SetLocale forces.
	locale, forced *Signal[string]
	settings       *Signal[LocaleSettings]
	displays       *Signal[*displayList]
	display        *Signal[DisplayInfo]
	systemRTL      *Signal[bool]
	language       *Memo[string]
	rtl            *Memo[bool]
	os             *Signal[OS]
	version        *Signal[OSVersion]
	idiom          *Signal[Idiom]
	pointer        *Signal[Pointer]
	lifecycle      *Signal[Lifecycle]
	thermal        *Signal[ThermalState]
	lowPower       *Signal[LowPowerMode]
	appClip        *Signal[bool]
	openedURL      *Signal[string]
	appearance     *Signal[Appearance]

	launchedAtLogin *Signal[bool]
	// frontmost is the application in front, and canReadFrontmost says the platform follows it, which only Scope.FrontmostApp and Scope.CanReadFrontmostApp read, as Access holds them.
	frontmost        *Signal[FrontmostAppInfo]
	canReadFrontmost *Signal[bool]
	customCursor     *Signal[bool]
	// systemDark is the system's appearance, which a forced one hides.
	systemDark   bool
	material     *Signal[WindowMaterial]
	appleKeys    *Signal[bool]
	keyCodeNames *Signal[map[string]string]
	tokens       *Signal[hostColors]
	// geometry is the fold geometry Tree.Resize sets; shown is it with each region's safe area.
	geometry    *Signal[Geometry]
	shown       *Memo[Geometry]
	verticalBar *Signal[VerticalBarEdge]
	// clear is whether the window or panel draws nothing of its own now (WindowTransparent), apart from transparent, whether the platform can make the scope's window so.
	clear *Signal[bool]
	// headers are the system header's heights, which only the tree reads.
	headers [2]Points
	// simulated is whether the OS is one the platform imitates (driver.Environment.Simulated), settled as the app starts.
	simulated bool
}

type displayList struct{ values []DisplayInfo }

// newEnv makes an environment owned by cx. A window's shares what the
// app sets for all of them with the first's, app.
func newEnv(cx *Scope, app *Env) *Env {
	e := &Env{
		deviceName:       NewSignal(cx, ""),
		deviceModel:      NewSignal(cx, ""),
		window:           NewSignal(cx, Size{}),
		geometry:         NewSignalFunc(cx, driver.Divide(Size{}, nil), Geometry.Equal),
		verticalBar:      NewSignal(cx, VerticalBarUnspecified),
		safe:             NewSignal(cx, Edges{}),
		titleBar:         NewSignal(cx, Edges{}),
		maximized:        NewSignal(cx, false),
		fullScreen:       NewSignal(cx, false),
		minimized:        NewSignal(cx, false),
		windowFrame:      NewSignal(cx, WindowFrame{}),
		keyboard:         NewSignal(cx, Points(0)),
		keyboardDuration: NewSignal(cx, 250*time.Millisecond),
		dark:             NewSignal(cx, false),
		reduce:           NewSignal(cx, false),
		contrast:         NewSignal(cx, false),
		systemRTL:        NewSignal(cx, false),
		reader:           NewSignal(cx, false),
		screenColor:      NewSignal(cx, false),
		speech:           NewSignal(cx, false),
		clipboardItems:   NewSignal(cx, false),
		audio:            NewSignal(cx, false),
		audioSchedule:    NewSignal(cx, false),
		hapticPatterns:   NewSignal(cx, false),
		nowPlaying:       NewSignal(cx, false),
		speak:            NewSignal(cx, false),
		gamepads:         NewSignal(cx, false),
		gpu:              NewSignal(cx, false),
		camera:           NewSignal(cx, false),
		scanCode:         NewSignal(cx, false),
		discoverServices: NewSignal(cx, false),
		canSendInput:     NewSignal(cx, false),
		canLockPointer:   NewSignal(cx, false),
		canLookUp:        NewSignal(cx, false),
		canQuickLook:     NewSignal(cx, false),
		canDockPanels:    NewSignal(cx, false),
		level:            NewSignal(cx, false),
		place:            NewSignal(cx, false),
		workspaces:       NewSignal(cx, false),
		hasWebView:       NewSignal(cx, false),
		appleSignIn:      NewSignal(cx, false),
		miniSignIn:       NewSignal(cx, false),
		purchases:        NewSignal(cx, false),
		canRequestReview: NewSignal(cx, false),
		gameServices:     NewSignal(cx, false),
		cloudStorage:     NewSignal(cx, false),
		scheduled:        NewSignal(cx, false),
		transparent:      NewSignal(cx, false),
		taskbar:          NewSignal(cx, false),
		fontScale:        NewSignal(cx, float32(1)),
		ratio:            NewSignal(cx, float32(1)),
		locale:           NewSignal(cx, ""),
		forced:           NewSignal(cx, ""),
		settings:         NewSignal(cx, LocaleSettings{}),
		displays:         NewSignal(cx, &displayList{}),
		display:          NewSignal(cx, DisplayInfo{}),
		os:               NewSignal(cx, OS("")),
		version:          NewSignal(cx, OSVersion{}),
		idiom:            NewSignal(cx, IdiomPhone),
		pointer:          NewSignal(cx, PointerCoarse),
		lifecycle:        NewSignal(cx, LifecycleActive),
		thermal:          NewSignal(cx, ThermalUnknown),
		lowPower:         NewSignal(cx, LowPowerUnknown),
		appClip:          NewSignal(cx, false),
		openedURL:        NewSignal(cx, ""),
		launchedAtLogin:  NewSignal(cx, false),
		frontmost:        NewSignal(cx, FrontmostAppInfo{}),
		canReadFrontmost: NewSignal(cx, false),
		customCursor:     NewSignal(cx, false),
		appearance:       NewSignal(cx, AppearanceSystem),
		appleKeys:        NewSignal(cx, false),
		keyCodeNames:     NewSignalFunc(cx, map[string]string(nil), maps.Equal[map[string]string]),
		material:         NewSignal(cx, WindowMaterialNone),
		clear:            NewSignal(cx, false),
		tokens:           NewSignalFunc(cx, hostColors{}, nil),
	}
	if app != nil {
		e.forced, e.appearance, e.tokens = app.forced, app.appearance, app.tokens
		e.network = app.network
	} else {
		e.network = newNetworkState(cx)
	}
	e.shown = NewMemoFunc(cx, func() Geometry {
		g := e.geometry.Get().Clone()
		safe, size := e.safe.Get(), e.window.Get()
		for i := range g.Regions {
			g.Regions[i].SafeArea = regionSafeArea(g.Regions[i].Frame, safe, size)
		}
		return g
	}, Geometry.Equal)
	e.language = NewMemo(cx, func() string {
		locale := e.Locale()
		if match := languageMatch.Load(); match != nil {
			return (*match)(locale)
		}
		return ""
	})
	// The app lays out in the direction of the language it shows, else of
	// the locale it forces, else as the system does.
	e.rtl = NewMemo(cx, func() bool {
		if lang := e.language.Get(); lang != "" {
			return rightToLeft(lang)
		}
		if forced := e.forced.Get(); forced != "" {
			return rightToLeft(forced)
		}
		return e.systemRTL.Get()
	})
	return e
}

// Env returns the environment of the window cx is in: its size, scale
// and safe area are the window's, and what the app forces is every
// window's.
func (cx *Scope) Env() *Env {
	rt := cx.rt
	if rt.env == nil {
		root := cx
		for root.parent != nil {
			root = root.parent
		}
		rt.env = newEnv(root, nil)
	}
	if t := cx.surf; t != nil && t != rt.tree {
		if t.env == nil {
			t.env = newEnv(t.root, rt.env)
		}
		return t.env
	}
	return rt.env
}

func (e *Env) set(v Environment) {
	if v.FontScale <= 0 {
		v.FontScale = 1
	}
	if v.PixelRatio <= 0 {
		v.PixelRatio = 1
	}
	if v.KeyboardDuration <= 0 {
		v.KeyboardDuration = 250 * time.Millisecond
	}
	e.safe.Set(v.SafeArea)
	e.deviceName.Set(v.DeviceName)
	e.deviceModel.Set(v.DeviceModel)
	e.titleBar.Set(v.TitleBar)
	e.maximized.Set(v.Maximized)
	e.fullScreen.Set(v.FullScreen)
	e.minimized.Set(v.Minimized)
	e.windowFrame.Set(v.WindowFrame)
	e.keyboard.Set(v.KeyboardHeight)
	e.keyCodeNames.Set(maps.Clone(v.KeyCodeNames))
	e.keyboardDuration.Set(v.KeyboardDuration)
	e.systemDark = v.Dark
	e.dark.Set(e.shownDark())
	e.reduce.Set(v.ReduceMotion)
	e.contrast.Set(v.IncreaseContrast)
	e.systemRTL.Set(v.RTL)
	e.reader.Set(v.ScreenReader)
	e.screenColor.Set(v.ScreenColor)
	e.speech.Set(v.Speech)
	e.audio.Set(v.Audio)
	e.audioSchedule.Set(v.Audio && v.AudioSchedule)
	e.hapticPatterns.Set(v.HapticPatterns)
	e.clipboardItems.Set(v.ClipboardItems)
	e.nowPlaying.Set(v.NowPlaying)
	e.speak.Set(v.Speak)
	e.gamepads.Set(v.Gamepads)
	e.gpu.Set(v.GPU)
	e.camera.Set(v.Camera)
	e.scanCode.Set(v.ScanCode)
	e.discoverServices.Set(v.DiscoverServices)
	e.canSendInput.Set(v.CanSendInput)
	e.canLockPointer.Set(v.CanLockPointer)
	e.canLookUp.Set(v.CanLookUp)
	e.canQuickLook.Set(v.CanQuickLook)
	e.level.Set(v.CanSetWindowLevel)
	e.place.Set(v.CanPlaceWindows)
	e.canDockPanels.Set(v.CanDockPanels)
	e.workspaces.Set(v.CanSetWindowWorkspaces)
	e.hasWebView.Set(v.HasWebView)
	e.appleSignIn.Set(v.SignInWithApple)
	e.miniSignIn.Set(v.MiniProgramSignIn)
	e.purchases.Set(v.Purchases)
	e.canRequestReview.Set(v.CanRequestReview)
	e.gameServices.Set(v.CanUseGameServices)
	e.cloudStorage.Set(v.CloudStorage)
	e.scheduled.Set(v.ScheduledNotifications)
	e.transparent.Set(v.CanMakeWindowsTransparent)
	e.taskbar.Set(v.CanSkipTaskbar)
	e.fontScale.Set(v.FontScale)
	e.ratio.Set(v.PixelRatio)
	e.locale.Set(v.Locale)
	e.settings.Set(v.LocaleSettings)
	if !slices.Equal(e.displays.GetUntracked().values, v.Displays) {
		e.displays.Set(&displayList{values: slices.Clone(v.Displays)})
	}
	var display DisplayInfo
	if v.DisplayID != "" {
		if i := slices.IndexFunc(v.Displays, func(d DisplayInfo) bool { return d.ID == v.DisplayID }); i >= 0 {
			display = v.Displays[i]
		}
	}
	e.display.Set(display)
	e.os.Set(v.OS)
	e.version.Set(v.OSVersion)
	e.appleKeys.Set(!v.Simulated && (v.OS == OSMacOS || v.OS == OSIOS) || v.AppleKeyboard)
	e.idiom.Set(v.Idiom)
	e.pointer.Set(pointerOf(v))
	e.lifecycle.Set(v.Lifecycle)
	e.thermal.Set(v.ThermalState)
	e.lowPower.Set(v.LowPowerMode)
	e.appClip.Set(v.AppClip)
	e.openedURL.Set(v.OpenedURL)
	e.launchedAtLogin.Set(v.LaunchedAtLogin)
	e.frontmost.Set(v.FrontmostApp)
	e.canReadFrontmost.Set(v.CanReadFrontmostApp)
	e.customCursor.Set(supportsCustomCursor(v))
	e.material.Set(v.WindowMaterial)
	e.clear.Set(v.WindowTransparent)
	e.verticalBar.Set(v.VerticalBarEdge)
	e.headers = [2]Points{v.HeaderHeight, max(v.LargeHeaderHeight, v.HeaderHeight)}
	e.simulated = v.Simulated
}

// DeviceName returns the name the user gave the device, such as "Ada's MacBook Pro", as the system's settings show it; it is empty on the web and where the platform keeps it private, as iOS does from apps without Apple's user-assigned device name entitlement.
func (e *Env) DeviceName() string { return e.deviceName.Get() }

// DeviceModel returns the device's hardware model: its model identifier on Apple's platforms, such as "iPhone15,2" or "Mac15,7", its maker and model on Android, such as "Google Pixel 8", and the firmware's product name on Windows and Linux; it is empty on the web and where the hardware names none.
func (e *Env) DeviceModel() string { return e.deviceModel.Get() }

// ThermalState returns the system's thermal pressure, or ThermalUnknown when the system does not report it.
func (e *Env) ThermalState() ThermalState { return e.thermal.Get() }

// LowPowerMode reports whether the system's energy-saving mode is on, or LowPowerUnknown when the system does not report it.
func (e *Env) LowPowerMode() LowPowerMode { return e.lowPower.Get() }

// Window returns the surface's size.
func (e *Env) Window() Size { return e.window.Get() }

// Geometry returns how a fold or hinge divides the surface: its usable regions, each with the safe area that falls in it, and the obstructions between them, in the surface's coordinates and measured at Window's size, which it changes with; and how far the device's hinge is open (State, Angle), whether or not an obstruction lies on the surface, as on a cover display or a screen opened flat. A surface without a fold, as on every target but Android and iOS 27.1, has one region, no obstructions and HingeUnknown. Core lays nothing out by it: an app or component reads it and chooses its own panes.
func (e *Env) Geometry() Geometry { return e.shown.Get().Clone() }

// VerticalBarEdge returns the edge the system chose for a vertical bar, such as a tab bar drawn down a side, where it may place one, as iOS 27.1 does; VerticalBarUnspecified elsewhere.
func (e *Env) VerticalBarEdge() VerticalBarEdge { return e.verticalBar.Get() }

// regionSafeArea is the part of safe, the surface's safe area at size, that falls in a region's frame, as insets of the region.
func regionSafeArea(f Bounds, safe Edges, size Size) Edges {
	inset := func(v, length Points) Points { return min(max(v, 0), length) }
	return Edges{
		Top:    inset(safe.Top-f.Origin.Y, f.Size.Height),
		Right:  inset(f.Origin.X+f.Size.Width-(size.Width-safe.Right), f.Size.Width),
		Bottom: inset(f.Origin.Y+f.Size.Height-(size.Height-safe.Bottom), f.Size.Height),
		Left:   inset(safe.Left-f.Origin.X, f.Size.Width),
	}
}

// SafeArea returns the insets system UI covers on each edge.
func (e *Env) SafeArea() Edges { return e.safe.Get() }

// SupportsGPU reports whether this target and device can host a gpu.View
// (docs/architecture/gpu.md).
func (e *Env) SupportsGPU() bool { return e.gpu.Get() }

// KeyboardHeight returns how much of the bottom the keyboard covers.
func (e *Env) KeyboardHeight() Points { return e.keyboard.Get() }

// TitleBar returns what a hidden desktop title bar covers: the core's
// part of desktop.TitleBar, which apps use.
func (e *Env) TitleBar() Edges { return e.titleBar.Get() }

// Maximized reports whether the window is maximized or zoomed: the
// core's part of desktop.Maximized, which apps use.
func (e *Env) Maximized() bool { return e.maximized.Get() }

// FullScreen reports whether the window is in full screen: the core's
// part of desktop.FullScreen, which apps use.
func (e *Env) FullScreen() bool { return e.fullScreen.Get() }

// Minimized reports whether the window is minimized: the core's part of desktop.Minimized, which apps use.
func (e *Env) Minimized() bool { return e.minimized.Get() }

// WindowFrame returns where the window is and how large: the core's part
// of desktop.Frame, which apps use.
func (e *Env) WindowFrame() WindowFrame { return e.windowFrame.Get() }

// CanSetWindowLevel reports whether SetWindowLevel stacks the scope's desktop window: the core's part of desktop.CanSetWindowLevel, which apps use.
func (e *Env) CanSetWindowLevel() bool { return e.level.Get() }

// CanPlaceWindows reports whether SetWindowFrame places the scope's window where it says, an Android overlay's included: the core's part of desktop.CanPlaceWindows, which apps use.
func (e *Env) CanPlaceWindows() bool { return e.place.Get() }

// CanDockPanels reports whether desktop.PanelOptions.Dock can attach a panel to a display's usable edge. Wayland needs gtk4-layer-shell and a compositor implementing layer-shell; an explicit display must still be connected.
func (e *Env) CanDockPanels() bool { return e.canDockPanels.Get() }

// CanSetWindowWorkspaces reports whether SetWindowWorkspaces puts the scope's desktop window on the workspaces it names: the core's part of desktop.CanSetWindowWorkspaces, which apps use.
func (e *Env) CanSetWindowWorkspaces() bool { return e.workspaces.Get() }

// KeyboardDuration returns how long the keyboard takes to move.
func (e *Env) KeyboardDuration() time.Duration { return e.keyboardDuration.Get() }

// shownDark is whether the app shows dark: as SetAppearance forces it,
// else as the system's appearance is.
func (e *Env) shownDark() bool {
	switch e.appearance.GetUntracked() {
	case AppearanceLight:
		return false
	case AppearanceDark:
		return true
	}
	return e.systemDark
}

// Dark reports whether the app shows dark: the appearance Scope.SetAppearance
// forces, else the system's.
func (e *Env) Dark() bool { return e.dark.Get() }

// Appearance returns the appearance Scope.SetAppearance forces, or
// AppearanceSystem.
func (e *Env) Appearance() Appearance { return e.appearance.Get() }

// FontScale returns the user's text size multiplier.
func (e *Env) FontScale() float32 { return e.fontScale.Get() }

// ReduceMotion reports whether the user asked for less motion.
func (e *Env) ReduceMotion() bool { return e.reduce.Get() }

// IncreaseContrast reports whether the user asked the system for stronger contrast, such as macOS Increase Contrast or a Windows contrast theme.
func (e *Env) IncreaseContrast() bool { return e.contrast.Get() }

// ScreenReader reports whether a screen reader is running.
func (e *Env) ScreenReader() bool { return e.reader.Get() }

// CanPickScreenColor reports whether Scope.PickScreenColor works here:
// macOS, Windows, Linux where the desktop Screenshot portal is available,
// and browsers with an EyeDropper.
func (e *Env) CanPickScreenColor() bool { return e.screenColor.Get() }

// CanRecognizeSpeech reports whether Scope.StartSpeech recognizes speech
// here: macOS and iOS (not tvOS or watchOS).
func (e *Env) CanRecognizeSpeech() bool { return e.speech.Get() }

// CanPlayAudio reports whether the platform plays the audio package's sounds and players; elsewhere they fail with AudioUnavailable without asking it.
func (e *Env) CanPlayAudio() bool { return e.audio.Get() }

// CanScheduleAudio reports whether the platform schedules audio.Sound playback at a requested time.
func (e *Env) CanScheduleAudio() bool { return e.audioSchedule.Get() }

// CanPlayHapticPattern reports whether the platform can play timed haptics.
func (e *Env) CanPlayHapticPattern() bool { return e.hapticPatterns.Get() }

// CanShowNowPlaying reports whether the system presents the app's Now Playing information.
func (e *Env) CanShowNowPlaying() bool { return e.nowPlaying.Get() }

// CanSpeak reports whether Scope.Speak speaks text here: macOS, iOS, tvOS, Android, Windows, the web where the browser has speechSynthesis, and Linux where speech-dispatcher is installed. Elsewhere an utterance fails with SpeakUnavailable without asking the platform.
func (e *Env) CanSpeak() bool { return e.speak.Get() }

// SupportsGamepads reports whether package gamepad reads game controllers here: macOS, iOS, Windows, Linux, Android and the web.
func (e *Env) SupportsGamepads() bool { return e.gamepads.Get() }

// CanTakePhoto reports whether Scope.TakePhoto shows a camera here: iOS and Android devices with one, HarmonyOS, mobile mini programs, browsers on touch screens, and the desktops where Scope.ScanCode has a camera: macOS and Windows machines with one, and Linux desktops with the Camera portal and GStreamer pipewiresrc.
func (e *Env) CanTakePhoto() bool { return e.camera.Get() }

// CanScanCode reports whether Scope.ScanCode shows a camera that reads codes here: macOS and iOS devices with a camera (not the simulator), Android devices with one whose app's manifest asks for CAMERA (nsgo.yaml's android.permissions), Windows machines with a camera, Linux desktops with the Camera portal and GStreamer pipewiresrc, and browsers with getUserMedia and BarcodeDetector.
func (e *Env) CanScanCode() bool { return e.scanCode.Get() }

// CanDiscoverServices reports whether this target supports local DNS-SD.
func (e *Env) CanDiscoverServices() bool { return e.discoverServices.Get() }

// CanSendInput reports whether Ref.SendInput can give a web view's page
// trusted input here. macOS supports this through AppKit; other backends
// currently drop it.
func (e *Env) CanSendInput() bool { return e.canSendInput.Get() }

// CanLockPointer reports whether the surface supports Ref.LockPointer.
func (e *Env) CanLockPointer() bool { return e.canLockPointer.Get() }

// CanLookUp reports whether desktop.LookUp can show the system dictionary panel.
func (e *Env) CanLookUp() bool { return e.canLookUp.Get() }

// CanQuickLook reports whether desktop.QuickLook can show the system's file preview panel.
func (e *Env) CanQuickLook() bool { return e.canQuickLook.Get() }

// HasWebView reports whether this target provides the native or browser web
// view host used by WebView.
func (e *Env) HasWebView() bool { return e.hasWebView.Get() }

// CanSignInWithApple reports whether Scope.SignInWithApple and Scope.AppleCredentialState reach Sign in with Apple here: macOS, iOS and tvOS.
func (e *Env) CanSignInWithApple() bool { return e.appleSignIn.Get() }

// CanSignInMiniProgram reports whether SignInMiniProgram reaches the host login API.
func (e *Env) CanSignInMiniProgram() bool { return e.miniSignIn.Get() }

// CanPurchase reports whether Scope.Products, Purchase and the other in-app purchase services reach a store here: the App Store on macOS, iOS and tvOS, in a build nsgo.yaml's apple.purchases gives StoreKit.
func (e *Env) CanPurchase() bool { return e.purchases.Get() }

// CanRequestReview reports whether Scope.RequestReview can ask the platform's store for an in-app rating prompt here.
func (e *Env) CanRequestReview() bool { return e.canRequestReview.Get() }

// CanUseGameServices reports whether package gameservices reaches a platform game service here: Game Center on macOS, iOS and tvOS where GameKit loads, and Google Play Games Services on Android where available. Elsewhere gameservices calls answer ErrUnsupported without asking the platform.
func (e *Env) CanUseGameServices() bool { return e.gameServices.Get() }

// CanUseCloudStorage reports whether package cloud has an available cloud storage backend here: an Apple device signed into iCloud, Android with Google Play Games saved games, or a test app whose in-memory cloud store is enabled.
func (e *Env) CanUseCloudStorage() bool { return e.cloudStorage.Get() }

// CanScheduleNotifications reports whether a Notification with a later At or a Repeat is scheduled here: natively (also after the app closes) on macOS, iOS and watchOS in an app bundle, Android and Windows, and while the app or page runs on Linux and the web where Notification is available. Elsewhere (tvOS and mini programs) ShowNotification answers false for one without asking the platform.
func (e *Env) CanScheduleNotifications() bool { return e.scheduled.Get() }

// CanMakeWindowsTransparent reports whether the scope's desktop window shows what is behind it while transparent (WindowOptions.Transparent, SetWindowTransparent): the core's part of desktop.CanMakeWindowsTransparent, which apps use.
func (e *Env) CanMakeWindowsTransparent() bool { return e.transparent.Get() }

// CanSkipTaskbar reports whether WindowOptions.SkipTaskbar and SetSkipTaskbar keep the scope's desktop window off the taskbar: the core's part of desktop.CanSkipTaskbar, which apps use.
func (e *Env) CanSkipTaskbar() bool { return e.taskbar.Get() }

// SupportsCustomCursor reports whether the platform shows image-based custom
// cursors (Style.CursorImage): true on macOS, Windows, Linux and the web;
// false on touch platforms such as iOS and Android.
func (e *Env) SupportsCustomCursor() bool { return e.customCursor.Get() }

func supportsCustomCursor(v Environment) bool {
	if v.CustomCursor {
		return true
	}
	if v.Simulated {
		return false
	}
	switch v.OS {
	case OSMacOS, OSWindows, OSLinux, OSWeb:
		return true
	}
	return false
}

// RTL reports whether the app lays out right to left: as the language it
// shows is written (Language), else the locale it forces, else as the
// system lays out.
func (e *Env) RTL() bool { return e.rtl.Get() }

// PixelRatio returns device pixels per point.
func (e *Env) PixelRatio() float32 { return e.ratio.Get() }

// Locale returns the locale the app runs in, a BCP 47 tag: the one
// Scope.SetLocale forces, else the embedding host's, else the user's.
func (e *Env) Locale() string {
	if forced := e.forced.Get(); forced != "" {
		return forced
	}
	return e.locale.Get()
}

// LocaleSettings returns how the user reads weeks, times and measures,
// apart from the language, where the platform says: zero where it does
// not. Package i18n's Regional fills them in from the locale's region.
func (e *Env) LocaleSettings() LocaleSettings { return e.settings.Get() }

// Displays returns the reported displays in screen coordinates. Frame is the
// whole display, VisibleFrame is the area available to windows, Scale is
// device pixels per point, and Main marks the primary display. It is empty
// where the platform has no display information.
func (e *Env) Displays() []DisplayInfo { return slices.Clone(e.displays.Get().values) }

// Display reports the window's current display. Bindings that read it
// follow changes to that display and moves to a different one. It returns
// the zero DisplayInfo where the platform has no current display.
func (e *Env) Display() DisplayInfo { return e.display.Get() }

// Language returns the language the app shows: of its catalogs'
// languages, the one that suits Locale best, as SetLanguageMatch says;
// "" for an app without catalogs. Package i18n's Language, which apps use,
// reads it.
func (e *Env) Language() string { return e.language.Get() }

// shownLanguage is the language the platform says its menus in: the one
// the app shows, else the locale it runs in.
func (t *Tree) shownLanguage() string {
	e := t.root.Env()
	if lang := e.language.GetUntracked(); lang != "" {
		return lang
	}
	if forced := e.forced.GetUntracked(); forced != "" {
		return forced
	}
	return e.locale.GetUntracked()
}

var languageMatch atomic.Pointer[func(locale string) string]

// SetLanguageMatch sets how the language the app shows follows its
// locale: package i18n's SetApp sets its catalog's match, nil none. It is
// set as the program initializes, for every app it runs.
func SetLanguageMatch(match func(locale string) string) {
	if match == nil {
		languageMatch.Store(nil)
		return
	}
	languageMatch.Store(&match)
}

// SetLocale makes the app run in locale, a BCP 47 tag, whatever the
// user's or the host's: the language it shows, its formats and its
// direction follow. "" follows theirs again. The app keeps the choice for
// later launches itself, as in its storage.
func (cx *Scope) SetLocale(locale string) {
	cx.Env().forced.Set(locale)
	if t := cx.tree(); t != nil {
		t.changed = true
	}
}

// rightToLeft reports whether lang, a BCP 47 tag, is written right to
// left: by its script when it names one, else by its language.
func rightToLeft(lang string) bool {
	parts := strings.Split(strings.ReplaceAll(lang, "_", "-"), "-")
	for _, p := range parts[1:] {
		if len(p) == 4 {
			switch strings.ToLower(p) {
			case "arab", "hebr", "syrc", "thaa", "nkoo", "adlm", "rohg", "mand", "samr", "mend", "yezi":
				return true
			}
			return false
		}
	}
	switch strings.ToLower(parts[0]) {
	case "ar", "arc", "ckb", "dv", "fa", "he", "iw", "ks", "ps", "sd", "syr", "ug", "ur", "yi", "lrc", "mzn", "glk":
		return true
	}
	return false
}

// OS returns the operating system.
func (e *Env) OS() OS { return e.os.Get() }

// Simulated reports whether the app runs as another OS than the platform's, as a web build does for --nsgo-os, or on an OS whose native parts it cannot reach, as a mini program does: OS is that one's, but none of its native parts are there, so the app's own views stand in for its system header, tab bar, menus and window.
func (e *Env) Simulated() bool { return e.simulated }

// host is the OS whose native parts the platform has, tracked as OS is: the web's while it simulates another. hostUntracked reads it without tracking.
func (e *Env) host() OS {
	os := e.os.Get()
	if e.simulated {
		return OSWeb
	}
	return os
}

// mobileBars reports whether the platform draws iOS's or Android's own navigation and tab bars: not on a watch, which reports iOS but draws neither, nor in a web build that runs as them.
func (e *Env) mobileBars() bool {
	os := e.host()
	return (os == OSIOS || os == OSAndroid) && e.Idiom() != IdiomWatch
}

func (e *Env) hostUntracked() OS {
	if e.simulated {
		return OSWeb
	}
	return e.os.GetUntracked()
}

// OSVersion returns the system's version: Android's API level as its
// major, zero on the web.
func (e *Env) OSVersion() OSVersion { return e.version.Get() }

// Idiom returns the kind of device.
func (e *Env) Idiom() Idiom { return e.idiom.Get() }

// Pointer returns the precision of the primary pointing device, matching CSS's
// pointer media feature (PointerFine, PointerCoarse or PointerNone): what the
// platform reports directly, as the web's pointer media query does, else
// derived from Idiom (fine on desktops and Macs, none on TVs, coarse on touch
// devices).
func (e *Env) Pointer() Pointer { return e.pointer.Get() }

func pointerOf(v Environment) Pointer {
	if v.Pointer != PointerUnspecified {
		return v.Pointer
	}
	switch v.Idiom {
	case IdiomDesktop, IdiomMac:
		return PointerFine
	case IdiomTV:
		return PointerNone
	default:
		return PointerCoarse
	}
}

// Lifecycle returns whether the app is in use: active, inactive (shown without the focus) or in the background. On desktops it is the window's the scope is in, a minimized or covered window's in the background while the others show, and the app's own scope's is every window's together.
func (e *Env) Lifecycle() Lifecycle { return e.lifecycle.Get() }

// AppClip reports whether the app runs as an iOS App Clip: the core's
// part of mobile.IsAppClip, which apps use.
func (e *Env) AppClip() bool { return e.appClip.Get() }

// OpenedURL returns the link that last opened the app, "" when none did: the core's part of mobile.OpenedURL, which apps use.
func (e *Env) OpenedURL() string { return e.openedURL.Get() }

// LaunchedAtLogin reports whether the system started this run as a login item: the core's part of desktop.LaunchedAtLogin, which apps use.
func (e *Env) LaunchedAtLogin() bool { return e.launchedAtLogin.Get() }

// AppleKeys reports whether shortcuts follow Apple's conventions: "mod"
// is Command and labels use ⌘.
func (e *Env) AppleKeys() bool { return e.appleKeys.Get() }

// WindowMaterial returns the material behind the window's content, or
// WindowMaterialNone when the window is opaque: the core's part of
// desktop.ShowsMaterial, which apps use.
func (e *Env) WindowMaterial() WindowMaterial { return e.material.Get() }

// WindowTransparent reports whether the scope's window or panel draws nothing of its own now, no background, frame or shadow, so what is behind it shows wherever the content draws nothing: a panel opened with PanelOptions.Transparent, or a window opened with WindowOptions.Transparent or made so by SetWindowTransparent, where the platform could. It is the core's part of desktop.Transparent, which apps use. CanMakeWindowsTransparent is whether a window could be; this is whether it is. Where it is false, give the content a background of its own.
func (e *Env) WindowTransparent() bool { return e.clear.Get() }

// Desktop reports whether the device is a desktop.
func (e *Env) Desktop() bool { return e.idiom.Get() == IdiomDesktop }

// insetsKey is provided by screens, modals and what keeps part of the safe area clear itself, such as ui.TabShell's tab bar: the insets their content keeps clear of.
var insetsKey = NewKey[func() Edges]("insets", nil)

// Insets returns what content under cx must keep clear of: the insets of the nearest screen, modal or ProvideInsets, or the surface's safe area.
func Insets(cx *Scope) Edges { return InsetsFunc(cx)() }

// InsetsFunc returns what reads Insets of cx's place, found now and
// tracked as it is called, wherever that is.
func InsetsFunc(cx *Scope) func() Edges {
	if f := insetsKey.Use(cx); f != nil {
		return f
	}
	env := cx.Env()
	return env.SafeArea
}

// ProvideInsets makes insets, read tracked, what content built under cx keeps clear of (Insets, EnvSafeArea), screens below it included: a component that keeps its own part of the safe area clear, such as ui.TabShell's drawn tab bar, takes that part away from its content.
func ProvideInsets(cx *Scope, insets func() Edges) { insetsKey.Provide(cx, insets) }
