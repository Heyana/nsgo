---
title: Terminal
description: Ein Programm ausführen oder eine entfernte Sitzung in einem Terminal-Raster anzeigen.
group: Kernkonzepte
order: 63
---

`base/terminal` stellt eine Sitzung (`Terminal`) und ihre `View` bereit. Desktops führen lokale Programme aus; iOS und Browser verwenden `Options.Conn`, einen `io.ReadWriteCloser`, der mit einem entfernten Programm verbunden ist. Die App besitzt die Sitzung und schließt sie, wenn sie fertig ist.

```go
t, err := terminal.New(terminal.Options{CursorBlink: true})
if err != nil {
	return core.Text{Content: err.Error()}
}
cx.Cleanup(func() { t.Close() })
return terminal.View{
	Terminal: t, AutoFocus: true,
	MinimumContrast: 4.5, PaddingColor: terminal.PaddingExtend,
	Style: core.Style{FlexGrow: 1},
}
```

`Options.Cursor` wählt als Standard Block, Balken oder Unterstrich. `CursorBlink` aktiviert sein Standard-Blinken; null bleibt ruhig. Programme überschreiben Form und Blinken über DECSCUSR oder DEC-Modus 12. Ein Cursor ohne Fokus ist hohl und ruhig; auch bei laufender Komposition und reduzierter Bewegung bleibt er ruhig. Die Animationsuhr der Plattform steuert das Blinken, ohne dass die Engine den Terminaltext neu zeichnet.

`View.Font`, `Theme` und `DarkTheme` legen Schrift und Palette fest. Das Raster zeichnet Unicode-Rahmen- und Blockzeichen sowie die Powerline-Trennzeichen U+E0B0–U+E0B7 selbst und richtet aneinandergrenzende Zellen an Gerätepixeln aus. Anderer Text verwendet die Schriften der Plattform und deren Ersatzschriften.

`MinimumContrast` erhöht den Kontrast gewöhnlichen Texts gegenüber dem tatsächlichen Hintergrund seiner Zelle, einschließlich inverser Farben, blassen Texts und Auswahl. Werte von 1–21 verwenden das Kontrastverhältnis nach WCAG; null lässt die Farben unverändert. Grafiken und Emoji behalten ihre Farben. Mit `Transparent` verwendet der Kontrast weiterhin den konfigurierten Hintergrund des Terminals.

`Padding` lässt Platz außerhalb des Rasters, in Punkten. `PaddingColor` wählt dessen Hintergrund:

| Wert | Verhalten |
| --- | --- |
| `PaddingBackground` | Terminalhintergrund, einschließlich Änderungen über OSC 11 |
| `PaddingExtend` | Hintergrund der nächstgelegenen Zelle; auf dem primären Bildschirm wird die Erweiterung oben/unten neben Prompt-Zeilen, Powerline-Zeilen oder Zeilen mit einer Zelle im Standardhintergrund unterdrückt |
| `PaddingExtendAlways` | Jeden Rand ohne diese Ausnahmen erweitern |

TSX-Apps importieren `tsx/terminal` wegen seines Nebeneffekts, `Terminal` in `nsgo/base` zu registrieren. Die Props sind `cursorBlink`, `minimumContrast` und `paddingColor`; letzteres akzeptiert `background`, `extend` und `extend-always`. Props für das Erscheinungsbild können sich ändern, während die Sitzung weiterläuft.

## In einem Browser

Die App bettet das festgelegte WebAssembly-Modul libghostty-vt ein und durchläuft dieselbe Terminal-View und denselben Eingabepfad. Zum Bauen der App sind weder Zig noch die Installation einer nativen Bibliothek noch ein Download zur Laufzeit nötig.

