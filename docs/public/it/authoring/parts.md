---
title: Parti e stati
description: Non conservare stato posseduto dall'app, stilizza ogni parte e comunica alla tecnologia assistiva cos'è il componente.
group: Scrivere componenti
order: 71
---

Un componente senza stile decide come si comporta un controllo e lascia a chi lo usa la scelta di come appare. Questa pagina costruisce `Toggle`, una pillola accesa o spenta, nello stesso modo in cui `base` costruisce i propri controlli.

## Valori controllati

Un controllo non conserva alcun valore posseduto dall'app: l'app lo passa come funzione e lo imposta tramite un callback. Il componente lo legge nel punto in cui viene mostrato, così solo quella parte si aggiorna quando cambia.

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

Un'app mantiene il valore in un segnale: `chips.Toggle{On: bold.Get, OnChange: bold.Set, Label: "Bold"}`.

## Parti

Ogni parte disegnata da un componente accetta una `Class`, uno `Style` e una `StyleFunc`, nominati in base alla parte (`DotClass`, `DotStyle`, `DotStyleFunc`). `Style` è fisso, `StyleFunc` segue i segnali che legge e la classe si applica sotto entrambi, così un'app può usare ciò che preferisce scrivere.

Una parte riceve lo stato del controllo tramite `PartStates` anziché dichiararne uno proprio, così le classi `checked:` e gli stili `Checked` si applicano a essa mentre la tecnologia assistiva vede un unico controllo:

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

Un'app lo stilizza usando solo le classi:

```go
chips.Toggle{
	On: bold.Get, OnChange: bold.Set, Label: "Bold",
	Class:    "flex-row items-center gap-2 rounded-full border border-zinc-300 px-3 py-1 checked:bg-blue-50",
	DotClass: "size-2 rounded-full bg-zinc-300 checked:bg-blue-500",
	Children: []core.Node{core.Text{Content: "Bold"}},
}
```

Analogamente, `PartStates.Invalid` applica le classi `aria-invalid:` e `Style.AriaInvalid` a una parte decorativa, inclusi gli stili `group-aria-invalid:` al suo interno, senza segnalare un altro controllo non valido. Imposta `Invalid` sul controllo effettivo o sull'input nativo affinché la tecnologia assistiva riceva il suo stato di validazione.

## Accessibilità

La radice di un controllo dichiara ciò che è: un `Role`, una `Label` quando il suo testo non lo esprime, un `Hint` per ciò che fa l'attivazione, e `Checked`, `Selected`, `Disabled`, `Invalid` o `Expanded` a seconda dei casi. Le sue parti restano silenziose. Un controllo che si sposta tra gli elementi con le frecce è un singolo punto di tabulazione, proprio come un elenco di schede. [Accessibilità](../accessibility.md) elenca i ruoli e le azioni.

## Cosa inoltrare

Un componente la cui radice è una vista propria accetta `OnLayout`, `Tooltip` e `TestID` di quella vista e li inoltra, così le app possono misurarlo, spiegarlo e testarlo come qualsiasi altra vista. I callback sono opzionali: un `OnChange` nullo crea un controllo che mostra un valore senza modificarlo.
