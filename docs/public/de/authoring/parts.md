---
title: Teile und Zustände
description: "Keinen Zustand halten, der der App gehört, jeden Teil stylen und assistiven Technologien mitteilen, was die Komponente ist."
group: Komponenten schreiben
order: 71
---

Eine ungestylte Komponente legt fest, wie sich ein Steuerelement verhält, und überlässt das Aussehen der aufrufenden Seite. Diese Seite baut `Toggle` – eine Pille, die ein- oder ausgeschaltet ist – genau so, wie `base` seine Steuerelemente aufbaut.

## Kontrollierte Werte

Ein Steuerelement hält keinen Wert, der der App gehört: Die App übergibt ihn als Funktion und ändert ihn über einen Callback. Die Komponente liest ihn dort, wo er angezeigt wird, sodass sich bei einer Änderung nur dieser Teil aktualisiert.

```go
// Toggle is a pill that is on or off. Its parts are the pill and a dot
// inside it, then its children, the label.
type Toggle struct {
	On       func() bool
	Disabled func() bool
	// OnChange receives the value a press asks for, the opposite of On's.
	OnChange func(on bool)

	Class        string
	Style        core.Style
	StyleFunc    func() core.Style
	DotClass     string
	DotStyle     core.Style
	DotStyleFunc func() core.Style
	Children     []core.Node
	Label        string
}
```

Eine App hält den Wert in einem Signal: `chips.Toggle{On: bold.Get, OnChange: bold.Set, Label: "Bold"}`.

## Teile

Jeder Teil, den eine Komponente zeichnet, akzeptiert `Class`, `Style` und `StyleFunc`, benannt nach dem jeweiligen Teil (`DotClass`, `DotStyle`, `DotStyleFunc`). `Style` ist statisch, `StyleFunc` folgt den Signals, die sie liest, und die Klasse liegt unter beiden, sodass eine App nutzen kann, was immer sie bevorzugt.

Ein Teil übernimmt den Zustand des Steuerelements über `PartStates`, statt einen eigenen zu melden – so greifen `checked:`-Klassen und `Checked`-Styles darauf, während assistive Technologien nur ein einziges Steuerelement sehen:

```go
func (t Toggle) Render(*core.Scope) core.Node {
	on := func() bool { return t.On != nil && t.On() }
	off := func() bool { return t.Disabled != nil && t.Disabled() }
	shows := func() core.PartStates { return core.PartStates{Checked: on(), Disabled: off()} }
	dot := core.View{Class: t.DotClass, Style: t.DotStyle, StyleFunc: t.DotStyleFunc, PartStates: shows}
	return core.View{
		Class: t.Class, Style: t.Style, StyleFunc: t.StyleFunc,
		Role: core.RoleSwitch, Label: t.Label, Checked: on, Disabled: t.Disabled,
		OnPress: func(core.PressEvent) {
			if t.OnChange != nil {
				t.OnChange(!on())
			}
		},
		Children: append([]core.Node{dot}, t.Children...),
	}
}
```

Eine App stylt sie allein mit Klassen:

```go
chips.Toggle{
	On: bold.Get, OnChange: bold.Set, Label: "Bold",
	Class:    "flex-row items-center gap-2 rounded-full border border-zinc-300 px-3 py-1 checked:bg-blue-50",
	DotClass: "size-2 rounded-full bg-zinc-300 checked:bg-blue-500",
	Children: []core.Node{core.Text{Content: "Bold"}},
}
```

`PartStates.Invalid` wendet entsprechend `aria-invalid:`-Klassen und `Style.AriaInvalid` auf einen dekorativen Teil an – einschließlich `group-aria-invalid:`-Styles darin –, ohne ein weiteres ungültiges Steuerelement zu melden. Setzen Sie `Invalid` auf das eigentliche Steuerelement oder das native Eingabefeld, damit assistive Technologien seinen Validierungszustand erhalten.

## Barrierefreiheit

Die Wurzel eines Steuerelements sagt, was es ist: eine `Role`, ein `Label`, wenn sein Text es nicht ausdrückt, ein `Hint` dafür, was das Aktivieren bewirkt, sowie `Checked`, `Selected`, `Disabled`, `Invalid` oder `Expanded`, je nachdem, was zutrifft. Seine Teile bleiben stumm. Ein Steuerelement, das sich mit den Pfeiltasten zwischen Einträgen bewegt, ist ein einziger Tab-Stopp – wie eine Tab-Leiste. [Barrierefreiheit](../accessibility.md) listet die Rollen und Aktionen auf.

## Was durchgereicht werden sollte

Eine Komponente, deren Wurzel eine eigene View ist, übernimmt `OnLayout`, `Tooltip` und `TestID` dieser View und reicht sie weiter, damit Apps sie wie jede andere View vermessen, erklären und testen können. Callbacks sind optional: Ein `nil`-Wert für `OnChange` ergibt ein Steuerelement, das einen Wert anzeigt, ohne ihn zu ändern.
