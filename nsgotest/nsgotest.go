// Package nsgotest runs NSGo apps in Go tests, as net/http/httptest runs
// handlers: headless by default, with approximate text, a manual clock
// and every batch applied to a model of the native views that tests
// query; `nsgo test -target` runs the same tests on a platform, with its
// native input.
//
//	func TestAdd(t *testing.T) {
//		app := nsgotest.Start(t, Root)
//		app.Type("id:draft", "Milk")
//		app.Press("role:button Add")
//		app.Find("text:Milk")
//	}
//
// Selectors name the views a user sees, in Playwright's manner:
//
//	id:<TestID>              the view with that test ID
//	label:<Label>            the view with that accessibility label
//	text:<text>              a text, or a field holding it (its placeholder while empty), containing text
//	role:<role>              a view with that role ("button", "checkbox", "textbox", ...)
//	role:<role> <name>       one whose label is name, or whose texts contain it
//	view:<node>              the view of that node ID, as traces print it
//	anything else            a test ID, else a label, else a whole text
//
// label:, text: and a role's name take /regexp/flags, with JavaScript's
// i, m and s flags. " >> " chains selectors, each part looking inside the
// views the last matched, and " >> nth=<i>" keeps one match. Matches come
// in the order a user finds them: the topmost modal's first, then the rest
// in tree order.
//
// Steps act as a user does and send what a platform's input sends: Press is a finger down and up, Type a key at a time, Scroll a drag 10 points a frame. A step whose selector matches no view, or more than one it would act on, or a disabled one, fails the test. Each step then waits, moving the clock a frame at a time for at most 2 seconds, until no animation that ends runs and every style transition has reached its OnTransitionEnd; ManualFrames leaves them to the test's Advance.
package nsgotest

