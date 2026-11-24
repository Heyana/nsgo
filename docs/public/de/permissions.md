---
title: Berechtigungen
description: "Die Berechtigungen des Systems prüfen und anfordern, ihre Einstellungen öffnen und den Nutzer unter macOS zu den Systemeinstellungen führen."
group: Plattformfunktionen
order: 49
---

## Prüfen und anfordern

Das System überlässt es dem Nutzer, einiges von dem zu gewähren, was eine App tun darf: das Mikrofon, die Kamera, Kontakte, Kalender, Erinnerungen, Fotos, Bluetooth, die Mediathek, den Standort und Benachrichtigungen auf Telefonen und Macs, Bewegung und Standort im Hintergrund auf Telefonen sowie unter macOS zusätzlich Bedienungshilfen, Bildschirmaufnahme, Eingabeüberwachung, Festplattenvollzugriff, App-Verwaltung, Entwicklertools und die Automation einer anderen App ([Standort und Bewegung](services.md#standort-und-bewegung), [Benachrichtigungen](notifications.md)). `core.Permissions()` listet sie als `core.Permission`-Schlüssel auf. `cx.PermissionStatus` gibt an, ob die App eine davon hat, ohne zu fragen, und `cx.RequestPermission` fragt mit der Abfrage des Systems, wo es eine gibt und noch nicht gefragt wurde. Fragen Sie, während der Nutzer etwas tut, das die Berechtigung benötigt. Face ID und Touch ID gehören nicht dazu: `cx.BiometricAvailability` gibt über sie Auskunft und `cx.AuthenticateBiometric` fragt mit ihrer eigenen Abfrage ([Biometrie](services.md#biometrie)).

```go
cx.RequestPermission(core.PermissionCamera, func(s core.PermissionStatus) {
	if s.Allowed() {
		startScanner()
	}
})
```

Unter macOS wird Automation pro Ziel-App gewährt: Übergeben Sie `core.PermissionAutomationOf("com.apple.finder")`, um sie für eine laufende Ziel-App anhand ihrer Bundle-ID zu prüfen oder anzufordern, oder `core.PermissionAutomation`, um den Bereich Automation in den Systemeinstellungen zu öffnen. Eine Ziel-App, die nicht läuft, antwortet mit `PermissionUnknown`.

| Status | `String()` | Bedeutung |
| --- | --- | --- |
| `PermissionNotAsked` | `prompt` | Noch nicht gefragt: Eine Anfrage zeigt die Abfrage des Systems |
| `PermissionGranted` | `granted` | Die App hat sie |
| `PermissionLimited` | `limited` | Teilweiser Zugriff, etwa auf ausgewählte Fotos oder Kontakte |
| `PermissionDenied` | `denied` | Verweigert; nur die Einstellungen des Systems ändern das |
| `PermissionUnknown` | `unknown` | Geschützt, aber das System oder die Plattform gibt keine Auskunft |
| `PermissionNotGated` | `not-gated` | Nichts hält sie von der App fern, wie unter Linux |
| `PermissionUnavailable` | `unavailable` | Die Plattform hat so etwas nicht |

`Allowed` ist true für gewährt, eingeschränkt und nicht geschützt. macOS antwortet für Bedienungshilfen, Bildschirmaufnahme und Eingabeüberwachung mit verweigert, bis der Nutzer sie einschaltet, da eine Anfrage die App nur in ihrem Bereich auflistet, und meldet eine Bildschirmaufnahme-Freigabe erst, wenn die App erneut startet.

## Nutzungsbeschreibungen

Unter macOS und iOS braucht eine Anfrage die Begründung der Berechtigung in der Info.plist der App, die `apple.info` in `nsgo.yaml` enthält; ohne sie antwortet die Anfrage mit nicht verfügbar und protokolliert den Schlüssel, da das System die App sonst beenden würde. Eine Nachricht aus den [Katalogen](localization.md) der App mit demselben Schlüssel, etwa `NSCameraUsageDescription: Scans the codes you point it at.` in `locales/en.yaml`, übersetzt die Begründung, mit der das System dann in der Sprache des Nutzers fragt; eine Sprache, die sie nicht übersetzt, verwendet die der Quelle.

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

Unter Android muss jede Berechtigung in `android.permissions` von `nsgo.yaml` aufgeführt sein; eine, die im Manifest fehlt, antwortet mit `PermissionUnavailable`. Unter `nsgo dev` rechnet macOS das, was die App tut, dem Terminal zu, das sie gestartet hat, sodass das Terminal die gefragte und aufgelistete App ist; bauen Sie die App, um ihre eigenen Berechtigungen zu testen.

## Die Einstellungen öffnen

`cx.OpenPermissionSettings` öffnet die Seite, auf der eine Berechtigung gewährt wird, und gibt an, ob eine geöffnet wurde: unter macOS „Datenschutz & Sicherheit“ beim Bereich der Berechtigung (einschließlich Automation, Kontakte, Fotos und Ortungsdienste) oder „Mitteilungen“, unter iOS die Seite der App in den Einstellungen (oder ab iOS 16 ihre Mitteilungsseite), unter Android die Seite der App in den Einstellungen und unter Windows die Seite für Mikrofon, Kamera oder Benachrichtigungen.

Für `PermissionLocation` und `PermissionBackgroundLocation` öffnet macOS die Ortungsdienste; `PermissionMotion` öffnet „Bewegung & Fitness“, obwohl macOS keine Bewegungsdaten liefert. iOS und Android öffnen für alle drei die Seite der App. Das Öffnen der Einstellungen fordert eine Berechtigung weder an noch gewährt es sie.

## Den Nutzer führen

`desktop.GuidePermission` öffnet diese Seite und wartet, bis der Nutzer die Berechtigung gewährt. Unter macOS folgt ein Panel dem Fenster der Systemeinstellungen, ohne den Fokus zu übernehmen, und hält für die Bereiche, deren Liste eine hineingezogene App annimmt, die App bereit, damit der Nutzer sie dorthin ziehen kann. Es endet, und seine Funktion erfährt den Status, sobald die App die Berechtigung hat, der Nutzer das Panel oder die Systemeinstellungen schließt, `stop` aufgerufen wird oder die Komponente verschwindet; eine neue Führung beendet die vorherige.

```go
stop := desktop.GuidePermission(cx, core.PermissionAccessibility, desktop.GuideOptions{}, func(s core.PermissionStatus) {
	trusted.Set(s == core.PermissionGranted)
})
```

Die bereitgehaltene App ist ihr Bundle, sonst ihre ausführbare Datei; `GuideOptions.App` benennt eine andere und `Name`, wie das Panel sie nennt. Windows öffnet die Seite und wartet, bis die Einstellungen geschlossen werden. Anderswo erfährt die Funktion den Status sofort.

## Plattformen

| Plattform | Worüber sie Auskunft gibt und wonach sie fragt |
| --- | --- |
| macOS | Jede Berechtigung (Automation pro laufender Ziel-App über `PermissionAutomationOf`, dazu Benachrichtigungen für gebündelte Apps); Automation allgemein, App-Verwaltung, Entwicklertools sowie Medien & Apple Music sind unbekannt; Bewegung ist nicht verfügbar |
| iOS | Das Mikrofon, die Kamera, Kontakte, Kalender, Erinnerungen, Fotos (mit `PermissionLimited`), Bluetooth, die Mediathek, Standort, Bewegung und Benachrichtigungen |
| Windows | Das Mikrofon, die Kamera und Benachrichtigungen, für die es nie eine Abfrage zeigt; der Rest ist nicht geschützt |
| Linux | Außerhalb von Sandboxes nicht geschützt; Benachrichtigungen spiegeln wider, ob ein Benachrichtigungsserver antwortet; in Flatpak und Snap die Kamera über das Desktop-Portal und das Mikrofon über das Portal und PipeWire |
| Web | Das Mikrofon und die Kamera (unbekannt in Firefox), Benachrichtigungen, Standort (nicht Standort im Hintergrund) und Bewegung in mobilen Browsern |
| Android | Das Mikrofon, die Kamera, Kalender, Bluetooth und die Mediathek, sofern `android.permissions` sie deklariert |
| Apple Watch, Mini-Programme | Benachrichtigungen, Standort und Bewegung über ihre Dienste; der Rest, den das System schützt, ist vorerst unbekannt |

Standort- und Bewegungsberechtigungen folgen den [Sensoren der jeweiligen Plattform](services.md#standort-und-bewegung), und `PermissionNotifications` folgt den [Benachrichtigungen](notifications.md). Fitness verwendet `PermissionActivityRecognition` (`ACTIVITY_RECOGNITION` unter Android, `NSMotionUsageDescription` bei Apple), einschließlich des Luftdrucks bei Apple; rohe Bewegungsdaten unter Android bleiben davon getrennt. In mobilem Safari fordern Sie `PermissionMotion` aus einem Druck-Handler heraus an.

## Testen

`nsgotest` antwortet aus seinem Modell: `app.Permissions` enthält jeden Status, anfangs „nicht gefragt“, und eine Anfrage nach einer noch nicht gefragten Berechtigung setzt sie auf den Wert aus `app.PermissionAnswers` für sie, sonst auf gewährt. `PermissionNotifications` bleibt mit `app.NotificationPermission` und `app.NotificationAnswer` synchron. `app.PermissionSettings` listet die geöffneten Einstellungen auf und `app.Guide` die angezeigte Führung, die `app.GrantPermission` und `app.CloseGuide` beenden.

```go
app.PermissionAnswers[core.PermissionMicrophone] = core.PermissionDenied
app.Press("id:record")
app.Find("text:The microphone is off")
```

## Weiter

- [Benachrichtigungen](notifications.md): ihre eigene Berechtigung, Badges und Drücke.
- [Bauen und ausliefern](building.md): die Entitlements und Capabilities, die ein Paket deklariert.