Passen Sie den WebSocket des Browsers an `io.ReadWriteCloser` an: Senden Sie binäre Bytes und fügen Sie eingehende Nachrichten als Stream zusammen oder teilen Sie sie auf. Die App und ihr Server sind für Authentifizierung, erneutes Verbinden und etwaige Größen- oder Steuernachrichten zuständig; NSGo verarbeitet Terminal-Bytes. Die [Browser-Abnahme-Fixture](https://github.com/nsgo-dev/nsgo/blob/main/internal/testhost/testdata/terminalweb/socket_js_test.go) zeigt einen vollständigen Byte-Stream-Adapter mit Behandlung des Schließens.

## Aktionen und Zoom

`View.Keybinds` überschreibt die Standardwerte mit `Keybind{Key, Action, Amount}`. Tasten verwenden dieselben Namen wie `Scope.OnKey`, etwa `mod+=` oder `ctrl+shift+c`. Spätere Einträge haben Vorrang. Tastenbelegungen greifen, solange die Eingabe dieses Terminals den Fokus hat und keine Eingabemethode gerade komponiert.

Die Aktionen sind `CopyToClipboard`, `PasteFromClipboard`, `SelectAll`, `ClearScreen`, `IncreaseFontSize`, `DecreaseFontSize`, `ResetFontSize`, `SetFontSize` und `Ignore`. Fenster- und Tab-Aktionen gehören in die Tastenbelegung der App. `Ignore` verbraucht eine Taste, ohne sie an das Programm zu senden.

Standardmäßig vergrößern `mod+=` und `mod+shift+=` die Schrift um einen Punkt, `mod+-` verkleinert sie, und `mod+0` setzt sie auf `View.Font.Size` zurück (13, wenn nicht gesetzt). `Amount` legt die Schrittweite für Vergrößern/Verkleinern fest, eins bei null, oder die Größe für `SetFontSize`. Größen werden auf 1–255 Punkte begrenzt; nicht endliche Anforderungen werden ignoriert. Der Zoom behält die Sitzung und die äußere View bei, aktualisiert die Zeilen und Spalten des Rasters und passt die Größe des Programms an.

```go
ref := &terminal.Ref{}
view := terminal.View{
	Terminal: session, Ref: ref,
	Keybinds: []terminal.Keybind{
		{Key: "ctrl+shift+k", Action: terminal.ClearScreen},
		{Key: "mod+=", Action: terminal.IncreaseFontSize, Amount: 2},
	},
}
```

`Ref.FontSize()` wird für Steuerelemente verfolgt, die sie anzeigen; `Ref.SetFontSize(size)` und `Ref.Perform(action, amount)` laufen auf der Engine der App, so wie Event-Handler. `Perform` meldet, ob eine Aktion verfügbar ist. Eine nicht gemountete Ref liefert die Größe null und lehnt Aktionen ab.

## Kontextmenü

Ein sekundärer Klick öffnet ein ungestyltes `base.ContextMenu` mit lokalisierten Einträgen für Kopieren, Einfügen, Alles auswählen und Löschen. Kopieren erfordert eine Auswahl. Löschen entfernt den Inhalt oberhalb des Prompts und ist auf dem alternativen Bildschirm deaktiviert. Touchscreens verwenden die Long-Press-Darstellung des Kontextmenüs. Stylen Sie die Oberfläche mit `MenuClass`/`MenuStyle` und die Einträge mit `MenuItemClass`/`MenuItemStyle`.

Setzen Sie `RightClickAction` auf `RightClickPaste`, `RightClickCopy`, `RightClickCopyOrPaste` (eine Auswahl kopieren, sonst einfügen) oder `RightClickIgnore`. Ignore überlässt das Ereignis einem umgebenden App-Menü; verwenden Sie es, wenn Sie ein gestyltes `ui.ContextMenu` bereitstellen, das die Aktionen der Ref aufruft.

Importieren Sie `tsx/terminal`, um `<Terminal>` in `nsgo/base` zu registrieren. Seine reaktiven Props `keybinds` und `rightClickAction` aktualisieren die View, während die Sitzung erhalten bleibt. Aktionswerte verwenden Ghostty-Namen wie `increase_font_size` und `copy_to_clipboard`; Werte für den Rechtsklick sind `context-menu`, `paste`, `copy`, `copy-or-paste` und `ignore`. Die Props für das Menü-Styling verwenden die Go-Namen in lowerCamelCase.

## Mausberichte

Programme, die Maus-Tracking aktivieren, erhalten die angeforderten Drücke, Loslassen, Ziehvorgänge und Hover, von libghostty-vt in ihrem gewählten Protokoll kodiert. Shift umgeht die Erfassung von Klicks und Ziehvorgängen zugunsten der Auswahl. `View.MouseReporting` ist ein optionales reaktives `func() bool`, true bei nil; geben Sie false zurück, um Mausauswahl und natives Scrollen im Scrollback beizubehalten, selbst wenn ein Programm die Erfassung anfordert. Der Rechtsklick eines Programms hat Vorrang vor einem umgebenden Kontextmenü, solange die Berichterstattung aktiviert ist.

Auf dem alternativen Bildschirm wird bei aktiviertem Alternate-Scroll-Modus und ohne angefordertes Maus-Tracking vertikale Mausradeingabe zu Cursortasten, gemäß dem normalen oder dem Anwendungs-Cursortastenmodus. `MouseScrollMultiplier{Precision: 1, Discrete: 3}` legt die Standardanzahl der Berichte für Touchpads und Mausräder fest; null verwendet diese Standardwerte, und positive Werte werden auf 0,01–10.000 begrenzt. Bruchteile von Eingaben sammeln sich an, bis ein vollständiger Schritt verfügbar ist. Der normale Scrollback behält das native Scrollen und die Trägheit der Plattform bei.

Importieren Sie `tsx/terminal`, um `<Terminal>` in `nsgo/base` zu registrieren. Sein Prop `mouseReporting` ist standardmäßig true; `mouseScrollPrecision` und `mouseScrollDiscrete` legen die Multiplikatoren fest.