import (
	"fmt"
	"io"
	"log"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// Option configures Start.
type Option func(*App)

// Size sets the surface size; 390 by 844 points by default, and under `nsgo test -target macos` the main window's default size, 480 by 640.
func Size(width, height driver.Points) Option {
	return func(a *App) { a.size, a.sized = driver.Size{Width: width, Height: height}, true }
}

// Storage starts the app with entries in its storage, as if kept from an
// earlier run; it starts empty otherwise.
func Storage(entries map[string]string) Option {
	return func(a *App) { a.store = core.NewStore(seed(entries)) }
}

// Clock runs the app on c instead of a new ManualClock, for a test that shares one clock with code of its own: App.Clock is c, and the app's timers and frames run as c moves. c must not be nil. Under Measure the app runs on the system's clock instead.
func Clock(c *core.ManualClock) Option {
	return func(a *App) { a.Clock = c }
}

// WallTime starts the app with its wall time (Scope.WallTime, a TSX app's Date) at t, in t's location, as ManualClock.SetWallTime sets it, so the first render reads t. The clock's own time, which timers and frames run on, starts where it would, and no timer moves. It applies after every option, Clock's included, in any order. Under Measure the app reads the system's time instead.
func WallTime(t time.Time) Option {
	return func(a *App) {
		wall := t
		a.wallTime = &wall
	}
}

// Secrets starts the app with values kept in its secure store, by account under the app's own service (core.Secrets.Service), as if kept from an earlier run; it starts empty otherwise.
func Secrets(values map[string]string) Option {
	return func(a *App) { a.seedSecrets = values }
}

// DiskSpace sets what Scope.DiskSpace answers in headless and native tests.
// Without it the service reports core.ErrDiskSpaceUnavailable.
func DiskSpace(space core.DiskSpace, err error) Option {
	return func(a *App) { a.diskSpace = driver.DiskSpaceResult{Space: space, Err: err} }
}

// UserDirs pins what Scope.UserDir answers in headless and native tests: a folder left out reports core.ErrUserDirUnavailable. Without it each folder is a directory of the test's temporary one, never the user's.
func UserDirs(dirs map[core.StandardDir]string) Option {
	return func(a *App) { a.userDirs = maps.Clone(dirs); a.userDirsPinned = true }
}

// UpdaterState supplies the updater snapshot before the first render.
func UpdaterState(state core.UpdaterState) Option { return func(a *App) { a.updater = state } }

// Environment starts the app in env from its first frame, as a platform reports it before the app mounts: screens have the system header where env has HeaderHeight. Under `nsgo test -target` it pins only the appearance, reduced motion, increased contrast, the login launch reason, the application in front and whether the platform follows it, camera and code scanner availability, and, when set, the locale, text size, device name, device model and vertical bar edge; the OS, the idiom, the safe area and the header heights stay the platform's, so a test of another OS's layout runs headless or on that OS's target.
func Environment(env driver.Environment) Option {
	env.AudioPlaybackControls = env.Audio
	return func(a *App) { a.env = &env }
}

// Idiom starts a headless app with idiom, core.IdiomPhone by default, and the rest of its environment as it would be otherwise: Environment's when the test passes one, before or after Idiom, the defaults when not. IdiomDesktop has what reads Env.Desktop take its desktop path, such as hover tooltips; keyboard focus follows the environment's OS instead. App.SetEnvironment later reports its environment whole, idiom included. Under `nsgo test -target` the platform's idiom stays, as Environment's does.
func Idiom(idiom driver.Idiom) Option {
	return func(a *App) { a.idiom = &idiom }
}

// VerticalBarEdge starts the app with the system's preferred vertical bar edge (core.Env.VerticalBarEdge) set to edge, combining with Environment in any order. On a native target, a non-unspecified edge pins it over the platform's.
func VerticalBarEdge(edge driver.VerticalBarEdge) Option {
	return func(a *App) { a.verticalBarEdge = &edge }
}

// HotKeyTaken has the headless platform answer the global shortcuts of keys, in core.Scope.OnKey's form, as desktop.HotKeyTaken, so a test runs an app's path for a combination another app holds; other keys register.
func HotKeyTaken(keys ...string) Option {
	return func(a *App) {
		if a.hotKeyStates == nil {
			a.hotKeyStates = map[string]core.HotKeyState{}
		}
		for _, key := range keys {
			a.hotKeyStates[driver.NormalizeKey(key)] = core.HotKeyTaken
		}
	}
}

// HotKeyUnsupported has the headless platform answer the global shortcuts of keys as desktop.HotKeyUnsupported.
func HotKeyUnsupported(keys ...string) Option {
	return func(a *App) {
		if a.hotKeyStates == nil {
			a.hotKeyStates = map[string]core.HotKeyState{}
		}
		for _, key := range keys {
			a.hotKeyStates[driver.NormalizeKey(key)] = core.HotKeyUnsupported
		}
	}
}

// Host runs the app with h as its channel to an embedding host, so a test
// sends it events and answers its calls as the host would.
func Host(h *core.Host) Option {
	return func(a *App) { a.host = h }
}

// TextEngine measures and lays out the app's text with e instead of
// core.ApproxTextEngine, as a test that checks frames another headless
// runner computed needs its measurements.
func TextEngine(e driver.TextEngine) Option {
	return func(a *App) { a.textEngine = e }
}

// Offscreen has selectors match every view mounted and not hidden, also
// one outside the window or clipped away, as NSH's headless runs act.
func Offscreen() Option {
	return func(a *App) { a.offscreen = true }
}

// PendingAlerts leaves the alerts the app shows unanswered unless the
// test answers them, as NSH's headless runs present none.
func PendingAlerts() Option {
	return func(a *App) { a.pendingAlerts = true }
}

// NoCaretMoves keeps typing from reporting where the caret moves, as
// NSH's headless model types: a test reports caret moves itself.
func NoCaretMoves() Option {
	return func(a *App) { a.noCaretMoves = true }
}

// ManualFrames keeps steps and waits from taking display frames so a test can inspect animations and style transitions as it advances the clock.
func ManualFrames() Option {
	return func(a *App) { a.manualFrames = true }
}

// Measure runs the app as it runs for its users, for a measurement on a target: on the system's clock, so Advance waits as long as it moves, with its own commits unchecked, whose check otherwise lays every node out again, and with steps that wait only for what animates to rest, not for the app to stop committing. Headless, it does nothing.
func Measure() Option {
	return func(a *App) { a.measure = true }
}

// Mark names what the performance monitor's readings measure from here
// on, a line of nsgo test -fps's output among them ("perf mark name");
// "-" ends it. It logs as the monitor does, so the two keep their order.
func (a *App) Mark(name string) { log.Print("nsgo: perf mark ", name) }

// Errors hands what the app reports failing to report instead of failing
// the test, as a host's overlay shows it while the app runs on. It
// applies headless only.
func Errors(report func(error)) Option {
	return func(a *App) { a.report = report }
}

// seed is a storage backend that loads entries and keeps nothing.
type seed map[string]string

func (s seed) Load() (map[string]string, error) { return s, nil }
func (seed) Write(core.StorageChange)           {}

// AwakeHold is a keep-awake hold the app took: the ID its release names in AwakeReleases, and what it asked for.
type AwakeHold struct {
	ID      uint32
	Reason  string
	Display bool
}

// AuthSession is an auth session the app opened (core.Scope.AuthSession): the redirect the model catches, the authorization URL the app built for it, whether it asked for an ephemeral session, and whether the app cancelled it or its timeout passed.
type AuthSession struct {
	Redirect  string
	URL       string
	Ephemeral bool
	Cancelled bool
	id        uint32
	scheme    bool
}

// AuthLoopback is where the model serves a loopback redirect, before the session's path.
const AuthLoopback = "http://127.0.0.1:49152"

// LookUp is a dictionary panel the app showed: its text and the box the text is drawn over, in the surface.
type LookUp struct {
	Text string
	At   core.Bounds
}

// OffscreenSurface is the headless model's record of a retained
// core.OffscreenImage rendered for canvases or image views. RenderCount
// increments only when the offscreen surface renders a new Version (on
// first use and after Invalidate or Resize), never when reused across
// subsequent frames.
type OffscreenSurface struct {
	ID          uint64
	Version     uint64
	RenderCount int
	Size        core.Size
	Scale       float32
	Ops         []driver.DrawOp
	Released    bool
}

// App is an app running in a test. Its methods run on the test's
// goroutine; the app's goroutines reach it through Scope.Post, which App
// runs when it settles.
type App struct {
	touchGeometry driver.TouchTracker
	pointerLock   driver.PointerLock
	// PreciseLocationAnswer is the authorization returned by an explicit accuracy upgrade.
	PreciseLocationAnswer bool
	// LocationCache is the last fix the platform keeps, which Locate takes when its MaxAge allows; the location and motion permissions are in Permissions, and the fixes and samples come from FeedLocation and FeedMotion.
	LocationCache     *core.LocationFix
	sensorRuns        map[uint32]*sensorRun
	offscreenSurfaces map[uint64]*OffscreenSurface
	diskSpace         driver.DiskSpaceResult
	userDirs          map[core.StandardDir]string
	userDirsPinned    bool

	t    testing.TB
	size driver.Size
	// sized is set by the Size option: without it a target that opens the app's windows opens the main window at its default size.
	sized bool
	// modality is the input modality last reported, as a platform reports
	// a key or a press before what it causes (inputBy).
	modality                  uint32
	pipSupported, pipPossible bool
	// report takes what the app reports failing (Errors).
	report func(error)
	// Clipboard is the clipboard's text, ClipboardImage its image as the
	// app last set it or reads it (a read of what is no PNG fails), and ClipboardFiles the file: URLs it
	// holds; Announced what screen readers were asked to say.
	Clipboard      string
	ClipboardImage []byte
	ClipboardFiles []string
	// ClipboardItems and ClipboardChangeCount track representation writes and the changes made to the test's clipboard.
	ClipboardItems       []core.ClipboardItem
	ClipboardChangeCount int64
	Announced            []string
	// selectedArea and selectedText remember the active selection area and its plain text set by SelectText or SelectTextRange, for mod+c, mod+a and Escape in headless tests.
	selectedArea driver.NodeID
	selectedText string
	// InputSource is the keyboard input source the app selected, as a
	// test sees it; inputSourceWas the one that selection replaced.
	InputSource    string
	inputSourceWas string
	// Speech is the speech session running, nil without one, which
	// HearSpeech and EndSpeech report to; sessions end unavailable unless
	// the environment says Speech.
	Speech        *SpeechSession
	localServices []core.LocalService
	speechID      uint32
	speechText    string

	// Spoken are the utterances the app spoke, oldest first, where the environment says Speak; the last speaks until FinishSpeaking ends it or the app stops it (Speaking). Voices are the voices the platform has, which Scope.Voices answers with.
	Spoken []*Utterance
	Voices []core.VoiceInfo
	// microphone is MicrophoneInput's fake recorder.
	microphone *Microphone
	widgets    *widgetServices
	// gameServices is this app's isolated in-memory fake for package gameservices.
	gameServices *fakeGameServices
	// The app publishes through its first surface, even when steps act on a window.
	nowPlaying    *driver.NowPlayingUpdate
	mediaTree     *core.Tree
	mediaRevision uint32
	// OpenedURLs are the URLs the app opened, oldest first. OpenURL
	// answers whether each opens; every one does when it is nil.
	OpenedURLs []string
	OpenURL    func(url string) bool
	// AuthSessions are the auth sessions the app opened, oldest first. The model catches the app's scheme as macOS's and iOS's session does, and serves a redirect without one on AuthLoopback, as desktops do. AuthAnswer answers each as it opens with the URL the provider redirects to, or an error: core.AuthCancelled for a user who closes it, another for a failure. Without AuthAnswer, or with neither, the session stays open until the app cancels it or its timeout passes.
	AuthSessions []*AuthSession
	AuthAnswer   func(s AuthSession) (callback string, err error)
	// UpdateChecks are the checks for a newer version the app asked the
	// platform's updater for (updater.Check), oldest first, true for one
	// in the background. Update answers each with what the updater found;
	// with none the app is the newest.
	updater      core.UpdaterState
	UpdateChecks []bool
	Update       func(background bool) core.UpdateCheck
	// Picks are the photo picks the app asked for, oldest first. PickImages answers each with the files the user picks; with none the user cancels. It answers headless and under `nsgo test -target` macos, windows and linux, in place of the system's picker; on the other targets the system's picker shows and PickImages is not asked.
	Picks      []ImagePick
	PickImages func(r ImagePick) []core.PickedFile
	// Photos are the photos the app asked to take, oldest first, where the environment says Camera. TakePhoto answers each with the photo the user takes, or with why it failed; with none the user cancels. It answers headless and under `nsgo test -target` through the test host, without opening a system camera.
	Photos    []ImagePick
	TakePhoto func(r ImagePick) (*core.PickedFile, error)
	// Scans are the codes the app asked to scan, oldest first, where the environment says ScanCode. ScanCode answers each with the code the camera reads, or with why it failed; with none the user closes the scanner. It answers headless and on every target, in place of the camera.
	Scans    []CodeScan
	ScanCode func(r CodeScan) (*core.ScannedCode, error)
	// PickScreenColor answers Scope.PickScreenColor with the color the user
	// samples; without it, or with false, the user cancels.
	PickScreenColor func() (driver.Color, bool)
	// FilePicks are the file picks the app asked for, oldest first.
	// PickFiles answers each with the files the user picks; with none
	// the user cancels.
	FilePicks []FilePick
	PickFiles func(r FilePick) []core.File
	// DirectoryPicks are the folder picks the app asked for, oldest
	// first. PickDirectories answers each with the folders the user
	// picks; with none the user cancels.
	DirectoryPicks  []DirectoryPick
	PickDirectories func(r DirectoryPick) []core.Directory
	// PersistedDirectories holds persisted folders keyed by Directory.Token.
	// PickDirectories with Persist stores returned directories that have a
	// Token (or Path, using Path as Token), OpenDirectory looks them up when
	// the OpenDirectory hook is nil, and ReleaseDirectory deletes them and
	// records their tokens in ReleasedDirectories.
	PersistedDirectories map[string]core.Directory
	OpenDirectory        func(token string) (core.Directory, error)
	ReleaseDirectory     func(token string) error
	ReleasedDirectories  []string
	// Saves are the saves the app asked for, oldest first. SaveFile
	// answers each with the path the user saves to, where the file is
	// written after any preflight check. An absent answer or "" cancels.
	Saves            []FileSave
	SaveFile         func(r FileSave) string
	saveDestinations map[saveKey]saveDestination
	// OpenedFiles are the files the app opened in other apps, oldest
	// first. OpenFile answers whether an app opens each; every one does
	// when it is nil.
	OpenedFiles []driver.SharedFile
	OpenFile    func(f driver.SharedFile) bool
	// RevealedFiles records paths shown in the system file manager. RevealFile
	// answers whether the manager accepted each request; nil accepts all.
	RevealedFiles []string
	RevealFile    func(path string) bool
	// TrashedFiles records paths moved to the system trash. TrashFile returns the path in the trash, or "" as Linux does, and an error, which may be core.ErrNoTrash; nil trashes each with no path.
	TrashedFiles []string
	TrashFile    func(path string) (string, error)
	// WebCookies are the cookies of the store the app's web views share (core.Scope.SetWebCookies, which core.Scope.WebCookies lists), oldest first, one the app sets again replacing the one of its name, domain and path. A test puts the cookies a run left there, as the store outlives the app. SetWebCookies answers each call before the store takes its cookies, nil to take them and an error to refuse them all; every call succeeds when it is nil.
	WebCookies    []core.WebCookie
	SetWebCookies func(cookies []core.WebCookie) error
	// WebsiteDataClears counts the app's core.Scope.ClearWebsiteData, each of which empties WebCookies.
	WebsiteDataClears int
	// Shares are the shares the app asked for, oldest first; each sheet
	// shows.
	Shares []Share
	// CaptureSurface answers headless core.Scope.Capture requests with PNG bytes
	// or an error. target is the captured *View, or nil for the whole surface.
	// When nil, headless capture answers core.CodeUnavailable.
	CaptureSurface func(target *View) ([]byte, error)
	// LookUps are the dictionary panels the app showed (desktop.LookUp), oldest first, where the environment sets CanLookUp.
	LookUps []LookUp
	// ReviewRequests counts the in-app store rating prompts the app asked for (core.Scope.RequestReview), where the environment sets CanRequestReview.
	ReviewRequests int
	// Installs are the packages handed to Linux's or Android's installer,
	// oldest first. InstallPackage can return the installation error. With
	// no function, Linux installs the package and Android opens the
	// installer. InstallAllowed is what Android's CanInstallPackages
	// answers. InstallSettings counts the requests to open its setting.
	Installs        []driver.SharedFile
	InstallPackage  func(path string) error
	InstallAllowed  bool
	InstallSettings int
	// Notifications are the notifications showing, oldest first, one of an ID replacing the one before; Badge is the count on the app's icon. NotificationPermission is what the platform answers whether the app may show them with, the user not asked at first; a request while not asked makes it NotificationAnswer, the user granting it when that is zero. Only a granted app shows them. PendingNotifications are those scheduled for a later time, where the environment says ScheduledNotifications, each At the next time it shows: Advance and FastForward show them as the clock's wall time reaches it, and a repeated one stays for its next time.
	Notifications        []core.Notification
	PendingNotifications []core.Notification
	Badge                int
	// TaskProgress is the last task progress sent by the app. A zero value
	// means the indicator is cleared.
	TaskProgress           core.TaskProgress
	NotificationPermission core.NotificationPermission
	NotificationAnswer     core.NotificationPermission
	// BiometricAvailability is the enrolled biometric the test platform
	// reports. BiometricAnswer is the result of each authentication request;
	// zero makes it unavailable. AuthenticationReasons records prompts.
	BiometricAvailability core.BiometricKind
	BiometricAnswer       core.BiometricResult
	AuthenticationReasons []string
	// MiniProgramSignIns records requests; MiniProgramSignIn supplies a code or error. Without it the platform answers unavailable.
	MiniProgramSignIns []core.MiniProgramSignInOptions
	MiniProgramSignIn  func(core.MiniProgramSignInOptions) (core.MiniProgramCredential, error)
	// AppleSignIns are the Sign in with Apple requests the app made where the environment says SignInWithApple, oldest first, their nonces hashed as the system receives them. AppleSignIn answers each with the credential the user signs in with, or an error, core.AppleSignInCancelled for a user who closes the sheet; without it the user cancels. AppleCredentialStates is what the platform answers a credential's state with, by user ID, AppleCredentialNotFound for one it does not hold.
	AppleSignIns          []AppleSignInRequest
	AppleSignIn           func(r AppleSignInRequest) (core.AppleCredential, error)
	AppleCredentialStates map[string]core.AppleCredentialState
	// Products are what the store sells where the environment says Purchases. PurchaseRequests are the in-app purchase requests the app made, oldest first. PurchaseAnswer answers each purchase with the transaction the user buys, or an error, core.PurchaseCancelled for a user who closes the sheet and core.PurchasePending for one waiting for approval; without it a product among Products is bought and any other fails. A transaction bought joins Unfinished, and Entitlements unless consumable; FinishTransaction takes it out of Unfinished into Finished, by ID, and CurrentEntitlements answers Entitlements. SyncAnswer is what SyncPurchases answers, nil by default; ManageSubscriptionsError answers ManageSubscriptions, nil by default; RefundError fails RequestRefund and otherwise RefundStatus answers it, core.RefundSuccess by default. UpdatePurchase delivers a transaction as the store does one made outside the app, and UnfinishedTransactions those the store hands the app as it starts.
	Products                 []core.Product
	PurchaseRequests         []PurchaseRequest
	PurchaseAnswer           func(r PurchaseRequest) (core.Transaction, error)
	Unfinished               []core.Transaction
	Entitlements             []core.Transaction
	Finished                 []string
	SyncAnswer               error
	ManageSubscriptionsError error
	RefundStatus             core.RefundStatus
	RefundError              error
	// transactions numbers the transactions the model's store makes.
	transactions int
	// Permissions is each permission's status as the platform answers it, PermissionNotAsked for one it does not hold; a request while not asked makes it PermissionAnswers' for it, the user granting it when that has none. Start makes both maps. PermissionSettings are the permissions whose settings the app opened, oldest first, and Guide the permission a guide shows, "" for none: GrantPermission and CloseGuide end it.
	Permissions        map[core.Permission]core.PermissionStatus
	PermissionAnswers  map[core.Permission]core.PermissionStatus
	PermissionSettings []core.Permission
	Guide              core.Permission
	guideID            uint32
	// Orientation is the app's current interface orientation request. Zero means the app uses the system policy.
	Orientation core.Orientation
	// Awake is the keep-awake holds the app keeps, oldest first, each gone once released. AwakeHolds records every hold the app took and AwakeReleases the IDs of the holds it released, oldest first. None changes the test machine's power state.
	Awake         []AwakeHold
	AwakeHolds    []AwakeHold
	AwakeReleases []uint32
	// SystemGestures is the main surface's current system gesture request.
	SystemGestures core.SystemGestureOptions
	// HapticPatterns lists the timed haptics the app played. A nil entry is a cancellation.
	HapticPatterns []core.HapticPattern
	// OverlayPermission is what the platform answers whether the app may show overlays over other apps with (mobile.OverlayPermission), denied at first, as Android's special access is. A request while denied opens the setting, which OverlaySettings counts, and makes it OverlayAnswer, which the test sets to OverlayGranted for a user who grants it. Overlay is the overlay the app opened last, kept once it closed; RevokeOverlayPermission takes the grant away.
	OverlayPermission core.OverlayPermission
	OverlayAnswer     core.OverlayPermission
	OverlaySettings   int
	Overlay           *Overlay
	// AudioDurations are the lengths of the audio the platform decodes, by a source's URI or, for bytes, their string; a source missing from it fails with audio.Decode, and a sound is cut at 5 seconds, as a platform cuts it. RefusePlayback has every player's play fail with audio.NotAllowed, as a browser's before a user gesture. Sounds are the sounds the platform decoded and Players the players the app opened, oldest first, where the environment says Audio; a player plays only as Advance moves the clock.
	AudioDurations map[string]time.Duration
	// AudioOutputLatency is the test output's latency, zero when unknown.
	AudioOutputLatency time.Duration
	RefusePlayback     bool
	Sounds             []*Sound
	audioPlaybacks     []*AudioPlayback
	Players            []*AudioPlayer
	// OnAlert answers each alert the app shows with the index of the
	// button the user presses, or -1 to dismiss it, once the step that
	// showed it ends. Without it Alerts lists them and Answer answers
	// them; one still unanswered when the next step starts gets its
	// default answer: its cancel button, else dismissed when cancelable,
	// else the step fails.
	OnAlert func(driver.AlertConfig) int
	// Appearance is the appearance the app last forced; Quits how often it asked to quit, or quit as the user asked: by a Quit menu role on Windows and Linux, or with a quit request it let proceed; Restarts how many of those restart it; BackUnhandled how many backs nothing took, which leave an Android app.
	Appearance    driver.Appearance
	Quits         int
	Restarts      int
	BackUnhandled int
	// PopupMenu is the context menu the app shows with PopupMenu, at PopupAt, or a tray icon's menu, with a zero PopupAt; nil once it closed.
	PopupMenu []MenuItem
	PopupAt   core.Point
	popupID   uint32
	// EditMenu is the touch edit menu an input opened with Ref.OpenEditMenu; nil when none shows.
	EditMenu *EditMenu
	// AccessibilityFocus is the view the app last moved the screen
	// reader's focus to.
	AccessibilityFocus *View
	// Keymap is the keyboard shortcuts the app binds, with a desktop environment in the main window; Title its title.
	Keymap []string
	Title  string
	// Language is the language the platform labels its menus in, and
	// LanguageForced whether the app forces it (core.Scope.SetLocale),
	// which Apple platforms keep for the system's words in later launches.
	Language       string
	LanguageForced bool

	// Menu is the application menu the app set: on macOS the one the main window or the app's own surface sent last, none once that surface closed, and on other desktops the main window's or else the app's own surface's; DockMenu the app icon's menu, the one any surface set last; Window the window commands it sent, oldest first.
	Menu     []MenuItem
	DockMenu []MenuItem
	Window   []WindowRequest

	// Opened are the surfaces the app opened, oldest first, as it opened them: with a desktop environment its windows, panels and tray views, a window's transparency, shadow and taskbar button among what it asked for, and on Android its overlays.
	Opened []SurfaceConfig
	// ImageSizes are the pixel sizes of the images the model's platform
	// loads, by URI, which answer Scope.ImageSize; others fail to load.
	ImageSizes map[string]driver.Size
	// ImageBytes are the encoded byte sizes of URI images the model's platform loads, by URI or ImageSource.Key, used when modeling SetImageCacheLimit. Inline ImageSource.Data sources automatically count len(Data) bytes.
	ImageBytes map[string]int64
	// ImageCacheLimit is the encoded image cache memory budget last set with core.Scope.SetImageCacheLimit, defaulting to core.DefaultImageCacheLimit.
	ImageCacheLimit int64
	// KeptImages maps each ImageSource.Key currently pinned with Scope.KeepImage
	// to its active pin count.
	KeptImages map[string]int
	// imageCache models the byte-bounded encoded image LRU cache by key.
	imageCache map[string]imageCacheEntry
	imageUse   uint64
	// canvasImages models the 64-source and byte-bounded unkept canvas image cache by key.
	canvasImages   map[string]imageCacheEntry
	canvasImageUse uint64
	// tree is the surface steps act on and the model shows: the app's only one, or with a desktop environment its main window (main) once it opened. appTree is the app's own surface with a desktop environment, which opens the windows, nil without one; surfaces are the windows that have opened, and clear the windows and panels that are transparent, opened so or made so by SetWindowTransparent, which report it as a compositing desktop does (surfaceEnv).
	// Secondary surface models share this app's graph and clock. owner is
	// the app that settles the graph and possesses app-wide services.
	owner         *App
	surfaceModels map[uint32]*Surface
	surfaceMain   *Surface
	tree          *core.Tree
	appTree       *core.Tree
	main          *core.Tree
	surfaces      map[uint32]bool
	clear         map[uint32]bool
	// own are the changes SetWindowEnvironment made to one window's environment, by surface, which surfaceEnv applies over the app's until SetEnvironment sets every window's again.
	own   map[uint32][]func(*driver.Environment)
	root  *core.Scope
	store *core.Store
	// Clock is the app's clock, which Advance and FastForward move: a ManualClock starting at 2026-01-01 00:00 UTC, its wall time with it, unless the Clock or WallTime option sets it. Options apply before the first render, so what they set is what it reads. Under Measure the app runs on the system's clock instead.
	Clock *core.ManualClock
	// wallTime is what the WallTime option sets.
	wallTime *time.Time
	// Secrets is the app's secure store, in memory: what Scope.Secrets keeps, by item, for a test to read or change, so no test touches the machine's keychain, also on a target. seedSecrets are what the Secrets option starts it with.
	Secrets     *core.MemorySecrets
	seedSecrets map[string]string
	// cloud is the app's isolated in-memory cloud store for package cloud; cloudInit is what CloudAvailable sets before Start.
	cloud     *memoryCloudStore
	cloudInit *bool
	// responseTree is the surface whose batch is being applied, which the platform's answers to what it asked go back to; guideTree the one that asked for the permission guide shown.
	responseTree *core.Tree
	guideTree    *core.Tree
	// menus are the application menus the surfaces set, by tree, nil under a target; menuBar is the surface that sent one last, menuTree the one Menu came from, dockTree the one that set DockMenu and popupTree the one that shows PopupMenu, where choosing their items goes.
	menus map[*core.Tree][]driver.MenuEntry
	// Menu entries keep the handler IDs used by steps and automation.
	menu      []driver.MenuEntry
	dockMenu  []driver.MenuEntry
	popupMenu []driver.MenuEntry
	menuBar   *core.Tree
	menuTree  *core.Tree
	dockTree  *core.Tree
	popupTree *core.Tree
	trays     map[uint32]trayRecord
	trayOrder []uint32
	// quitRequests numbers the requests a Quit menu role or RequestQuit sends, for the test's app.
	quitRequests uint32

	// StatusBar is the status bar the app last set.
	StatusBar StatusBarConfig
	// LayoutAnimation is the layout animation the last batch to carry
	// one asked the platform for.
	LayoutAnimation *driver.LayoutAnimation

	mu    sync.Mutex
	posts []func()
	// frameWanted is set while the graph waits for a display frame.
	frameWanted bool

	views map[driver.NodeID]*View
	// alertTrees are the surfaces that showed alerts, alongside them.
	alertTrees []*core.Tree
	anim       *driver.Animator
	start      time.Time
	alerts     []driver.AlertConfig
	rootID     driver.NodeID
	edits      map[driver.NodeID]uint32
	env        *driver.Environment
	// idiom is the Idiom option's, which a headless start applies over env.
	idiom *driver.Idiom
	// verticalBarEdge is the VerticalBarEdge option's, applied over env.
	verticalBarEdge *driver.VerticalBarEdge
	// keyBindings is the keymap as the platform matches it, which Key reads.
	keyBindings []driver.KeyBinding
	// heldKeys is whether the keymap asked for every key going down and up (driver.MountItem.HeldKeys).
	heldKeys bool
	// held are the keys KeyDown holds, by the name the platforms report (KeyEvent.Key).
	held map[string]heldKey
	// heldMouse is whether the keymap asked for every mouse button going down and up (driver.MountItem.HeldMouseButtons).
	heldMouse bool
	// heldButtons are the mouse buttons MouseDown holds.
	heldButtons map[driver.MouseButton]heldMouseButton
	// mouseClicks tracks per-button consecutive click counts for MouseDown / MouseUp.
	mouseClicks map[driver.MouseButton]mouseClickState
	// gamepads is the per-app synthetic gamepad source for headless tests (gamepad.go).
	gamepads *fakeGamepads

	// blocked is set while the touch responder keeps native drags from
	// starting (ItemBlockNative).
	blocked bool
	// hovered are the views the mouse is over, innermost first.
	hovered []*View
	// mouse is where it is in the surface; mouseIn says the pointer is inside.
	mouse   driver.Point
	mouseIn bool
	// stepName names the step running, for traces; trace receives them.
	stepName string
	trace    io.Writer
	// target drives the app on a platform under `nsgo test -target`.
	target *target
	// manualFrames keeps steps from waiting for animations.
	manualFrames bool
	// measure runs a target's app as its users do (Measure).
	measure bool
	// host is the embedding host's channel, nil without one.
	host *core.Host
	// bound are the views' gesture and scroll bindings.
	bound map[driver.NodeID]*Bindings
	// offscreen has selectors match views outside the window too.
	offscreen bool
	// pendingAlerts leaves alerts unanswered between steps.
	pendingAlerts bool
	// noCaretMoves keeps typing from reporting caret moves.
	noCaretMoves bool
	// hotKeyStates are the headless platform's configured registration answers.
	hotKeyStates map[string]core.HotKeyState
	// textEngine measures text; core.ApproxTextEngine when nil.
	textEngine driver.TextEngine
	// lastPress and lastPressAt are the view pressed last and when;
	// clicks counts the presses of it in a row.
	lastPress   driver.NodeID
	lastPressAt time.Time
	clicks      uint32
	// noGPU disables Env.SupportsGPU() in headless mode (NoGPU).
	noGPU bool
	// noGPUCompute disables compute shaders and storage buffers (NoGPUCompute).
	noGPUCompute bool
	gpuViews     map[driver.NodeID]*gpuViewState
	gpuFrames    []GPUFrameRecord
	// presenting holds modals currently playing their entrance animation before EventModalShown is dispatched.
	presenting map[driver.NodeID]*modalWait
}

type trayRecord struct {
	id      uint32
	title   string
	removed bool
}

type modalWait struct {
	stop func() bool
}

// Start runs app, a root view, and settles it; the test's cleanup disposes it. Under `nsgo test -target` it opens the app on that platform instead, in the main window on macOS, as there nsgo.Run shows a root view. With a macOS Environment app shows in the main window, as nsgo.Run shows a root view there, which steps act on; with any other, Windows and Linux included, it fills the app's one surface, as nsgo.Run's does. An app run with its function is tested with StartApp.
func Start(t testing.TB, app func(cx *core.Scope) core.Node, opts ...Option) *App {
	t.Helper()
	return start(t, app, nil, opts)
}

// StartApp runs the app's function app, as nsgo.Run(app) does, and settles it; the test's cleanup disposes it. app runs once in the app's own scope, where what it registers, such as its application menu, quit handlers and global shortcuts, lives. With a desktop Environment (macOS, Windows or Linux) it opens windows as on a desktop, and the view SetMainView sets shows in the main window, which steps act on; with any other that view fills the app's one surface. Under `nsgo test -target` it opens core.AppRoot(app) on that platform instead, which on macOS opens its windows as there nsgo.Run(app) does, steps acting on the main window.
func StartApp(t testing.TB, app func(app *core.Scope), opts ...Option) *App {
	t.Helper()
	return start(t, nil, app, opts)
}

// start starts a root view, view, or an app's function, fn.
func start(t testing.TB, view func(cx *core.Scope) core.Node, fn func(app *core.Scope), opts []Option) *App {
	t.Helper()
	a := &App{
		updater:  core.UpdaterState{Managed: core.UpdaterSelf, AutomaticChecks: true},
		t:        t,
		size:     driver.Size{Width: 390, Height: 844},
		Clock:    core.NewManualClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		views:    map[driver.NodeID]*View{},
		edits:    map[driver.NodeID]uint32{},
		surfaces: map[uint32]bool{},

		Permissions:       map[core.Permission]core.PermissionStatus{},
		PermissionAnswers: map[core.Permission]core.PermissionStatus{},
		Secrets:           &core.MemorySecrets{},
		ImageCacheLimit:   core.DefaultImageCacheLimit,
		KeptImages:        map[string]int{},
		diskSpace:         driver.DiskSpaceResult{Err: core.ErrDiskSpaceUnavailable},
		gameServices:      newFakeGameServices(),
	}
	for _, o := range opts {
		o(a)
	}
	if a.Clock == nil {
		t.Fatalf("nsgotest: the Clock option needs a clock, not nil")
	}
	if a.wallTime != nil {
		a.Clock.SetWallTime(*a.wallTime)
	}
	if a.microphone != nil && !onTarget() {
		a.microphone.connect(t)
	}
	if a.widgets != nil {
		a.widgets.connect(a)
	}
	if os.Getenv("NSGO_TEST_TRACE") != "" {
		a.trace = os.Stdout
		// Sites name the computations each step's commit ran.
		driver.TraceCauses, devtools.Enabled = true, true
	}
	a.stepName = "start"
	if !onTarget() && a.localServices != nil {
		var env driver.Environment
		if a.env != nil {
			env = *a.env
		}
		env.DiscoverServices = true
		a.env = &env
	}
	if a.verticalBarEdge != nil {
		env := driver.Environment{}
		if a.env != nil {
			env = *a.env
		}
		env.VerticalBarEdge = *a.verticalBarEdge
		a.env = &env
	}
	if onTarget() && a.manualFrames {
		t.Skip("nsgotest: ManualFrames steps the clock, which runs headless only")
	}
	if onTarget() {
		a.startTarget(view, fn)
		return a
	}
	if a.idiom != nil {
		// On a copy, as one Environment option may start other apps too.
		env := driver.Environment{}
		if a.env != nil {
			env = *a.env
		}
		env.Idiom = *a.idiom
		a.env = &env
	}
	a.cloud = newMemoryCloudStore(a.Clock.WallTime)
	if a.cloudInit != nil {
		env := driver.Environment{}
		if a.env != nil {
			env = *a.env
		}
		env.CloudStorage = *a.cloudInit
		a.env = &env
		a.cloud.setAvailable(*a.cloudInit)
	} else if a.env != nil {
		a.cloud.setAvailable(a.env.CloudStorage)
	}
	// Where nsgo.Run opens windows the app does here: on macOS, and on Windows and Linux for an app's function. Its main view shows in the main window, where CurrentWindow, MainWindow and OnClosing work and CloseWindow closes it.
	var desktop bool
	if a.env != nil {
		switch a.env.OS {
		case driver.OSMacOS:
			desktop = true
		case driver.OSWindows, driver.OSLinux:
			desktop = fn != nil
		}
	}
	var windows *core.WindowOptions
	if desktop {
		windows = &core.WindowOptions{Width: a.size.Width, Height: a.size.Height}
	}
	// A library may run headless inside a browser, whose runtime has no filesystem. Its app directories are empty, as on the web backend.
	var data, cache, logs string
	userDirs := a.userDirs
	if runtime.GOOS != "js" {
		dir := t.TempDir()
		if !a.userDirsPinned {
			userDirs = testhost.MakeUserDirs(filepath.Join(dir, "user"))
		}
		data, cache, logs = filepath.Join(dir, "data"), filepath.Join(dir, "cache"), filepath.Join(dir, "logs")
		for _, path := range []string{data, cache, logs} {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatalf("nsgotest: cannot create app directory: %v", err)
			}
		}
	}
	a.root = core.NewRoot(core.RootOptions{
		// Tests check that each commit sends all that changed; a running app the automation follows lays each commit out once, as its users' does.
		CheckCommits: !automationBuild,
		Post: func(fn func()) {
			a.mu.Lock()
			a.posts = append(a.posts, fn)
			a.mu.Unlock()
		},
		Clock: a.Clock,
		Report: func(err error) {
			if a.report != nil {
				a.report(err)
				return
			}
			t.Errorf("nsgotest: the app failed: %v", err)
		},
		// Frames run as the clock advances, as a platform's display does.
		RequestFrame: func() {
			a.mu.Lock()
			a.frameWanted = true
			a.mu.Unlock()
		},
		Storage:   a.store,
		ReadAsset: assetReader(),
		DataDir:   data, CacheDir: cache, LogsDir: logs,
		UserDir:      func(d core.StandardDir) string { return userDirs[d] },
		Secrets:      a.Secrets,
		Cloud:        a.cloud,
		GameServices: a.gameServices,
		Updater:      a.updater,
		Host:         a.host,
		HotKeys:      true,
		Windows:      windows,
	})
	a.gamepads = newFakeGamepads()
	unbindGamepads := driver.BindGamepadSource(a.root, a.gamepads)
	t.Cleanup(func() {
		// Keys KeyDown and buttons MouseDown still hold go with the app; on a target they come up through the platform (target.releaseHeld).
		a.held = nil
		a.heldButtons = nil
		a.root.Dispose()
		unbindGamepads()
	})
	a.seed()
	a.start = a.Clock.Now()
	a.anim = driver.NewAnimator(func(ev driver.Event) {
		a.mu.Lock()
		a.posts = append(a.posts, func() { a.tree.Dispatch(ev) })
		a.mu.Unlock()
	})
	var engine driver.TextEngine = core.ApproxTextEngine{}
	if a.textEngine != nil {
		engine = a.textEngine
	}
	a.tree = core.NewTree(a.root, engine)
	a.tree.SetGPUSubmitter(a.submitGPU)
	// Steps reach every view, not only those in sight.
	a.tree.SetViewPacing(false)
	a.tree.Resize(a.size, driver.Geometry{})
	env := driver.Environment{}
	if a.env != nil {
		env = *a.env
	}
	if !a.noGPU {
		env.GPU = true
	} else {
		env.GPU = false
	}
	a.env = &env
	env.ClipboardItems = true
	a.tree.SetEnvironment(env)
	if desktop {
		// The app's own surface runs the app's function, as on a desktop; a root view is one showing it as the main view, as nsgo.Run makes it.
		a.appTree = a.tree
		if fn == nil {
			main := view
			fn = func(cx *core.Scope) { cx.SetMainView(main) }
		}
	}
	if fn != nil {
		view = core.AppRoot(fn)
	}
	a.launchPurchases()
	a.tree.Mount(view)
	a.done()
	return a
}

