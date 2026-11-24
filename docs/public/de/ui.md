---
title: Komponentenreferenz
description: Die meistgenutzten Komponenten, ihre Varianten, Zustände und Barrierefreiheit.
group: Komponenten
order: 30
---

Dies sind die Komponenten von `ui`, im Aussehen von NSGo und dem Theme folgend. Diese Referenz beginnt mit den Komponenten, zu denen eine App zuerst greift; jede Seite beginnt mit dem kürzesten Beispiel und behandelt dann ihre Teile, Zustände und Barrierefreiheit. Die Galerie umfasst jede Komponente, auch solche, die hier noch keine eigene Seite haben.

Die [Galerie](https://nsgo.dev/gallery/) führt im Browser eine Demo jeder Komponente aus.

## Aktionen

- [Button](ui/button.md): ein Druckknopf, schlicht oder in der Akzentfarbe, und der Icon-Button.

## Text und Eingabe

- [Text](ui/text.md): Absätze, Spans, Links, Schriften und Dokumenttext.
- [Input](ui/input.md): das Textfeld der Plattform, kontrolliert oder nicht.
- [Checkbox](ui/checkbox.md): ein Kontrollkästchen, aktiviert, deaktiviert oder unbestimmt.
- [Switch](ui/switch.md): eine Einstellung, die ein- oder ausgeschaltet ist.
- [Select](ui/select.md): ein Wert aus einer Liste, als Popup oder als Sheet.
- [Slider](ui/slider.md): ein Wert oder ein Bereich entlang einer Spur.

## Sammlungen

- [List](ui/list.md): Zeilen in beliebiger Anzahl, nur im sichtbaren Bereich gebaut, mit Auswahl und Aktivierung.
- [Scroll](ui/scroll.md): die Scroll-Ansicht der Plattform, mit Paging, Einrasten, Zoom und Aktualisieren.

## Navigation

- [Tabs](ui/tabs.md): eine Tab-Liste, ein Segmented Control und die Tab-Leiste des Systems.

## Overlays

- [Dialog](ui/dialog.md): eine modale Fläche, ein Sheet und der Hinweisdialog des Systems.
- [Menu](ui/menu.md): ein Menü über einen Auslöser, ein Kontextmenü und ihre Einträge.

## Mehr

Die Galerie deckt den Rest ab: Tabellen, Diagramme, den Code-Editor, Rich Text, das Terminal, das Dock, die Befehlspalette, Farb-, Datums- und Emoji-Auswahl (`ui.EmojiPicker`), Toasts und mehr. Zu jeder gibt es dort eine Demo.

`base/terminal.View` öffnet vorhandene lokale Pfade mit Command+Klick (Control+Klick unter Windows und Linux), so wie es URLs öffnet. Relative Pfade verwenden das über OSC 7 gemeldete Arbeitsverzeichnis der Shell; eine lokale Shell, die keines gemeldet hat, verwendet `terminal.Options.Dir` oder, wenn dieses nicht gesetzt ist, das Home-Verzeichnis. Setzen Sie Dateinamen, die Leerzeichen enthalten, in Anführungszeichen. Entfernte Verzeichnisangaben und Verbindungen ohne Meldung eines lokalen Verzeichnisses werden nicht gegen lokale Dateien aufgelöst. Dateien und Ordner verwenden `Scope.OpenFile`; URL-Links laufen weiterhin über `Scope.OpenURL` und den URL-Handler des einbettenden Hosts.

`base/terminal.View` verweigert Programmen standardmäßig das Lesen und Schreiben der Zwischenablage. Setzen Sie `AllowClipboardRead` und `AllowClipboardWrite` nur, wenn die App dies erlaubt; das `AccessClipboard` des anfragenden Views gilt weiterhin. Dieselben Props stehen beim `Terminal` von TSX zur Verfügung und können sich ändern, ohne dessen Sitzung neu zu starten. Apps stellen die Oberfläche für die Zustimmung bereit. Ein ausdrückliches Einfügen durch die Nutzerin oder den Nutzer bietet unabhängig davon Programmen, die das Kitty-Zwischenablageprotokoll (Modus 5522) verwenden, einen Text-/PNG-Schnappschuss an; dessen Einmalpasswort kann nur diesen Schnappschuss lesen. Programme ohne diesen Modus erhalten gewöhnlichen Text im Bracketed-Paste-Format. Ob Bilder in der Zwischenablage verfügbar sind, richtet sich nach den Zwischenablagediensten der Plattform; die primäre Auswahl von X11 wird nicht bereitgestellt.

## Wie es weitergeht

- [Komponenten und Zustand](components.md): wie eine Komponente Elemente zurückgibt und wie Signale sie aktualisieren.
- [Styles und Themes](styles.md): das Theme, aus dem Komponenten ihre Farben beziehen.
- [Komponenten ohne Design](authoring/base.md): dieselben Steuerelemente ohne Aussehen, für ein eigenes Aussehen.
- [Barrierefreiheit](accessibility.md): Labels, Rollen und Werte für jedes Steuerelement.
