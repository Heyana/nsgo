---
title: Native Bibliotheken
description: Vorgefertigte Shared Libraries mit einem Go-Package über die üblichen Paketierungs-, Lizenz- und Signierschritte von NSGo ausliefern.
group: Toolchain
order: 23
---

Ein Go-Package kann die Shared Libraries, die es über purego lädt, ausliefern, indem es `nsgo-natives.yaml` in sein Package-Verzeichnis legt. Wenn eine App das Package importiert, wählt NSGo dessen Deklarationen für die Zielplattform aus, prüft die Dateien und bindet sie vor dem Signieren und dem Erstellen der Installer ein. Die App benötigt keine zusätzliche Konfiguration und keinen eigenen Paketierungsbefehl.

## Bibliotheken deklarieren

```yaml
version: 1
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
```

Die unterstützten Schlüssel sind `macos_arm64`, `macos_x64`, `windows_arm64`, `windows_x64`, `linux_arm64` und `linux_x64`. Deklarieren Sie jede Zielplattform, auf der das Go-Package ausgewählt wird; `[]` gibt ausdrücklich an, dass diese Zielplattform keine gebündelte Bibliothek benötigt. Alternativ können Go-Build-Tags das Package von Zielplattformen fernhalten, die es nicht unterstützt. `macos_universal` verwendet die beiden macOS-Einträge.

Jede Bibliothek hat:

- `name`: ihren installierten Dateinamen, endend auf `.dylib` unter macOS, `.dll` unter Windows oder `.so` bzw. ein versioniertes `.so.*` unter Linux.
- Genau eines von `path`, einer Datei relativ zu diesem Package-Verzeichnis, oder `url`, einer HTTP(S)-URL zur Bibliothek selbst. Veröffentlichen Sie entpackte Bibliotheken statt Archiv-URLs. URLs enthalten keine eingebetteten Benutzernamen und Passwörter.
- `sha256`: die SHA-256-Prüfsumme der Originaldatei, für beide Quellen erforderlich. Unter macOS gibt sie beispielsweise `shasum -a 256 lib/macos-arm64/libexample.dylib` aus.
- `licenses`: nicht leere Pfade zu den originalen Lizenz- und Hinweisdateien, relativ zu diesem Package-Verzeichnis. Diese Texte werden in `THIRD_PARTY_NOTICES.txt` ausgeliefert und gelangen in Release-Builds zu `core.ThirdPartyNotices()`. Die bestehende [Lizenzrichtlinie](building.md#license-notices) der App, einschließlich exakter Modulausnahmen, gilt unabhängig davon auch für native Lizenzen.

Lokale Pfade und symbolische Links bleiben innerhalb des deklarierenden Packages. NSGo weist unbekannte Felder, Prüfsummenabweichungen, Dateien für ein anderes Betriebssystem oder eine andere Architektur sowie widersprüchliche installierte Namen zurück. Zwei Packages können eine gleichnamige Datei mit derselben Prüfsumme gemeinsam nutzen. Eine Datei ist auf 1 GiB begrenzt.

## Paketieren und laden

| Zielplattform | Bibliotheksverzeichnis |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows und Linux | Neben der ausführbaren Datei |

Das Package lädt seine Bibliothek aus diesem Verzeichnis und verwendet dabei den Speicherort der ausführbaren Datei statt des Arbeitsverzeichnisses. Bauen Sie Abhängigkeiten so vor, dass ihre Partnerbibliotheken aus dem Verzeichnis geladen werden, in dem sie ausgeliefert werden. Deklarieren Sie jede benötigte Partnerdatei und jeden Dateinamen, den der Loader benötigt.

NSGo prüft zwischengespeicherte Downloads vor der Wiederverwendung erneut und koordiniert gleichzeitige Builds über eine Dateisperre. Der Inhalts-Cache liegt im Benutzer-Cache-Verzeichnis des Betriebssystems unter `nsgo/natives`. Lokale Dateien werden beim Kopieren geprüft.

Ein universelles macOS-Paket erfordert für beide Architekturen dieselben Bibliotheksnamen. NSGo fügt die Slices mit `lipo` zusammen; eine bereits universelle Bibliothek eines Herausgebers kann für beide Einträge dienen. Bibliotheken werden vor der App signiert und durchlaufen dann die üblichen Schritte der App für Disk-Image, Signierung und Notarisierung. Die Windows-Signierung schließt deklarierte DLLs auch mit `build.installers: false` ein. Linux-Installer enthalten dieselben Dateien und Hinweise wie das Verzeichnis der ausführbaren Datei.

Entwicklung und Tests auf nativen Desktop-Zielplattformen verwenden dieselbe Package-Auswahl und dieselben Kopierschritte. Deklarationen werden anhand der Zielplattform des Builds, der Tags und des generierten Go-Overlays ausgewählt, einschließlich lokaler Ersetzungen und vendorter Packages. Andere Zielplattformen paketieren derzeit keine Bibliotheken aus dieser Datei.

## Dateien beim Vendoring erhalten

Der Vendor-Vorgang von Go benötigt eingebettete Daten, um Dateien unterhalb eines Package-Verzeichnisses zu erhalten. Ein Package kann sein Manifest und seine Lizenzen zur Aufnahme markieren, ohne ihre Bytes zur Laufzeit zu verwenden:

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

Nehmen Sie bei Verwendung von `path` auch lokale Bibliothekspfade in diese Embed-Direktive auf, zum Beispiel `lib/*`. URL-Downloads benötigen im Go-Package nur das Manifest und die Lizenzdateien.