// seed keeps the values of the Secrets option, before the app runs.
func (a *App) seed() {
	s := a.root.Secrets()
	for account, v := range a.seedSecrets {
		if err := s.Set(account, v); err != nil {
			a.t.Fatalf("nsgotest: seeding the secret %q: %v", account, err)
		}
	}
}

// Storage returns the app's storage, to read what it kept or change it as
// another of the app's windows would.
func (a *App) Storage() core.Storage {
	if a.target != nil {
		return a.target.storage()
	}
	return a.root.Storage()
}

// Settle runs what the app's goroutines posted and applies the commits that follow, until nothing is left. On a target it first waits for the engine to hand its pending commits to the platform, then until no batch came and nothing finite animated for 100 ms, checking the engine again before it returns, five seconds at most in all.
func (a *App) Settle() {
	if a.owner != nil {
		a.owner.Settle()
		return
	}
	if a.target != nil {
		a.target.settle()
		return
	}
	for {
		a.mu.Lock()
		posts := a.posts
		a.posts = nil
		a.mu.Unlock()
		for _, fn := range posts {
			fn()
		}
		began := time.Now()
		var batches int
		commit := func(t *core.Tree) {
			b := t.Commit()
			if b == nil {
				return
			}
			batches++
			a.traceBatch(b, time.Since(began))
			a.responseTree = t
			a.setTransparent(t, b)
			if s := a.surfaceModels[t.ID()]; s != nil {
				s.updateTitle(b)
			}
			if t == a.tree {
				a.apply(b)
			} else {
				a.applyAside(b)
				if window := a.surfaceModels[t.ID()]; window != nil {
					window.model.applySurface(b)
				}
			}
			a.responseTree = nil
		}
		if a.appTree == nil {
			a.openSurface(a.surfaceConfig(a.tree.ID()), a.tree, true)
			commit(a.tree)
		} else {
			// The app's own surface first: it opens the windows, the main one among them, whose batches follow.
			commit(a.appTree)
			for _, surface := range slices.Clone(a.appTree.Surfaces()) {
				if surface.Closed() {
					a.closeSurface(surface.ID())
					a.closed(surface)
					surface.Forget()
					continue
				}
				if !a.surfaces[surface.ID()] {
					a.surfaces[surface.ID()] = true
					surface.SetViewPacing(false)
					if a.env != nil {
						surface.SetEnvironment(a.surfaceEnv(surface, *a.env))
					}
					// The main window, which SetMainView opens, is the one steps act on, from its first batch: not a panel, a tray view or another window the app opened before it.
					if a.main == nil && surface.Window() != nil && surface.Window() == a.root.MainWindow() {
						a.main, a.tree = surface, surface
						a.showMenu()
					}
				}
				s := a.openSurface(a.surfaceConfig(surface.ID()), surface, surface == a.main)
				if s.model != a && a.env != nil {
					env := a.surfaceEnv(surface, *a.env)
					s.model.env = &env
				}
				commit(surface)
				if s.model != a {
					s.model.animate()
				}
			}
		}
		overlaid := a.commitOverlay()
		a.animate()
		a.mu.Lock()
		more := len(a.posts) > 0
		a.mu.Unlock()
		if batches == 0 && !overlaid && len(posts) == 0 && !more {
			if runtime.GOOS != "js" || !a.yieldToPage() {
				return
			}
		}
	}
}

