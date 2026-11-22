---
title: Permissions
description: Check and ask for the system's permissions, open their settings, and guide the user to System Settings on macOS.
group: Platform features
order: 49
---

## Checking and asking

The system keeps some of what an app may do for the user to give: the microphone, the camera, calendars, reminders, Bluetooth, the media library and location on phones and Macs, motion and location in the background on phones, and on macOS also Accessibility, Screen Recording, Input Monitoring, Full Disk Access, App Management and Developer Tools ([location and motion](services.md#location-and-motion)). `core.Permissions()` lists them as `core.Permission` keys. `cx.PermissionStatus` answers whether the app has one without asking, and `cx.RequestPermission` asks with the system's prompt where it has one and has not asked yet. Ask as the user does something that needs the permission. Face ID and Touch ID are not among them: `cx.BiometricAvailability` tells them and `cx.AuthenticateBiometric` asks with their own prompt ([biometrics](services.md#biometrics)).

```go
cx.RequestPermission(core.PermissionCamera, func(s core.PermissionStatus) {
	if s.Allowed() {
		startScanner()
	}
})
```

| Status | `String()` | Meaning |
| --- | --- | --- |
| `PermissionNotAsked` | `prompt` | Not asked yet: a request shows the system's prompt |
| `PermissionGranted` | `granted` | The app has it |
| `PermissionDenied` | `denied` | Refused; only the system's settings change it |
| `PermissionUnknown` | `unknown` | Kept, but the system or the platform does not tell it |
| `PermissionNotGated` | `not-gated` | Nothing keeps it from the app, as on Linux |
| `PermissionUnavailable` | `unavailable` | The platform has no such thing |

`Allowed` is true for granted and not gated. macOS answers Accessibility, Screen Recording and Input Monitoring denied until the user turns them on, as asking only lists the app in their pane, and tells a Screen Recording grant once the app starts again.

## Usage descriptions

On macOS and iOS, asking needs the permission's reason in the app's Info.plist, which `nsgo.yaml`'s `apple.info` holds; without it the request answers unavailable and logs the key, as the system would end the app. A message of the app's [catalogs](localization.md) with the same key, such as `NSCameraUsageDescription: Scans the codes you point it at.` in `locales/en.yaml`, translates the reason, which the system then asks in the user's language; a language that does not translate it says the source's.

```yaml
apple:
  info:
    NSMicrophoneUsageDescription: Records your voice notes.
    NSCameraUsageDescription: Scans the codes you point it at.
    NSCalendarsFullAccessUsageDescription: Adds your bookings to your calendar.  # macOS 14 and iOS 17; NSCalendarsUsageDescription before
    NSRemindersFullAccessUsageDescription: Turns tasks into reminders.            # NSRemindersUsageDescription before
    NSBluetoothAlwaysUsageDescription: Finds your headphones.
    NSAppleMusicUsageDescription: Plays your library.                            # iOS
windows:
  device_capabilities: [microphone, webcam]  # what a store build's MSIX declares
```

Under `nsgo dev`, macOS charges what the app does to the terminal that started it, so the terminal is the app asked and listed; build the app to test its own permissions.

## Opening the settings

`cx.OpenPermissionSettings` opens the page that gives a permission and answers whether one opened: Privacy & Security at the permission's pane on macOS, the app's page in Settings on iOS, and the microphone's or the camera's page on Windows.

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
| macOS | Every permission; App Management, Developer Tools and Media & Apple Music are unknown |
| iOS | The microphone, the camera, calendars, reminders, Bluetooth and the media library |
| Windows | The microphone and the camera, which it never asks for; the rest is not gated |
| Linux | Not gated outside sandboxes; in Flatpak and Snap, the camera through the desktop portal and the microphone through the portal and PipeWire |
| Web | The microphone and the camera, unknown in Firefox |
| Android, Apple Watch, mini programs | Unknown for now |

Location and motion permissions follow each [platform's sensors](services.md#location-and-motion). On mobile Safari, ask for `PermissionMotion` from a press handler.

## Testing

`nsgotest` answers from its model: `app.Permissions` holds each status, not asked at first, and a request of one not asked makes it `app.PermissionAnswers`' for it, else granted. `app.PermissionSettings` lists the settings opened and `app.Guide` the guide showing, which `app.GrantPermission` and `app.CloseGuide` end.

```go
app.PermissionAnswers[core.PermissionMicrophone] = core.PermissionDenied
app.Press("id:record")
app.Find("text:The microphone is off")
```

## Next

- [Notifications](notifications.md): their own permission, badges and presses.
- [Building and shipping](building.md): the entitlements and capabilities a package declares.
