package driver

import (
	"crypto/sha256"
	"fmt"
	"image"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// NodeID identifies a view of one tree. IDs start at 1, stay dense and
// are reused after a Delete.
type NodeID uint32

// ViewKind is the kind of native view a node mounts.
type ViewKind uint8

const (
	// KindView is a container: background, border, corners and presses.
	KindView ViewKind = iota + 1
	// KindText draws a TextLayout the engine prepared.
	KindText
	// KindInput is the platform's text field.
	KindInput
	// KindScroll scrolls its children; ItemScrollContent sizes them.
	KindScroll
	// KindImage shows an image the platform loads (ItemImage).
	KindImage
	// KindModal is a layer covering the root view or an outer modal,
	// presented when inserted and dismissed when removed (ItemModal).
	KindModal
	// KindCanvas plays a Drawing (ItemCanvas).
	KindCanvas
	// KindWeb hosts the platform's web view, filling it (ItemWeb).
	KindWeb
	// KindVideo hosts the platform's video player, filling it (ItemVideo).
	KindVideo
	// KindNative hosts a view of a component the app registered, filling
	// it (ItemNative).
	KindNative
	// KindShader fills itself with a fragment shader on the GPU
	// (ItemShader).
	KindShader
	// KindGPU hosts a native 3D GPU-backed surface (ItemGPU, docs/architecture/gpu.md).
	KindGPU
)

// ItemKind is what a mount item does.
type ItemKind uint8

const (
	// ItemCreate makes a detached view of Kind for ID.
	ItemCreate ItemKind = iota + 1
	// ItemInsert inserts or moves ID into Parent, below sibling Before
	// (on top when 0). Siblings rather than indices, so views a platform
	// adds of its own cannot shift positions.
	ItemInsert
	// ItemRemove detaches ID from its parent and keeps it.
	ItemRemove
	// ItemDelete detaches and destroys ID; its ID may be created again
	// later in the same batch.
	ItemDelete
	// ItemFrame moves and sizes ID relative to its parent (Frame).
	ItemFrame
	// ItemViewStyle sets what any view draws (ViewStyle).
	ItemViewStyle
	// ItemText sets a text view's content (Text).
	ItemText
	// ItemInput configures a text field (Input).
	ItemInput
	// ItemInputValue sets a text field's value (Value) unless the user
	// edited it after the edit numbered Count.
	ItemInputValue
	// ItemEvents sets which events the view reports (Events).
	ItemEvents
	// ItemViewProps sets accessibility and identity (ViewProps).
	ItemViewProps
	// ItemScrollConfig configures a scroll view (Scroll).
	ItemScrollConfig
	// ItemScrollContent sets the size of a scroll view's content (Size).
	ItemScrollContent
	// ItemFocus moves keyboard focus to ID.
	ItemFocus
	// ItemBlur takes keyboard focus away from ID.
	ItemBlur
	// ItemImage sets an image view's source and look (Image).
	ItemImage
	// ItemModal configures a modal (Modal).
	ItemModal
	// ItemAlert shows a native alert (Alert); the platform answers with
	// EventAlert.
	ItemAlert
	// ItemDismissKeyboard takes focus away from the focused text field.
	ItemDismissKeyboard
	// ItemScrollTo scrolls ID (a scroll view or multiline input) to Point (its content's offset, UIKit's contentOffset: ContentInset makes it negative at the start), animated when Animated; the platform clamps it and reports EventScroll. Platforms count the ItemScrollTo and ItemScrollBy items they applied to each view and report the count with
	// its offsets.
	ItemScrollTo
	// ItemCanvas replaces a canvas's drawing (Drawing), in the view's
	// coordinates.
	ItemCanvas
	// ItemSetClipboard puts Value on the clipboard.
	ItemSetClipboard
	// ItemReadClipboard asks for what Clipboard names; the platform
	// answers with EventClipboard carrying Count.
	ItemReadClipboard
	// ItemAnnounce has the screen reader say Value.
	ItemAnnounce
	// ItemInputSource selects the keyboard input source named Value, or
	// with none goes back to the one it replaced, where the platform lets
	// an app.
	ItemInputSource
	// ItemKeymap replaces the surface's keyboard shortcuts (Keys), and whether it reports every key going down and up (HeldKeys); the platform consumes matching key presses and reports EventKey.
	ItemKeymap
	// ItemStatusBar sets the status bar (StatusBar): the layers of
	// settings merged.
	ItemStatusBar
	// ItemTitle sets the window's or page's title (Value).
	ItemTitle
	// ItemBackUnhandled says nothing took the last EventBack: Android
	// leaves the app, as its back button does; others ignore it.
	ItemBackUnhandled
	// ItemWindow asks the desktop window to change (Window); platforms
	// without one ignore it.
	ItemWindow
	// ItemMenu replaces the application menu (Menu; nil restores the
	// platform's); a chosen item is reported with EventMenu.
	ItemMenu
	// ItemWeb configures a web view (Web).
	ItemWeb
	// ItemWebCommand has a web view do WebCommand, with Value for the message or script and Data for commands such as WebLoad.
	ItemWebCommand
	// ItemVideo configures a video (Video).
	ItemVideo
	// ItemVideoSeek jumps a video to Time.
	ItemVideoSeek
	// ItemNative names a native view's component and sets its changed
	// props (Native).
	ItemNative
	// ItemNativeCommand has a native view's component run the command
	// Value with Data.
	ItemNativeCommand
	// ItemAnimation changes the native animation graph (Animation), after
	// the batch's views exist.
	ItemAnimation
	// ItemZoomTo zooms a zoomable scroll view (Zoom), animated when
	// Animated; the platform clamps it and reports EventScroll.
	ItemZoomTo
	// ItemImageInfo loads Image's Source, keeping its bytes for the views
	// that show it next; the platform answers with EventImageInfo carrying
	// Count.
	ItemImageInfo
	// ItemSelection selects Selection in text field ID, after the batch's
	// value; the platform clamps it to the text.
	ItemSelection
	// ItemScrollBy moves scroll view ID's offset by Point without
	// clamping it, carrying a drag or glide in flight along, so the
	// visible content stays in place as rows above it change size.
	ItemScrollBy
	// ItemStickyCells replaces the sticky cells of scroll view ID
	// (Sticky): on every scroll the platform moves each along the main
	// axis by clamp(offset - Start, 0, End - Start) from its frame.
	ItemStickyCells
	// ItemAccessibilityLabel changes the label (Value) of a view whose
	// ViewProps were sent before, and nothing else: a label following app
	// state changes alone, so the rest is not sent again.
	ItemAccessibilityLabel
	// ItemAccessibilityFocus moves the screen reader's focus to ID.
	ItemAccessibilityFocus
	// ItemScreen configures a screen's system header (Screen) on the
	// platforms that report header heights, before the screen is
	// inserted and when it changes.
	ItemScreen
	// ItemOverflow gives the area ID's children cover outside its bounds by their layout frames (Frame, in its own coordinates), or zero Bounds when they stay inside, for platforms to look for a touch's target there; see Reach. Transformed says a child, or one of theirs, has been drawn transformed and may show anywhere, so a hit test looks in the children wherever the point is; see ReachView. Sent only when either changes.
	ItemOverflow
	// ItemBlockNative stops the scroll views around ID, the touch
	// responder, from taking the touches until they end, or lets them
	// again with no ID.
	ItemBlockNative
	// ItemShader sets a shader view's program and uniforms (Shader), or a
	// view's effect, transition or screen shader (Shader.Role), nil
	// Shader taking it away.
	ItemShader
	// ItemSetClipboardImage puts the encoded image Data ([]byte) on the
	// clipboard.
	ItemSetClipboardImage
	// ItemOpenURL opens Value outside the app; the platform answers with
	// EventOpenURL carrying Count.
	ItemOpenURL
	// ItemPickImages shows the system's photo picker for Pick; the
	// platform answers with EventPick carrying Count.
	ItemPickImages
	// ItemAppearance shows the app in Appearance whatever the system's,
	// native chrome included.
	ItemAppearance
	// ItemPopupMenu shows Menu as the platform's context menu at Point,
	// in the root's points; desktops report a chosen item with EventMenu
	// and answer with EventMenuClosed carrying Count.
	ItemPopupMenu
	// ItemQuit ends the app: desktops close the window and return from
	// Run, Android finishes its activity; iOS and the web ignore it. On
	// says the app restarts, which nsgo.Run does once Run returns.
	ItemQuit
	// ItemBackPreview answers EventBackStarted with the screen a back now
	// pops (ID), for the platform to move it with the gesture; 0 when back
	// goes elsewhere first or nothing pops.
	ItemBackPreview
	// ItemHistory is how many entries back can go through (Count): the
	// screens above each stack's first and the modals shown. The web
	// keeps that many browser history entries; others ignore it.
	ItemHistory
	// ItemPickFiles shows the system's file picker for Files; the
	// platform answers with EventFiles carrying Count.
	ItemPickFiles
	// ItemSaveFile shows the system's save dialog for Save and saves the
	// file where the user chose; the platform answers with EventFiles
	// carrying Count.
	ItemSaveFile
	// ItemDirection says whether the app lays out right to left (RTL), as
	// the language it shows is written, whatever the system's, and that
	// language (Language), and whether the app forces it (Forced,
	// Scope.SetLocale): on its first commit and when any changes.
	// Platforms whose own motion follows a direction, as iOS's navigation,
	// take it, those that draw menus label them in the language, and
	// standalone apps apply a forced one to what the system says in the
	// app (from the next launch on Apple platforms, and immediately on
	// Android 13+, Windows and Linux).
	ItemDirection
	// ItemPickScreenColor lets the user pick a color on the screen: the
	// system's color sampler or the browser's EyeDropper, answered with
	// EventFiles Count, Finished with the color in Data, a Color. Sent only
	// where the environment says ScreenColor.
	ItemPickScreenColor
	// ItemPickDirectories shows the system's folder picker for
	// Directories; the platform answers with EventFiles carrying Count.
	ItemPickDirectories
	// ItemOpenFile opens the file Open in the app the system associates
	// with it; the platform answers with EventFiles carrying Count.
	ItemOpenFile
	// ItemShare shows the system's share sheet for Share; the platform
	// answers with EventFiles carrying Count once it shows.
	ItemShare
	// ItemInstallPackage hands Open to the platform's package installer.
	// EventFiles carrying Count reports when Linux finishes or when
	// Android opens the system installer for the user to confirm.
	ItemInstallPackage
	// ItemCanInstallPackages asks whether the user lets the app install
	// packages; Android answers with EventFiles carrying Count.
	ItemCanInstallPackages
	// ItemInstallSettings opens the system's setting that lets the app
	// install packages; Android answers with EventFiles carrying Count.
	ItemInstallSettings
	// ItemOpenSurface opens a window, a panel or a tray icon's view
	// (Surface) for the tree the core numbered Surface.ID, which the
	// platform attaches to the engine (engine.Engine.Attach). Only the
	// app's own surface, the first, is sent it.
	ItemOpenSurface
	// ItemTray adds, changes or removes a tray icon (Tray); the app's own
	// surface is sent it, and EventTray reports its clicks.
	ItemTray
	// ItemTrayMenu shows Menu at the place of tray icon Tray.ID, numbered
	// Count as ItemPopupMenu's menus are.
	ItemTrayMenu
	// ItemHotKeys replaces the app's global shortcuts (Keys, each with its ID), which fire whichever app has the focus with EventHotKey. The platform answers each key with EventHotKeyState once it knows whether the key registered.
	ItemHotKeys
	// ItemDock shows the app in the Dock and the app switcher (On), or
	// leaves it to its tray icon and windows.
	ItemDock
	// ItemSetLaunchAtLogin has the system start the app as the user logs
	// in (On), or no more.
	ItemSetLaunchAtLogin
	// ItemLaunchAtLogin asks whether the system starts the app at login;
	// the platform answers with EventLaunchAtLogin carrying Count.
	ItemLaunchAtLogin
	// ItemShaderTransition starts view ID's shader transition, or a
	// page's or a dialog's (ShaderTransition, ShaderNav): it comes first in
	// its batch, so the platform pictures what the view shows before the
	// batch as the transition's first side.
	ItemShaderTransition
	// ItemShaderInput says what view ID's shader pictures changed in the
	// batch, so the platform pictures it again once the batch is applied:
	// an effect's or a transition's children, a screen shader's surface.
	ItemShaderInput
	// ItemSpeech starts, stops or aborts speech session Count (Speech);
	// the platform reports it with EventSpeech carrying Count.
	ItemSpeech
	// ItemTakePhoto shows the system's camera to take one photo, prepared
	// as Pick says; the platform answers with EventPick carrying Count,
	// Text saying what failed. Sent only where the environment says
	// Camera.
	ItemTakePhoto
	// ItemMatch has view ID, which the batch creates and inserts, move and
	// resize from the frame view Before shows, which the batch deletes,
	// on the batch's LayoutAnimation Update instead of appearing with its
	// Create; Before stops showing at once (View.MatchID). It comes after
	// the round's inserts and before its deletes, so both views are in
	// place, and only in a batch whose LayoutAnimation has an Update.
	ItemMatch
	// ItemCheckForUpdates asks the platform's updater for a newer version
	// of the app, in the background when On; the platform answers with
	// EventUpdate carrying Count.
	ItemCheckForUpdates
	// ItemHaptic plays haptic feedback Count (a Haptic) where the device
	// has it and the user allows it.
	ItemHaptic
	// ItemNotificationPermission asks whether the app may show
	// notifications, with the system's prompt when On; the platform
	// answers with EventNotification carrying Count and the permission as
	// Index (a NotificationPermission).
	ItemNotificationPermission
	// ItemShowNotification shows Notification, or schedules it for its At, replacing one of its ID, shown or pending; the platform answers with EventNotification carrying Count, Finished when the system took it.
	ItemShowNotification
	// ItemCloseNotification removes the notification whose ID is Value, shown or pending, and every notification ShowNotification showed when Value is empty (core.Scope.CloseNotifications).
	ItemCloseNotification
	// ItemBadge shows Count on the app's icon; 0 clears it.
	ItemBadge
	// ItemBiometricAvailability asks whether an enrolled biometric can be
	// used; the platform answers with EventBiometric and its kind in Index.
	ItemBiometricAvailability
	// ItemBiometricAuthenticate asks the user to authenticate with the
	// reason in Value; the platform answers with EventBiometric and its
	// result in Index.
	ItemBiometricAuthenticate
	// ItemQuitResponse answers EventQuitRequest Count, after the kinds Android's Protocol.java mirrors so that they keep their values. On permits the platform's pending termination request; false cancels it.
	ItemQuitResponse
	// ItemOverlayPermission asks whether the app may show windows over other apps, Android's SYSTEM_ALERT_WINDOW, opening the system's setting for the app when On; the platform answers with EventOverlayPermission carrying Count and the permission as Index (an OverlayPermission), after the setting once the app is back in front.
	ItemOverlayPermission
	// ItemPermission, after the kinds Android's Protocol.java mirrors, carries Permission, a request numbered Count about one of the system's permissions; the platform answers each one but PermissionStopGuide with EventPermission carrying Count, the status as Index.
	ItemPermission
	// ItemRevealFile shows Open in the system file manager, selected.
	ItemRevealFile
	// ItemTrashFile moves Open to the system Trash or Recycle Bin.
	ItemTrashFile
	// ItemKeepAwake takes a power assertion described by Awake and identified
	// by Count; ItemReleaseAwake releases the assertion identified by Count.
	ItemKeepAwake
	ItemReleaseAwake
	// ItemTaskProgress, after the kinds Android's Protocol.java mirrors, sets the app's task progress bar to Progress.
	ItemTaskProgress
	// ItemAudio asks of sound or player Count what Audio says; the platform reports it with EventAudio carrying Count. The core sends none where the environment says no Audio.
	ItemAudio
	// ItemAuthSession asks of auth session Count what Auth says; the platform reports it with EventAuthSession carrying Count.
	ItemAuthSession
	// ItemSignInWithApple shows Sign in with Apple's sheet for AppleSignIn; the platform answers with EventSignInWithApple carrying Count, Data an *AppleCredential once the user signs in, else Text the AppleSignIn error. The core sends none where the environment says no SignInWithApple.
	ItemSignInWithApple
	// ItemAppleCredentialState asks the state of the Sign in with Apple credential of the user whose ID is Value; the platform answers with EventSignInWithApple carrying Count, Index the state (AppleCredentialAuthorized, ...), else Text the AppleSignIn error. The core sends none where the environment says no SignInWithApple.
	ItemAppleCredentialState
	// ItemDockMenu, after the kinds Android's Protocol.java mirrors, replaces the app icon's menu (Menu; nil removes it); a chosen item is reported with EventDockMenu.
	ItemDockMenu
	// ItemRecentFile records Value in the platform's recent documents, or clears the app's when On.
	ItemRecentFile
	// ItemSpeak asks the platform's speech synthesizer what Speak says, of utterance or request Count; the platform reports it with EventSpeak carrying Count. The core sends none where the environment says no Speak.
	ItemSpeak
	// ItemSensor starts, stops or queries sensor session Count.
	ItemSensor
	// ItemPurchase asks the platform's store what Purchase says; the platform answers with EventPurchase carrying Count and Data a *PurchaseAnswer. The core sends none where the environment says no Purchases.
	ItemPurchase
	// ItemWebCookies sets, lists or clears what Cookies asks of the persistent store the app's web views share; the platform answers with EventWebCookies carrying Count, Text why it failed and, for a list, Data.
	ItemWebCookies
	// ItemLookUp shows the platform's dictionary panel for Value, drawn over Frame, a box in the surface.
	ItemLookUp
	// ItemPictureInPicture configures a detached custom-content tree.
	ItemPictureInPicture
	// ItemPictureInPictureCommand starts, stops or acknowledges restoration.
	ItemPictureInPictureCommand
	// ItemScanCode shows the system's camera to read one code of the kinds Scan asks for; the platform answers with EventPick carrying Count and Data the *ScannedCode read, none when the user closed it, Text saying what failed. Sent only where the environment says ScanCode.
	ItemScanCode
	// ItemQuickLook shows the system's file preview panel on QuickLook's Paths, starting at its Index. Sent only where the environment says CanQuickLook.
	ItemQuickLook
	// ItemOrientation asks the app to use the Orientation mask in Count, or the platform default when zero.
	ItemOrientation

	// ItemKeepImage pins the canvas image cache entry named by Value
	// (ImageSource.Key) so it stays out of LRU eviction while kept;
	// ItemReleaseImage releases one pin of Value, letting it be evicted once
	// its count reaches zero.
	ItemKeepImage
	ItemReleaseImage
	// ItemReleaseOffscreen releases the cached offscreen surface for OffscreenID.
	ItemReleaseOffscreen
	// ItemRequestReview asks the platform's store to show an in-app rating prompt. Sent only where the environment says CanRequestReview.
	ItemRequestReview
	// ItemGPU configures a KindGPU view's swapchain color/depth formats and sample count (GPU).
	ItemGPU
	// ItemSavePreflightAnswer answers EventSavePreflight Count. On permits
	// the write and Value holds the reason for a refusal.
	ItemSavePreflightAnswer
	// ItemSaveFilePreflight is ItemSaveFile with preflight on Android's pipe.
	// It keeps the existing save item's wire format unchanged.
	ItemSaveFilePreflight
	// ItemDiskSpace asks for the capacity of the volume containing Value, answered by EventDiskSpace carrying Count.
	ItemDiskSpace

	// ItemDiscovery starts or stops a DNS-SD operation numbered by Count.
	ItemDiscovery
	// ItemSystemGestures replaces the surface's gesture deferrals with SystemGestures.
	ItemSystemGestures
	// ItemCursorPoint asks for the desktop pointer position and receives EventCursorPoint with the same Count.
	ItemCursorPoint
	// ItemPointerLock captures relative motion for ID, numbered Count.
	// On requests a lock. A false On releases the matching request.
	ItemPointerLock
	// ItemHapticPattern replaces the current haptic pattern, or stops it when empty.
	ItemHapticPattern
	// ItemPageHead replaces the web document's scoped title and description. Sent only to a web backend; an embedded app leaves its host's head alone.
	ItemPageHead
	// ItemEditMenu opens or closes the platform's touch edit menu on input ID (UIEditMenuInteraction on iOS, a floating ActionMode on Android), anchored clear of Frame in the surface's points and offering the EditAction bits in Count; Count 0 closes any edit menu open on the input. Choosing Cut, Copy or Select All reports EventEditAction on ID with that action in Count; Paste runs the input's own paste (EventPaste).
	ItemEditMenu
	// ItemImageCacheLimit sets the maximum number of encoded image bytes kept in memory (Bytes). Negative values are clamped to zero by the core.
	ItemImageCacheLimit
	// ItemMiniProgramSignIn asks for a platform login code, answered by EventMiniProgramSignIn. On permits a host-account prompt on Douyin. Sent only where MiniProgramSignIn is available.
	ItemMiniProgramSignIn
	// ItemAppClipOverlay presents (On) or dismisses the calling scope's full-app recommendation. Count identifies its owner request and Value is bottom or bottom-raised. iOS App Clips alone send it.
	ItemAppClipOverlay
	// ItemSetClipboardItems replaces the clipboard with Data ([]ClipboardItem). When Count is nonzero, EventClipboard reports the outcome.
	ItemSetClipboardItems
	// ItemNowPlaying replaces the app's publication (NowPlaying), or clears it when nil.
	ItemNowPlaying
)

// EditAction is an editing action of an input's touch edit menu (ItemEditMenu, EventEditAction).
type EditAction uint32

const (
	// EditCut cuts the selection.
	EditCut EditAction = 1 << iota
	// EditCopy copies the selection.
	EditCopy
	// EditPaste pastes from the clipboard through the input's own paste.
	EditPaste
	// EditSelectAll selects the whole document or field.
	EditSelectAll
)

// PageHead is the merged metadata of the visible web.Head components. An unset field restores the page's original value; a set empty one clears it.
type PageHead struct {
	Title, Description       string
	HasTitle, HasDescription bool
}

// DefaultImageCacheLimit is the default maximum number of encoded image bytes kept in memory by the image cache (32 MiB).
const DefaultImageCacheLimit int64 = 32 << 20

// AppleSignInRequest is an ItemSignInWithApple's request, as core.AppleSignInOptions: the scopes it asks for and Nonce, which the core hashes as the request carries it, "" for none.
type AppleSignInRequest struct {
	FullName, Email bool
	Nonce           string
}

// AppleCredential is what EventSignInWithApple answers a sign-in with, as core.AppleCredential.
type AppleCredential struct {
	User, IdentityToken, AuthorizationCode, Email                       string
	GivenName, FamilyName, MiddleName, NamePrefix, NameSuffix, Nickname string
}

// Why EventSignInWithApple's Text says a sign-in or a credential state failed, as core.AppleSignInError.
const (
	AppleSignInCancelled   = "cancelled"
	AppleSignInFailed      = "failed"
	AppleSignInUnavailable = "unavailable"
)

// The states of a Sign in with Apple credential, as core.AppleCredentialState.
const (
	AppleCredentialAuthorized = iota + 1
	AppleCredentialRevoked
	AppleCredentialNotFound
	AppleCredentialTransferred
)

// OverlayPermission is what EventOverlayPermission answers an ItemOverlayPermission with, as core's.
const (
	OverlayDenied = iota
	OverlayGranted
	OverlayUnavailable
)

// Why EventWindowEnded says the platform ended an overlay, as core's OverlayEnd.
const (
	OverlayEndDenied = iota + 1
	OverlayEndUnavailable
	OverlayEndRevoked
	OverlayEndRemoved
	OverlayEndFailed
)

// Biometric kinds, as reported by an availability request.
const (
	BiometricNone = iota
	BiometricFace
	BiometricFingerprint
)

// Biometric results, as reported by an authentication request.
const (
	BiometricSuccess = iota + 1
	BiometricFailure
	BiometricCancelled
	BiometricUnavailable
)

// NoBiometric is how a platform without biometric authentication answers a biometric request: no kind for availability, unavailable for authentication.
func NoBiometric(it *MountItem) Event {
	result := BiometricNone
	if it.Kind == ItemBiometricAuthenticate {
		result = BiometricUnavailable
	}
	return Event{Kind: EventBiometric, Count: it.Count, Index: result}
}

// Notification is a system notification (ItemShowNotification), as core.Notification. At is when it shows, zero for now; core sends a later one only where Environment.ScheduledNotifications says so, and with Repeat, At is the first time it shows, after now, and its clock reading in At's location the time of day it shows again, in the device's time zone.
type Notification struct {
	ID, Title, Body string
	Silent          bool
	At              time.Time
	Repeat          NotificationRepeat
}

// NotificationRepeat is how a scheduled notification shows again, as core's.
type NotificationRepeat uint8

const (
	NotificationOnce NotificationRepeat = iota
	NotificationDaily
	NotificationWeekly
)

// NotificationPermission is what EventNotification answers an
// ItemNotificationPermission with, as core's.
const (
	NotificationsNotAsked = iota
	NotificationsGranted
	NotificationsDenied
	NotificationsUnavailable
)

// NoNotification is how a platform that shows no notifications answers ItemNotificationPermission and ItemShowNotification: the permission is unavailable, and the notification is not shown.
func NoNotification(it *MountItem) Event {
	if it.Kind == ItemNotificationPermission {
		return Event{Kind: EventNotification, Count: it.Count, Index: NotificationsUnavailable}
	}
	return Event{Kind: EventNotification, Count: it.Count}
}

// Haptic is what ItemHaptic plays, as core.Haptic.
type Haptic = uint32

// The haptics, as core's.
const (
	HapticSelection Haptic = iota + 1
	HapticLight
	HapticMedium
	HapticHeavy
	HapticSoft
	HapticRigid
	HapticSuccess
	HapticWarning
	HapticError
)

// MountItem is one change to the native views. Kind says which fields
// are set; the large groups are pointers so items stay small.
type MountItem struct {
	Kind   ItemKind
	Head   *PageHead // ItemPageHead
	ID     NodeID
	View   ViewKind // ItemCreate
	Parent NodeID   // ItemInsert
	Before NodeID   // ItemInsert
	Frame  Bounds   // ItemFrame
	Size   Size     // ItemScrollContent
	Value  string   // ItemInputValue
	Count  uint32   // ItemInputValue
	// Address is ItemHistory's optional application address, applied after reconciling Count's navigation depth. Push asks for an entry of its own.
	Address *PageAddress
	// Transition, when set, moves an ItemFrame's view from where it shows
	// to Frame, as a batch's LayoutAnimation Update does, which wins.
	Transition *LayoutTransition
	// Point and Animated are ItemScrollTo's.
	Point    Point
	Animated bool
	// Transformed is ItemOverflow's.
	Transformed bool
	// Clipboard is what an ItemReadClipboard reads.
	Clipboard ClipboardRead
	// Appearance is ItemAppearance's, RTL, Language and Forced
	// ItemDirection's.
	Appearance Appearance
	RTL        bool
	Language   string
	Forced     bool
	Events     EventMask

	ViewStyle *ViewStyle
	Text      *TextContent
	Input     *InputConfig
	ViewProps *ViewProps
	Scroll    *ScrollConfig
	Image     *ImageConfig
	Modal     *ModalConfig
	Alert     *AlertConfig
	Drawing   *Drawing
	Shader    *Shader
	GPU       *GPUViewConfig
	// Keys are ItemKeymap's bindings, oldest first; see FindKey.
	Keys []KeyBinding
	// HeldKeys is ItemKeymap's ask for every key going down and up on the surface, not only those of a focused view that hears keys: the platform reports a down no such view hears with ID 0 (EventKeyDown) and its up (EventKeyUp), for core.KeysDown. It leaves out keys typed while a text input or a web view has the focus or an input method composes, as for a view's keys.
	HeldKeys bool
	// HeldMouseButtons is ItemKeymap's ask for every mouse button going down and up on the surface, not only those on a view that hears mouse buttons: the platform reports a button press no such view hears with ID 0 (EventMouseDown) and its release (EventMouseUp), for core.MouseButtonsDown.
	HeldMouseButtons bool
	Window           *WindowCommand
	Menu             []MenuEntry
	Web              *WebConfig
	// WebCommand is ItemWebCommand's.
	WebCommand       WebCommand
	PictureInPicture *PictureInPictureConfig
	Video            *VideoConfig
	Time             time.Duration // ItemVideoSeek
	Native           *NativeConfig
	// Data is ItemNativeCommand's argument.
	Data      any
	Animation []AnimationOp
	Zoom      *ZoomRequest
	// Selection is ItemSelection's range.
	Selection TextRange
	// Sticky are ItemStickyCells' cells.
	Sticky    []StickyCell
	StatusBar *StatusBarConfig
	Screen    *ScreenConfig
	// Pick is ItemPickImages' and ItemTakePhoto's request.
	Pick *PickRequest
	// Scan is ItemScanCode's request.
	Scan *ScanRequest
	// Discovery is ItemDiscovery's request.
	Discovery *DiscoveryRequest
	// Sensor is an ItemSensor's request, Speech an ItemSpeech's, Audio an ItemAudio's.
	Sensor *SensorRequest
	Speech *SpeechRequest
	Audio  *AudioRequest
	// HapticPattern is the timed feedback ItemHapticPattern plays.
	HapticPattern *HapticPattern
	// NowPlaying is the app's publication (ItemNowPlaying).
	NowPlaying *NowPlayingUpdate
	// Auth is an ItemAuthSession's request.
	Auth *AuthRequest
	// Cookies is an ItemWebCookies' request.
	Cookies *WebCookiesRequest
	// AppleSignIn is an ItemSignInWithApple's request.
	AppleSignIn *AppleSignInRequest
	// Speak is an ItemSpeak's request.
	Speak *SpeakRequest
	// Purchase is an ItemPurchase's request.
	Purchase *PurchaseRequest
	// Notification is what ItemShowNotification shows.
	Notification *Notification
	// Permission is an ItemPermission's request.
	Permission *PermissionRequest
	// Awake is ItemKeepAwake's power assertion.
	Awake *KeepAwake
	// Progress is ItemTaskProgress's state.
	Progress *TaskProgress
	// Files is ItemPickFiles' request, Save ItemSaveFile's,
	// Directories ItemPickDirectories', Open ItemOpenFile's and
	// ItemInstallPackage's and Share ItemShare's.
	Files       *FilesRequest
	Save        *SaveRequest
	Directories *DirectoriesRequest
	Open        *SharedFile
	// QuickLook is ItemQuickLook's request.
	QuickLook *QuickLookRequest
	// SystemGestures is ItemSystemGestures' complete surface preference.
	SystemGestures *SystemGestureOptions
	Share          *ShareRequest
	// Surface is ItemOpenSurface's window, Tray ItemTray's icon, and On
	// ItemDock's and ItemSetLaunchAtLogin's choice and whether
	// ItemCheckForUpdates checks in the background.
	Surface *SurfaceConfig
	Tray    *TrayConfig
	// UpdatePreference changes updater settings instead of checking its feed.
	UpdatePreference *UpdatePreference
	On               bool
	// OffscreenID is the resource ID of an ItemReleaseOffscreen.
	OffscreenID uint64
	// Bytes is ItemImageCacheLimit's memory budget in bytes.
	Bytes int64
}

// QuickLookRequest is an ItemQuickLook's files, regular files the core checked, and the index of the one shown first, within them.
type QuickLookRequest struct {
	Paths []string
	Index int
}

// StickyCell is a sticky list cell and its travel, in its scroll view's main-axis offsets: from Start it moves with the offset, as far as End (which may be infinite). End below Start pins to the viewport's end, with a negative shift back toward the content's start.
type StickyCell struct {
	Cell       NodeID
	Start, End Points
	// Node, when set, is the animated node the cell's inset from the
	// viewport's start follows, in place of Inset, which Start counts.
	Node  uint32
	Inset Points
}

// StickyShift is how far a sticky cell moves from its frame along the
// main axis at offset.
func (c StickyCell) StickyShift(offset Points) Points {
	travel := c.End - c.Start
	return min(max(offset-c.Start, min(travel, 0)), max(travel, 0))
}

// StickyShift is c.StickyShift with the cell's inset the value its node
// has, as backends place a cell bound to an animated node. A nil
// animator keeps the inset c was laid out with.
func (a *Animator) StickyShift(c StickyCell, offset Points) Points {
	if c.Node != 0 && a != nil {
		if v, _, ok := a.Value(c.Node); ok {
			offset += Points(v) - c.Inset
		}
	}
	return c.StickyShift(offset)
}

// MountBatch is one commit's changes, applied in order on the main thread.
// Root is the view the platform shows in its surface.
type MountBatch struct {
	Root  NodeID
	Items []MountItem
	// GeometryVersion identifies the committed tree for DevTools' native geometry snapshots. Zero when no displayed geometry was requested.
	GeometryVersion uint64
	// LayoutAnimation, when set, animates the batch's frame changes,
	// insertions and removals on the platform (Scope.AnimateLayout).
	LayoutAnimation *LayoutAnimation
	// Sent is when the engine handed the batch to the platform, for the
	// performance monitor's latency reading; the zero time when no monitor
	// measures.
	Sent time.Time
	// Causes are what the commit followed from, recorded while TraceCauses is set; nil otherwise.
	Causes *Causes
}

// ViewStyle is everything a platform draws for a view itself.
type ViewStyle struct {
	Background Color
	// CornerRadius rounds every corner, unless Corners is set: then each
	// corner has its own radius. Platforms fit radii that do not fit
	// with Corners.Fit.
	CornerRadius Points
	Corners      Corners
	// ContinuousCorners eases the corners into the edges (RoundedRect)
	// instead of drawing quarter circles.
	ContinuousCorners bool
	// BorderWidth and BorderColor are the border on every side, unless
	// Borders is set: then each side has its width there and its color
	// in BorderColors.
	BorderWidth  Points
	BorderColor  Color
	Borders      Edges
	BorderColors EdgeColors
	// BorderStyle dashes or dots the border; 0 is solid.
	BorderStyle BorderStyle
	Outline     Outline
	Opacity     float32
	// Clip clips children to the bounds and corners.
	Clip         bool
	ShadowColor  Color
	ShadowOffset Point
	ShadowRadius Points
	// TintColor recolors an image's opaque pixels; 0 keeps its colors.
	TintColor Color
	// Cursor is the pointer's look over the view; Auto inherits the
	// nearest ancestor's, and a pressable view's is a pointer on the web.
	Cursor Cursor
	// CursorImage is an optional custom cursor image over the view; it
	// travels behind a pointer so ViewStyle stays comparable, and the core
	// keeps the same pointer while Equal.
	CursorImage *CustomCursor
	// Transform moves, scales and rotates the view around its transform
	// origin without moving its children's frames; hit testing follows
	// it. The core has resolved its percentages.
	Transform Transform
	// Origin is where the transform acts, from the view's center, and
	// OriginZ how far toward the viewer.
	Origin  Point
	OriginZ Points
	// BackfaceHidden hides the view while its rotations turn its back to
	// the viewer.
	BackfaceHidden bool
	// ChildPerspective, when set, is the depth children's rotations out
	// of the plane are seen from, at the view's center (ChildPerspective
	// the function).
	ChildPerspective Points
	BlendMode        BlendMode
	// Material and Glass are how iOS draws a backdrop blur, and macOS one of a material other than BackdropUltraThin or of glass.
	Material BackdropMaterial
	// Elevation is Android's: how high the view floats, which casts the
	// system's shadow under it where the style casts none.
	Elevation Points
	Glass     GlassEffect
	// GlassInteractive lets Liquid Glass answer presses on platforms that support it (macOS 27 and iOS 26).
	GlassInteractive bool
	// Effects are shadows, gradients, filters and masks, nil without any.
	Effects *Effects
	// Transition, when it has a duration, animates the change from the
	// previous ViewStyle to this one.
	Transition Transition
	// Animation runs keyframes over the view while it is set; a change
	// starts it again.
	Animation KeyframeAnimation
	// Hidden hides the view and its children (display none, visibility
	// hidden).
	Hidden bool
}

// DefaultViewStyle is the style of a view no style changes: opaque and
// nothing else. Backends create views in it, and the core sends a new
// view's style only when it differs, as it sends no empty ViewProps and
// no empty EventMask.
var DefaultViewStyle = ViewStyle{Opacity: 1}

// Radii returns the radius of each corner, fitted to size.
func (vs *ViewStyle) Radii(size Size) Corners {
	c := vs.Corners
	if c == (Corners{}) {
		c = AllCorners(vs.CornerRadius)
	}
	return c.Fit(size)
}

// Rounded reports whether some corner is rounded.
func (vs *ViewStyle) Rounded() bool { return vs.CornerRadius > 0 || vs.Corners != (Corners{}) }

// Rim is how a rounded view with a fill and one solid border, either of
// them opaque, keeps the fill from showing past it. Drawn as CSS says, both reach the
// box's edge, and an edge pixel a curve covers partly takes the fill's
// color before the border's. So the fill stops inset under the border,
// and the border is rim, what it shows over the fill, the outer edge
// being the border's alone. Inset most of the border's width, a line one
// pixel wide keeps the two apart too, and the rest closes the seam
// inside. Browsers do the same for CSS; ok is false
// where nothing needs it or the border must let what is under it show.
func (vs *ViewStyle) Rim() (rim Color, inset Points, ok bool) {
	if !vs.Rounded() || vs.BorderWidth <= 0 || vs.Borders != (Edges{}) || vs.BorderColor&0xff == 0 || vs.Background&0xff == 0 ||
		vs.Background&0xff != 0xff && vs.BorderColor&0xff != 0xff || vs.BorderStyle == BorderDashed || vs.BorderStyle == BorderDotted || (vs.Effects != nil && len(vs.Effects.Gradients) > 0) {
		return 0, 0, false
	}
	return vs.BorderColor.Over(vs.Background), vs.BorderWidth * 3 / 4, true
}

// Matrix returns the view's transform for a box of size, around its
// origin.
func (vs *ViewStyle) Matrix(size Size) Matrix3D {
	m := vs.Transform.Matrix(Point{X: size.Width/2 + vs.Origin.X, Y: size.Height/2 + vs.Origin.Y})
	if z := float64(vs.OriginZ); z != 0 {
		m = Translate3D(0, 0, z).Mul(m).Mul(Translate3D(0, 0, -z))
	}
	return m
}

// ImageSampling selects how a raster image is sampled when scaled.
type ImageSampling uint8

const (
	ImageSamplingSmooth ImageSampling = iota
	ImageSamplingNearest
)

// ImageConfig is an image view's source and look.
type ImageConfig struct {
	Source       ImageSource
	Placeholder  ImageSource
	ResizeMode   ResizeMode
	Sampling     ImageSampling
	BlurRadius   Points
	FadeDuration time.Duration
	HitTestAlpha float32
	// Position is the style's ObjectPosition: where the fitted image sits
	// (ResizeMode.Fit). It travels with the image, not the ViewStyle.
	Position ObjectPosition
}

// ModalConfig is how a modal appears and goes.
type ModalConfig struct {
	Animation ModalAnimation
	// Presentation, NoCloseOnSwipe and Orientations are iOS's. A sheet
	// reports its size with EventModalSize, its swipes with
	// EventRequestClose.
	Presentation   ModalPresentation
	NoCloseOnSwipe bool
	Orientations   Orientation
	// NoCloseOnEscape and NoCloseOnBackdropPress are the app's: the
	// core and base keep the modal on those, and tests read them, as
	// StatusBarTranslucent, Android's, which the core lays the modal out
	// by.
	NoCloseOnEscape, NoCloseOnBackdropPress bool
	StatusBarTranslucent                    bool
}

// ScreenConfig is a screen's system header, which the platform draws
// over the top of the screen's view, HeaderHeight tall.
type ScreenConfig struct {
	Title string
	// HeaderHidden hides the header; HeaderHeight is then 0.
	HeaderHidden      bool
	HeaderTransparent bool
	LargeTitle        bool
	// TitleFont and LargeTitleFont override the native title fonts; zero fields keep their platform defaults.
	TitleFont, LargeTitleFont Font
	// RightWithLargeTitle moves Android's trailing view beside its large title, returning to the toolbar as the header collapses.
	RightWithLargeTitle bool
	// CanGoBack says a screen is below; BackHidden hides the back button
	// to it, and BackTitle is its label.
	CanGoBack  bool
	BackHidden bool
	BackTitle  string
	// OwnBackTitle is the screen's own HeaderBackTitle: the label of the
	// back button to it on the screen above, which that screen's
	// BackTitle shows.
	OwnBackTitle string
	// NoPopOnSwipe turns the swipe back off, as NoPopOnBack does with the
	// back button's popping: a press still reports EventBack.
	NoPopOnSwipe bool
	NoPopOnBack  bool
	// Background, TitleColor and TintColor are the header's colors; 0
	// keeps the platform's.
	Background, TitleColor, TintColor Color
	// Left, Right and TitleView are the header views, each a layout root that no view holds, or 0. Left and Right take their content's size; TitleView expands to fill the room between the bar's ends when TitleFills is set and otherwise takes its content's size bounded by that room.
	Left, Right, TitleView NodeID
	TitleFills             bool
	// Header is the screen's own header, which a platform without a
	// native one would show; the native header leaves it unused.
	Header NodeID
	// RightSystemItem, with a Right view, is the UIKit button iOS shows
	// in Right's place; a press on it presses Right's first view that
	// takes presses.
	RightSystemItem HeaderSystemItem
	// RightSymbols, with a Right view, are the UIKit buttons iOS shows in
	// its place, a line each of an SF Symbol's name, a tab and its label. A
	// press on one presses that of Right's views that take presses.
	RightSymbols string
	// HeaderHeight is the area the header covers from the screen's top:
	// the bar and the part of the status bar the screen reaches under.
	HeaderHeight Points
	// Animation is how the screen enters and leaves.
	Animation ScreenAnimation
}

// HeaderSystemItem is a button iOS draws in a navigation bar in UIKit's
// own look, its symbol or label.
type HeaderSystemItem uint8

const (
	HeaderItemAdd HeaderSystemItem = iota + 1
	HeaderItemDone
	HeaderItemClose
	HeaderItemEdit
	HeaderItemCompose
	HeaderItemSearch
	HeaderItemShare
	HeaderItemTrash
	HeaderItemSave
	HeaderItemCancel
)

// TextContent is a text view's content, laid out by the engine.
type TextContent struct {
	// Text is the plain text, for accessibility.
	Text string
	// Layout is the platform's TextLayout, ready to draw. The platform
	// owns it from now on and releases the one it replaces.
	Layout TextLayout
	// Content is where Layout goes in the view: inside padding and border.
	Content Bounds
	// Style, Spans, Shadow, MaxLines and Ellipsize are the text's, as it
	// was prepared, for tests to read; platforms draw Layout and read
	// Spans' links for presses.
	Style     TextStyle
	Spans     []TextSpan
	Shadow    TextShadow
	MaxLines  int
	Ellipsize EllipsizeMode
}

// InputConfig is a text field's look and behavior.
type InputConfig struct {
	Style        TextStyle
	Placeholder  string
	Masked       bool
	Readonly     bool
	Disabled     bool
	KeyboardType KeyboardType
	// Content is where the text goes in the view: inside padding and border.
	Content Bounds
	// Padding surrounds Content inside the border. A multi-line editor
	// scrolls in the padding box, its text inset by Padding.
	Padding Edges
	// Accessory is the InputAccessoryView that rides above the keyboard
	// while the field has focus, or 0; AccessoryID is the ID the field
	// names it by, which tests read.
	Accessory   NodeID
	AccessoryID string
	// Multiline takes several lines in a multi-line control.
	Multiline bool
	// MaxLength caps what the user enters, in UTF-16 units; 0 sets none.
	MaxLength int
	// SubmitBehavior is as set; platforms read Submit.
	SubmitBehavior SubmitBehavior
	ReturnKey      ReturnKeyType
	AutoCapitalize AutoCapitalize
	NoAutoCorrect  bool
	AutoComplete   AutoComplete
	// KeyboardAppearance is iOS's.
	KeyboardAppearance KeyboardAppearance
	// PlaceholderColor is 0 for the platform's: the text color at half
	// strength where the platform styles placeholders itself.
	PlaceholderColor Color
	// SelectionColor colors the selection and CaretColor the caret, which
	// takes SelectionColor without one; 0 is the platform's.
	SelectionColor Color
	CaretColor     Color
	// AccentColor is the accent of the input's theme, for what the
	// platform draws around it, such as an input method's candidates.
	AccentColor Color
	HideCaret   bool
	// HideIndicator hides a multiline input's native scroll bar.
	HideIndicator bool
	// PasteBehavior says whether a paste inserts its text.
	PasteBehavior PasteBehavior
}

// KeyboardType picks a software keyboard.
type KeyboardType uint8

const (
	KeyboardDefault KeyboardType = iota
	KeyboardEmail
	KeyboardNumber
	KeyboardDecimal
	KeyboardPhone
	KeyboardURL
)

// ViewProps is a view's accessibility and identity.
type ViewProps struct {
	Role Role
	// Href is the browser address of a link; other backends ignore it.
	Href string
	// Level is a heading's or a tree item's level, from 1; 0 for none.
	Level int
	Label string
	// LabelledBy lists the visible elements naming this view. The slice is immutable, keeping ViewProps comparable. Label is the resolved name too, for backends without a native relationship API.
	LabelledBy *[]NodeID
	// LabelFor is the control this visible label activates. LabelPress says EventPress exists only to forward that activation: backends must not infer a button role or a Tab stop from it.
	LabelFor   NodeID
	LabelPress bool
	// Description says more about the element, read after its name, as
	// ARIA's aria-description; Hint says what activating it does.
	Description string
	Hint        string
	Disabled    bool
	// Selected, Checked and Indeterminate (mixed) are reported to
	// assistive technology, and Expanded when Expandable.
	Selected bool
	// MultiSelectable says a RoleGrid, RoleListbox or RoleTree permits selecting more than one of its items.
	MultiSelectable bool
	// SelectionItem says the view is one item in a selectable collection; it stays true when the item is not currently selected.
	SelectionItem bool
	Checked       bool
	Indeterminate bool
	// Pressed is a toggle that is on and Busy an element still loading;
	// Invalid a field whose content the app rejected (aria-invalid).
	Pressed    bool
	Busy       bool
	Invalid    bool
	Expandable bool
	Expanded   bool
	// Value is a range's or a textual element's value.
	Value AccessibilityValue
	// Accessible makes the view one element, or none.
	Accessible Accessible
	// ElementsHidden hides the view and its subtree from assistive
	// technology.
	ElementsHidden bool
	LiveRegion     LiveRegion
	// Actions are the actions offered to assistive technology, reported
	// with EventAccessibilityAction. The list is shared and never changed,
	// so ViewProps stay comparable; read it with ActionList.
	Actions *[]AccessibilityAction
	TestID  string
	// WindowDrag, PanActivation and selections are decided on the main
	// thread.
	WindowDrag    WindowDrag
	PanActivation PanActivation
	// PanScrollHandoff: the view's pan shares a drag with the scroll view
	// inside it where the drag starts, on iOS (View.PanScrollHandoff).
	PanScrollHandoff bool
	// SelectionArea is a view's, Selectable and SelectionColor a text's.
	SelectionArea  bool
	Selectable     Selectable
	SelectionColor Color
	// NoFocus keeps a pressable view out of the Tab stops, and Focusable
	// makes a Tab stop of a view that takes no presses.
	NoFocus, Focusable bool
	// TabAfter is a shown dialog overlay's place in the Tab order, the
	// view it follows, before that view's children: the native view
	// right before it where the app put it, as WAI-ARIA's non-modal
	// popups follow their trigger. Backends that order Tab themselves
	// take it; the toolkits ordering it by their own trees (AppKit's key
	// view loop, the browser's DOM) leave the overlay where it is.
	TabAfter NodeID
	// Topmost marks a Topmost overlay hoisted above modals on its host.
	Topmost bool
	// Appearance is the one its subtree forces (Scope.SetSubtreeAppearance),
	// for the platform's views in it; AppearanceSystem follows the window.
	Appearance Appearance
	// PointerEvents, HitSlop and DelayLongPress are the view's; a zero
	// DelayLongPress is the platform's 500 ms.
	PointerEvents  PointerEvents
	HitSlop        Edges
	DelayLongPress time.Duration
	// Ripple is Android's press ripple, nil for none. It is shared and
	// never changed, as Actions is.
	Ripple *Ripple
}

// HelpText is what a platform with one place for what is read after the
// name reads there: the description, then the hint, each a sentence.
func (p *ViewProps) HelpText() string {
	switch {
	case p.Description == "":
		return p.Hint
	case p.Hint == "":
		return p.Description
	}
	d := p.Description
	if r, _ := utf8.DecodeLastRuneInString(d); unicode.IsLetter(r) || unicode.IsDigit(r) {
		d += "."
	}
	return d + " " + p.Hint
}

// Ripple is Android's ripple over a view while it is pressed.
type Ripple struct {
	// Color is the ripple's; 0 takes the theme's.
	Color Color
	// Borderless spreads it past the view's shape.
	Borderless bool
	// Radius is 0 for the platform's.
	Radius Points
	// Foreground draws it over the view's content, else behind it.
	Foreground bool
}

// Role is a view's accessibility role.
type Role uint8

const (
	RoleNone Role = iota
	RoleButton
	RoleLink
	RoleHeader
	RoleImage
	RoleCheckbox
	RoleSwitch
	RoleRadio
	RoleTab
	RoleText
	RoleTextField
	RoleRadioGroup
	RoleTabList
	RoleMenu
	RoleMenuItem
	RoleDialog
	RoleAlert
	// RoleAdjustable is a slider: its Value is a range that the
	// increment and decrement actions adjust.
	RoleAdjustable
	// RoleProgressBar shows progress, its Value a range.
	RoleProgressBar
	// RoleTable is a table of the app's rows; RoleGrid one whose cells
	// the user selects and moves between. RoleRowGroup holds a header's,
	// a body's or a footer's rows, RoleRow a row's cells, and
	// RoleColumnHeader and RoleCell are a row's cells.
	RoleTable
	RoleGrid
	RoleRowGroup
	RoleRow
	RoleColumnHeader
	RoleCell
	// RoleListbox holds RoleOptions the user picks from, as a select's
	// popup does.
	RoleListbox
	RoleOption
	// RoleGroup, RoleRegion and RoleNavigation are parts of a page, the
	// last two landmarks.
	RoleGroup
	RoleRegion
	RoleNavigation
	// RoleTree holds RoleTreeItems.
	RoleTree
	RoleTreeItem
	RoleToolbar
	// RoleMenuItemCheckbox and RoleMenuItemRadio are menu items with a
	// checked state.
	RoleMenuItemCheckbox
	RoleMenuItemRadio
	// RoleSpinButton is a field for a number: its Value is what it reads
	// as, between its bounds, and its actions step it.
	RoleSpinButton
	// RoleCombobox picks a value from a list it opens, as a select's
	// trigger.
	RoleCombobox
	// RoleAlertDialog is a dialog that asks for an answer to what it
	// says, as a confirmation, ARIA's alertdialog; RoleDialog is any
	// other.
	RoleAlertDialog
	// RoleStatus is advisory information that changes, such as "Saved" or "3 rows selected", ARIA's status; the core makes it a polite live region by default.
	RoleStatus
	// RoleSearchField is a text field used to filter or find content, ARIA's searchbox.
	RoleSearchField
)

// ScrollConfig configures a scroll view.
type ScrollConfig struct {
	Horizontal bool
	// HideIndicator hides the scroll bar.
	HideIndicator bool
	Behavior      ScrollBehavior
	// HeaderInset is the part of Behavior.ContentInset's top that a
	// transparent or large system header covers, which a large title
	// collapses into as the content scrolls under it.
	HeaderInset Points
	// Refreshable says the app listens for EventRefresh: pulling the
	// content down past its top refreshes. Refreshing shows the spinner;
	// a config after EventRefresh says whether the app keeps it.
	Refreshable bool
	Refreshing  bool
	// MinZoomScale and MaxZoomScale bound pinches (1 and at least
	// MinZoomScale); the content zooms when Zooms. ZoomScale is the app's
	// scale, which the platform zooms to when it changes.
	MinZoomScale, MaxZoomScale float32
	ZoomScale                  float32
	// NoBouncesZoom stops pinches at the limits; NoPinchZoom ignores
	// pinches; CenterContent centers content smaller than the viewport.
	NoBouncesZoom bool
	NoPinchZoom   bool
	CenterContent bool
	// Edges are the effects at its sides, in the order of Edges: top,
	// right, bottom, left.
	Edges [4]ScrollEdge
}

// TextEngine prepares text for layout. The engine calls it on its own
// goroutine, so implementations must not touch main-thread-only objects.
type TextEngine interface {
	// Prepare shapes text once; the result measures it at any number of
	// widths and lays it out at its final size.
	Prepare(text *AttributedText) PreparedText
}

// FontRegistrar is a TextEngine that registers fonts as the app runs, as a TSX module's imports do: RegisterFont makes the families of the font file data (TrueType, OpenType or a collection) available to the text it prepares from then on. A platform that loads a font later tells the engine (FontsChanged), which prepares its text and records its canvases again.
type FontRegistrar interface {
	RegisterFont(data []byte) error
}

// PreparedText is text a TextEngine shaped.
type PreparedText interface {
	// Measure returns the size of the text wrapped at maxWidth (Auto: no
	// wrapping).
	Measure(maxWidth Points) Size
	// Layout returns the text laid out in size, for the main thread to
	// draw.
	Layout(size Size) TextLayout
	// Release frees what the prepared text holds.
	Release()
}

// TextLayout is a platform's laid-out text, drawn on the main thread and
// released there when replaced or deleted.
type TextLayout interface {
	Release()
}

// Selectable says whether the user can select a text.
type Selectable uint8

const (
	// SelectableAuto selects the text with its selection area when it is
	// in one. Elsewhere only the web selects it, on desktops, as a page's
	// text.
	SelectableAuto Selectable = iota
	// SelectableOn selects the text, alone when it is in no area.
	SelectableOn
	// SelectableOff keeps the text out of selections, its area's too.
	SelectableOff
)

// ImageSource is where an image comes from: a URI (https, file path, file:, data:), bytes, such as a file embedded with go:embed, pixels the app draws, or a retained offscreen canvas drawing.
type ImageSource struct {
	URI  string
	Data []byte
	// Pixels, when set, is the image itself, premultiplied as image.RGBA is: nothing loads or decodes, and the view fires no load events and fades nothing in. The core copies it when the commit that carries a new Pixels or Version runs, so the app may write the next frame into the same buffer from then on, from its next RequestAnimationFrame callback for one. Without Size it measures its pixels in points. An Image's Source, a canvas's images and a Shader's Images take pixels; a Placeholder, which shows while a source loads, ignores them.
	Pixels *image.RGBA
	// Offscreen, when set, is a recorded 2D drawing that platforms render once into a retained offscreen surface keyed by (ID, Version) and reuse across frames without re-executing its drawing operations. Like Pixels, it loads nothing and fires no load events.
	Offscreen *OffscreenCanvas
	// Version tells frames written into the same Pixels apart: the same Pixels with another Version is shown again in place, the previous frame staying until the new one is drawn. For Offscreen it mirrors Offscreen.Version.
	Version uint64
	// Size is the image's size in points, when known up front.
	Size Size
	// Scales are the pixel densities an "asset:" image ships in (a.png,
	// a@2x.png, ...): the core mounts the one for the screen's pixel
	// ratio. Empty for a single file.
	Scales ImageScales
	// Variants, for a source whose variants are not asset: files (a TSX
	// bundle's data: URLs), are the URIs of the scales past 1x by scale.
	Variants map[float32]string
}

// Density returns the pixels a point of the image s names has: an
// offscreen canvas's backing scale, the density an "asset:" file's name
// gives (a@2x.png is 2), else 1.
func (s ImageSource) Density() float32 {
	if s.Offscreen != nil {
		return s.Offscreen.EffectiveScale()
	}
	if !strings.HasPrefix(s.URI, "asset:") {
		return 1
	}
	name := s.URI[strings.LastIndexByte(s.URI, '/')+1:]
	if dot := strings.LastIndexByte(name, '.'); dot > 0 {
		name = name[:dot]
	}
	at := strings.LastIndexByte(name, '@')
	if at < 0 || !strings.HasSuffix(name, "x") {
		return 1
	}
	d, err := strconv.ParseFloat(name[at+1:len(name)-1], 32)
	if err != nil || d <= 0 {
		return 1
	}
	return float32(d)
}

// Key returns a string naming the image s loads, for caches: its URI, a digest of its bytes, an offscreen canvas's ID and version, or where its pixels are and their version. Bytes are named by what they hold, not where: a collected buffer's address can come back in a new buffer of the same length, which must not be handed the image cached for the old one (#738).
func (s ImageSource) Key() string {
	if s.Offscreen != nil {
		return fmt.Sprintf("offscreen:%d:%d", s.Offscreen.ID, s.Offscreen.Version)
	}
	if s.Pixels != nil {
		return fmt.Sprintf("pixels:%p:%d", s.Pixels, s.Version)
	}
	if len(s.Data) > 0 {
		return fmt.Sprintf("data:%x", sha256.Sum256(s.Data))
	}
	return s.URI
}

// Same reports whether s and o load the same image: inline data must be the same bytes, not equal ones, pixels the same buffer at the same Version, and offscreen canvases the same ID and Version.
func (s ImageSource) Same(o ImageSource) bool {
	return s.URI == o.URI && s.Size == o.Size && s.Scales == o.Scales && len(s.Data) == len(o.Data) &&
		(len(s.Data) == 0 || &s.Data[0] == &o.Data[0]) && s.Pixels == o.Pixels && s.Offscreen.Same(o.Offscreen) && s.Version == o.Version
}

// Empty reports whether s names no image.
func (s ImageSource) Empty() bool {
	return s.URI == "" && len(s.Data) == 0 && s.Pixels == nil && s.Offscreen == nil
}

// CopyPixels returns a copy of img's pixels in a buffer of its own, at the origin with rows of 4 bytes a pixel, as the core hands them to a platform; nil for nil.
func CopyPixels(img *image.RGBA) *image.RGBA {
	if img == nil {
		return nil
	}
	b := img.Rect
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	row := 4 * b.Dx()
	if img.Stride == row && len(img.Pix) >= row*b.Dy() {
		copy(out.Pix, img.Pix[:row*b.Dy()])
		return out
	}
	for y := range b.Dy() {
		at := img.PixOffset(b.Min.X, b.Min.Y+y)
		copy(out.Pix[y*row:(y+1)*row], img.Pix[at:at+row])
	}
	return out
}

// ResizeMode fits an image or a video into its view. The zero value is
// the element's default: Cover for an image, Contain for a video, as in
// NSH. Platforms receive the resolved mode.
type ResizeMode uint8

const (
	// Cover fills the view, cropping what overflows.
	Cover ResizeMode = iota + 1
	// Contain fits the whole image, leaving space.
	Contain
	// Stretch fills the view, distorting the image.
	Stretch
	// Center keeps the image's natural size, cropped where it is larger
	// than the view (CSS's object-fit: none).
	Center
	// ScaleDown keeps the natural size, shrunk to fit where it is larger
	// than the view (CSS's object-fit: scale-down, React Native's center).
	ScaleDown
)

// ModalPresentation is how a modal covers the screen on iOS.
type ModalPresentation uint8

const (
	// ModalFullScreen covers the whole screen with the modal's own look.
	ModalFullScreen ModalPresentation = iota
	// ModalPageSheet is the system's sheet, which leaves the content
	// behind it in sight.
	ModalPageSheet
	// ModalFormSheet is a smaller sheet, centered on large screens.
	ModalFormSheet
)

// Orientation is a set of interface orientations.
type Orientation uint8

const (
	OrientationPortrait Orientation = 1 << iota
	OrientationPortraitUpsideDown
	OrientationLandscapeLeft
	OrientationLandscapeRight

	OrientationLandscape = OrientationLandscapeLeft | OrientationLandscapeRight
)

// ScreenAnimation is how a screen of a stack enters and leaves: a push
// takes the pushed screen's, a pop the popped one's. ScreenDefault is the
// platform's, a slide where it animates screens.
type ScreenAnimation uint8

const (
	ScreenDefault ScreenAnimation = iota
	ScreenSlide
	ScreenFade
	ScreenNone
)

// ModalAnimation is how a modal appears.
type ModalAnimation uint8

const (
	ModalSlide ModalAnimation = iota
	ModalFade
	ModalNone
)

// Duration is how long a modal takes to appear or go with a, as the
// platforms present it: what its component waits before it reports the
// modal shown or gone.
func (a ModalAnimation) Duration() time.Duration {
	switch a {
	case ModalSlide:
		return 300 * time.Millisecond
	case ModalFade:
		return 200 * time.Millisecond
	}
	return 0
}

// Same reports whether a and b show the same image the same way.
func (a ImageConfig) Same(b ImageConfig) bool {
	return a.Source.Same(b.Source) && a.Placeholder.Same(b.Placeholder) && a.ResizeMode == b.ResizeMode && a.Sampling == b.Sampling &&
		a.BlurRadius == b.BlurRadius && a.FadeDuration == b.FadeDuration && a.HitTestAlpha == b.HitTestAlpha && a.Position == b.Position
}

// ObjectPosition is where an image sits in its view once its ResizeMode
// has sized it, as CSS's object-position places it. The parts of a
// covering image that overflow are cropped. The zero value centers it.
type ObjectPosition struct {
	X, Y PositionAxis
}

// PositionAxis is an ObjectPosition along one axis: Offset from the start
// edge (left, top), or from the end edge (right, bottom) when FromEnd.
// A percentage is of the free space, the view's size less the image's, so
// Pct(0) is the start, Pct(50) the center and Pct(100) the end; an auto
// Offset, the zero value, is the center.
type PositionAxis struct {
	Offset  Length
	FromEnd bool
}

// Resolve returns where content starts along the axis, from the start
// edge, given the free space: the box's size less the content's.
func (a PositionAxis) Resolve(free Points) Points {
	var d Points
	switch a.Offset.unit {
	case unitPoints:
		d = Points(a.Offset.value)
	case unitPercent:
		d = free * Points(a.Offset.value) / 100
	default:
		d = free / 2
	}
	if a.FromEnd {
		return free - d
	}
	return d
}

// Place returns the top left corner of content of the given size placed
// in a box of size.
func (p ObjectPosition) Place(box, content Size) Point {
	return Point{X: p.X.Resolve(box.Width - content.Width), Y: p.Y.Resolve(box.Height - content.Height)}
}

// Fit returns where an image of natural size, in points, goes in a view
// of size box: scaled as m scales it (Center keeps the natural size, as
// CSS's none; ScaleDown shrinks an image larger than the box, as CSS's
// scale-down), then placed at pos. The rectangle may reach outside the
// box, which crops it. An image without a size fills the box. Every
// backend fits images and videos with it, as NSH's ResizeMode::fit.
func (m ResizeMode) Fit(natural, box Size, pos ObjectPosition) Bounds {
	nw, nh := natural.Width, natural.Height
	if nw <= 0 || nh <= 0 || m == Stretch {
		return Bounds{Size: box}
	}
	contain := min(box.Width/nw, box.Height/nh)
	var scale Points
	switch m {
	case Contain:
		scale = contain
	case Center:
		scale = 1
	case ScaleDown:
		scale = min(contain, 1)
	default: // Cover
		scale = max(box.Width/nw, box.Height/nh)
	}
	content := Size{Width: nw * scale, Height: nh * scale}
	return Bounds{Origin: pos.Place(box, content), Size: content}
}

// ImageScaleValues are the pixel densities an image asset can ship in, as
// its file name's suffix says ("@1.5x" to "@4x"; the file without one is
// 1x).
var ImageScaleValues = [...]float32{1, 1.5, 2, 3, 4}

// ImageScales is a set of ImageScaleValues: the variants an image asset
// ships in.
type ImageScales uint8

// Pick returns the scale to load at ratio, as React Native picks: the
// smallest at least the ratio, else the largest; 0 for an empty set.
func (set ImageScales) Pick(ratio float32) float32 {
	var pick float32
	for i, s := range ImageScaleValues {
		if set&(1<<i) != 0 {
			pick = s
			if s >= ratio {
				break
			}
		}
	}
	return pick
}

// StatusBarStyle is the color of a phone's status bar content.
type StatusBarStyle uint8

const (
	// StatusBarInherit keeps the style of the layers below.
	StatusBarInherit StatusBarStyle = iota
	// StatusBarAuto follows the appearance: dark content on light.
	StatusBarAuto
	StatusBarDark
	StatusBarLight
)

// StatusBarConfig is the status bar the platform shows (ItemStatusBar).
type StatusBarConfig struct {
	// Style is never StatusBarInherit.
	Style     StatusBarStyle
	Hidden    bool
	Animated  bool
	Immersive bool
}

// VideoConfig is everything a video plays with (ItemVideo), with the
// defaults applied: Volume in 0 to 1, Rate above 0, a ResizeMode and
// ProgressInterval at least 16 milliseconds.
type VideoConfig struct {
	Source               string
	Headers              *VideoHeaders
	Paused               bool
	Muted                bool
	Volume               float32
	Rate                 float32
	Repeat               bool
	ResizeMode           ResizeMode
	HitTestAlpha         float32
	Controls             bool
	PictureInPicture     bool
	AutoPictureInPicture bool
	ProgressInterval     time.Duration
	AudioSource          string
	AudioSession         AudioSession
}

// AudioSession is what a video's sound does to the rest of the device's
// audio while it plays, as NSH's audioSession.
type AudioSession uint8

const (
	// AudioSessionDefault leaves the app's audio settings to decide: on
	// iOS the device's silent mode silences it.
	AudioSessionDefault AudioSession = iota
	// AudioSessionPlayback plays in iOS's silent mode as speech, pausing
	// other apps' audio until it pauses, ends or goes; Android holds the
	// audio focus while it plays.
	AudioSessionPlayback
	// AudioSessionMixWithOthers plays in iOS's silent mode as speech, with
	// other apps' audio going on.
	AudioSessionMixWithOthers
)
