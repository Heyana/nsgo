---
title: Komponenten schreiben
description: "Komponenten entwickeln, die andere in ihre Apps einbauen – genau wie NSGos eigene Pakete base und ui."
group: Komponenten schreiben
order: 70
---

Diese Seiten richten sich an Autorinnen und Autoren von Komponenten: eine Bibliothek aus Steuerelementen, ein Design-System oder eine komplexe View, die Apps als Go-Paket importieren. Sie setzen [Komponenten und Zustand](../components.md) sowie [Styles und Themes](../styles.md) voraus.

## Eine Komponente

Eine Komponente ist ein Typ mit einer `Render`-Methode – genau wie in einer App. Seine Felder sind das, was die App setzt; was die App nicht setzt, erhält den Nullwert, sodass der Nullwert jedes Felds der Normalfall sein sollte.

```go
package chips

import "github.com/nsgo-dev/nsgo/core"

// Chip is a short label in a pill.
type Chip struct {
	Text string
}

// Render implements core.Node.
func (c Chip) Render(*core.Scope) core.Node {
	return core.View{Class: "self-start rounded-full px-3 py-1", Children: []core.Node{
		core.Text{Content: c.Text, Class: "text-sm"},
	}}
}
```

Eine App nutzt sie wie jedes Element: `chips.Chip{Text: "New"}`.

## Verhalten und Aussehen

NSGo führt jedes Steuerelement zweifach. `base` enthält Verhalten, Zustand und Barrierefreiheit ohne Aussehen und nimmt jeden Style Teil für Teil von der App entgegen; `ui` legt ein Aussehen über die `base`-Form, sodass das Verhalten an einer einzigen Stelle lebt. Eine Bibliothek kann derselben Aufteilung folgen:

- Eine **ungestylte** Komponente verwaltet Drücke, Tastatur, Fokus und das, was Screenreader vorlesen, und stellt den Style jedes Teils bereit – wie `base.Switch`. Apps und andere Bibliotheken geben ihr ihr eigenes Aussehen ([Teile und Zustände](parts.md)).
- Eine **gestylte** Komponente umhüllt eine ungestylte und wählt ausschließlich Styles aus dem aktiven Theme – wie `ui.Switch` ([Aussehen und Themes](looks.md)).

Eine Komponente ohne nennenswertes gemeinsames Verhalten, wie etwa ein Badge, kann direkt gestylt werden.

## Ein Paket

- Halten Sie eine Komponente pro Datei, benannt nach ihr (`chip.go`), zusammen mit den Typen, die nur sie verwendet; was mehrere teilen, bekommt eine eigene Datei.
- Geben Sie einer komplexen Komponente ein eigenes Paket, dessen API auf den Komponentennamen verzichtet, wie `dock.Area` aus `base/dock`.
- Legen Sie die Texte einer Komponente in `locales/` des Pakets ab ([Texte, Tests und Veröffentlichung](shipping.md)).
- Treffen Sie keine impliziten Entscheidungen anhand des Geräts: Eine Größe oder Form, die sich auf Smartphone oder Desktop ändert, ist ein Feld, das die App setzt.

## Weiter

- [Teile und Zustände](parts.md): kontrollierte Werte, gestylte Teile und Barrierefreiheit.
- [Aussehen und Themes](looks.md): Farben aus dem Theme, Varianten und Größen.
- [Komponenten ohne Design](base.md): NSGos eigene Steuerelemente ohne Aussehen, zum Stylen oder Weiterbauen.
- [Texte, Tests und Veröffentlichung](shipping.md): Übersetzungen, Tests und TSX.
