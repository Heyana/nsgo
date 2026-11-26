---
title: Native Bibliotheken
description: Vorgefertigte Shared Libraries und Apple-Frameworks mit einem Go-Package über die üblichen Paketierungs-, Lizenz- und Signierschritte von NSGo ausliefern.
group: Toolchain
order: 23
---

Ein Go-Package kann die Shared Libraries, die es über purego lädt, ausliefern, indem es `nsgo-natives.yaml` in sein Package-Verzeichnis legt, ab Version 2 auch seine Frameworks für iOS und tvOS. Wenn eine App das Package importiert, wählt NSGo dessen Deklarationen für die Zielplattform aus, prüft die Dateien und bindet sie vor dem Signieren und dem Erstellen der Installer ein. Die App benötigt keine zusätzliche Konfiguration und keinen eigenen Paketierungsbefehl.

Desktop-App-Builds und Tests auf Zielplattformen deaktivieren cgo für App-Code. Nativer Code muss purego und eine in `nsgo-natives.yaml` deklarierte Bibliothek verwenden. NSGo weist App-Packages mit cgo-Dateien zurück, bevor der Build beginnt.

## Bibliotheken deklarieren

```yaml
version: 2
libraries:
  macos_arm64:
    - name: libexample.dylib
      path: lib/macos-arm64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  macos_x64:
    - name: libexample.dylib
      url: https://downloads.example.org/v1.0/macos-x64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  windows_arm64: []
  windows_x64: []
  linux_arm64: []
  linux_x64: []
  ios:
    - name: Example.framework
      url: https://downloads.example.org/v1.0/ios/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  ios_simulator:
    - name: Example.framework
      path: lib/ios-simulator/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  tvos: []
  tvos_simulator: []
```

Version 2 unterstützt die Schlüssel `macos_arm64`, `macos_x64`, `windows_arm64`, `windows_x64`, `linux_arm64`, `linux_x64`, `ios`, `ios_simulator`, `tvos` und `tvos_simulator`. Version 1 unterstützt nur die sechs Desktop-Schlüssel; eine Datei wird nur für die Schlüssel ihrer Version gelesen, sodass iOS- und tvOS-Builds aus einer Datei der Version 1 nichts paketieren. Deklarieren Sie jede Zielplattform, auf der das Go-Package ausgewählt wird; `[]` gibt ausdrücklich an, dass diese Zielplattform keine gebündelte Bibliothek benötigt. Alternativ können Go-Build-Tags das Package von Zielplattformen fernhalten, die es nicht unterstützt. `macos_universal` verwendet die beiden macOS-Einträge, eine Terminal-Zielplattform die ihres Desktops: `tui_macos_arm64` verwendet `macos_arm64`.

Jede Bibliothek hat:

