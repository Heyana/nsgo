---
title: Die Codebasis
description: Wo jeder Teil von NSGo liegt und welche Dokumente das Warum erklären.
group: Mitwirken
order: 81
---

NSGo ist ein einzelnes Go-Modul. Apps importieren dessen Pakete von oben; das Backend jeder Plattform liegt ganz unten und erhält, was sich geändert hat, als native Views zum Erstellen, Aktualisieren und Entfernen.

## Pakete

| Paket | Inhalt |
| --- | --- |
| `nsgo` (das Stammverzeichnis) | `nsgo.Run` und `nsgo.NewView`, mit Auswahl eines Backends über Build-Tags |
| `core` | Die API der App: Elemente, Styles, Klassen, Themes, Signals und Dienste; Layout und Textvorbereitung |
| `base` | Ungestylte Komponenten, je eine Datei pro Komponente, sowie komplexe Komponenten als Pakete (`base/dock`, `base/editor`) |
| `ui` | Die Komponenten aus `base` mit einem Erscheinungsbild sowie Pakete wie `ui/chat` |
| `mobile`, `desktop` | Funktionen, die nur Smartphones oder nur Desktops haben |
| `tsx` | TSX-Apps, ihr Bundler und Compiler sowie die Module, die sie importieren |
| `i18n`, `*/locales` | Nachrichten-Kataloge und der daraus generierte Code |
| `nsgotest` | Tests, die wie eine Nutzerin oder ein Nutzer agieren – headless oder auf einer Plattform |
| `internal/engine`, `internal/driver`, `internal/layout` | Die Engine-Goroutine, das Mount-Protokoll und das Layout |
| `internal/platform/<target>` | Ein Backend: native Views, Text und Eingabe für eine Zielplattform |
| `cmd/nsgo` | Das Werkzeug `nsgo` |
| `examples/` | Die Gallery und die übrigen Beispiel-Apps |

Pakete importieren ausschließlich nach unten; `internal/moduletest/layering_test.go` prüft den Abhängigkeitsgraphen auf jeder Zielplattform.

## Designdokumente

[ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) beschreibt die Pakete, die Threads und die Abläufe von einem Commit bis zum Bildschirm und listet die Themen in `docs/architecture/` auf: eines pro Teilbereich, etwa Layout, Text, Scrollen, Themes oder das CLI-Werkzeug. Lesen Sie das Thema des Bereichs, den Sie ändern, bevor Sie ihn bearbeiten; es dokumentiert, wie der Teil funktioniert und wo er von [Native Shell](https://github.com/longbridge/nsh) abweicht, dessen Design NSGo portiert.

[AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md) enthält die Regeln, denen jede Änderung folgt – sowohl für Menschen als auch für Coding-Agents. Einige davon prägen die meisten Änderungen:

- Jede API hat genau ein Zuhause: `core` für Elemente und Umgebung, `base` für Steuerelemente, `ui` für deren Aussehen, `mobile` und `desktop` für jeweils eine Geräteklasse.
- `base` hat kein eigenes Erscheinungsbild; eine `ui`-Komponente ist ihre `base`-Form mit einem solchen.
- Nichts ändert sich implizit je nach Gerät: keine größeren Steuerelemente auf Desktops oder kleineren auf Smartphones in `core` oder `base`.
- Native Views überall, gleich auf jeder Plattform: Was eine Plattform nativ mitbringt, ahmen die anderen nach.
- Kein handgeschriebenes C und nichts über macOS, Xcode und Go hinaus, um jede Zielplattform außer dem x86_64-Android-Emulator zu bauen.

Was plattformweise noch fehlt, steht in der [Roadmap](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md).
