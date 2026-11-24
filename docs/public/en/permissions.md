---
title: Permissions
description: Check and ask for the system's permissions, open their settings, and guide the user to System Settings on macOS.
group: Platform features
order: 49
---

## Checking and asking

The system keeps some of what an app may do for the user to give: the microphone, the camera, contacts, calendars, reminders, photos, Bluetooth, the media library, the local network, location and notifications on phones and Macs, motion and location in the background on phones, and on macOS also Accessibility, Screen Recording, Input Monitoring, Full Disk Access, App Management, Developer Tools and Automation of another app ([location and motion](services.md#location-and-motion), [notifications](notifications.md)). `core.Permissions()` lists them as `core.Permission` keys. `cx.PermissionStatus` answers whether the app has one without asking, and `cx.RequestPermission` asks with the system's prompt where it has one and has not asked yet. Ask as the user does something that needs the permission. Face ID and Touch ID are not among them: `cx.BiometricAvailability` tells them and `cx.AuthenticateBiometric` asks with their own prompt ([biometrics](services.md#biometrics)).

```go
cx.RequestPermission(core.PermissionCamera, func(s core.PermissionStatus) {
	if s.Allowed() {
		startScanner()
	}
})
```

On macOS, Automation is granted per target app: pass `core.PermissionAutomationOf("com.apple.finder")` to check or ask for a running target app by its bundle ID, or `core.PermissionAutomation` to open the Automation pane in System Settings. A target app that is not running answers `PermissionUnknown`.

| Status | `String()` | Meaning |
| --- | --- | --- |
| `PermissionNotAsked` | `prompt` | Not asked yet: a request shows the system's prompt |
| `PermissionGranted` | `granted` | The app has it |
| `PermissionLimited` | `limited` | Partial access, such as selected photos or contacts |
| `PermissionDenied` | `denied` | Refused; only the system's settings change it |
| `PermissionUnknown` | `unknown` | Kept, but the system or the platform does not tell it |
| `PermissionNotGated` | `not-gated` | Nothing keeps it from the app, as on Linux |
| `PermissionUnavailable` | `unavailable` | The platform has no such thing |

`Allowed` is true for granted, limited and not gated. macOS answers Accessibility, Screen Recording and Input Monitoring denied until the user turns them on, as asking only lists the app in their pane, and tells a Screen Recording grant once the app starts again.

## Usage descriptions

On macOS and iOS, asking needs the permission's reason in the app's Info.plist, which `nsgo.yaml`'s `apple.info` holds; without it the request answers unavailable and logs the key, as the system would end the app. A message of the app's [catalogs](localization.md) with the same key, such as `NSCameraUsageDescription: Scans the codes you point it at.` in `locales/en.yaml`, translates the reason, which the system then asks in the user's language; a language that does not translate it says the source's.

```yaml
apple:
  info:
    NSMicrophoneUsageDescription: Records your voice notes.
    NSCameraUsageDescription: Scans the codes you point it at.
    NSContactsUsageDescription: Finds friends who already use the app.
    NSCalendarsFullAccessUsageDescription: Adds your bookings to your calendar.  # macOS 14 and iOS 17; NSCalendarsUsageDescription before
    NSRemindersFullAccessUsageDescription: Turns tasks into reminders.            # NSRemindersUsageDescription before
    NSPhotoLibraryUsageDescription: Imports photos from your library.
    NSBluetoothAlwaysUsageDescription: Finds your headphones.
    NSAppleMusicUsageDescription: Plays your library.                            # iOS
    NSLocalNetworkUsageDescription: Finds your computer to sync with.            # iOS, macOS 15
    NSAppleEventsUsageDescription: Reveals files in Finder.                      # macOS Automation
android:
  permissions:
    - RECORD_AUDIO
    - CAMERA
    - READ_CALENDAR
    - WRITE_CALENDAR
    - BLUETOOTH_CONNECT      # Android 12+; BLUETOOTH on Android 9–11
    - READ_MEDIA_AUDIO       # Android 13+; READ_EXTERNAL_STORAGE on Android 9–12
windows:
  device_capabilities: [microphone, webcam]  # what a store build's MSIX declares
```

On Android, each permission must be listed in `nsgo.yaml`'s `android.permissions`; one left out of the manifest answers `PermissionUnavailable`. Under `nsgo dev`, macOS charges what the app does to the terminal that started it, so the terminal is the app asked and listed; build the app to test its own permissions.

## The local network

From iOS 14 and macOS 15, a connection to a device on the local network fails until the user lets the app find and connect to devices there, and the system tells an app neither whether it may nor whether it was asked. Ask for `core.PermissionLocalNetwork` before the first connection, such as when the user starts pairing: the request advertises and browses a Bonjour service of NSGo's own, which shows the system's prompt the first time, and answers once the user does, granted or denied, or unknown after two minutes. `cx.PermissionStatus` answers what the last request in this run learned, and unknown before one. Android, Windows and Linux answer not gated, as macOS does before 15 and for a program started from a terminal, such as under `nsgo dev`; the web answers unavailable.

```go
cx.RequestPermission(core.PermissionLocalNetwork, func(s core.PermissionStatus) {
	if s.Allowed() {
		pair()
	} else if s == core.PermissionDenied {
		showLocalNetworkOff()
	}
})
```

Asking needs `NSLocalNetworkUsageDescription` in `apple.info`. With it, `nsgo build` lists the service's type, `_nsgo-lnp._tcp`, in `NSBonjourServices` after the app's own, as iOS refuses to browse a type the Info.plist does not list; an app embedding NSGo in its own host lists the type in the host's Info.plist.

## Opening the settings

`cx.OpenPermissionSettings` opens the page that gives a permission and answers whether one opened: Privacy & Security at the permission's pane (including Automation, Contacts, Photos, Location Services and, from macOS 15, Local Network) or Notifications on macOS, the app's page in Settings (or its Notifications page from iOS 16) on iOS, the app's page in Settings on Android, and the microphone's, camera's or notifications' page on Windows.

For `PermissionLocation` and `PermissionBackgroundLocation`, macOS opens Location Services; `PermissionMotion` opens Motion & Fitness, even though macOS does not supply motion samples. iOS and Android open the app's page for all three. Opening settings neither requests nor grants a permission.

## Guiding the user

`desktop.GuidePermission` opens that page and waits for the user to give the permission. On macOS a panel follows System Settings' window without taking the focus and, for the panes whose list takes an app dragged into it, holds the app for the user to drag there. It ends, and its function hears the status, once the app has the permission, the user closes the panel or System Settings, `stop` is called or the component goes; a new guide ends the one before.

```go
stop := desktop.GuidePermission(cx, core.PermissionAccessibility, desktop.GuideOptions{}, func(s core.PermissionStatus) {
	trusted.Set(s == core.PermissionGranted)
})
```

The app held out is its bundle, else its executable; `GuideOptions.App` names another and `Name` what the panel calls it. Windows opens the page and waits for Settings to close. Elsewhere the function hears the status at once.

## Platforms

| Platform | What it tells and asks |
| --- | --- |
| macOS | Every permission (Automation per running target app via `PermissionAutomationOf`, plus notifications for bundled apps); bare Automation, App Management, Developer Tools and Media & Apple Music are unknown; motion is unavailable; the local network from macOS 15, as asking learns it |
| iOS | The microphone, the camera, contacts, calendars, reminders, photos (with `PermissionLimited`), Bluetooth, the media library, the local network as asking learns it, location, motion and notifications |
| Windows | The microphone, the camera and notifications, which it never prompts for; the rest is not gated |
| Linux | Not gated outside sandboxes; notifications reflect whether a notification server answers; in Flatpak and Snap, the camera through the desktop portal and the microphone through the portal and PipeWire |
| Web | The microphone and the camera (unknown in Firefox), notifications, location (not background location), and motion on mobile browsers |
| Android | The microphone, the camera, calendars, Bluetooth and the media library, where `android.permissions` declares them; the local network is not gated |
| Apple Watch, mini programs | Notifications, location and motion through their services; the rest kept by the system is unknown for now |

Location and motion permissions follow each [platform's sensors](services.md#location-and-motion), and `PermissionNotifications` follows [notifications](notifications.md). Fitness uses `PermissionActivityRecognition` (`ACTIVITY_RECOGNITION` on Android, `NSMotionUsageDescription` on Apple), including Apple pressure; raw Android motion remains separate. On mobile Safari, ask for `PermissionMotion` from a press handler.

## Testing

`nsgotest` answers from its model: `app.Permissions` holds each status, not asked at first, and a request of one not asked makes it `app.PermissionAnswers`' for it, else granted. `PermissionNotifications` stays in sync with `app.NotificationPermission` and `app.NotificationAnswer`. `app.PermissionSettings` lists the settings opened and `app.Guide` the guide showing, which `app.GrantPermission` and `app.CloseGuide` end.

```go
app.PermissionAnswers[core.PermissionMicrophone] = core.PermissionDenied
app.Press("id:record")
app.Find("text:The microphone is off")
```

## Next

- [Notifications](notifications.md): their own permission, badges and presses.
- [Building and shipping](building.md): the entitlements and capabilities a package declares.