// yieldToPage lets the page's engine run what it queued, as the reactions
// of the promises a TSX app settled, which run only once Go gives the page
// its turn, and reports whether that posted anything.
func (a *App) yieldToPage() bool {
	time.Sleep(time.Millisecond)
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.posts) > 0
}

// Post runs fn on the app's graph goroutine as the next step or wait settles: the test goroutine headless, the engine goroutine on a target. It is safe to call from any goroutine, fn may read or set the app's signals and post more, and the step settles what they post too. As fn may run off the test goroutine, it takes no steps and does not call t.Fatal or t.FailNow: check what it did after the step.
func (a *App) Post(fn func()) {
	if a.owner != nil {
		a.owner.Post(fn)
		return
	}
	a.postTest(func() { a.postGraph(fn) })
}

// postGraph hands a queued test function to the graph on its owning goroutine.
func (a *App) postGraph(fn func()) {
	if t := a.target; t != nil && !t.live {
		t.postApp(fn)
		return
	}
	fn()
}

// postTest queues fn to run on the test goroutine as the next step or wait settles, also on a target.
func (a *App) postTest(fn func()) {
	a.mu.Lock()
	a.posts = append(a.posts, fn)
	a.mu.Unlock()
}

// settleLimit is how long of the clock's time a step waits for the app
// to be quiet.
const settleLimit = 2 * time.Second