- `name`: ihren installierten Dateinamen, endend auf `.dylib` unter macOS, `.dll` unter Windows oder `.so` bzw. ein versioniertes `.so.*` unter Linux. Unter iOS und tvOS ist es der Name eines Frameworks, `<Name>.framework`, wobei `Name` aus ASCII-Buchstaben, Ziffern, `.` und `-` besteht und mit einem Buchstaben oder einer Ziffer beginnt und endet.
- Genau eines von `path`, einer Datei relativ zu diesem Package-Verzeichnis, oder `url`, einer HTTP(S)-URL zur Bibliothek selbst. Version 2 akzeptiert nur https-URLs, und ein Download schlägt fehl, wenn eine Weiterleitung https verlässt. Veröffentlichen Sie entpackte Bibliotheken statt Archiv-URLs, bei einem Framework nur dessen Binärdatei. URLs enthalten keine eingebetteten Benutzernamen und Passwörter.
- `sha256`: die SHA-256-Prüfsumme der Originaldatei, für beide Quellen erforderlich. Unter macOS gibt sie beispielsweise `shasum -a 256 lib/macos-arm64/libexample.dylib` aus.
- `licenses`: nicht leere Pfade zu den originalen Lizenz- und Hinweisdateien, relativ zu diesem Package-Verzeichnis. Diese Texte werden in `THIRD_PARTY_NOTICES.txt` ausgeliefert und gelangen in Release-Builds zu `core.ThirdPartyNotices()`. Die bestehende [Lizenzrichtlinie](building.md#lizenzhinweise) der App, einschließlich exakter Modulausnahmen, gilt unabhängig davon auch für native Lizenzen.

Lokale Pfade und symbolische Links bleiben innerhalb des deklarierenden Packages. NSGo weist unbekannte Felder, Prüfsummenabweichungen, Dateien für ein anderes Betriebssystem oder eine andere Architektur sowie widersprüchliche installierte Namen zurück. Zwei Packages können eine gleichnamige Datei mit derselben Prüfsumme gemeinsam nutzen. Eine Datei ist auf 1 GiB begrenzt.

## Paketieren und laden

| Zielplattform | Bibliotheksverzeichnis |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows, Linux und Terminal-Zielplattformen | Neben der ausführbaren Datei |
| iOS und tvOS | `<app>.app/Frameworks/<Name>.framework/<Name>` |

Das Package lädt seine Bibliothek aus diesem Verzeichnis und verwendet dabei den Speicherort der ausführbaren Datei statt des Arbeitsverzeichnisses. Bauen Sie Abhängigkeiten so vor, dass ihre Partnerbibliotheken aus dem Verzeichnis geladen werden, in dem sie ausgeliefert werden. Deklarieren Sie jede benötigte Partnerdatei und jeden Dateinamen, den der Loader benötigt.

NSGo prüft zwischengespeicherte Downloads vor der Wiederverwendung erneut und koordiniert gleichzeitige Builds über eine Dateisperre. Der Inhalts-Cache liegt im Benutzer-Cache-Verzeichnis des Betriebssystems unter `nsgo/natives`. Lokale Dateien werden beim Kopieren geprüft.

Ein universelles macOS-Paket erfordert für beide Architekturen dieselben Bibliotheksnamen. NSGo fügt die Slices mit `lipo` zusammen; eine bereits universelle Bibliothek eines Herausgebers kann für beide Einträge dienen. Bibliotheken werden vor der App signiert und durchlaufen dann die üblichen Schritte der App für Disk-Image, Signierung und Notarisierung. Die Windows-Signierung schließt deklarierte DLLs auch mit `build.installers: false` ein. Linux-Installer enthalten dieselben Dateien und Hinweise wie das Verzeichnis der ausführbaren Datei. Auf einem Mac signieren Terminal-Zielplattformen macOS-Bibliotheken ad hoc; auf anderen Systemen liefern sie sie wie veröffentlicht aus, eine universelle vollständig, und lehnen eine arm64-Bibliothek ab, die nicht bereits signiert ist.

Entwicklung, `-run` und Tests auf Zielplattformen verwenden dieselbe Package-Auswahl und dieselben Kopierschritte, auf Desktops wie unter iOS und tvOS. Deklarationen werden anhand der Zielplattform des Builds, der Tags und des generierten Go-Overlays ausgewählt, einschließlich lokaler Ersetzungen und vendorter Packages. Android, HarmonyOS, watchOS, das Web und Mini-Programme paketieren nichts aus dieser Datei.

## Frameworks für iOS und tvOS

NSGo prüft die Binärdatei jedes Frameworks und bricht den Build unter Angabe des Grundes ab, wenn sie nicht:

- eine arm64-Mach-O-Dynamic-Library ist, die für die Plattform der Zielplattform gebaut wurde, Geräte oder Simulator, wie ihr `LC_BUILD_VERSION` angibt; eine universelle Datei liefert ihren arm64-Slice; arm64e wird abgelehnt;
- ein Mindestbetriebssystem verlangt, das nicht neuer ist als das der App, `apple.info.MinimumOSVersion` oder sonst iOS 15.0 und tvOS 17.0 ([Bauen und Ausliefern](building.md#nsgoyaml));
- den Installationsnamen `@rpath/<Name>.framework/<Name>` trägt;
- über `@rpath` nur Frameworks lädt, die die Packages der App für dieselbe Zielplattform deklarieren.

NSGo schreibt die `Info.plist` des Frameworks, deren `CFBundleIdentifier` die ID der App, ein Punkt und `Name` in Kleinbuchstaben ist, mit der Plattform und dem Mindestbetriebssystem der App. Simulator-Builds signieren das Framework ad hoc. Ein Geräte-Build überlässt es der Signierung der App: `sign.profile` signiert es vor der App mit deren Identität, wie es Xcode für `nsgo build -run` und `nsgo dev` auf einem Gerät tut. Ein Framework, das Partner-Frameworks lädt, braucht den Run-Path `@loader_path/..` oder `@executable_path/Frameworks`, um sie in `Frameworks` zu finden, da die ausführbare Datei der App keinen hat, und lädt über `@rpath` nichts anderes, sofern es keinen System-Run-Path wie `/usr/lib/swift` hat. Ein Framework, dessen Bundle-ID ein anderes Bundle der App bereits hat, ohne Rücksicht auf Groß- und Kleinschreibung, etwa `StoreKit.framework` neben NSGos `NSGoStoreKit.framework`, stoppt den Build.

`nsgo build -embed` bricht ab und nennt das Package, wenn die App ein Package einbindet, das iOS-Frameworks deklariert: Das XCFramework für eine Host-App kann sie noch nicht mitführen.

## Dateien beim Vendoring erhalten

Der Vendor-Vorgang von Go benötigt eingebettete Daten, um Dateien unterhalb eines Package-Verzeichnisses zu erhalten. Ein Package kann sein Manifest und seine Lizenzen zur Aufnahme markieren, ohne ihre Bytes zur Laufzeit zu verwenden:

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

Nehmen Sie bei Verwendung von `path` auch lokale Bibliothekspfade in diese Embed-Direktive auf, zum Beispiel `lib/*`. URL-Downloads benötigen im Go-Package nur das Manifest und die Lizenzdateien.
