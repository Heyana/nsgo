---
title: Parties et états
description: Ne conserver aucun état appartenant à l'application, styliser chaque partie et indiquer aux technologies d'assistance ce qu'est le composant.
group: Écrire des composants
order: 71
---

Un composant sans style décide du comportement d'un contrôle et laisse son apparence à celui qui l'utilise. Cette page construit `Toggle`, une pilule activée ou désactivée, de la même manière que `base` construit ses contrôles.

## Valeurs contrôlées

Un contrôle ne conserve aucune valeur appartenant à l'application : l'application la transmet sous forme de fonction et la met à jour depuis un rappel (*callback*). Le composant la lit là où elle est affichée, de sorte que seule cette partie se mette à jour lorsqu'elle change.

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

L'application conserve la valeur dans un signal : `chips.Toggle{On: bold.Get, OnChange: bold.Set, Label: "Bold"}`.

## Parties

Chaque partie dessinée par un composant accepte `Class`, `Style` et `StyleFunc`, nommés d'après la partie (`DotClass`, `DotStyle`, `DotStyleFunc`). `Style` est fixe, `StyleFunc` suit les signaux qu'elle lit, et la classe s'applique sous les deux, afin qu'une application puisse utiliser la forme qu'elle préfère.

Une partie reçoit l'état du contrôle via `PartStates` plutôt que de déclarer le sien, de sorte que les classes `checked:` et les styles `Checked` s'y appliquent tandis que les technologies d'assistance ne voient qu'un seul contrôle :

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

Une application le stylise uniquement avec des classes :

```go
chips.Toggle{
	On: bold.Get, OnChange: bold.Set, Label: "Bold",
	Class:    "flex-row items-center gap-2 rounded-full border border-zinc-300 px-3 py-1 checked:bg-blue-50",
	DotClass: "size-2 rounded-full bg-zinc-300 checked:bg-blue-500",
	Children: []core.Node{core.Text{Content: "Bold"}},
}
```

`PartStates.Invalid` applique de même les classes `aria-invalid:` et `Style.AriaInvalid` à une partie décorative, y compris les styles `group-aria-invalid:` à l'intérieur, sans signaler un autre contrôle invalide. Placez `Invalid` sur le véritable contrôle ou champ natif afin que les technologies d'assistance reçoivent son état de validation.

## Accessibilité

La racine d'un contrôle dit ce qu'il est : un `Role`, un `Label` lorsque son texte ne l'exprime pas, un `Hint` indiquant ce que fait son activation, ainsi que `Checked`, `Selected`, `Disabled`, `Invalid` ou `Expanded` selon le cas. Ses parties restent silencieuses. Un contrôle qui passe d'un élément à l'autre avec les touches fléchées forme un seul arrêt de tabulation, comme une liste d'onglets. [Accessibilité](../accessibility.md) répertorie les rôles et les actions.

## Ce qu'il faut transmettre

Un composant dont la racine est une vue propre accepte les champs `OnLayout`, `Tooltip` et `TestID` de cette vue et les transmet, afin que les applications puissent le mesurer, l'expliquer et le tester comme n'importe quelle vue. Les rappels sont facultatifs : un `OnChange` égal à `nil` produit un contrôle qui affiche une valeur sans la modifier.