// step starts the step name: the alerts the last one left unanswered get
// their default answer.
func (a *App) step(format string, args ...any) {
	a.t.Helper()
	a.stepName = fmt.Sprintf(format, args...)
	if a.owner != nil {
		a.owner.step(format, args...)
		return
	}
	for len(a.alerts) > 0 && !a.pendingAlerts {
		index, err := defaultAnswer(a.alerts[0])
		if err != nil {
			a.t.Fatalf("nsgotest: %v", err)
		}
		a.answerAt(0, index)
	}
}

// done ends a step: it settles, then takes display frames while an animation that ends runs, a style transition waits for its OnTransitionEnd or lists bind the rows they pace, for at most settleLimit of the clock's time, on a target only the transitions' as the platform runs the rest, and has OnAlert answer the alerts shown, taking the frames of what those answers start in turn. Frames the app asks for itself do not hold a step, since its own loops may never end: Advance runs them.
func (a *App) done() {
	a.t.Helper()
	if a.owner != nil {
		a.owner.done()
		return
	}
	answered := 0
	for {
		if a.target != nil {
			a.target.takeFrames()
		} else {
			a.takeFrames()
		}
		if !a.answerAlerts(&answered) {
			return
		}
	}
}

// alertAnswers is how many alerts OnAlert answers in one step before the step fails: an answer that brings its alert back each time would never end the step.
const alertAnswers = 100

