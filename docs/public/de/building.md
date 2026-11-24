---
title: Bauen und Ausliefern
description: "Eine App für jede Plattform paketieren, Installer erstellen, signieren und notarisieren, auf GitHub veröffentlichen und in andere Apps einbetten."
group: Toolchain
order: 22
---

## Build

```sh
go tool nsgo build
```

`build` schreibt für jedes in `nsgo.yaml` aufgeführte Ziel ein gestripptes Release-Paket nach `dist/<name>-<version>-<target>`: eine `.app`, eine `.ipa`, eine `.apk`, Verzeichnisse für Windows und Linux oder eine Website, deren Seite jedes in einem `import()` benannte TSX-Modul lädt, sobald der Import ausgeführt wird. `-target` ersetzt diese Liste, und `all` steht für alle Ziele, die nur Go und Xcode benötigen; ergänzen Sie `android_x64`, wenn ein Android NDK installiert ist, sowie `miniprogram` (oder einzeln `miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay` bzw. `miniprogram_douyin`) für [Mini-Programme](#mini-programs) und `harmonyos` für eine [HarmonyOS](#harmonyos)-Vorschau. `-run` startet das für diesen Rechner gebaute Paket oder installiert es auf dem iPhone, Simulator oder Android-Gerät, das `-device` auswählt.

Alles zur Auslieferung landet in `dist/releases` – dem einzigen Verzeichnis, in das Sie schauen müssen:

| In `dist/releases` | Inhalt |
| --- | --- |
| `*.dmg`, `*-setup.exe`, `*.deb`, `*.pkg.tar.gz`, `*.AppImage` | [Installer](#installers) für macOS, Windows und Linux |
| `*.AppImage.zsync` | Delta-Update-Steuerdateien für AppImageUpdate und kompatible Launcher |
| `*-ios.ipa`, `*-tvos.ipa`, `*.apk` | Die iOS-, tvOS- und Android-Pakete für die Stores oder Geräte |
| `*.pkg`, `*.msix`, `*.aab`, `*.flatpak`, `*.tar.gz` und `flatpak/` | Die [Store-Pakete](#store-packages) eines `store`-Builds |
| `install.sh`, `install.ps1` | Skripte zur Installation der App aus [GitHub-Releases](#publish-on-github) |
| `appcast-<target>.xml` | Die Feeds, die Apps zur [Aktualisierung](#updates) lesen |
| `homebrew/<name>.rb`, `winget/` | Direkte [Homebrew-Cask- und winget-Manifeste](#channels) |
| `*-web.zip` | Die Website zum Entpacken und direkten Ausliefern |
| `miniprogram/<platform>/` | Das Projekt jedes Mini-Programms zum Hochladen über das jeweilige Entwicklerwerkzeug |

Jede Datei heißt `<name>-<version>-<system>-<arch>`, etwa `hello-1.2.0-windows-x64-setup.exe`, sodass Nutzende auf einer Release-Seite direkt die für ihr System passende Datei finden, und liegt neben einer zugehörigen `.sha256`-Datei. Ein Build einer anderen Version leert das Verzeichnis zuvor; Builds derselben Version – auch von anderen Maschinen – ergänzen es. Die Pakete für Simulatoren und watchOS bleiben in ihren Verzeichnissen `dist/<name>-<version>-<target>`.

Die Website enthält `.wasm.br` und `.wasm.gz`, sofern diese kleiner als das Originalmodul sind. Aktivieren Sie vorkomprimierte Dateien auf Hosts, die diese unterstützen. Liefern Sie sie unter der ursprünglichen `.wasm`-URL mit `Content-Type: application/wasm`, dem passenden `Content-Encoding` und `Vary: Accept-Encoding` aus. Das Originalmodul funktioniert auf jedem statischen Host. [Veröffentlichen im Web](web.md) behandelt semantische Überschriften, Browser-Links, seitenspezifische Metadaten und das Laden über CDNs.

### Offline-Webpakete

Setzen Sie `web.offline: true` in `nsgo.yaml`, um ein Web-Release beim ersten Online-Start zwischenzuspeichern.

```yaml
web:
  offline: true
```

Liefern Sie das veröffentlichte Verzeichnis über HTTPS aus oder verwenden Sie `go tool nsgo build -target web -run` auf localhost. Der Service Worker speichert die Seite, WebAssembly, Skripte und paketierte Ressourcen im Cache. Online-Besuche laden die veröffentlichte Version. Nach erfolgreicher Installation und dem Schließen aller Tabs der vorherigen Version kann die neueste Version aus dem Cache gestartet werden. Anfragen an Online-Dienste einer App erfordern weiterhin eine Verbindung.

## Terminal

Führen Sie die App in einem interaktiven Terminal mit `go tool nsgo dev -target tui` aus. `go tool nsgo test -target tui` führt ihre Tests dort über `nsgotest.Main` aus, eine Anwendung pro Testprozess. Das Terminal bestimmt die Größe: Eine Zelle entspricht 8 mal 16 Layout-Punkten. Wählen Sie die Styles Ihrer Anwendung explizit; Komponenten behalten ihre gewohnten Formen und Größen. `examples/tui` demonstriert Felder, Buttons, Tastaturkürzel und Größenänderungen.

Erstellen Sie eine Konsolenanwendung mit `go tool nsgo build -target tui_macos_arm64` oder dem passenden `tui_macos_x64`, `tui_windows_arm64`, `tui_windows_x64`, `tui_linux_arm64` oder `tui_linux_x64`. Die Build-/Check-Gruppe `tui` umfasst alle sechs, und `all` umfasst sie ebenfalls. Jedes Paketverzeichnis enthält die ausführbare Datei und Ressourcen; es hat keinen GUI-Installer. Alle sechs bauen allein mit Go.

Das Backend unterstützt Zellentext, Hintergründe, durchgezogene Rahmen, Clipping, Scrollen, Fokus und Textbearbeitung. Es verwendet Schriftart und Farben des Terminals; Control ist `mod`, auch auf dem Mac. Strg+C kopiert innerhalb eines Editors und beendet das Programm, wenn kein Binding oder Editor es abfängt. Bracketed Paste fügt Text ein, ohne Tastaturkürzel auszulösen, und die Zwischenablage ist prozesslokal. Canvas, Bilder, Medien, nativ gehostete Komponenten, Systemmenüs/-hinweise, Animationen, Zoom, bidirektionales Shaping, Auslassungspunkte am Anfang/in der Mitte und eine System-Zwischenablage-Brücke sind noch nicht implementiert. `base/terminal` bleibt das in einer GUI-Anwendung eingebettete Terminal-Steuerelement.

Solange `nsgo dev` das Terminal belegt, gehen Werkzeug- und Anwendungsdiagnosen nach `.nsgo/tui.log`. Beim Beenden werden Bildschirm, Cursor und Terminalmodi wiederhergestellt, bevor zur Shell zurückgekehrt wird. Terminal-Test-Screenshots rastern die Zellen mit einer festen portablen Schriftart, statt den Emulator abzufotografieren; nutzen Sie Text-Assertions für Unicode-Glyphen, die in dieser Schrift fehlen.

## nsgo.yaml

```yaml
name: Hello
id: dev.example.hello   # the bundle ID; without it, dev.nsgo. and the name's letters and digits
version: 0.1.0
build_number: 1         # CFBundleVersion and Android's versionCode; else from version
# copyright: © 2026 Example  # the Windows executable's details and installer, and the macOS About panel
# appearance: dark      # light or dark whatever the system's
# theme: theme.json     # a TSX app's theme: NSGo's or a shadcn/ui one
# i18n: { dir: locales, source: en }  # the app's catalogs and the language they are written in
icon:
  apple: assets/AppIcon.icon  # macOS, iOS and watchOS, or an .icns for macOS
  windows: assets/IconWindows.png  # optional Windows artwork
  linux: assets/IconLinux.png      # optional Linux artwork
  png: assets/Icon.png        # every other target
  tvos: assets/AppIconTV.brandassets  # the Apple TV's layered icon and top shelf art, or a wide PNG
  watchos: assets/AppIconWatch.icon   # the Apple Watch's own, drawn in circles, or a square PNG
  android:
    png: assets/IconAndroid.png  # optional bitmap; valid without adaptive layers
    foreground: assets/IconForeground.png  # Android's adaptive icon: 432 px, the logo within the center 264 px
    background: "#1F1F1F"                 # a color or a square PNG
    monochrome: assets/IconMonochrome.png  # optional: the shape Android 13's themed icons tint
dev:
  target: native        # native, web, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android, harmonyos or miniprogram_<platform>
  hot_reload: true      # false builds once, watching no files
  # icon: { png: assets/Icon-dev.png }  # replaces icon's files in dev's builds; defaults to assets/*-dev.*
build:
  output: dist
  tags: []              # Go build tags every build, test and check of the app takes, such as timetzdata
  ldflags: []           # the app's own linker flags, such as -X main.releaseDate=2026-11-17
  open: false           # true opens the output directory after building
  installers: true      # false leaves out the disk image, the Windows installer and the Linux packages
  jobs: 0               # how many targets build at once; 0 is half the CPUs, 1 one after another
  targets: [macos_arm64, macos_x64, windows_arm64, windows_x64, linux_arm64, linux_x64, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android_arm64, web]
# apple: { sign_in_with_apple: true }  # the Sign in with Apple entitlement, for iOS, tvOS and Mac App Store builds
# apple: { purchases: true }  # StoreKit's library, which in-app purchases go through, in macOS, iOS and tvOS packages
# apple: { privacy: { accessed_apis: { NSPrivacyAccessedAPICategoryDiskSpace: [E174.1] }, tracking: false } }  # add required-reason APIs and tracking declarations to PrivacyInfo.xcprivacy
# apple: { requires_full_screen: false }  # let an iPad run the app in Split View, Slide Over and Stage Manager, in all four orientations
# apple: { orientations: [portrait, landscape_left] }  # the orientations a full-screen app takes on an iPad
# apple: { multiple_scenes: true }  # let an iPad open more than one window of the app, each showing the main view with its own state
# apple: { info: { UIBackgroundModes: [audio] } }  # background playback for iOS Picture in Picture
# android: { permissions: [REQUEST_INSTALL_PACKAGES] }  # manifest permissions beyond the runtime's
# android: { recording: true }  # the microphone service microphone.RecordWAV records in, with RECORD_AUDIO, the foreground-service permissions and POST_NOTIFICATIONS, which RecordWAV asks for so its notification shows from Android 13; a refusal still records
# android: { uses_cleartext_traffic: true }  # allow plain HTTP to any host in the APK and App Bundle
# android: { background_downloads: true }  # dataSync foreground service for cx.Download
# android: { background_motion: true }  # explicit finite motion shortService; streams also need a notification title
# android: { loopback_cleartext: true }  # allow plain HTTP to loopback in the APK and App Bundle
# commands: [{ name: hello-cli, package: ./cmd/hello-cli }]  # command-line programs the desktop packages ship beside the app: see Commands
# windows: { command_line: true }  # add <name>.com, a console companion that forwards arguments and streams to <name>.exe; not in store builds
# windows: { path: true }  # the installer puts the app's folder on the user's Path, so consoles find its commands
# windows: { publisher: Example Ltd }  # the company the Windows executable's details, its installer and the uninstall entry name; else windows.store.publisher_name
# linux: { maintainer: "Jane Doe <jane@example.com>" }  # the Linux packages' maintainer, else the app's name
# linux: { categories: [Office, Dictionary] }  # the freedesktop menu categories of the app's desktop entry, at least one a main category such as Office, Utility or Game, else menus file it under Other; your own start with X-
# linux: { share: packaging/share }  # files the .deb and Arch package install under /usr/share and the AppImage in its usr/share, such as metainfo/<id>.metainfo.xml or bash-completion/completions/<command>
# linux: { depends: { deb: [gstreamer1.0-plugins-good], arch: [gst-plugins-good] }, recommends: { deb: [gstreamer1.0-libav], arch: ["gst-libav: H.264 and AAC video"] } }  # packages the .deb and Arch package also depend on and recommend, after GTK 4 and WebKitGTK 6, each as dpkg or pacman writes it
# release: { github: me/hello }  # the GitHub repository the install scripts and updates install from, else the Git remote origin's; tag_prefix: v by default
# updates: { key: <public key> }  # how the app updates, by platform and channel: see Updates
# store: { dir: store }           # store listings directory (YAML and Markdown per language)
# miniprogram: { appid: { wechat: wx0123456789abcdef, alipay: "2021000000000000" }, custom_navigation: false }  # the mini programs'
```

Ein reiner Betriebssystemname in `targets` – oder in `nsgo build -target` sowie `nsgo check -target` – steht für dessen Architekturen (`windows` bedeutet `windows_arm64` und `windows_x64`; `android` nur `android_arm64`, da der x86_64-Build ein NDK erfordert). `macos_universal`, das von `macos` und `all` ausgelassen wird, baut beide macOS-Architekturen und fügt sie zu einer einzigen App zusammen, einschließlich ihrer Bibliotheken und Widgets. Der Steam-Kanal nutzt das Universal-App-Bundle. Installer-Builds verpacken es in ein einziges Disk-Image mit einem Sparkle-Feed auf dem Direct-Kanal. `nsgo test -target` nimmt ebenfalls den Namen der Architektur entgegen, für die die Tests laufen: bei einem Desktop-OS auf der Architektur dieser Maschine und bei Android auf `arm64`, sodass `windows_x64` das `windows`-Ziel auf einer x64-Windows-Maschine ausführt, während eine Architektur, für die das Ziel nicht läuft, mit dem zu verwendenden Namen abgelehnt wird.

Flags ersetzen die jeweils benannten Werte wie `-target`, `-version` oder `-output`; `-h` listet alle auf.

`build.ldflags` enthält die eigenen Linker-Flags der App, jeweils so geschrieben, wie `go build -ldflags` sie entgegennimmt, etwa `-X main.releaseDate=2026-11-17` oder `-X 'main.channel=beta builds'`. Jeder Build, der die App linkt, übernimmt sie: `dev` samt seinen Reloads, `build` (auch mit `-embed`) und `test` (auch mit `-target`) auf jeder Zielplattform. `-ldflags` bei `build` und `test` ergänzt sie statt sie zu ersetzen – mehrfach angegeben für weitere Flags. Sie stehen vor den eigenen `-X`-Flags des Werkzeugs, und der Linker behält den letzten Wert einer Variable bei, sodass Name, ID und Version in `core.App` die aus `nsgo.yaml` bleiben. Das Werkzeug übergibt `go` seine eigenen `-ldflags`, was ein Flag in `GOFLAGS` ersetzt – Linker-Flags gehören daher hierher; ein `-ldflags` nach dem Verzeichnis bei einem Headless-`test` gehört zu `go test` und ersetzt alle.

Jeder `go`-Befehl, den das Werkzeug für die App ausführt, übernimmt `build.tags` – in `dev` samt seinen Reloads, in `build` (auch mit `-embed`), in `test` (auch mit `-target`) und in `check`, auf jeder Zielplattform. NSGos eigene Tags wie `nsgodev` in `dev` oder `nsgotest` in Zielplattform-Tests werden hinzugefügt, ebenso wie die Tags aus `-tags` in `GOFLAGS`, die sonst ersetzt würden; der Rest von `GOFLAGS` gilt unverändert.

`apple.info.LSMinimumSystemVersion` hebt das Minimum des macOS-Pakets an, und `apple.info.MinimumOSVersion` hebt das des gewählten iOS-, tvOS- oder watchOS-Pakets an. Schreiben Sie in Anführungszeichen gesetzte Versionszeichenketten wie `"14.2"` oder `"18.0"` mit ein bis drei numerischen Teilen. Die Laufzeit-Untergrenzen sind macOS 13.0 (Go 1.27), iOS 15.0, tvOS 17.0 und watchOS 26.0; niedrigere oder fehlerhafte Werte schlagen fehl. Der Mobile-Schlüssel muss die Untergrenze jedes ausgewählten mobilen Ziels erfüllen. App Clips behalten ihr eigenes `app_clip.minimum`. Die Werte gelangen in Paket-Metadaten, Asset-Kompilierung, Signaturprojekte und den Linker der App; eingebettete iOS-Frameworks nutzen sie ebenfalls, mit Mac Catalyst mindestens 16.0 für Gos macOS-13-Laufzeit.

`icon.windows` und `icon.linux` wählen quadratische PNGs für die jeweiligen App-Pakete, Installer-Grafiken und Store-Logos. Standardmäßig nutzen sie `assets/IconWindows.png` und `assets/IconLinux.png` und fallen sonst auf `icon.png` zurück; `dev.icon` akzeptiert dieselben Einstellungen sowie `-dev`-Standarddateien. `icon.android.png` wählt unabhängig eine Android-Bitmap aus, auch wenn keine adaptiven Ebenen konfiguriert sind. Andere Zielplattformen behalten den gemeinsamen Fallback `icon.png`.

Das Icon eines Apple TV ist eine mehrschichtige Karte, kein Quadrat: `tvos` akzeptiert Xcodes `.brandassets` mit seinen `App Icon`-Stapeln und Top-Shelf-Bildern oder ein breites Bild wie 1280×768, das das Werkzeug über seine Randfarbe passend in die Karte einpasst und auf den Top-Shelf-Bildern zentriert. Fehlt beides, sitzt das PNG auf drei Vierteln der Kartenhöhe. Eine Apple Watch maskiert ihr Icon kreisförmig, sodass `watchos` eine speziell für die Kreise von watchOS gestaltete `.icon`-Datei oder ein quadratisches PNG erwartet; ohne sie verwendet die App `apple`, dann `png`. Beide fallen wie `apple` und `png` auf `assets/AppIconTV.*` und `assets/AppIconWatch.*` zurück.

Auf Android machen `icon.android.foreground` und `background` das Launcher-Icon adaptiv, sodass Launcher es in ihre eigene Form maskieren und nach Belieben bewegen, statt `png` auf eine eigene Grundplatte zu schrumpfen. Jede Ebene ist 108 dp quadratisch, ein auf 432 px skaliertes PNG; der Launcher zeigt die mittleren 72 dp (288 px) und maskiert auf eine Form, die alles außerhalb der mittleren 66 dp (264 px) abschneiden kann – halten Sie das Logo also innerhalb dieses Kreises und lassen Sie den Hintergrund das gesamte Quadrat ausfüllen. `background` ist ein PNG oder eine Farbe wie `"#1F1F1F"`. `monochrome`, eine Form, bei der allein der Alpha-Kanal zählt, färben Android 13 und neuer ein, wenn Nutzende thematisierte Icons aktivieren; ohne sie behält das Icon dort seine Farben. Das Paket behält `icon.android.png`, sonst `icon.png`, als einfaches Bitmap-Icon, das kein Launcher ab Android 9 – der ältesten von NSGo unterstützten Version – gegenüber dem adaptiven vorzieht; ohne `png` rendert das Werkzeug es aus Vorder- über Hintergrund. Ohne adaptive Ebenen zeigt der Launcher das Android-Bitmap oder das gemeinsame `png`.

`android.uses_cleartext_traffic: true` erlaubt einer APK oder einem App Bundle unverschlüsseltes HTTP zu jedem Host und setzt sowohl das Manifest-Flag als auch die Netzwerksicherheitsrichtlinie. Ohne diese Freigabe erlaubt `android.loopback_cleartext: true` unverschlüsseltes HTTP nur zu `localhost`, `127.0.0.1` und `::1`, etwa für von der App selbst bereitgestellte Videos. Die reine Loopback-Richtlinie untersagt Klartextverbindungen zu anderen Hosts; die Freigabe für alle Hosts hat Vorrang, wenn beide Einstellungen wahr sind. Bei einem eingebetteten AAR konfigurieren Sie die Netzwerksicherheitsrichtlinie der Host-App.

Jeder Build akzeptiert beim Start zudem NSGos eigene Flags wie `--nsgo-fps` ([Ausführen und Debuggen](debugging.md#flags-of-every-build)). In `nsgo dev` lädt Command/Control+R aus jedem offenen Fenster neu und bleibt auch nach dem Schließen der Haupt-View verfügbar, solange ein anderes Fenster eine App-Funktion aktiv hält.

### Startbildschirm

Setzen Sie `launch_screen`, um auf iOS sowie ab Android 12 einen deckenden Hintergrund, ein zentriertes PNG oder beides anzuzeigen. Bildpfade sind relativ zu `nsgo.yaml`. Das Bild passt sich proportional in 128 × 128 Punkte bzw. dp ein. Ein transparentes PNG mit einer längsten Seite von mindestens 512 Pixeln bleibt auf hochauflösenden Bildschirmen scharf.

```yaml
launch_screen:
  background: "#F5F1EA"
  image: assets/Launch.png
  dark:
    background: "#201C26"
    image: assets/LaunchDark.png
```

Das System wählt das Erscheinungsbild, sofern `appearance` es nicht festlegt. Lassen Sie ein Feld unter `dark` weg, um den entsprechenden hellen Wert wiederzuverwenden. Ein weggelassener Hintergrund oder ein weggelassenes Bild behält den Standard der Plattform bei. Eingebettete Apps erhalten ihren Startbildschirm vom Host.

## Native Android-Klassen

Listen Sie vorkompilierte Android-Klassen in der `nsgo.yaml` der App auf, um eine eigene View über `core.Native` zu verwenden.

```yaml
android:
  classes: [java/BadgeView.jar, java/Helpers.dex]
```

Pfade sind relativ zu `nsgo.yaml`, sofern nicht absolut angegeben. Jeder Pfad benennt eine `.dex`-Datei oder ein JAR mit den von D8 erzeugten `classes.dex`, `classes2.dex` und weiteren Shards auf oberster Ebene. Bereiten Sie diese Dateien mit D8 unter Angabe von `--min-api 28` vor. Auf arm64 baut NSGo die APK oder das App Bundle aus diesen vorkompilierten Dateien allein mit Go. Android-Entwicklungssitzungen bauen neu, sobald sich eine aufgeführte Datei ändert. Bei einem eingebetteten AAR stellt der Android-Build des Hosts seine Java-Abhängigkeiten bereit. [D8-Dokumentation](https://developer.android.com/tools/d8)

Das [Android-Native-Beispiel](https://github.com/nsgo-dev/nsgo/tree/main/examples/android-native) registriert eine Klasse mit einem öffentlichen `Context`-Konstruktor und konfiguriert sie über eine Go-Prop.

## Spiele

Setzen Sie `game: true` in `nsgo.yaml` für ein Spiel. Android stuft die App als Spiel ein und nutzt die Game-Mode-Optimierungen des Systems. macOS und iOS aktivieren den Spielmodus, macOS wählt standardmäßig die Kategorie „Spiele“, und Linux ergänzt `Game` im Desktop-Eintrag. Ein explizites `macos.category` oder ein Game-Mode-Schlüssel in `apple.info` hat Vorrang.

Setzen Sie `gamepad: true`, wenn Ihre App ein Hardware-Gamepad unterstützt. Android deklariert sowohl Gamepad als auch Touchscreen als optional. macOS, iOS und tvOS deklarieren das Extended-Gamepad-Profil, und tvOS zusätzlich das Micro-Gamepad der Siri Remote, wobei Controller-Schlüssel in `apple.info` Vorrang haben.

## Lizenzhinweise

`nsgo build` legt `THIRD_PARTY_NOTICES.txt` in den Ressourcen des eigenständigen Pakets ab. Es enthält die Lizenztexte der von diesem Ziel verwendeten Go-Module, der Go-Laufzeit sowie von NSGos gebündelter Java- oder JavaScript-Laufzeit. Web-Builds benennen die Datei wie andere Ressourcen anhand ihres Inhalts. Verwenden Sie `core.ThirdPartyNotices()`, um denselben Text in einem Credits-Bildschirm in der App anzuzeigen. Die Funktion liefert in der Entwicklung, in Tests und in Builds eingebetteter Bibliotheken einen leeren String zurück. [Deklarationen nativer Bibliotheken](native-libraries.md) fügen ihre ursprünglichen Lizenztexte denselben Hinweisen hinzu.

Standardmäßig lässt eine unbekannte oder fehlende Drittanbieterlizenz den Build fehlschlagen, und eine erkannte Copyleft-Lizenz gibt eine Warnung aus. Module, die im Go-Workspace der App (`go.work`) aufgeführt sind, werden wie eigener Code zusammen mit dem eigenen Modul der App behandelt: Alle darin enthaltenen Lizenzdateien werden eingebunden, und sie lösen keine Drittanbieterprüfungen aus. Setzen Sie jede der beiden Regeln auf `error` oder `warn`. Die `allow`-Liste nimmt exakte Modulpfade von beiden Regeln aus und behält alle vom Build gefundenen Lizenztexte bei.

```yaml
licenses:
  unknown: error
  copyleft: warn
  allow: [example.org/reviewed-module]
```

## Bibliotheken aus Go-Paketen

Apps, die `base/terminal` importieren, liefern zusätzlich `libghostty-vt` aus: Windows- und Linux-Pakete legen die Bibliothek neben die ausführbare Datei; macOS- und iOS-Bundles signieren sie in ihre Frameworks. [Einbetten](embedding.md) erläutert das begleitende XCFramework für iOS-Hosts.

Eine Abhängigkeit kann vorgebaute Desktop-Shared-Libraries in `nsgo-natives.yaml` deklarieren. NSGo prüft deren SHA-256-Prüfsumme, kopiert sie vor dem Signieren in die App und nimmt sie in die Installer auf; für Autorinnen und Autoren der App ist kein zusätzlicher Schritt nötig. [Native Bibliotheken](native-libraries.md) beschreibt Format und Ladepfade für Paketautorinnen und -autoren.

## Befehle

Eine App kann Kommandozeilenprogramme neben ihrer Desktop-Ausführungsdatei mitliefern, etwa ein CLI, das auf den Daten der App arbeitet. Listen Sie sie unter `commands` auf, jeweils gebaut aus einem Go-`main`-Paket:

```yaml
commands:
  - name: hello-cli             # the file's name, hello-cli.exe on Windows
    package: ./cmd/hello-cli    # a main package, built for each desktop target with CGO_ENABLED=0 and build.tags
    ldflags: [-X main.channel=beta]  # its own linker flags, after build.ldflags
windows:
  path: true                    # the installer puts the app's folder on the user's Path
```

Ein Name besteht aus Buchstaben, Ziffern, `-` und `_` und darf in keiner Groß-/Kleinschreibung mit der ausführbaren Datei der App, ihrem Linux-Befehl, `Uninstall`, `AppRun` oder einem anderen Befehl übereinstimmen. `macos_universal` führt die beiden macOS-Builds jedes Befehls zusammen, genau wie die der App.

Ein Paket linkt mit `build.ldflags` und `-ldflags`, dann mit seinen eigenen `ldflags`, deren `-X`-Werte gewinnen, und schließlich mit der Identität der App, sodass `core.App()` dem Befehl Name, ID, Version und Build-Nummer aus `nsgo.yaml` liefert – genau wie der App selbst. Ein Befehl, der `core` nicht importiert, erhält die Version per `-X`, etwa `-ldflags "-X main.version=1.2.0"` bei `build`, das sowohl die App als auch ihre Befehle übernehmen.

| Paket | Befehle |
| --- | --- |
| macOS | In `Contents/MacOS`, vor der App genauso wie diese signiert, als `<id>.<command>`: ad hoc oder mit `sign.identity`, Hardened Runtime und Zeitstempel, sodass die App [notarisiert](#notarize) wird – jedoch ohne die Entitlements der App. Das Disk-Image legt nichts in `PATH` ab; der [Homebrew-Cask](#channels) verlinkt jeden Befehl, und Nutzende können `/Applications/<name>.app/Contents/MacOS/<command>` selbst verlinken |
| Windows | `<command>.exe` neben `<name>.exe`, genauso wie diese [signiert](#sign-windows-packages), vom Installer installiert und vom Uninstaller entfernt; mit `windows.path` trägt der Installer den Ordner zusätzlich in `Path` der nutzenden Person ein – wie `windows.command_line` –, sodass nach der Installation geöffnete Konsolen die Befehle finden. Ein Befehl, den ein Update umbenennt oder entfernt, bleibt im Ordner – wie jede Datei, die die App nicht mehr ausliefert –, da der Uninstaller nur entfernt, was seine Version installiert hat |
| Linux | In `/opt/<slug>` mit einem `/usr/bin/<command>`-Link, der dem `.deb`- und Arch-Paket gehört, sodass das Entfernen des Pakets ihn entfernt und `dpkg` sowie `pacman` sich weigern, über das Paket eines anderen zu installieren; das AppImage führt sie neben der App mit und legt nichts in `PATH` ab |

Die Installer, ihre Prüfsummen und die [Update](#updates)-Appcasts umfassen die Befehle wie die übrigen Dateien der App, sodass ein Update sie zusammen mit der App ersetzt. [Store-Pakete](#store-packages) lassen sie weg, da die Sandbox des Mac App Store, das MSIX und das Flatpak nur die eigene ausführbare Datei der App ausführen.

## Installer

Für die Desktop-Ziele schreibt `build` die Dateien, mit denen Nutzende die App installieren, nach `dist/releases`, jeweils mit einer `.sha256`-Prüfsumme:

| Zielplattform | Installer |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.dmg`, ein Disk-Image mit der App, einem Pfeil und einem Link zum Programme-Ordner zum Hinüberziehen, signiert mit `sign.identity` und [notarisiert](#notarize) |
| Windows | `<name>-<version>-windows-<arch>-setup.exe`, ein nativer Windows-Installer, der für die nutzende Person installiert – bei erneutem Ausführen im zuvor gewählten Ordner, wobei eine laufende App geschlossen wird –, mit Startmenü-Verknüpfung, optionaler Desktop-Verknüpfung und einem Uninstaller, der nur entfernt, was er selbst installiert hat, [signiert mit `sign.certificate`](#sign-windows-packages), wo die App Windows-Pakete signiert; `windows.command_line` installiert zusätzlich den `.com`-Begleiter und trägt den Ordner der App in `Path` der nutzenden Person ein, sodass nach der Installation geöffnete Konsolen den App-Befehl ausführen, während Win+R – das den Begleiter ebenfalls findet – die App startet und die geöffnete Konsole schließt; der Uninstaller entfernt den Ordner nur dann aus `Path`, wenn ihn ein Installer hinzugefügt hat, niemals einen selbst angelegten Eintrag |
| Linux | `<name>-<version>-linux-<arch>.deb` für Debian und Ubuntu sowie `<name>-<version>-linux-<arch>.pkg.tar.gz` für Arch Linux, die die App in `/opt/<slug>` samt Befehl, Links auf ihre [Befehle](#commands), Desktop-Eintrag und Icon installieren (das Arch-Paket enthält `.MTREE` für `pacman -Qkk`), sowie `<name>-<version>-linux-<arch>.AppImage`, das auf jeder Distribution ohne Installation läuft |

Setup und Uninstaller für Windows bauen allein mit Go auf jedem System und verwenden native Windows-Dialoge. Sie sprechen diejenigen Sprachen aus den App-Katalogen, für die sie Texte haben – einschließlich vereinfachtem und traditionellem Chinesisch –, und wählen die Sprache der Systemoberfläche; `/LANG=<LCID>` wählt eine Sprache für einen einzelnen Aufruf, etwa `/LANG=2052` für vereinfachtes Chinesisch. `/S` installiert oder deinstalliert still, ein abschließendes `/D=<folder>` wählt den Ordner, auch mit Leerzeichen, und `/RELAUNCH` öffnet die App nach einem Update. Ohne `/D` installiert ein Upgrade in den Ordner der Installation, die es ersetzt. `-installers=false` oder `build.installers: false` baut ausschließlich die Pakete.

Die Installer übernehmen eigene Bilder und Icons der App; andernfalls wird jeweils das App-Icon oder das eingebaute Bild verwendet:

```yaml
macos:
  dmg_background: assets/DMGBackground.png  # the disk image's window, which takes its size in points
  dmg_icon: assets/DMGIcon.png              # the disk image's icon: an .icns or a square PNG
windows:
  installer_icon: assets/Setup.png          # the setup program's icon: an .ico or a square PNG
  uninstaller_icon: assets/Uninstall.png    # the uninstaller's
```

Ein Hintergrund-PNG mit einem Zwilling in doppelter Größe, etwa `DMGBackground@2x.png`, bleibt auf Retina-Bildschirmen scharf. Das Icon der App sitzt ein Viertel von links, der Programme-Ordner ein Viertel von rechts, jeweils mit seinem Namen vertikal zentriert dort, wo das eingebaute Bild seinen Pfeil zeichnet; lassen Sie in einem eigenen Hintergrund dafür Platz. Die Linux-Pakete haben keine eigenen Bilder; sie installieren das App-Icon.

Die Linux-Pakete installieren außerdem die AppStream-Metadaten der App unter `/usr/share/metainfo/<id>.metainfo.xml`, anhand derer Software-Center sie auflisten – erstellt aus `linux.summary`, `linux.license` (falls gesetzt) und den Namen der App, mit `Utility` als Kategorie, wenn `linux.categories` keine angibt; das AppImage legt sie im `usr/share` seines AppDir ab, neben einem Link zu seinem Desktop-Eintrag in `usr/share/applications`, den die Metadaten benennen. Dateien, die das System aus `/usr/share` liest – etwa ausführlichere Metadaten, weitere Desktop-Einträge, Shell-Vervollständigungen oder Handbuchseiten –, gehören in das Verzeichnis, das `linux.share` relativ zu `nsgo.yaml` angibt: Das `.deb`- und Arch-Paket installieren dessen Baum unter `/usr/share` und das AppImage unter `usr/share` seines AppDir. Dessen `metainfo/<id>.metainfo.xml` ersetzt die von NSGo geschriebene Datei, und eine Datei an einem Pfad, den das Paket selbst schreibt – wie der Desktop-Eintrag oder das Icon der App –, lässt den Build fehlschlagen. Dateien in `resources/` bleiben unabhängig von ihrem Namen neben der ausführbaren Datei in `/opt/<slug>`, und die Flatpaks der Store-Builds verwenden ausschließlich NSGos Metadaten.

Das AppImage benötigt FUSE, das die meisten Distributionen mitbringen, und bezieht GTK 4 wie die Pakete vom System; seine Laufzeitumgebung wird einmalig in den Benutzer-Cache heruntergeladen.

AppImages betten einen `gh-releases-zsync`-Locator mithilfe von `release.github` oder dem GitHub-Remote `origin` ein und wählen das neueste stabile Release sowie die Architektur dieser App. Laden Sie jedes `.AppImage` zusammen mit seiner erzeugten `.AppImage.zsync`-Datei hoch. Für einen anderen Host setzen Sie `linux.appimage_update` auf `zsync|https://example.com/latest.AppImage.zsync`, eine stabile URL mit HTTP-Range-Unterstützung; außerdem wird `gh-releases-zsync|owner|repo|latest-pre|name-*-linux-x64.AppImage.zsync` für einen Vorab-Kanal akzeptiert. Ein expliziter leerer String deaktiviert die eingebettete Erkennung; ohne GitHub-Repository oder konfigurierten Locator bleibt sie ebenfalls leer. Die `.zsync`-Datei wird dennoch erzeugt – getrennt vom signierten Appcast.

## Store-Pakete

`-channel store` oder `updates.channel: store` baut die Desktop- und Android-Ziele für deren Stores, die auch die Updates übernehmen, anstelle der Installer und Installationsskripte:

| Zielplattform | Store-Paket |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.pkg` für den Mac App Store: die App in der App-Sandbox, signiert mit der Identität des Provisioning-Profils und paketiert mit der Installer-Identität Ihres Teams |
| Windows | `<name>-<version>-windows-<arch>.msix` für den Microsoft Store, der es bei der Veröffentlichung signiert |
| Linux | Installierbares `<name>-<version>-linux-<arch>.flatpak`, plus ein AppStream-tragendes `.tar.gz` und `flatpak/<id>.yml` für Flathub oder `flatpak-builder` auf der GNOME-Laufzeit |
| Android | `<name>-<version>-android-<arch>.aab`, ein App Bundle für Google Play, signiert mit `sign.keystore`, dem Upload-Schlüssel |

Das MSIX liest den Anzeigenamen jeder Sprache aus `appName` der Kataloge und fällt auf die Ausgangssprache zurück. Seine Start-, Taskleisten- und Store-Logos umfassen 100%-, 125%-, 150%-, 200%- und 400%-Varianten. Das Werkzeug schreibt deren Ressourcenindex unter macOS; ein Windows SDK wird nicht benötigt.

`-target macos` wählt auf dem Store-Kanal beide macOS-Architekturen aus und schreibt ein `<name>-<version>-macos-universal.pkg`, genau wie `-target macos_universal`. Wählen Sie `-target android_arm64,android_x64` für ein gemeinsames `<name>-<version>-android-universal.aab` mit `arm64-v8a` und `x86_64`. Der x86_64-Slice erfordert ein Android NDK.

```yaml
macos:
  category: public.app-category.productivity  # LSApplicationCategoryType, which the Mac App Store requires
  # entitlements: { com.apple.security.files.downloads.read-write: true }  # beyond the network, the files the user picks and what apple.info explains
windows:
  store:                               # as Partner Center's Product identity lists them
    identity: 12345Example.Hello
    publisher: CN=00000000-0000-0000-0000-000000000000
    publisher_name: Example
  # device_capabilities: [microphone, webcam]  # the devices the app uses, which Windows asks the user for
linux:
  summary: Says hello                  # one line for software centers
  license: MIT                         # an SPDX expression; LicenseRef-proprietary by default
  # flatpak_runtime: "51"              # GNOME's runtime version
android:
  target_sdk: 36                       # the targetSdkVersion Google Play requires
sign:
  macos_profile: assets/MacAppStore.provisionprofile  # the Mac App Store's profile
  # installer: "3rd Party Mac Developer Installer: Example (ABCDE12345)"  # by default the keychain's of the profile's team
```

Die Laufzeit zielt auf API 36, wenn `android.target_sdk` nicht gesetzt ist. Ein Android-App-Bundle-Build schlägt unterhalb von API 36 fehl, was [Google Play erfordert](https://support.google.com/googleplay/android-developer/answer/11926878) – für neue mobile Apps und Updates ab dem 31. August 2026. Ein eigenständiger APK-Build warnt und fährt fort.

Setzen Sie `sign.macos_profile`, um das Mac-Paket für den Upload zu signieren. Ohne Widgets läuft das unsignierte Paket dennoch in der Sandbox, sodass Sie den Zugriff auf den Container der App, das Netzwerk und von Nutzenden gewählte Dateien testen können. Ein Store-Build mit Widgets erfordert sowohl `sign.macos_profile` für `<id>` als auch ein macOS-`sign.widgets_profile` für `<id>.widgets` vom selben Team, das `apple.app_group` (standardmäßig `group.<id>`) gewährt. Beide Bundles nutzen diesen gemeinsamen Container; es wird keine Home-Verzeichnis-Ausnahme hinzugefügt. `-installers=false` behält Sandbox und Profilsignaturen bei, lässt aber `.pkg` und seine Installer-Identität weg. Laden Sie die `.msix` im Partner Center und das `.aab` in der Play Console hoch. Reichen Sie `flatpak/<id>.yml` bei Flathub ein, sobald das Release die darin genannten Archive enthält. `flatpak-builder` baut es auch lokal. Installieren Sie ein Bundle mit `flatpak install --user ./<name>-<version>-linux-<arch>.flatpak`; die passende GNOME-Laufzeit muss installiert oder über ein konfiguriertes Remote verfügbar sein. Builds schreiben das Bundle in Go ohne Linux-Toolchain. Store-Pakete verwenden ein transparentes Icon, wenn keine Grafik vorhanden ist, und `Utility`, wenn `linux.categories` nicht gesetzt ist. Für die Installation außerhalb des Stores setzen Sie [`sign.certificate`](#sign-windows-packages), um das MSIX zu signieren; dessen Zertifikat-Subjekt muss mit `windows.store.publisher` übereinstimmen, und der Zielrechner muss dem Zertifikat vertrauen. Ohne Signatur-Einstellungen signiert der Store das hochgeladene Paket.

Auf macOS und iOS zeichnen die Kanteneffekte einer Scroll-View (`Scroll.EdgeEffects`), macOS-Backdrop-Blurs ohne `IOS.BackdropMaterial` sowie Backdrop-Farbfilter (allein oder über einem Blur ohne `IOS.BackdropMaterial`) über Core Animations private Klassen `CABackdropLayer` und `CAFilter` – genau wie AppKits und UIKits eigene Materialien und Kanteneffekte, da keine öffentliche API mit einem Radius weichzeichnet, einen Blur über eine View variiert oder eine Backdrop-Farbmatrix anwendet. Die App-Store-Prüfung kann Apps ablehnen, die private APIs verwenden: NSGo ruft sie nur bei Scroll-Views mit Kantenleisten und jenen Backdrop-Filtern auf, aber jeder macOS- und iOS-Build enthält ihre Namen.

Jedes von `build` geschriebene Apple-Bundle – die App, ihre Widget-Erweiterung, ihr App Clip und ihre Apple-Watch-App sowie ein `-dynamic` `-embed`-Framework – trägt ein Privacy-Manifest `PrivacyInfo.xcprivacy`, das die von NSGo und Gos Laufzeit genutzten Required-Reason-APIs deklariert: User Defaults (`CA92.1`), Dateizeitstempel (`C617.1`) und Systemstartzeit (`35F9.1`). `apple.privacy` fügt die der App hinzu: `accessed_apis`, die Gründe jeder Kategorie, `tracking`, `tracking_domains` und `collected_data` – Einträge von `NSPrivacyCollectedDataTypes`, wie Apple sie benennt; eine `PrivacyInfo.xcprivacy` in `resources/` wird damit zusammengeführt statt ersetzt.

Die `Info.plist` jedes iOS-, tvOS- und watchOS-Bundles, das `build` für Geräte schreibt – das der App, ihrer Widget-Erweiterung und ihres App Clips –, enthält die von App Store Connect geprüften SDK- und Xcode-`DT`-Schlüssel, ausgelesen aus dem bauenden Xcode, sowie `UIRequiredDeviceCapabilities: [arm64]`; Simulator-Bundles lassen sie weg, und ein durch `apple.info` gesetzter Schlüssel behält seinen Wert. `apple.multiple_scenes: true` setzt `UIApplicationSupportsMultipleScenes` im Szenen-Manifest einer iOS-App, das standardmäßig falsch ist: Auf einem iPad können Nutzende dann mehr als ein Fenster der App öffnen, und jedes zeigt die Haupt-View der App neu an – mit eigenem Zustand sowie den gemeinsamen Signals und dem gemeinsamen Speicher der App –, bis die nutzende Person es schließt.

### Bei Google Play veröffentlichen

Aktivieren Sie die Google Play Developer API und erteilen Sie einem Dienstkonto die Berechtigung, die App für den gewählten Track zu veröffentlichen. Setzen Sie `NSGO_PLAY_SERVICE_ACCOUNT` auf den JSON-Schlüssel des Kontos oder den Pfad zu dessen Schlüsseldatei. [Einrichtung der Google Play API](https://developers.google.com/android-publisher/getting_started)

```sh
go tool nsgo build -target android -channel store
go tool nsgo publish -target android -track internal
```

Der Befehl sucht das App Bundle für die Version des Projekts in `build.output/releases`. Übergeben Sie nach den Flags einen `.aab`-Pfad, um bei mehreren Builds eine Datei auszuwählen. Verwenden Sie `internal`, `production` oder die Track-ID eines geschlossenen Tests. Der Befehl liest Paketnamen und Versionscode aus dem Bundle, verlangt einen Code höher als jedes Release auf diesem Track, lädt das Bundle hoch und schließt das Release ab.

## Store-Einträge

Store-Metadaten (Titel, Untertitel, Beschreibungen, Schlüsselwörter, Versionshinweise, URLs, Screenshots und Vorschauen) liegen als Klartext-YAML und Markdown unter `store/` (oder `store.dir` in `nsgo.yaml`) im Repository:

```text
store/
  listing.yaml              # version: 1, default_locale, app_store and google_play settings
  en-US/
    listing.yaml            # version: 1, title, subtitle, short_description, keywords, URLs, media
    description.md          # full description (optional file in place of YAML description)
    release_notes.md        # what's new (optional file in place of YAML release_notes)
    promotional_text.md     # App Store promotional text (optional file)
    screenshots/
  zh-Hans/
    listing.yaml
    description.md
    release_notes.md
    screenshots/
```

Beide `listing.yaml`-Dateien erfordern `version: 1` und lehnen unbekannte Schlüssel ab. Kanonische Locale-Verzeichnisse wie `zh-Hans` werden automatisch auf den Code des jeweiligen Stores abgebildet (`zh-Hans` bei App Store Connect, `zh-CN` bei Google Play), sofern nicht pro Store über `app_store.locale` oder `google_play.locale` überschrieben.

```sh
go tool nsgo publish -store all -dry-run                  # validate lengths, locales and media without uploading
go tool nsgo publish -store appstore,play                 # validate and upload to App Store Connect and Google Play
go tool nsgo publish -store play -track production        # also update release notes on the named Play track
```

Die Validierung läuft vor dem Lesen von Zugangsdaten und vor jeder HTTP-Anfrage und prüft Pflichtfelder, Locale-Codes, URL-Schemas, Mediendateien und -anzahlen (bis zu 10 Screenshots und 3 Vorschauen pro App-Store-Anzeigetyp; bis zu 8 Screenshots pro Google-Play-Bildtyp) sowie Zeichenbegrenzungen (App Store: Titel 30, Untertitel 30, Schlüsselwörter 100, Werbetext 170, Beschreibung 4000, Versionshinweise 4000; Google Play: Titel 30, Kurzbeschreibung 80, Beschreibung 4000, Versionshinweise 500).

Upload-Zugangsdaten stammen ausschließlich aus der Umgebung: `NSGO_APP_STORE_KEY`, `NSGO_APP_STORE_KEY_ID` und `NSGO_APP_STORE_ISSUER` (mit Fallback auf den Notarisierungsschlüssel `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID` und `NSGO_NOTARY_ISSUER`) für App Store Connect sowie `NSGO_PLAY_SERVICE_ACCOUNT` (Pfad zur Service-Account-JSON-Datei oder JSON-Text) für Google Play.

### Store-Compliance

Verwenden Sie `nsgo.yaml`, um den Status jedes Release-Fragebogens festzuhalten.

```yaml
compliance:
  app_store: { age_rating: false }
  google_play: { data_safety: false }
  steam: { content_survey: false }
```

Setzen Sie einen Eintrag auf `true`, nachdem Sie seinen Fragebogen abgeschlossen haben. Ein passender `-channel store`- oder `-channel steam`-Build erinnert Sie an Einträge, die auf `false` stehen. Lassen Sie alle Einträge weg, die Sie nicht in YAML verfolgen.

- Füllen Sie den [Fragebogen zur Altersfreigabe](https://developer.apple.com/help/app-store-connect/manage-app-information/set-an-app-age-rating) und die [App-Datenschutzdetails](https://developer.apple.com/help/app-store-connect/manage-app-information/manage-app-privacy) in App Store Connect aus.
- Für Apples [Verschlüsselungsfragebogen](https://developer.apple.com/help/app-store-connect/manage-app-information/determine-and-upload-app-encryption-documentation) setzen Sie `apple.info.ITSAppUsesNonExemptEncryption` als YAML-Boolean auf die Antwort Ihrer App. Falls Apple einen Freigabecode sendet, tragen Sie ihn als String in `apple.info.ITSEncryptionExportComplianceCode` ein. Sie können die Schlüssel auch weglassen und beim Upload antworten. Apple-Store-Builds prüfen die Typen dieser Schlüssel sowie die Struktur des Privacy-Manifests im Bundle.
- Füllen Sie die [Datensicherheit](https://support.google.com/googleplay/android-developer/answer/10787469) in der Play Console aus. Apps, die ausschließlich über den internen Test-Track verteilt werden, sind davon ausgenommen.
- Füllen Sie die [Inhaltsumfrage](https://partner.steamgames.com/doc/gettingstarted/contentsurvey) in Steamworks aus, bevor Sie das Spiel zur Prüfung einreichen.

## App Store Connect

Erstellen Sie eine App in App Store Connect und signieren Sie eine iOS-Geräte-IPA mit einem App-Store-Distributionsprofil oder eine macOS-Store-PKG mit der Installer-Identität Ihres Teams. Auf einem Mac mit Xcode setzen Sie `NSGO_APP_STORE_KEY` auf die `.p8`-Datei Ihres App-Store-Connect-Team-Schlüssels oder deren Text, `NSGO_APP_STORE_KEY_ID` auf die Schlüssel-ID und `NSGO_APP_STORE_ISSUER` auf die Aussteller-ID; ohne diese wird wie bei den Store-Einträgen der Notarisierungsschlüssel (`NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID`, `NSGO_NOTARY_ISSUER`) verwendet. Geben Sie diesem Schlüssel Zugriff auf die App und die Berechtigung zum Hochladen von Builds.

```sh
go tool nsgo publish ios path/to/Hello.ipa
go tool nsgo publish macos path/to/Hello.pkg
```

Das Werkzeug prüft vor dem Hochladen die Build-Nummer des Bundles und meldet anschließend Apples Verarbeitungsergebnis für TestFlight. Erhöhen Sie `build_number` für den nächsten Upload. macOS-Build-Nummern steigen über alle App-Versionen hinweg an, während iOS die Sequenz bei einer neuen Version neu starten kann. Reichen Sie einen verarbeiteten Build separat in App Store Connect für die App-Prüfung ein. [Apples Distributionsleitfaden](https://developer.apple.com/documentation/xcode/preparing-your-app-for-distribution)

## Store-Asset-Packs

Deklarieren Sie große Asset-Verzeichnisse in `mobile.asset_packs`. Dateien in diesen Verzeichnissen werden getrennt vom Basis-Android-App-Bundle und der iOS-App ausgeliefert. Pack-Namen beginnen mit einem Kleinbuchstaben und dürfen Kleinbuchstaben, Ziffern sowie Unterstriche enthalten.

```yaml
mobile:
  asset_packs:
    - {name: levels, dir: resources/levels, delivery: install-time}
    - {name: voices, dir: large-assets/voices, delivery: fast-follow}
```

Erstellen Sie Android-Packs mit `go tool nsgo build -channel store -target android_arm64`. Verwenden Sie `bundletool build-apks --local-testing` und `bundletool install-apks`, um das Bundle auf einem Gerät zu testen. NSGo öffnet installierte `install-time`-Packs. Die `fast-follow`-Module werden für die Play-Auslieferung eingebunden; das Lesen dieser Packs über das Play Asset Delivery SDK steht noch aus.

Auf iOS bildet `go tool nsgo build -target ios` `install-time` auf Apples Essential-Richtlinie und `fast-follow` auf Prefetch ab – sowohl bei der Erstinstallation als auch bei Updates. Xcode 26 oder neuer baut den Background-Assets-Downloader. Verwenden Sie eine gemeinsame `apple.app_group` oder den Standard `group.<id>`. Sowohl `sign.profile` der App als auch `sign.asset_packs_profile` der Erweiterung `<id>.assetdownloader` benötigen diese Gruppe. Laden Sie die `.aar`-Dateien aus `dist/releases` zusammen mit der IPA über den Asset-Pack-Upload-Workflow von App Store Connect hoch. [Verwaltete Asset-Packs erstellen](https://developer.apple.com/documentation/backgroundassets/creating-managed-asset-packs)

Rufen Sie aus einer Goroutine `mobile.AssetPackReady(ctx, "levels")` auf, um die Bereitschaft zu prüfen, und `mobile.OpenAssetPackFile("levels", "intro.map")`, um eine Datei relativ zum deklarierten Verzeichnis zu öffnen. Schließen Sie den zurückgegebenen Reader nach Gebrauch. [Apple-Asset-Packs lokal testen](https://developer.apple.com/documentation/backgroundassets/testing-asset-packs-locally)

## Steam-Depots

```sh
go tool nsgo build -channel steam -target macos_universal,windows_x64,linux_x64
```

Setzen Sie `updates.channel: steam`, um den Kanal standardmäßig zu verwenden. Wählen Sie macOS-, Windows- und Linux-Ziele aus. Laden Sie jedes Verzeichnis `dist/<name>-<version>-<target>` in sein Steam-Depot hoch – mit dem App-Bundle unter macOS, der ausführbaren Datei unter Windows und Linux arm64 sowie `steam-launch.sh` unter Linux x64 als Startoption. Steam übernimmt die Updates. Die Verzeichnisse enthalten weder Installer noch einen Selbst-Updater oder `steam_appid.txt`.

Linux-x64-Depots enthalten GTK 4 und seine Abhängigkeiten für Steam Linux Runtime 3.0 und 4.0. Der Launcher nutzt sie nur, wenn auf dem System kein GTK 4.14 oder neuer vorhanden ist. Belassen Sie `nsgo-gtk/` neben dem Launcher: Sein `licenses/` enthält die Lizenzen der Bibliotheken, die Quellen, aus denen sie gebaut wurden, sowie das Build-Skript – was die LGPL für GLib, Pango und GTK vorschreibt. Linux arm64 benötigt GTK 4 in seiner Laufzeitumgebung.

Für lokale Tests außerhalb des Steam-Clients setzen Sie `steam: {app_id: 480}` in `nsgo.yaml`, wobei Sie 480 durch Ihre Steamworks-App-ID ersetzen. `go tool nsgo dev` schreibt die ID nach `steam_appid.txt` in sein natives Build-Verzeichnis und startet die App mit diesem Verzeichnis als Arbeitsverzeichnis. [Steamworks API](https://partner.steamgames.com/doc/api/steam_api), [SteamPipe](https://partner.steamgames.com/doc/sdk/uploading).

## Auf GitHub veröffentlichen

Wenn der Build ein macOS-, Windows- oder Linux-Ziel enthält, liegen in `dist/releases` außerdem `install.sh` und `install.ps1`, die die App aus den Releases des in `release.github` genannten GitHub-Repositorys (oder sonst des Git-Remotes `origin` der App) installieren; ohne beides schreibt `build` keines der Skripte und weist darauf hin. Laden Sie die Dateien der obersten Ebene ohne `miniprogram/` in das Release mit dem Tag `v<version>` hoch (`release.tag_prefix` ersetzt `v`; die Skripte kodieren reservierte URL-Zeichen in diesem Präfix):

```sh
gh release create v1.2.0 dist/releases/*.*
```

Nutzende installieren oder aktualisieren die App dann mit einer einzigen Zeile, die der Build ausgibt:

```sh
curl -fsSL https://github.com/me/hello/releases/latest/download/install.sh | sh    # macOS and Linux
```

```powershell
irm https://github.com/me/hello/releases/latest/download/install.ps1 | iex         # Windows
```

Unter macOS kopiert das Skript die App aus dem Disk-Image nach `/Applications` oder – falls es dort keine Schreibrechte hat – nach `~/Applications`; unter Linux installiert es das Arch-Paket mit `pacman`, andernfalls das Debian-Paket mit `apt-get` oder `dpkg` über `sudo`; unter Windows führt es den Installer still aus, der dort installiert, wo die App bereits liegt. Jedes Skript prüft zuvor die Prüfsumme der Datei. `<NAME>_VERSION` (der App-Name in Großbuchstaben, etwa `HELLO_VERSION=1.1.0`) installiert ein anderes Release, und `<NAME>_DOWNLOAD_URL` lädt von einem Mirror herunter.

## Auf itch.io veröffentlichen

Setzen Sie `itch.project` in `nsgo.yaml` auf den Slug `owner/game` Ihres itch.io-Spiels. Um Kanalnamen zu überschreiben, setzen Sie `itch.channels` nach Build-Ziel. Die Standardwerte sind `html5` für Web, `osx-<arch>` für macOS, `windows-<arch>` für Windows und `linux-<arch>` für Linux.

```yaml
itch:
  project: example/my-game
  channels:
    macos_arm64: osx-arm64-beta
```

Mit `BUTLER_API_KEY` als Secret in der CI bauen Sie die Ziele und laden sie hoch.

```sh
go tool nsgo build -channel none -installers=false -target web,macos_arm64
go tool nsgo publish itch -target web,macos_arm64
```

Verwenden Sie bei beiden Befehlen dasselbe `-output` und `-version`, falls Sie sie überschreiben. Der Upload nimmt das Paketverzeichnis jedes Ziels und setzt seine itch.io-Version auf die Version des Builds. NSGo nutzt `butler` aus `PATH` oder lädt eine festgeschriebene Version von itch.io herunter. Nach dem ersten Web-Upload stellen Sie den Spieltyp auf itch.io auf HTML und markieren den Upload als im Browser spielbar. [Butler-Dokumentation](https://itch.io/docs/butler/pushing.html)

## Updates

`updates` sorgt dafür, dass sich die App so aktuell hält, wie es ihre Plattform und der Bezugskanal der Nutzenden vorsehen:

```yaml
updates:
  key: zE0hx2FZwwHy6X7LbEP3GjTTE3KWfCAp5gsaBysC+vg=  # the public key nsgo keygen prints
  # channel: store                    # direct, store, steam or none; direct by default with a key, none without
  # feed: https://example.com/hello/  # where the appcasts and installers are; by default the latest GitHub release
  # automatic: false                  # check only when asked; true by default
  # automatic_downloads: true          # Sparkle downloads automatically; false by default, macOS only
```

`nsgo keygen` gibt ein Schlüsselpaar aus und schreibt keine Datei: Tragen Sie den öffentlichen Schlüssel in `updates.key` ein und committen Sie ihn, und halten Sie den geheimen Schlüssel aus dem Repository heraus – in einem Passwortmanager und in `NSGO_UPDATES_KEY`, wo Releases gebaut werden, etwa als GitHub-Actions-Secret. Jeder Direct-Build signiert die Installer mit dem geheimen Schlüssel und schreibt daneben `appcast-<target>.xml`, einen Feed im Format von [Sparkle](https://sparkle-project.org), den die App unter `feed` abruft: Laden Sie die Appcasts zusammen mit den Installern hoch. Ohne den geheimen Schlüssel schreibt `build` keinen macOS- oder Windows-Appcast, da diese Systeme nur signierte Installer installieren; Linux- und Android-Apps, die einen unsignierten Appcast lesen, öffnen stattdessen die Release-Seite, anstatt das Paket zu installieren.

`-release-notes notes.md` fügt eine Markdown-Datei mit den Versionshinweisen – etwa den entsprechenden Changelog-Abschnitt – in jeden Appcast ein, gerendert als HTML ohne eigenes Roh-HTML aus der Datei. Sparkle und WinSparkle zeigen sie in ihren Update-Dialogen an, und die Abfrage unter Linux und Android stellt sie als Text unter der Version mit bis zu 1.500 Zeichen dar; Apps lesen sie über `core.UpdateCheck.Notes`.

| Zielplattform | Wie ein Direct-Build aktualisiert |
| --- | --- |
| macOS | Sparkle, das `build` in die App einfügt, prüft täglich, bietet die neue Version an, lädt das Disk-Image herunter, prüft dessen Signatur, installiert sie und startet die App neu; das Standard-App-Menü enthält „Check for Updates…“, das eine App mit eigenem Menü über `updater.Check` ergänzt |
| Windows | WinSparkle neben der ausführbaren Datei tut dasselbe in der Sprache, die die App beim Start anzeigt (sofern WinSparkle sie hat), und führt den Installer still aus, der anschließend die neue Version startet |
| Linux, Android | Die App liest ihren Appcast, und sobald die nutzende Person „Aktualisieren“ wählt, streamt und verifiziert sie das signierte Paket direkt auf das Laufwerk mit Fortschrittsanzeige (`updater.Status` und die Fortschrittsanzeige der Plattform-Taskleiste) und installiert es anschließend über `pkexec dpkg`, `pkexec pacman` (`.pkg.tar.gz`/`.pkg.tar.zst`), atomare AppImage-Ersetzung (mit Neustart unter Linux) oder den Paket-Installer von Android; Installationen außerhalb dieser Formate oder ein unsignierter Appcast bieten stattdessen die Release-Seite an |
| iOS | Die App fragt den App Store ab, der sie aktualisiert, und bietet ihre Seite dort an |

Bei einer veröffentlichten eigenständigen Web-App vergleicht `updater.Check(cx)` das laufende Release mit einer frischen `nsgo.json` und bietet nach Bestätigung ein Neuladen an. Es erkennt auch geänderte Build-Dateien unter derselben Versionsbezeichnung. Es benötigt keinen Appcast-Schlüssel; explizite `none`/`steam`-Kanäle deaktivieren Prüfungen weiterhin, und eingebettete Hosts verwalten ihre Neuladevorgänge selbst. Mini-Programme nutzen den Update-Manager ihrer Plattform und starten erst neu, sobald das heruntergeladene Update bereitsteht. Diese Zielplattformen prüfen, wenn die App den Updater aufruft; der Zeitpunkt von Download und Prüfung bei Mini-Programmen liegt beim Host. Sie bieten keine Setter für automatische Prüfeinstellungen. Ein eigener `cx.CheckForUpdates`-Handler verwendet `UpdateCheck.Restart` für ein vorbereitetes Update, dessen Versionsbezeichnung fehlen kann.

### Kanäle

Der Kanal bestimmt, wer die App aktualisiert. `-channel` ersetzt `updates.channel` für einen einzelnen Build, sodass dasselbe Projekt für jeden Kanal baut:

| Kanal | Für | Die Pakete |
| --- | --- | --- |
| `direct` | GitHub, eine Website, Homebrew-Casks und winget, die die eigenen Dateien des Releases installieren | Aktualisieren sich wie oben beschrieben selbst |
| `store` | Den App Store, Mac App Store, Microsoft Store, Google Play, Flathub und weitere Stores | Sind die [Store-Pakete](#store-packages) und enthalten keinen Selbst-Updater; iOS und macOS fragen ohne Schlüssel ihren App Store ab, und eine Google-Play-Installation kann Plays nativen Update-Ablauf nutzen |
| `steam` | Steam | Erzeugen [Depot-Verzeichnisse](#steam-depots), die Steam aktualisiert |
| `none` | Linux-Distributionen und alle, die die App neu paketieren | Prüfen nichts |

Stores lehnen Apps ab, die sich selbst aktualisieren – laden Sie daher stets einen `store`-Build hoch, niemals einen Direct-Build. Jedes Direct-Release mit macOS- oder Windows-Installern erzeugt zudem einen Homebrew-Cask (`dist/releases/homebrew/<name>.rb`, der `auto_updates true`, Prüfsummen und Download-URLs für die gebauten Disk-Images sowie einen `binary`-Link für jeden der [Befehle](#commands) der App deklariert) und ein mehrdateiiges winget-Manifest (`dist/releases/winget/<id>.yaml`, `<id>.installer.yaml` und `<id>.locale.en-US.yaml` für die gebauten NSIS-Installer). Überall dort, wo das System angibt, wer die App installiert hat, prüft die App auch das: Eine Android-App aus einem anderen Store, ein Flatpak oder Snap sowie ein TestFlight-Build überlassen Updates jenem Store. Ein manueller Aufruf von `updater.Check` in einer Google-Play-Installation bittet Play In-App Updates, seinen unmittelbaren Ablauf anzubieten, sofern zulässig; Play übernimmt Einwilligung, Download, Installation und Neustart. Entwicklungs-Builds betreten diesen Ablauf nie. Eigenständige Android-Pakete bündeln das SDK; [einbettende Hosts](embedding.md#android) stellen es über Gradle bereit.

### Aus der App heraus prüfen

`updater.Check(cx)` prüft wie ein Menüpunkt *Nach Updates suchen…* und zeigt an, was gefunden wird; `updater.CheckInBackground(cx)` zeigt nur eine neuere Version an, und `updater.CheckDaily(cx)` tut dies höchstens einmal pro Tag. `updater.Status(cx)` liest unter Linux und Android den Live-Fortschritt von Download und Installation. Solange `automatic` aktiv ist, prüft die App auf diese Weise unter Linux, bei Direct-Android-Builds und bei Apple-App-Store-Builds beim Start sowie immer dann, wenn Nutzende zu ihr zurückkehren; eine fehlgeschlagene Prüfung – etwa offline – wird eine Stunde später erneut versucht. Entwicklungs-Builds aktualisieren nie. In Steam-Builds tun diese drei Funktionen nichts.

`updater.CapabilityOf(cx)` meldet ohne Netzwerkanfrage, wie sich diese Kopie aktualisiert: `ManagedSelf`, `ManagedPage`, `ManagedDevelopment`, `ManagedChannel`, `ManagedStore` (mit seinem `Store`-Namen) oder `ManagedUnavailable`. Ein Entwicklungs-Build, ein deaktivierter Kanal oder eine Store-Installation liefert beim Prüfen eine ursachenspezifische Erklärung; `core.UpdateCheck.Err` ist Fehlern vorbehalten wie einem fehlerhaften Feed oder einem gebündelten Updater, der nicht laden kann.

Lesen Sie `updater.Status(cx)` innerhalb eines Bindings, um `Capability`, `AutomaticChecks`, `AutomaticDownloads`, `Available`, `Notes` und `LastCheck` sowie unter Linux und Android `Phase`, `Loaded` und `Total` eines Updates (`Fraction()` für einen Fortschrittsbalken) zu verfolgen. `updater.SetAutomaticChecks(cx, on)` speichert die Wahl der nutzenden Person dauerhaft und ändert den täglichen Zeitplan zur Laufzeit. Sparkle und WinSparkle verwalten ihre Einstellungen selbst; Linux, Android und iOS bewahren die Wahl in einer privaten Speicherpartition auf. `updater.SetAutomaticDownloads(cx, on)` wird nur unter macOS unterstützt und nutzt standardmäßig `updates.automatic_downloads`; andere Plattformen melden `false` und ignorieren den Setter. Nicht unterstützte und extern verwaltete Builds melden beide Einstellungen als `false`. WinSparkle meldet Metadaten eines gefundenen Updates, indem es den Appcast nach seinem Found-Callback liest; es steuert weiterhin die Installation. Linux ermittelt beim Start die Zugehörigkeit zu Debian- oder Arch-Paketen sowie die AppImage-Identität; solche Kopien melden `ManagedSelf`, während andere unpaketierte Kopien vor jeder Prüfung `ManagedPage` melden.

## Bei Anmeldung starten

`desktop.SetLaunchAtLogin(cx, on)` registriert die App beim Anmeldemechanismus der Plattform, und `desktop.LaunchAtLogin(cx, done)` liest den Status aus. Ein vom aktuellen Werkzeug gebautes MSIX enthält automatisch eine deaktivierte Startup-Aufgabe; ihre Aktivierung respektiert den Schalter für Autostart-Apps sowie Administratorrichtlinien. Herkömmliche Windows- und Linux-Installationen nutzen ihren Run-Wert bzw. XDG-Autostart-Eintrag. macOS verwendet den nativen Anmeldedienst der gebündelten App.

Flatpak fragt das Background-Portal an, was um Zustimmung bitten kann, ohne Zugriff auf Konfigurationsdateien des Hosts zu gewähren. Ein anschließendes Auslesen wartet auf die native Antwort. Das Portal besitzt keine schreibgeschützte Startup-Abfrage: Flatpak-Lesevorgänge liefern nur die zuletzt bestätigte Antwort in diesem Lauf (`false` vor einer ersten) und können weder eine Änderung außerhalb der App erkennen noch die Einstellung beim nächsten Start wiederherstellen.

Ein extern paketiertes Snap deklariert den Autostart-Dateinamen für seine App:

```yaml
apps:
  app:
    command: bin/app
    autostart: app.desktop
```

NSGo schreibt diesen Dateinamen unter `$SNAP_USER_DATA/.config/autostart`, den `snapd` über den Wrapper der App startet. `desktop.LaunchedAtLogin(cx)` erkennt einen Start bei Anmeldung, noch bevor die App baut – einschließlich einer MSIX-StartupTask-Aktivierung.

## Dateitypen

`file_types` registriert die Dokumente, die eine App öffnet: über Finder, Dock und „Öffnen mit“ unter macOS, über „Öffnen mit“ des Explorers durch den Windows-Installer oder das MSIX unter Windows, über den Desktop-Eintrag der Debian-, Arch-, AppImage- und Flatpak-Pakete unter Linux sowie über „Öffnen in“ unter iOS:

```yaml
file_types:
  - name: Flow File
    extensions: [flow]
    # id: dev.example.hello.flow   # <id>.<first extension> by default; another's ID imports its type
    # icon: assets/Flow.icns       # macOS document icon; or a square PNG
    # role: viewer                 # editor by default
    # conforms: [public.json]      # public.data by default
    # mime: [application/json]     # existing MIME types; otherwise inferred on Linux
```

Der Windows-Installer trägt die App in „Öffnen mit“ ein, ohne die Standardanwendung der nutzenden Person zu überschreiben, und entfernt seine Registrierungen bei Aktualisierung und Deinstallation. Linux bestimmt die MIME-Typen gängiger Dateiendungen anhand einer festen Tabelle und vergibt für eine unbekannte Endung des eigenen Typs der App `application/x-<type id>` samt einer Shared-MIME-Info-Definition. `mime` benennt stattdessen vorhandene Typen; ein importierter Typ, dessen Endung in der Tabelle fehlt, benötigt dies. Das Ausführen eines AppImage registriert nichts: Ein Desktop-Integrationswerkzeug installiert dessen Eintrag und Definitionen.

Die App empfängt die Dateien mit `desktop.OnOpenFiles`, einschließlich eines späteren Windows- oder Linux-Starts während des Betriebs sowie iOS-Szenen-Dokumentöffnungen. Dateien, mit denen die App gestartet wurde, warten auf den ersten Handler:

```go
desktop.OnOpenFiles(cx, func(paths []string) { open(paths[0]) })
```

Unter iOS werden reguläre Dokumente mit ihren Originalnamen in app-eigene temporäre Dateien importiert. File-Provider-Lesevorgänge werden außerhalb des UI-Threads koordiniert, und ihre Security-Scopes enden, bevor der Handler läuft. Verschieben Sie einen Import nach `cx.DataDir()`, um ihn zu behalten; Bearbeitungen betreffen die Kopie. iOS nutzt das Dokument-Icon der App; `icon` gilt unter macOS. App Clips und tvOS-Pakete registrieren keine Dokumenttypen. Dateiöffnungen bleiben von `OnOpenLinks` getrennt.

`desktop.AddRecentFile(cx, path)` merkt ein von der nutzenden Person geöffnetes Dokument vor: im Dock-Menü sowie unter Ablage › Benutzte Dokumente unter macOS, in der Kategorie „Zuletzt verwendet“ der Jump List unter Windows und in den zuletzt verwendeten Dateien des Desktops unter Linux. Eine Auswahl erreicht denselben Handler. `desktop.ClearRecentFiles` löscht die Einträge der App und lässt die Historie anderer Apps zu gemeinsam genutzten Dateien unter Linux unangetastet; Linux respektiert zudem GTKs Verlaufseinstellung.

`desktop.SetDockMenu(cx, items)` setzt Aktionen auf das Icon der App: macOS' Dock-Menü, Windows' Jump-List-Aufgaben und die Quicklist im Ubuntu-Dock, Plasma und Dash to Dock unter Linux, sofern Quicklists unterstützt werden. Linux benötigt einen installierten Desktop-Eintrag passend zur `id` der App und behält Untermenüs, Trennlinien sowie abgehakte und deaktivierte Einträge bei. Das Menü gehört dem Scope, der es gesetzt hat, und der letzte Aufruf über die Fenster der App hinweg ersetzt es.

## Links

`url_schemes` gibt der App eigene Links wie `myapp://settings`, die sie aus einem Browser, einer anderen App, `open`, `start` oder `xdg-open` öffnen. Die macOS-, iOS- und Android-Apps registrieren sie ebenso wie der Windows-Installer, das MSIX-Paket und der Linux-Desktop-Eintrag, den Debian-, Arch-, AppImage- und Flatpak-Pakete mitführen. Eine Deinstallation entfernt sie.

```yaml
url_schemes: [myapp]
# app_links: [links.example.com]  # HTTPS hosts for the full app
```

Die App empfängt sie über `desktop.OnOpenLinks`, auch im laufenden Betrieb: Unter Windows und Linux übergibt ein zweiter Start seinen Link an die laufende App und beendet sich. Ein Link, der die App gestartet hat, wartet auf den ersten Handler. Jede Person kann einen solchen Link schreiben, und unter Windows sowie Linux empfängt der Handler jede URL, mit der die App gestartet wird – prüfen Sie daher das Schema und fragen Sie Nutzende, bevor ein Link etwas verändert:

```go
desktop.OnOpenLinks(cx, func(links []string) { show(links[0]) })
```

Unter iOS und Android empfängt `mobile.OnOpenLinks` Links beim Start und während des Betriebs. Öffnen Sie einen Link mit `xcrun simctl openurl booted myapp://settings` im iOS-Simulator oder `adb shell am start -a android.intent.action.VIEW -d myapp://settings` auf Android. `app.OpenLinks("myapp://settings")` liefert einen im Test zu.

Für universelle Links unter iOS deklarieren Sie explizite HTTPS-Hosts mit `app_links: [links.example.com]`. Jeder Host ist ein DNS-Name ohne Schema, Pfad, Port, IP-Adresse oder Wildcard; Namen werden in Kleinbuchstaben umgewandelt und Duplikate abgelehnt. Das Werkzeug schreibt `applinks:<host>` in das Associated-Domains-Entitlement der vollständigen App, auch bei von Xcode verwalteter Signierung; ein IPA-Profil muss dies autorisieren. App Clips behalten ihre separaten `appclips:`-Einträge aus `app_clip.domains`.

Jeder Host muss eine `apple-app-site-association`-Datei unter `https://<host>/.well-known/apple-app-site-association` ausliefern, die die vollständige App benennt; Paket-Metadaten allein können keinen universellen Link autorisieren. [Apples Anforderungen für Associations](https://developer.apple.com/documentation/xcode/supporting-associated-domains) beschreiben die Server-Einrichtung. Ad-hoc-Simulator-Signaturen lassen eingeschränkte Entwickler-Entitlements weg, sodass ein Delegate-Test keine Server-Zuordnung belegt.

Unter iOS verfolgt `mobile.OpenedURL(cx)` den jüngsten App-Clip-Aufruf, universellen Link oder benutzerdefinierten Schema-Link – einschließlich des Anfangswerts vor dem Einhängen der Wurzel. Die Zustellung eines benutzerdefinierten Schemas erreicht weiterhin `mobile.OnOpenLinks`; der Aufrufwert ist aktuell, wenn dieser Handler läuft. Dateiöffnungen bleiben in `OnOpenFiles` und ersetzen die Aufruf-URL nicht. Unterstützung für Android-/Web-Aufrufe wird separat in #1475 verfolgt.

## Eingehende Freigaben unter Android

Wählen Sie die MIME-Typen aus, die die App über das Android-Teilen-Menü entgegennimmt:

```yaml
android:
  share_types: [text/plain, 'image/*', application/pdf]
```

APK- und App-Bundle-Builds registrieren diese Typen sowohl für einzelne als auch für mehrere Elemente. Registrieren Sie [`mobile.OnShared`](services.md#opening-links-and-files), um jede Freigabe zu empfangen.

Für eine Web-App mit pfadbasierten Adressen setzen Sie `web.base` auf ihr Bereitstellungsverzeichnis (`/` oder `/my-app/`) und konfigurieren Ihren Produktionsserver so, dass er für Anwendungsrouten `index.html` ausliefert. Der Entwicklungsserver bietet diesen Fallback für HTML-Navigationsanfragen bereits an. Assets bleiben relativ zu dieser festen HTML-Basis, wenn ein Deep Link neu geladen wird. Siehe [Anwendungsadressen](addresses.md).

## Eingehende Freigaben unter iOS

Deklarieren Sie die Uniform Type Identifiers, die die Share-Erweiterung akzeptiert, wobei URL- und Texttypen vor allgemeineren Dateitypen stehen:

```yaml
apple:
  share_types: [public.url, public.plain-text, public.image, com.adobe.pdf]
  # app_group: group.com.example.app  # defaults to group.<id>
sign:
  # share_profile: assets/Share.mobileprovision
```

`build` und `dev` paketieren eine Go-Share-Erweiterung `<id>.share` in die App. Xcode signiert sie und registriert ihre App Group für Geräteläufe. Für eine IPA aktivieren Sie dieselbe App Group für `<id>` und `<id>.share`; `sign.profile` und `sign.share_profile` müssen sie gewähren. App Clips, tvOS und watchOS haben keine Share-Erweiterung.

Die Erweiterung kopiert bis zu 128 Elemente und 512 MiB an Dateien in die App Group und schließt dann das Teilen-Menü. `mobile.OnShared` empfängt diese Batches, während die App aktiv ist oder wenn Nutzende sie als Nächstes öffnen; sie überstehen das Beenden von Erweiterung und App vor der Zustellung. iOS bietet keine unterstützte API, mit der eine Share-Erweiterung ihre enthaltende App in den Vordergrund holen oder starten kann. Diese Übergabe öffnet die App daher nicht automatisch.

## Mini-Programme

```sh
go tool nsgo build -target miniprogram_wechat -run
go tool nsgo dev -target miniprogram_alipay
```

Jedes Ziel schreibt ein Projekt für das Entwicklerwerkzeug seiner Plattform – ebenfalls in `dist/releases/miniprogram/<platform>` –, das `-run` und `dev` öffnen; `dev` baut beim Speichern neu, und das Werkzeug lädt neu:

| Zielplattform | Plattform | Werkzeug |
| --- | --- | --- |
| `miniprogram_wechat` | WeChat | [WeChat DevTools](https://developers.weixin.qq.com/miniprogram/dev/devtools/download.html), mit eingeschaltetem Service-Port (Settings, Security) |
| `miniprogram_qq` | QQ | [QQs Entwicklerwerkzeug](https://q.qq.com/wiki/tools/devtool/) unter macOS |
| `miniprogram_alipay` | Alipay | [Alipays Mini Program Studio](https://opendocs.alipay.com/mini/ide/download) unter macOS |
| `miniprogram_douyin` | Douyin und weitere Apps von ByteDance | [Douyins Entwicklerwerkzeug](https://developer.open-douyin.com/docs/resource/zh-CN/mini-app/develop/developer-instrument/download/developer-instrument-update-and-download) unter macOS |

`miniprogram.appid` gibt die AppID des Mini-Programms auf jeder Plattform an (`wechat`, `qq`, `alipay`, `douyin`); ohne eine solche öffnet WeChat DevTools die Vorschau mit seiner Tourist-AppID, und die anderen Werkzeuge fragen danach. Bei WeChat schließen und öffnen `-run` sowie jeder `dev`-Rebuild das Projekt automatisch über die CLI, sodass DevTools die frischen Dateien kompiliert. Dies wurde mit DevTools 2.01.2510290 geprüft. Die Navigationsleiste der Plattform zeigt den Titel der App; mit `custom_navigation` füllt die App die Seite, und ihre Safe Area hält sie vom Menü-Button frei. Die App liest das Betriebssystem des Telefons mit `Env.Simulated` auf `true`, wie im Web mit `--nsgo-os`: Die Seiten eines Stacks zeigen ihren eigenen `Header`, da kein System-Header ihn ersetzt. QQ, Alipay und Douyin wurden in ihren Werkzeugen noch nicht erprobt, und Alipay unterstützt bislang weder Zurück durch die App noch Keyframes oder die Tastaturhöhe.

Mini-Programme sind experimentell. Ein Mini-Programm führt ausschließlich Go-Apps aus, kein TSX, und die Plattformen begrenzen das komprimierte Modul der App auf 2 MB: Eine App mit wenigen Bildschirmen auf `base` passt hinein – wie `examples/gallery-miniprogram` –, eine mit Steuerelementen aus `ui` noch nicht, und `build` meldet, wenn eine App das Limit überschreitet. Shader zeichnen dort nichts, und Web-Views, Videos, native Komponenten sowie eigene Schriftdateien der App werden noch nicht dargestellt.

## HarmonyOS

```sh
go tool nsgo build -target harmonyos -run
```

`harmonyos` schreibt `<name>-<version>-harmonyos.hap` nach `dist/releases`. Die Engine läuft als WebAssembly in ArkWeb, und ein ArkTS-Host zeichnet das Einhängte mit ArkUI-Views, sodass Text, Felder, Scrollen und Dialoge vom System selbst stammen. Mit `hdc` in `PATH` installiert `-run` die HAP auf dem verbundenen Gerät oder Emulator und startet sie, und `nsgo test -target harmonyos` führt die Tests einer App aus einer unsignierten HAP auf einem Emulator unter einer separaten `.nsgotest`-Bundle-Identität aus und entfernt diese Test-HAP nach dem Lauf, sodass die installierte App verfügbar bleibt.

Um den ArkTS-Host aus den Vorlagen zu kompilieren und die `id` der App als Bundle-Namen zu verwenden, legen Sie `hvigorw` in `PATH` und setzen Sie `OHOS_BASE_SDK_HOME` auf ein OpenHarmony SDK mit API 26 (`26.0.0`), der Compile- und Target-SDK-Version der Vorlagen. `build.harmonyos_bundles` wählt `auto` (Standard), `exact` (erfordert das SDK) oder `padded` (füllt die ID auf die feste Länge des eingecheckten Hosts auf und benötigt kein SDK). Zielplattform-Tests nutzen dieselbe Einstellung und geben aus, welchen Host sie paketiert haben. `harmonyos.permissions` listet die `ohos.permission`-Namen auf, die die App anfordert, wie `MICROPHONE` und `CAMERA`, und `widget.title` bestimmt, was ihre Startbildschirm-Karte anzeigt.

HarmonyOS befindet sich in der Vorschau. Emulatoren können unsignierte HAPs installieren. [Die Architekturhinweise](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/harmonyos.md) führen die ArkUI-Funktionen auf, die die Vorschau darstellt.

### Signierte HAPs

Setzen Sie `harmonyos.signing`, um eine HAP mit Ihren AppGallery-Connect-Materialien zu signieren. Eine direkte Installation auf einem Huawei-Telefon erfordert ein Debug-Zertifikat und ein Profil für dieses Gerät. Verwenden Sie für die Verteilung die Release-Materialien. [Huawei-Signierungsanleitung](https://developer.huawei.com/consumer/en/doc/harmonyos-guides-V2/command-line-building-app-hap-hvigor-0000001553061909-V2/)

```yaml
harmonyos:
  signing:
    keystore: signing/app.p12
    key_alias: app
    certificate: signing/app.cer
    profile: signing/app.p7b
```

Pfade sind relativ zu `nsgo.yaml`. Setzen Sie `NSGO_HARMONY_KEYSTORE_PASSWORD` und `NSGO_HARMONY_KEY_PASSWORD` in der Umgebung. Das Signieren nutzt `hap-sign-tool.jar` aus dem SDK sowie Java aus `JAVA_HOME` oder `PATH`. Der Build behält die exakte `id` der App bei, um dem Profil zu entsprechen, weshalb das Signieren auch im `auto`-Modus das SDK und `hvigorw` benötigt. `go tool nsgo doctor`, ausgeführt im App-Verzeichnis, prüft die Signaturkonfiguration und den Profil-Bundle-Namen ohne Passwörter. `build -run` übergibt `hdc` die signierte HAP.

## Signieren

```yaml
sign:
  identity: "Apple Development: you@example.com (ABCDE12345)"  # signs the macOS app, else ad hoc
  profile: assets/AppStore.mobileprovision  # signs the IPA
  keystore: assets/release.p12              # signs APKs, with key_alias
  certificate: assets/codesign.pfx          # signs the Windows app and its installer
  # widgets_profile: assets/Widgets.mobileprovision  # the iOS extension's profile; use a macOS profile for a Mac App Store build
  # share_profile: assets/Share.mobileprovision      # signs the IPA's share extension
  # clip_profile: assets/Clip.mobileprovision        # signs the IPA's App Clip
```

Die Passwörter des Keystores stammen aus `NSGO_KEYSTORE_PASSWORD` und `NSGO_KEY_PASSWORD`. `-team` signiert das Paket für das iPhone. `-profile`, `-widgets-profile`, `-share-profile`, `-clip-profile` und `-macos-profile` ersetzen die Provisioning-Profile der App, der Widget-Erweiterung, der Share-Erweiterung, des App Clips und des Mac App Store über die Kommandozeile. `-installer` ersetzt die Mac-Installer-Distribution-Identität.

### Windows-Pakete signieren

Authenticode identifiziert den Herausgeber eines Windows-Downloads; [SmartScreen prüft dessen Reputation separat](https://learn.microsoft.com/windows/apps/package-and-deploy/smartscreen-reputation). `sign.certificate`, eine `.pfx`-Datei, signiert die ausführbare Datei, den Befehlsbegleiter, die Programme und Bibliotheken aus den Ressourcen der App, den Installer samt Uninstaller sowie das für den Store-Kanal gebaute MSIX; sein Passwort stammt aus `NSGO_CERTIFICATE_PASSWORD`. Die Signaturen werden in Go genau so erstellt, wie `signtool` sie erstellt, sodass ein Mac sie ohne Windows SDK signieren kann:

```yaml
sign:
  certificate: assets/codesign.pfx
  timestamp_url: http://timestamp.digicert.com  # the RFC 3161 server that timestamps them
```

`sign.timestamp_url` verwendet standardmäßig den Server von DigiCert, und `none` lässt die Signaturen ohne Zeitstempel, wodurch sie bei Ablauf des Zertifikats ungültig werden. Ein Zertifikat auf einem Hardware-Token oder in einem Cloud-Dienst signiert stattdessen über einen eigenen Befehl, in dem `%1` die zu signierende Datei darstellt und den der Build für jede der obigen Dateien ausführt:

```yaml
sign:
  sign_command: "signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 /a %1"
```

`build` bricht ab, wenn der Befehl erfolgreich zurückkehrt, aber die ausführbare Datei der App, der Befehlsbegleiter, eine PE-Ressource oder die Setup-Datei ohne eingebettete Authenticode-Signatur verbleibt.

Der Uninstaller innerhalb des Installers wird vor dem Einbetten signiert, mit demselben `sign.certificate` bzw. `sign.sign_command` wie die App und das Setup. `-certificate`, `-sign-command` und `-timestamp-url` setzen diese Werte über die Kommandozeile.

### Notarisieren

Auf einem Mac heruntergeladene Apps öffnen sich ohne Gatekeeper-Warnung, sobald Apple sie notarisiert hat. Signieren Sie mit einer Developer ID und benennen Sie die Zugangsdaten, die `notarytool` in Ihrem Schlüsselbund ablegt:

```sh
xcrun notarytool store-credentials hello --apple-id you@example.com --team-id ABCDE12345
```

```yaml
sign:
  identity: "Developer ID Application: Example Inc (ABCDE12345)"
  notarize: hello   # the keychain profile above; or -notarize hello
```

`build` signiert dann mit Hardened Runtime, reicht das Disk-Image ein, wartet einige Minuten auf Apples Bescheid und heftet das Ticket an das Image und die App; eine Ablehnung gibt Apples Protokoll aus. In der CI treten `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID` und `NSGO_NOTARY_ISSUER` – `.p8`-Datei, ID und Aussteller eines App-Store-Connect-API-Schlüssels – an die Stelle des Profils. Die Hardened Runtime verweigert Mikrofon, Kamera, Kalender und Standort, sofern die App sie nicht beansprucht – was `build` veranlasst, wenn `apple.info` `NSMicrophoneUsageDescription`, `NSCameraUsageDescription`, eine Kalender-/Erinnerungs-Nutzungsbeschreibung oder eine Standort-Nutzungsbeschreibung enthält.

## Widgets und Live-Aktivitäten

`build` und `dev` kompilieren die Erweiterung, die die Widgets und Live-Aktivitäten der App zeichnet, ohne separaten Schritt mit Xcodes Swift-Compiler in die macOS- und iOS-Pakete. Unter iOS teilen sich App und Widgets eine App Group: `-run` auf einem iPhone lässt Xcode beide signieren und die Gruppe registrieren; für eine IPA aktivieren Sie sie für `<id>` und `<id>.widgets` und signieren die Erweiterung mit `sign.widgets_profile`. Eigenständige Android-APKs und -AABs enthalten App-Widget-Provider und Ressourcen aus `widgets`; die vorgebaute Java-Laufzeit zeichnet deren Zeitachsen, ohne dass App-Builds ein SDK oder einen Java-Compiler benötigen. Android 16 zeigt Live-Aktivitäten als Live Updates an, die keine Erweiterung erfordern. [Widgets](widgets.md) und [Live-Aktivitäten](live-activities.md) behandeln Konfiguration und API.

## Android App Links

`app_links` benennt explizite HTTPS-Hosts, deren Links die vollständige Android-App öffnen:

```yaml
app_links: [example.com, shop.example.com]
```

APK und App Bundle deklarieren für jeden Host einen browsable `ACTION_VIEW`-Filter mit `autoVerify`. Verwenden Sie DNS-Hosts ohne Schema, Pfad, Port, Wildcard oder IP-Adresse. Liefern Sie `https://<host>/.well-known/assetlinks.json` mit der Paket-ID der App und dem Fingerabdruck des Signaturzertifikats aus; Android verifiziert diese Zuordnung, bevor Links an die App geleitet werden. Das Werkzeug veröffentlicht keine Zuordnungsdateien; [Androids Verifizierungsanleitung](https://developer.android.com/training/app-links/verify-applinks) beschreibt deren Einrichtung. Ein eingebetteter Android-Host deklariert seine eigenen Intent-Filter und leitet akzeptierte Links an seine `NsgoView` weiter.

`mobile.OpenedURL(cx)` liest den akzeptierten Aufruf vor dem ersten Wurzel-Render und ändert sich, sobald ein weiterer Link die laufende App öffnet. `mobile.OnOpenLinks` empfängt weiterhin jeden akzeptierten Link. Eine ganzseitige Web-App liest ihre aktuelle Seitenadresse über `mobile.OpenedURL`, einschließlich Browserverlauf und Hash-Änderungen; eine in eine andere Seite eingebettete App liest keine Adresse der umgebenden Seite. Verwenden Sie [`cx.AppURL`](addresses.md), wenn die App ihre Adresse auch selbst schreibt.

## App Clips

Ein App Clip ist ein kleiner Teil einer iOS-App, der über einen Link, einen Code oder ein NFC-Tag startet, ohne die App zu installieren. `app_clip` baut einen solchen in das iOS-Paket – aus einem eigenen `main`-Paket, sodass er nur das linkt, was er anzeigt:

```yaml
app_clip:
  main: ./clip              # the clip's main package, the app's own by default
  domains: [example.com]    # whose links open it
  # name: Hello Clip        # "<name> Clip" by default
  # id: dev.example.hello.Clip  # <id>.Clip by default
  # minimum: "17.0"         # 16.0 by default; 17.0 allows 100 MB when only links open it
  # notifications: true     # ask for notifications for a day
  # location: true          # let the clip confirm where it was opened
```

Der Clip liest den Link, der ihn geöffnet hat, über `mobile.OpenedURL`, was sich ändert, sobald ihn ein anderer Link erneut öffnet, und `mobile.IsAppClip` gibt an, ob der Code als Clip läuft. Das System lädt einen Clip bis 15 MB herunter – oder ab iOS 17 bis 100 MB, wenn ausschließlich Links ihn öffnen –, und `build` lehnt einen größeren ab; ein Clip, der `ui` linkt, benötigt rund 11 MB. `dev -target ios_simulator -clip https://example.com/order?table=12` startet den Clip allein im Simulator, geöffnet über diesen Link; `-target ios` startet ihn auf einem iPhone und gibt den QR-Code des Links aus: Tragen Sie den Link unter Einstellungen > Entwickler > Lokale Erlebnisse ein und scannen Sie ihn dann mit der Kamera. Verwenden Sie `-release`, falls Entwicklungssymbole den Clip über das QR-Größenlimit drücken. Apple signiert App Clips ausschließlich für Teams des Apple Developer Program, nicht für persönliche Teams.

`dev -target ios` und `build -run -target ios` belassen den Clip auch innerhalb der vollständigen App: Xcode signiert ihn als separates Ziel und bettet ihn vor dem Signieren der App ein. Automatisches Signieren erfordert ein authentifiziertes Konto eines bezahlten Teams in Xcode mit Provisioning für App und Clip. Für den App Store aktivieren Sie App Clips für `<id>`, signieren den Clip mit `sign.clip_profile` und liefern die `apple-app-site-association`-Datei der Domains aus. `examples/appclip` im Repository ist ein Café, dessen Clip Bestellungen am Tisch aufnimmt.

Um eine Bestellung oder andere lokale Daten an die vollständige App zu übergeben, setzen Sie neben `app_clip` eine explizite Gruppe:

```yaml
apple:
  app_group: group.dev.example.hello
```

Clip und vollständige App teilen sich dann `cx.DataDir()` und `cx.Storage()` im Container der Gruppe. Dateien im Datenverzeichnis und Schlüssel-Wert-Einträge, die der Clip schreibt, stehen beim Start der vollständigen App bereit; `cx.LogsDir()` folgt diesem Verzeichnis, während jedes Bundle seinen eigenen Cache behält. Die Xcode-Signierung beansprucht die Gruppe für beide Ziele; bei Verwendung von Profilen aktivieren Sie dieselbe Gruppe für beide App-IDs und gewähren sie in `sign.profile` und `sign.clip_profile`. Eine fehlende Freigabe verhindert die Signierung, und eine nicht zugängliche Gruppe stoppt den Start. Ein explizites Datenverzeichnis eines Hosts hat weiterhin Vorrang.

Ohne `apple.app_group` behält jedes Bundle seinen eigenen Sandbox-Speicher. Das Aktivieren der Einstellung migriert keine zuvor in diesen Sandboxes abgelegten Daten, und reine Widget-Apps behalten ihren bisherigen Speicherort. Bewahren Sie gemeinsame Geheimnisse im Schlüsselbund statt im einfachen Speicher auf. Das Café-Beispiel speichert die Bestellung seines Clips dauerhaft und zeigt sie in der vollständigen App an.

Rufen Sie aus einer expliziten Benutzeraktion im Clip heraus `mobile.ShowAppClipOverlay(cx, mobile.AppClipOverlayBottom)` auf, um StoreKit zu bitten, die zugehörige vollständige App zu empfehlen. `AppClipOverlayBottomRaised` lässt Platz für eine untere Leiste. Die zurückgegebene Funktion blendet diese Anfrage wieder aus; auch das Aufräumen des Scopes blendet sie aus, und eine ältere Anfrage kann keine neuere schließen. Aufrufe außerhalb eines nativen iOS-App-Clips tun nichts. Der App Store entscheidet, ob er die Empfehlung laden und anzeigen kann; der Aufruf meldet daher weder ein angezeigtes Angebot noch eine Installation. Halten Sie die Aufgabe des Clips auch ohne Installation der vollständigen App nutzbar. [Apples Overlay-Leitfaden](https://developer.apple.com/documentation/appclip/recommending-your-app-to-app-clip-users)

## In eine andere App einbetten

`-embed` paketiert die App für eine eigene iOS- oder Android-App: ein XCFramework (oder mit `-dynamic` dynamische Frameworks) bzw. ein AAR, dessen `NSGoView` oder `NsgoView` die App anzeigt. Go-Desktop-Programme zeigen Apps mit `nsgo.NewView` an, und Webseiten mit `nsgo.js` der Website, deren Host der der Seite ist oder der in `web.host` in `nsgo.yaml` genannte. `examples/embed-*` im Repository zeigt jede Variante; [Einbetten](embedding.md) behandelt die Host-Seite.
