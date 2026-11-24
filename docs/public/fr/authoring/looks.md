---
title: Apparences et thèmes
description: Tirer les couleurs du thème actif et doter un composant de variantes et de tailles.
group: Écrire des composants
order: 72
---

Un composant stylisé choisit des styles et rien d'autre : il encapsule un composant sans style, dans un paquet dédié tel que `chipsui`, et y laisse son comportement. Ses couleurs proviennent du thème, de sorte qu'il suive le thème de l'application, le thème propre d'un panneau et l'apparence claire ou sombre sans être reconstruit.

## Couleurs issues du thème

Lisez les couleurs à l'intérieur d'une `StyleFunc` : elle s'exécute à nouveau lorsque ce qu'elle a lu change, si bien qu'un changement de thème ou d'apparence restylise le composant sur place. `cx.Palette()` fournit la palette affichée à l'endroit où se trouve le composant, et `ui.ColorsOf(cx)` nomme les rôles utilisés par `ui`.

```go
// Toggle is chips.Toggle in the theme's colors.
type Toggle struct {
	On       func() bool
	OnChange func(bool)
	Label    string
}

func (t Toggle) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	on := func() bool { return t.On != nil && t.On() }
	return chips.Toggle{On: t.On, OnChange: t.OnChange, Label: t.Label,
		Class: "flex-row items-center gap-2 rounded-full px-3 py-1",
		StyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: colors.Accent()}
			}
			return core.Style{Background: colors.Inset()}
		},
		DotClass: "size-2 rounded-full",
		DotStyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: colors.OnAccent()}
			}
			return core.Style{Background: colors.Faint()}
		},
		Children: []core.Node{ui.Text(t.Label, 13, ui.Medium, colors.Text)},
	}
}
```

Les couleurs codées en dur cassent en mode sombre et sous le thème d'une application ; tirez chaque couleur d'un rôle.

## Vos propres rôles

Une couleur pour laquelle la palette n'a pas de rôle va dans `Palette.Colors` sous un nom choisi par vous, défini dans les deux apparences. Les applications le remplacent dans le code ou dans leur fichier de thème, sous `light` et `dark` :

```go
theme := core.DefaultTheme()
theme.Light.SetColor("chip", core.RGB(0xeef5ff))
theme.Dark.SetColor("chip", core.RGB(0x1b2a3d))
cx.SetTheme(theme)
```

Un composant la lit avec `cx.Palette().Color("chip")`, qui indique si le thème la définit, afin que le composant puisse se rabattre sur un rôle existant dans le cas contraire. Les dégradés, ombres et autres finitions ne sont pas des couleurs : conservez-les dans le code, choisis selon `cx.Dark()`.

## Variantes

Un composant doté de plusieurs apparences prend un champ `Variant`, d'un type propre dont la valeur zéro correspond à l'apparence par défaut. Il nomme uniquement les apparences que le composant dessine, couleur comprise, plutôt que des champs booléens ou un type de couleur partagé :

```go
// ToggleVariant is a toggle's look.
type ToggleVariant uint8

const (
	// ToggleSolid fills the pill with the accent when on.
	ToggleSolid ToggleVariant = iota
	// ToggleOutline draws its edge in the accent instead.
	ToggleOutline
)
```

## Tailles

La plupart des composants n'ont besoin que d'une seule taille, ce qui maintient les contrôles alignés sans que l'application ait à choisir. Un composant qui en nécessite plusieurs prend un champ `Size`, de type `ui.Size` (par défaut, petit ou grand), afin de correspondre aux 28, 24 et 32 points d'`ui.Button` ; `Size.Height()` donne la hauteur sur laquelle aligner les autres contrôles. Un composant qui garde une même forme à toute échelle, comme un avatar, prend plutôt `Size` en points.