// takeFrames settles, then takes display frames while an animation that ends runs, a style transition waits for its OnTransitionEnd or lists bind the rows they pace, for at most settleLimit of the clock's time.
func (a *App) takeFrames() {
	a.t.Helper()
	a.Settle()
	limit := a.Clock.Now().Add(settleLimit)
	for !a.manualFrames && a.surfaceWork() && a.Clock.Now().Before(limit) {
		a.Clock.Advance(driver.FrameInterval)
		a.runFrame()
		a.Settle()
	}
}

// answerAlerts has OnAlert answer the alerts shown, counting them in answered, and reports whether it answered any; the step fails once the step's answers pass alertAnswers.
func (a *App) answerAlerts(answered *int) bool {
	a.t.Helper()
	took := false
	for a.OnAlert != nil && len(a.alerts) > 0 {
		if *answered == alertAnswers {
			a.t.Fatalf("nsgotest: %s: OnAlert answered %d alerts and another shows (%q): an answer that brings its alert back each time never ends the step", a.stepName, alertAnswers, a.alerts[0].Title)
		}
		*answered++
		a.answerAt(0, a.OnAlert(a.alerts[0]))
		took = true
	}
	return took
}

// defaultAnswer is how a user backing out answers alert: its cancel
// button, else dismissing it when it can be.
func defaultAnswer(alert driver.AlertConfig) (int, error) {
	for i, b := range alert.Actions {
		if b.Role == driver.AlertCancel {
			return i, nil
		}
	}
	if alert.Cancelable {
		return -1, nil
	}
	name := alert.Title
	if name == "" {
		name = alert.Message
	}
	return 0, fmt.Errorf("the alert %q needs an answer: answer it with Answer or OnAlert", name)
}

