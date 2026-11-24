---
title: Im Vergleich zu anderen Frameworks
description: Wo NSGo neben NSH, MyGo und anderen UI-Frameworks steht und worauf es verzichtet.
order: 2
---

Der Platz von NSGo unter den UI-Frameworks ergibt sich aus drei Entscheidungen: Apps werden in Go geschrieben (oder mit TSX darauf aufbauend), jede Plattform zeichnet mit ihren eigenen Views, und eine einzige Codebasis deckt Smartphones, Desktops und das Web ab. Andere Frameworks treffen andere Entscheidungen, und jede Entscheidung hat ihren Preis.

## Auf einen Blick

| Framework | Sprache | Zeichnet mit | Zielplattformen |
| --- | --- | --- | --- |
| NSGo | Go, TSX | Den Views der jeweiligen Plattform | iOS, App Clips, tvOS, watchOS, Android, macOS, Windows, Linux, Web; Mini-Programme (experimentell) |
| [NSH](https://nsh.dev) | Rust, TSX | Den Views der jeweiligen Plattform | iOS, Android, macOS, Windows, Linux, Web, GPUI-Apps, Terminals; HarmonyOS und WeChat-Mini-Programme über Community-Plattformen |
| [MyGo](https://github.com/egoist/mygo) | Go, Webseiten | Einem eigenen GPU-Renderer oder dem System-WebView | macOS, Windows, Linux, Web |
| [Fyne](https://fyne.io), [Gio](https://gioui.org) | Go | Einem eigenen Renderer | Desktops, Smartphones, Web |
| [Wails](https://wails.io) | Go, Webseiten | Dem System-WebView | Desktops |
| [Flutter](https://flutter.dev) | Dart | Einem eigenen Renderer | Smartphones, Desktops, Web |
| [Compose Multiplatform](https://www.jetbrains.com/compose-multiplatform/) | Kotlin | Einem eigenen Renderer; auf Android Compose | Smartphones, Desktops, Web |
| [React Native](https://reactnative.dev) | JavaScript, TSX | Den Views der jeweiligen Plattform | iOS, Android; weitere über Community-Plattformen |
| [.NET MAUI](https://dotnet.microsoft.com/apps/maui) | C# | Den Steuerelementen der jeweiligen Plattform | iOS, Android, macOS, Windows |
| [Avalonia](https://avaloniaui.net) | C# | Einem eigenen Renderer | Desktops, Smartphones, Web |
| [Qt](https://www.qt.io) | C++, QML | Einem eigenen Renderer | Desktops, Smartphones, Embedded-Geräte |
| [Electron](https://www.electronjs.org), [Tauri](https://tauri.app) | Webseiten; Rust für Tauri | Einem gebündelten Chromium; dem System-WebView bei Tauri | Desktops; Smartphones bei Tauri |

## Stärken

- **Native Views.** Text, Eingabemethoden, Textauswahl, Scrollen, Barrierefreiheit und Menüs stammen von der jeweiligen Plattform selbst. Eine App verhält sich daher genau so, wie Nutzerinnen und Nutzer es dort erwarten – während ein Framework, das selbst zeichnet, jede dieser Funktionen neu nachbauen muss. Was nur eine Plattform besitzt, wie etwa Liquid Glass unter iOS, ahmen die anderen auf dieselbe Weise nach, statt darauf zu verzichten.
- **Feingranulare Updates.** Komponenten laufen genau einmal, und Signals aktualisieren ausschließlich das, was sie liest – ohne virtuelles DOM oder eine pro Frame neu ausgewertete View-Funktion zum Diffing. Animationen laufen auf dem Frame-Takt der Plattform.
- **Kleinste Pakete und schnellster Start.** In den [Benchmarks](https://nsgo.dev/benchmarks/) packt dieselbe Counter-App auf macOS, Windows und Linux auf rund 8 MB – kleiner als bei MyGo, GPUI und NSH (33 bis 45 MB bei NSH); sie startet zudem am schnellsten und benötigt weniger Arbeitsspeicher als Frameworks, die über die GPU zeichnen, wie etwa MyGo und GPUI.
- **Nur Go.** Engine, Layout und Backends bestehen vollständig aus Go – ohne C, ohne cgo und für Go-Apps auch ohne JavaScript-Engine.
- **Jede Plattform von einem Mac aus.** Ein Mac mit Go und Xcode cross-kompiliert für jede Zielplattform: kein NDK, kein Android SDK, kein JDK, kein GTK und keine Windows-Toolchain nötig.
- **Alle Release-Dateien mit einem Befehl.** `nsgo build` erzeugt ein signiertes, notarisiertes Disk-Image für macOS, einen signierten Windows-Installer, deb- und Arch-Pakete sowie ein AppImage, Pakete für den Mac App Store, den Microsoft Store, Google Play und Flathub sowie Installationsskripte für GitHub-Releases ([Bauen und Ausliefern](building.md)).
- **Plattformeigene Updates.** Unter macOS lädt, verifiziert und installiert Sparkle die nächste Version, unter Windows WinSparkle; unter Linux und Android findet die App das Update und öffnet die zugehörige Release-Seite; unter iOS übernimmt das der App Store. Wurde eine App über einen Store installiert, bleibt das Update diesem Store überlassen ([Updates](building.md#updates)).
- **Jede Plattform.** Smartphones, Tablets, Fernseher, Uhren, Desktops und das Web teilen sich eine Engine und eine API – zusammen mit iOS App Clips, iPhone- und Mac-Widgets sowie experimentell Mini-Programmen für WeChat, QQ, Alipay und Douyin.
- **Einbetten.** Eine NSGo-App lässt sich als View in eine bestehende iOS-, Android-, Go-Desktop- oder Web-App einbetten, um eine Anwendung Bildschirm für Bildschirm zu migrieren ([Einbetten](embedding.md)).
- **Schneller Entwicklungszyklus.** Das Speichern einer Go-Datei baut die App neu und startet sie neu; eine TSX-Änderung aktualisiert die laufende App direkt an Ort und Stelle und behält ihren Zustand; Build-Fehler und Panics beim Rendern werden direkt über der App angezeigt ([Ausführen und Debuggen](debugging.md)).
- **Ein einziger Testsatz für überall.** Tests finden und bedienen Steuerelemente anhand von Text oder Rolle genau wie Nutzerinnen und Nutzer – blitzschnell ohne Fenster oder unverändert in echten Fenstern auf jeder Zielplattform mit plattformeigener Eingabe und Screenshots ([Tests](testing.md)).
- **Eingebaute Lokalisierung.** Texte der App und ihrer Komponenten liegen in Katalogen, und Pseudo-Locales spüren unübersetzte, abgeschnittene oder noch nicht für Rechts-nach-links vorbereitete Stellen auf ([Lokalisierung](localization.md)).
- **Shader in Go.** `core.Shader` führt GPU-Shader aus, die in Go-Syntax oder WGSL geschrieben sind – auf jeder Plattform ([Canvas](canvas.md#shaders)).
- **TSX bei Bedarf.** Bildschirme können auch in TSX mit Tailwind-Klassen geschrieben werden und laufen auf derselben Engine wie Go-Komponenten.

## Schwächen

- **Frühes Stadium.** 0.1.0 ist das erste Release, und die API ist noch nicht stabil; die [Roadmap](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md) führt auf, was noch fehlt, etwa ein Heap-Limit für zur Laufzeit ausgeliefertes TSX.
- **Kleines Ökosystem.** Im Vergleich zu Flutter, React Native oder Qt gibt es online noch wenige Drittanbieter-Komponenten, Beispiele oder Antworten.
- **Plattformen unterscheiden sich.** Native Views bedeuten kleine Unterschiede zwischen den Plattformen, sodass eine App auf jeder Plattform geprüft werden muss. Ein selbst zeichnendes Framework sieht überall identisch aus.
- **Eigene Bridges.** Auf Android läuft das Go-Programm neben der Java-Laufzeitumgebung und kommuniziert mit ihr über Pipes, und unter Linux benötigen Nutzerinnen und Nutzer GTK 4.14 oder neuer.
- **TSX wird interpretiert.** TSX läuft in goja, einem in Go geschriebenen JavaScript-Interpreter, der langsamer ist als die Engines von React Native oder Electron.

## NSH

NSGo portiert das Design von NSH: Die Architektur, die Backends, die Namen der API und die ungestylten Komponenten in `base` stammen von NSH ([Status](index.md#status)).

- **Wählen Sie NSGo**, wenn Sie Apps lieber in Go statt in Rust oder TSX schreiben möchten, jede Zielplattform von einem Mac mit Go und Xcode aus bauen wollen oder die Ergänzungen von NSGo nutzen möchten: GPU-Shader in Go, Vektor-Icons, ein Terminal, Apps mit eigenen Fenstern, Panels und Tray-Icons sowie gestylte Komponenten in `ui`.
- **Wählen Sie NSH**, wenn Sie ein reiferes Projekt suchen, TSX unter QuickJS mit Bytecode und Heap-Limits ausführen möchten, das GPUI-Backend oder 32-Bit-Android benötigen.

## MyGo

Beide bieten UI in Go ohne cgo, aber MyGo zeichnet seine native Oberfläche selbst auf der GPU und baut sie in jedem Frame aus einer View-Funktion neu auf, während NSGo nativen Views nur mitteilt, was sich geändert hat.

- **Wählen Sie MyGo** für Desktop-Apps, die auf jedem Desktop gleich aussehen sollen, für Webseiten-Fenster mit typisierten Aufrufen in Go sowie für dessen Desktop-APIs und Auslieferung: Tray-Icons, Dateiverknüpfungen, Installer, Notarisierung und signierte Delta-Updates.
- **Wählen Sie NSGo**, um zusätzlich Smartphones zu erreichen, die plattformeigene Textverarbeitung, Eingabe und Barrierefreiheit zu nutzen oder Zustand in Signals zu halten, statt ihn aus den App-Daten heraus neu zu zeichnen.

## Weitere Go-Toolkits

- **Fyne und Gio** zeichnen genau wie MyGo alles selbst, und beide erreichen Smartphones sowie das Web. Fyne bietet ein vollständiges Widget-Set mit eigenem Erscheinungsbild; Gio arbeitet im Immediate Mode, ist hardwarenah und flexibel. Beide haben stabile Releases und aktive Nutzer; keines von beiden sieht nativ aus oder fühlt sich so an.
- **Wails** legt ein Web-Frontend in das System-WebView mit Go dahinter: Jede Web-Bibliothek funktioniert, aber die Oberfläche bleibt eine Webseite – und das nur auf dem Desktop.

## Frameworks in anderen Sprachen

- **Flutter und Compose Multiplatform** zeichnen jedes Pixel selbst: auf jeder Plattform identisch und ausgereift, mit großen Ökosystemen, dafür aber mit nachgebauter statt nativer Textdarstellung, Scroll- und Eingabelogik sowie Dart oder Kotlin als eigener Sprache.
- **React Native** teilt mit NSGo das Modell eines reaktiven Kerns, der native Views steuert, und verfügt über das größte Ökosystem seiner Art. Der Fokus liegt auf iOS und Android, und die App-Logik läuft in JavaScript; NSGo führt sie in Go aus und behandelt Desktops sowie das Web als gleichberechtigte Zielplattformen.
- **.NET MAUI** kapselt die Steuerelemente der jeweiligen Plattform in C#, allerdings ohne Linux oder das Web. **Avalonia** zeichnet selbst, ähnlich wie WPF, auf jedem Desktop, auf Smartphones und im Web.
- **Qt** ist ausgereift, schnell und erreicht auch Embedded-Geräte, zeichnet seine Widgets jedoch selbst, erfordert C++ oder QML und steht unter der LGPL oder einer kommerziellen Lizenz.
- **Electron und Tauri** stellen Webseiten dar: Sie bieten die Werkzeuge und Bibliotheken des Webs in vollem Umfang, um den Preis des Speicherbedarfs eines Browsers – vor allem bei Electron – und einer Oberfläche, die sich wie eine Webseite anfühlt.

## Entscheidungshilfe

Wählen Sie NSGo für eine App, die sich auf Smartphones, Desktops und im Web aus einer einzigen Go-Codebasis heraus nativ anfühlen soll und bei der ein junges Projekt vertretbar ist. Für eine App, die bald auf einer stabilen API ausgeliefert werden muss, greifen Sie zu einem ausgereiften Framework aus der obigen Tabelle: Flutter oder React Native für Smartphones, oder MyGo, Wails beziehungsweise Tauri für reine Desktop-Anwendungen.
