---
title: Einführung
description: Was NSGo ist, worauf es läuft und wie es funktioniert.
order: 1
---

NSGo ist ein plattformübergreifendes natives UI-Framework, geschrieben in Go. Sie schreiben eine App einmal, in Go oder in TSX, und sie läuft auf iOS und dessen App Clips, tvOS, watchOS, Android, macOS, Windows, Linux und im Web, gezeichnet von den eigenen Views der jeweiligen Plattform. Go-Apps laufen experimentell auch in Mini-Programmen von WeChat, QQ, Alipay und Douyin: Die Plattformen begrenzen eine App auf 2 MB, was größere Apps überschreiten. HarmonyOS führt Go-Apps als Vorschau aus, gezeichnet von ArkUI.

## Was Sie schreiben

Eine App ist ein Go-`main`-Package. Ihre Komponenten geben Elemente zurück: Views, Text, Bilder, Eingabefelder und die Steuerelemente aus `ui`. Der Zustand liegt in Signals, und nur was ein Signal liest, wird aktualisiert, wenn es sich ändert.

```go
func Root(cx *core.Scope) core.Node {
	count := core.NewSignal(cx, 0)
	return core.View{Style: core.Style{Padding: core.All(24), Gap: 12}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }},
		ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }},
	}}
}
```

Dieselbe App lässt sich in TSX schreiben und mit Tailwind-Klassen gestalten ([TSX-Apps](tsx.md)).

## Wie es funktioniert

Eine Go-Engine verwaltet die App: den reaktiven Graphen, Styles, Flexbox- und Grid-Layout sowie die Textaufbereitung. Jede Aktualisierung erzeugt einen Stapel von Änderungen, und ein schlankes Backend wendet ihn auf die nativen Views der Plattform an. Animationen laufen auf dem Frame-Takt der Plattform, sodass nicht einmal pro Frame etwas übertragen wird.

| Plattform | Zeichnet mit |
| --- | --- |
| iOS | UIKit und CoreText |
| Android | Android Views |
| macOS | AppKit und CoreText |
| Windows | Direct2D und DirectWrite |
| Linux | GTK 4, Pango und Cairo |
| Web | WebAssembly und DOM-Elementen |

Jede Zielplattform lässt sich auf einem Mac mit Go bauen; iOS benötigt zusätzlich Xcode. Das Werkzeug `nsgo` erstellt, startet, testet und paketiert Apps.

## Status

NSGo steht am Anfang: 0.1.0 ist die erste Version, die API ist noch nicht stabil, und was fehlt, steht in der [Roadmap](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md). Es überträgt das Design von [Native Shell](https://github.com/longbridge/nsh) nach Go.

## Wie Sie die Dokumentation lesen

Jede Seite beantwortet eine Frage. Ein Codebeispiel ist ein Fragment einer Komponente, sofern die Seite nichts anderes sagt: `cx` ist der `*core.Scope`, den eine Komponente erhält. Apps verwenden die Komponenten aus `ui`, die ein Erscheinungsbild haben; `base` enthält dieselben Steuerelemente ohne Erscheinungsbild, für Komponentenautoren.

- **Erste Schritte**: [Einrichtung](getting-started.md), dann Schritt für Schritt aufgebaute Beispiel-Apps: [eine Todo-App](tutorials/todo.md), [eine Notizen-App](tutorials/notes.md) und [beliebte GitHub-Repositories](tutorials/repos.md).
- **Toolchain**: [Ausführen und Debuggen](debugging.md), [Testen](testing.md), [Bauen und Ausliefern](building.md) und [Einbetten](embedding.md).
- **Komponenten**: [die meistgenutzten Komponenten](ui.md), ihre Varianten, Zustände und Barrierefreiheit.
- **Kernkonzepte**: [Komponenten und Zustand](components.md), [Layout](layout.md), [Styles](styles.md), [Text](text.md), [Listen](lists.md), [Navigation](navigation.md), [Formulare](forms.md), [Lokalisierung](localization.md), [TSX](tsx.md) und der Rest.
- **Plattformfunktionen**: [Plattformdienste](services.md), [Benachrichtigungen](notifications.md), [Haptik](haptics.md), [Widgets](widgets.md), [Live-Aktivitäten](live-activities.md) und [Berechtigungen](permissions.md).
- **Komponenten schreiben**: [Komponenten für die Apps anderer](authoring/index.md), ungestylt und gestylt.
- **Mitwirken**: [NSGo aus dem Quellcode bauen](contributing.md), sich darin zurechtfinden und eine Änderung einreichen.
- Die [Live-Demo](https://nsgo.dev/gallery/): jede Komponente, in Ihrem Browser.