func (a *App) wantsFrame() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.frameWanted
}

// Advance moves the clock by d, runs the timers and display frames that
// come due and settles. A frame runs every FrameInterval while the app
// waits for one, as a display's frames do. Measuring on a target, it
// waits d.
func (a *App) Advance(d time.Duration) {
	a.t.Helper()
	if a.target != nil {
		a.target.advance(d, true)
		return
	}
	a.stepName = fmt.Sprintf("advance(%v)", d)
	a.elapse(d)
	a.Settle()
}

// elapse moves the clock by d a frame at a time, as Advance does.
func (a *App) elapse(d time.Duration) {
	if a.owner != nil {
		a.owner.elapse(d)
		return
	}
	for d > 0 {
		step := min(d, driver.FrameInterval)
		a.Clock.Advance(step)
		a.playAudio(step)
		a.showDueNotifications()
		d -= step
		a.runFrame()
		a.Settle()
	}
}

// FastForward jumps the clock ahead by d, as a laptop closed for a while
// and opened: each timer that comes due on the way runs once, at the
// end, and so does one display frame.
func (a *App) FastForward(d time.Duration) {
	a.t.Helper()
	if a.target != nil {
		a.target.advance(d, false)
		return
	}
	a.stepName = fmt.Sprintf("fastForward(%v)", d)
	a.Clock.Jump(d)
	a.playAudio(d)
	a.showDueNotifications()
	a.runFrame()
	a.Settle()
}

// FrameAt runs the display frame the app waits for, stamped at, and settles; the clock does not move. A zero at stamps the frame with the clock's now, as today's frame is. A frame stamped a frame ahead of the clock, or before it, is what a display that stamps a frame's show time hands the app. It skips a target run, which drives its own frames.
func (a *App) FrameAt(at time.Time) {
	a.t.Helper()
	if a.target != nil {
		a.t.Skip("nsgotest: FrameAt drives the headless frame loop")
	}
	a.step("frameAt(%s)", at.Format(time.RFC3339Nano))
	a.mu.Lock()
	wanted := a.frameWanted
	a.frameWanted = false
	a.mu.Unlock()
	if wanted {
		a.root.RunFrameAt(at)
	}
	a.Settle()
}

// runFrame runs the display frame the app waits for, if it waits for one.
func (a *App) runFrame() {
	a.mu.Lock()
	wanted := a.frameWanted
	a.frameWanted = false
	a.mu.Unlock()
	if wanted {
		a.root.RunFrame()
	}
}

// WaitFor settles until selector matches a view, failing the test after
// timeout of real time. Use it for work on other goroutines.
func (a *App) WaitFor(selector string, timeout time.Duration) *View {
	a.t.Helper()
	a.WaitUntil(timeout, func() bool { return len(a.Query(selector)) == 1 })
	return a.Find(selector)
}

// WaitForGone settles until no view matches selector, failing the test
// after timeout of real time.
func (a *App) WaitForGone(selector string, timeout time.Duration) {
	a.t.Helper()
	a.WaitUntil(timeout, func() bool { return len(a.Query(selector)) == 0 })
}

// WaitUntil settles until cond holds, failing the test after timeout of real time. Headless it advances one frame between checks unless ManualFrames was set; on a target it takes a step's frames between checks, moving the clock only while a style transition waits for its OnTransitionEnd.
func (a *App) WaitUntil(timeout time.Duration, cond func() bool) {
	a.t.Helper()
	deadline := time.Now().Add(timeout)
	clockApp := a
	if a.owner != nil {
		clockApp = a.owner
	}
	a.Settle()
	for !cond() {
		if time.Now().After(deadline) {
			a.t.Fatalf("nsgotest: the condition did not hold after %v\n%s", timeout, a.Dump())
		}
		time.Sleep(time.Millisecond)
		switch {
		case a.target != nil:
			a.target.takeFrames()
		case !clockApp.manualFrames:
			clockApp.elapse(driver.FrameInterval)
		default:
			a.Settle()
		}
	}
}

// surfaceEnv is the app's environment env as the platform reports it to surface: a window or a panel that is transparent reports it, as on a compositing desktop, and a window SetWindowEnvironment changed reports its own state.
func (a *App) surfaceEnv(surface *core.Tree, env driver.Environment) driver.Environment {
	env.ClipboardItems = true
	env.WindowTransparent = a.clear[surface.ID()]
	for _, change := range a.own[surface.ID()] {
		change(&env)
	}
	return env
}

// setTransparent records the transparency a window's batch b asks for (WindowSetTransparent) and reports it in the window's environment, as a compositing desktop does.
func (a *App) setTransparent(t *core.Tree, b *driver.MountBatch) {
	if a.appTree == nil || t == a.appTree {
		return
	}
	// A window's own, which a panel keeps as it opened, as the platforms do.
	if i := slices.IndexFunc(a.Opened, func(s SurfaceConfig) bool { return s.ID == t.ID() }); i < 0 || a.Opened[i].Kind != SurfaceWindow {
		return
	}
	for _, it := range b.Items {
		if it.Kind != driver.ItemWindow || it.Window.Action != driver.WindowSetTransparent || a.clear[t.ID()] == it.Window.On {
			continue
		}
		if a.clear == nil {
			a.clear = map[uint32]bool{}
		}
		a.clear[t.ID()] = it.Window.On
		if a.env != nil && a.surfaces[t.ID()] {
			t.SetEnvironment(a.surfaceEnv(t, *a.env))
		}
	}
}

// SetEnvironment reports env as a platform would and settles.
func (a *App) SetEnvironment(env driver.Environment) {
	a.t.Helper()
	a.env = &env
	if a.target != nil {
		a.target.setEnvironment(env)
		return
	}
	env.AudioPlaybackControls = env.Audio
	env.ClipboardItems = true
	if a.cloud != nil {
		a.cloud.setAvailable(env.CloudStorage)
	}
	if a.appTree != nil {
		// Every window shares the app's environment, and those it opens later start from it.
		a.own = nil
		a.appTree.SetEnvironment(env)
		for _, surface := range a.appTree.Surfaces() {
			if !surface.Closed() {
				surface.SetEnvironment(a.surfaceEnv(surface, env))
			}
		}
	} else {
		a.env = &env
		a.tree.SetEnvironment(env)
	}
	a.reportScreenInsets()
	for _, s := range a.surfaceModels {
		if s.model != a {
			s.model.reportScreenInsets()
		}
	}
	a.Settle()
}

// SetVerticalBarEdge updates only the system's preferred vertical bar edge on the current environment and settles.
func (a *App) SetVerticalBarEdge(edge driver.VerticalBarEdge) {
	a.t.Helper()
	var env driver.Environment
	if a.env != nil {
		env = *a.env
	}
	env.VerticalBarEdge = edge
	a.SetEnvironment(env)
}

// VerticalBarEdge returns the system's preferred vertical bar edge reported to the app's environment.
func (a *App) VerticalBarEdge() driver.VerticalBarEdge {
	if a.root != nil {
		return a.root.Env().VerticalBarEdge()
	}
	if a.env != nil {
		return a.env.VerticalBarEdge
	}
	return driver.VerticalBarUnspecified
}

// SetWindowEnvironment reports one window's environment changed by change, as a platform reports a window minimized, zoomed, full screen or out of sight while the app's other windows stay as they are, and settles: the window the app opened titled title, or the main window for "". The app's own surface, whose Lifecycle is every window's together, and the other windows keep the app's environment. Changes add up until SetEnvironment sets every window's again; each is applied again whenever the window's environment is reported, so change should only set fields of env. It needs a desktop environment, where the app opens windows, and runs headless only.
func (a *App) SetWindowEnvironment(title string, change func(*driver.Environment)) {
	a.t.Helper()
	if a.target != nil {
		a.t.Skipf("nsgotest: SetWindowEnvironment reports what a platform would: it runs headless only")
	}
	if a.env == nil || a.appTree == nil {
		a.t.Fatalf("nsgotest: SetWindowEnvironment needs a desktop environment, where the app opens windows")
	}
	window := a.window(title)
	if a.own == nil {
		a.own = map[uint32][]func(*driver.Environment){}
	}
	a.own[window.ID()] = append(a.own[window.ID()], change)
	window.SetEnvironment(a.surfaceEnv(window, *a.env))
	a.Settle()
}

// window is the open window titled title, or the main window for "", failing the test when there is none or more than one.
func (a *App) window(title string) *core.Tree {
	a.t.Helper()
	if title == "" {
		if a.main == nil || a.main.Closed() {
			a.t.Fatalf("nsgotest: no main window is open")
		}
		return a.main
	}
	var found []*core.Tree
	for _, s := range a.Opened {
		if s.Kind != SurfaceWindow || s.Title != title {
			continue
		}
		for _, surface := range a.appTree.Surfaces() {
			if surface.ID() == s.ID && !surface.Closed() {
				found = append(found, surface)
			}
		}
	}
	if len(found) != 1 {
		a.t.Fatalf("nsgotest: %d open windows are titled %q", len(found), title)
	}
	return found[0]
}

// Resize sets the surface size and settles.
func (a *App) Resize(width, height driver.Points) {
	a.t.Helper()
	a.size = driver.Size{Width: width, Height: height}
	if a.target != nil {
		a.target.resize(a.size)
		return
	}
	// With a desktop environment the surface is the main window's; other windows keep their own sizes.
	if a.appTree != nil && a.appTree != a.tree {
		a.appTree.Resize(a.size, driver.Geometry{})
	}
	a.tree.Resize(a.size, driver.Geometry{})
	a.Settle()
}

// FindMounted returns the one view selector matches among every view
// mounted, shown or not: hidden, empty, scrolled away or clipped, as a
// test reading what the platform received of a view finds it.
func (a *App) FindMounted(selector string) *View {
	a.t.Helper()
	vs := a.QueryMounted(selector)
	switch len(vs) {
	case 0:
		a.t.Fatalf("nsgotest: no view matches %q\n%s", selector, a.Dump())
	case 1:
		return vs[0]
	default:
		a.t.Fatalf("nsgotest: %d views match %q\n%s", len(vs), selector, a.Dump())
	}
	return nil
}

// QueryMounted returns the views selector matches among every view
// mounted, shown or not, in tree order.
func (a *App) QueryMounted(selector string) []*View {
	a.t.Helper()
	sel, err := parseSelector(selector)
	if err != nil {
		a.t.Fatalf("nsgotest: %v", err)
	}
	return a.resolveIn(sel, a.mounted())
}

// Find returns the one visible view selector matches.
func (a *App) Find(selector string) *View {
	a.t.Helper()
	vs := a.Query(selector)
	switch len(vs) {
	case 0:
		a.t.Fatalf("nsgotest: no view matches %q\n%s", selector, a.Dump())
	case 1:
		return vs[0]
	default:
		a.t.Fatalf("nsgotest: %d views match %q\n%s", len(vs), selector, a.Dump())
	}
	return nil
}

// Exists reports whether a visible view matches selector.
func (a *App) Exists(selector string) bool {
	a.t.Helper()
	return len(a.Query(selector)) > 0
}

// Enabled reports whether the one view selector matches takes input:
// neither it nor a view around it is disabled, and an input is editable.
func (a *App) Enabled(selector string) bool {
	a.t.Helper()
	return !a.Find(selector).disabled(nil)
}

// Query returns the visible views selector matches, the topmost modal's
// first, then the rest in tree order.
func (a *App) Query(selector string) []*View {
	a.t.Helper()
	sel, err := parseSelector(selector)
	if err != nil {
		a.t.Fatalf("nsgotest: %v", err)
	}
	return a.resolve(sel)
}

// Texts returns the texts shown, in the order Query finds them.
func (a *App) Texts() []string {
	var out []string
	for _, v := range a.byLayer() {
		if v.Kind == KindText {
			out = append(out, v.Text)
		}
	}
	return out
}

// SpanStyle returns the resolved style of the characters selector names
// in a text: "text:<chars>" (or a chain ending in it) finds the text
// that shows them, and the style is that of their first character, as a
// span sets it over the text's.
func (a *App) SpanStyle(selector string) driver.TextStyle {
	a.t.Helper()
	return a.Span(selector).Style
}

// Span returns the span of the characters selector names in a text, as
// SpanStyle finds them: its style and what it draws behind them; the
// text's own style where no span sets one.
func (a *App) Span(selector string) driver.TextSpan {
	a.t.Helper()
	sel, err := parseSelector(selector)
	if err != nil {
		a.t.Fatalf("nsgotest: %v", err)
	}
	last := sel.parts[len(sel.parts)-1]
	v := a.Find(selector)
	if v.Kind != KindText || last.kind != "text" {
		a.t.Fatalf("nsgotest: %q names no characters of a text; end it in text:", selector)
	}
	at := -1
	if last.text.re != nil {
		if loc := last.text.re.FindStringIndex(v.Text); loc != nil {
			at = loc[0]
		}
	} else {
		at = strings.Index(v.Text, last.text.want)
	}
	if at < 0 || at >= len(v.Text) {
		a.t.Fatalf("nsgotest: %q names no character of %q", selector, v.Text)
	}
	for _, sp := range v.Spans {
		if at >= sp.Start && at < sp.End {
			return sp
		}
	}
	return driver.TextSpan{Start: 0, End: len(v.Text), Style: v.TextStyle}
}

// checkForUpdates records a check for a newer version and answers it as
// Update says.
func (a *App) checkForUpdates(it *driver.MountItem) {
	if p := it.UpdatePreference; p != nil {
		if p.AutomaticChecks != nil {
			a.updater.AutomaticChecks = *p.AutomaticChecks
		}
		if p.AutomaticDownloads != nil {
			a.updater.AutomaticDownloads = *p.AutomaticDownloads
		}
		state := driver.UpdaterState{Managed: driver.UpdateManaged(a.updater.Managed), Store: a.updater.Store, NativePreferences: a.updater.NativePreferences,
			AutomaticChecks: a.updater.AutomaticChecks, AutomaticDownloads: a.updater.AutomaticDownloads, Available: a.updater.Available, Notes: a.updater.Notes, LastCheck: a.updater.LastCheck}
		a.post(driver.Event{Kind: driver.EventUpdate, Data: &state})
		return
	}
	a.UpdateChecks = append(a.UpdateChecks, it.On)
	var c core.UpdateCheck
	if a.Update != nil {
		c = a.Update(it.On)
	}
	ev := driver.Event{Kind: driver.EventUpdate, Count: it.Count, Finished: c.Shown}
	if !c.Shown {
		u := &driver.Update{Restart: c.Restart, App: c.App, Current: c.Current, Version: c.Version, URL: c.URL, Notes: c.Notes,
			Download: c.Download, Signature: c.Signature, SHA256: c.SHA256, Key: c.Key, Size: c.Size, Managed: driver.UpdateManaged(c.Managed), Store: c.Store}
		if c.Err != nil {
			u.Err = c.Err.Error()
		}
		ev.Data = u
	}
	a.post(ev)
}

// Alerts returns the alerts shown and not answered yet, oldest first.
func (a *App) Alerts() []driver.AlertConfig { return a.alerts }

// Answer chooses the button at index (-1: dismiss) of the oldest alert
// shown. It is a step as Press is: it returns once what the answer starts,
// as a sheet's exit, has run its frames.
func (a *App) Answer(index int) {
	a.t.Helper()
	if len(a.alerts) == 0 {
		a.t.Fatal("nsgotest: no alert to answer")
	}
	a.stepName = fmt.Sprintf("answer(%d)", index)
	a.answerAt(0, index)
	a.done()
}

// answerAt chooses the button at index of the alert at i in Alerts, headless on the surface that showed it, as each numbers its alerts itself.
func (a *App) answerAt(i, index int) {
	alert := a.alerts[i]
	a.alerts = slices.Delete(slices.Clone(a.alerts), i, i+1)
	if a.target != nil {
		a.target.answer(alert, index)
		return
	}
	tree := a.alertTrees[i]
	a.alertTrees = slices.Delete(slices.Clone(a.alertTrees), i, i+1)
	a.dispatchOn(tree, driver.Event{Kind: driver.EventAlert, Count: alert.ID, Index: index})
}

// PictureInPictureSupport configures the headless system's PiP support
// and current readiness. Native tests always use their platform's state.
func PictureInPictureSupport(supported, possible bool) Option {
	return func(a *App) { a.pipSupported = supported; a.pipPossible = supported && possible }
}
