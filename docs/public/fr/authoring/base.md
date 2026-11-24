---
title: Composants sans style
description: Les contrôles propres à NSGo sans apparence, à styliser partie par partie ou sur lesquels bâtir une bibliothèque stylisée.
group: Écrire des composants
order: 73
---

`base` contient chaque contrôle présent dans `ui`, avec son comportement, son état et son accessibilité, mais sans apparence. Chaque partie accepte `Class`, `Style` et `StyleFunc` ([Parties et états](parts.md)), de sorte qu'une bibliothèque ou une application dotée de son propre design les utilise à la place de `ui`, ou les encapsule comme le fait `ui` ([Apparences et thèmes](looks.md)). Cette page présente les contrôles de la [référence des composants](../ui.md) ; la galerie montre chacun d'eux à côté de sa forme `ui`.

Les contrôles tels que `Button`, `Checkbox`, `Select`, `Slider` et `NumberInput` acceptent `Invalid: rejected.Get` pour suivre un signal de validation. Stylisez cet état avec `Class: "aria-invalid:border-red-500"` ou `Style.AriaInvalid`. Cela met à jour l'accessibilité et les styles sans désactiver le contrôle. Les champs composites le transmettent à leur véritable entrée et à leurs parties décoratives ; `ColorPicker` le partage avec ses contrôles.

Pour les valeurs applicatives glissées entre vues, utilisez [`base/dragdrop`](../drag-drop.md) : une zone relie des sources et des cibles tandis que l'application fournit leur contenu, leur aperçu et leur apparence.

## Button

`base.Button` tire son apparence de `Style` et son contenu de `Children`. `OnPress` s'exécute lors d'un appui complet ; `OnPressDown` et `OnPressUp` suivent le début et la fin d'un appui, pour un retour visuel personnalisé.

```go
base.Button{
	Label:   "Save",
	OnPress: func(core.PressEvent) { save() },
	Style: core.Style{Background: core.RGB(0x007aff), Padding: core.Symmetric(8, 14), CornerRadius: 8, AlignSelf: core.AlignStart,
		Active: &core.Style{Opacity: core.Alpha(0.85)}},
	Children: []core.Node{core.Text{Content: "Save", Style: core.Style{Color: core.RGB(0xffffff)}}},
}
```

`Disabled`, `Busy`, `Selected` et `Expanded` signalent un état à l'application, au style et aux technologies d'assistance : une action toujours en cours utilise `Busy` (stylisé avec `aria-busy:`), un bouton bascule ou l'élément courant d'une liste `Selected`, une section dépliable `Expanded`. `NoFocus` retire un bouton des arrêts de tabulation lorsqu'une ligne ou une liste l'atteint autrement.

## Checkbox

`base.Checkbox` prend un style pour la case, un pour l'indicateur et votre coche à l'intérieur, affichée selon la valeur détenue par l'application.

```go
base.Checkbox{
	Checked:        done.Get,
	OnChange:       done.Set,
	Label:          "Remember me",
	Style:          core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	IndicatorStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), BorderWidth: 1, CornerRadius: 5},
	Indicator:      core.Icon{SVG: lucide.Check(), Style: core.Style{FontSize: 14}},
	Children:       []core.Node{core.Text{Content: "Remember me"}},
}
```

L'indicateur reçoit son état de la case via `PartStates`, de sorte que les classes `checked:` le stylisent sans en faire un second arrêt.

Pour les touches modificatrices, définissez `OnChangeEvent func(checked bool, press core.PressEvent)`. Elle remplace `OnChange` et laisse la valeur à l'application ; `press.Modifiers.Shift` permet à une liste de sélectionner une plage. L'activation par Espace/Entrée sur ordinateur, le web, Android et iOS/tvOS préserve les modificateurs, sauf si un raccourci de l'application intercepte la touche d'abord ; sous Windows, Alt avec Espace ou Entrée ouvre plutôt le menu de la fenêtre. En TSX, `onChangeEvent` reçoit `{ checked, press }`.

## Switch

`base.Switch` comprend une piste (*track*), un curseur (*thumb*) à l'intérieur, puis ses enfants comme libellé. Un appui signale la valeur demandée, à l'opposé de `Checked`.

```go
base.Switch{
	Checked:    dark.Get,
	OnChange:   dark.Set,
	Label:      "Dark appearance",
	Style:      core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8},
	TrackStyle: core.Style{Width: core.Pt(38), Height: core.Pt(22), CornerRadius: 11},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

La piste et le curseur acceptent chacun leur propre `StyleFunc`, et leurs styles d'état suivent `Checked`.

## Slider

`base.Slider` prend un `base.SliderValue` : `base.SingleValue(v)` ou `base.RangeValue(low, high)`. Il stylise séparément la piste, l'indicateur, la plage et les curseurs (*thumbs*).

```go
level := core.NewSignal(cx, 3.0)
base.Slider{
	Value:      func() base.SliderValue { return base.SingleValue(level.Get()) },
	OnChange:   func(v base.SliderValue) { level.Set(v.End) },
	Min:        func() float64 { return 0 },
	Max:        func() float64 { return 10 },
	Step:       func() float64 { return 1 },
	Label:      "Level",
	Style:      core.Style{Height: core.Pt(28)},
	TrackStyle: core.Style{Height: core.Pt(4), CornerRadius: 2},
	ThumbStyle: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9},
}
```

`Min`, `Max` et `Step` sont des accesseurs réactifs : modifier leurs signaux met à jour le curseur monté et sa plage accessible, y compris pendant un glissement. `Axis` le rend vertical et `Inverted` place son minimum à l'autre extrémité, comme ceux de `base.Progress` ; `Marks` place vos propres repères sur l'indicateur, et `OnChangeEnd` signale la valeur une fois le glissement terminé.

## Select

`base.Select` prend des `base.SelectItem` et une valeur `*string`, de sorte que « aucun » soit `nil`. Vous stylisez le déclencheur, la fenêtre contextuelle, ses lignes et ses groupes.

```go
choice := core.NewSignal[*string](cx, nil)
base.Select{
	Items: func() []base.SelectItem {
		return []base.SelectItem{{Value: "s", Title: "Small"}, {Value: "m", Title: "Medium"}}
	},
	Value:      choice.Get,
	OnChange:   choice.Set,
	Label:      "Size",
	Style:      core.Style{Height: core.Pt(28), Padding: core.Symmetric(4, 10), BorderWidth: 1, CornerRadius: 6},
	PopupStyle: core.Style{Padding: core.All(4), CornerRadius: 8},
	ItemStyle:  core.Style{Padding: core.Symmetric(4, 8), CornerRadius: 4},
}
```

Les éléments adjacents ayant le même `Group` partagent un en-tête, la saisie anticipée (*typeahead*) correspond aux titres des éléments, `SearchField` dessine votre propre champ de recherche, et `Cleanable` affiche un bouton d'effacement. `OnCancel` écoute Échap, le bouton retour et un appui à l'extérieur.

## Tabs

`base.Tabs` contient des `base.Tab` et dessine l'indicateur à partir d'`IndicatorStyle` ; l'application conserve l'onglet sélectionné. `base.Tab.OnPress` s'exécute à chaque appui, y compris sur l'onglet déjà sélectionné.

```go
base.Tabs{Label: "Sections", IndicatorStyle: core.Style{Height: core.Pt(2), Background: core.RGB(0x007aff)}, Children: []core.Node{
	base.Tab{Label: "Overview", Selected: func() bool { return section.Get() == "overview" },
		OnPress: func(core.PressEvent) { section.Set("overview") }, Children: []core.Node{core.Text{Content: "Overview"}}},
	base.Tab{Label: "Activity", Selected: func() bool { return section.Get() == "activity" },
		OnPress: func(core.PressEvent) { section.Set("activity") }, Children: []core.Node{core.Text{Content: "Activity"}}},
}}
```

## Dialog

`base.Dialog` prend ses styles d'arrière-plan (*backdrop*), de calque et de fenêtre contextuelle, une `Animation` ou un `TransitionShader`, et vos enfants. `Open`, `DefaultOpen` et `OnOpenChange` le contrôlent ; `OnOpen`, `OnClose` et `OnCancel` rendent compte de ce qui s'est passé. `OnConfirm` est l'action par défaut : Entrée l'exécute, sauf si le focus clavier se trouve sur un bouton ou un champ qui intercepte lui-même Entrée ou si une autre fenêtre modale recouvre la boîte de dialogue ; celle-ci se ferme alors avec `ReasonSelect`. `Role` et `AccessibilityDescription` sont ce qu'annoncent les lecteurs d'écran, `RoleAlertDialog` pour une confirmation. `NoCloseOnSwipe` empêche le balayage iOS de la fermer, `Presentation` choisit la présentation iOS, et `StatusBar` modifie la barre d'état tant qu'elle est ouverte.

`base.Sheet` est un panneau sur n'importe quel bord, avec un arrière-plan et votre contenu ; avec `CloseOnSwipe`, un panneau inférieur suit un glissement vers le bas et se ferme au-delà d'un seuil. Il accepte `OnConfirm`, `Role` et `AccessibilityDescription` comme `base.Dialog`. `base.ShowAlertDialog` affiche l'alerte ou la feuille d'actions du système avec des boutons classés par rôle :

```go
base.ShowAlertDialog(cx, base.AlertDialog{
	Title: "Delete the note?",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(label string) {
	if label == "Delete" {
		remove()
	}
})
```

## Menu

`base.Menu` ouvre des `base.MenuItem` depuis votre propre déclencheur, et `base.ContextMenu` à la position du pointeur. Un élément déclenche `OnSelect` et ferme le menu sauf si `NoCloseOnSelect` est défini ; `Checked` en fait un élément cochable, `Indicator` dessine votre marque avant son contenu, et `Role` choisit ce qu'entend la technologie d'assistance : un élément de menu, une case à cocher ou un bouton radio. Un `base.Submenu` contient d'autres éléments.

```go
base.Menu{Trigger: core.Text{Content: "Actions"}, Children: []core.Node{
	base.MenuItem{Label: "Show sidebar", Checked: sidebar.Get, OnSelect: func() { sidebar.Set(!sidebar.Get()) }},
	base.Submenu{Label: "Share", Children: []core.Node{
		base.MenuItem{Label: "Copy link", OnSelect: copyLink},
		base.MenuItem{Label: "Email", OnSelect: emailLink},
	}},
}}
```

`Placement`, `Align`, `Offset` et `Margin` positionnent le menu contextuel sur ordinateur, et `PopupStyle` et `BackdropStyle` le stylisent.

## EmojiPicker

`base.EmojiPicker` recherche tous les emojis Unicode d'après leurs noms CLDR localisés, mots-clés et codes courts (*shortcodes*) dans les langues de l'application (ou des paramètres explicites `Languages` / `LanguagesFunc`), avec des onglets de catégories, des entrées facultatives `Recent`, la sélection du teint de peau, la navigation au clavier et une grille virtualisée. Tous les libellés par défaut et noms de catégories proviennent de `base/locales`, et le jeu de données d'emojis n'est lié que dans les applications qui utilisent `EmojiPicker` ou ses fonctions utilitaires (`base.SearchEmojis`, `base.EmojisInCategory`, `base.EmojiByString`).

```go
base.EmojiPicker{
	Recent:   recent.Get,
	OnSelect: func(e base.Emoji) { insert(e.Emoji) },
	Style:    core.Style{Width: core.Pt(320), Height: core.Pt(360)},
}
```

`Category` et `DefaultCategory` sélectionnent le groupe parcouru. Les onglets de catégories et les choix de teint de peau forment chacun un arrêt de tabulation avec navigation par flèches. `EmojiPickerRef` fournit `Focus`, `Query`, `SetQuery`, `SetCategory` et `MatchedCount` ; le sélecteur stylisé utilise le même état de requête. `LabelFunc`, `SearchLabelFunc` et `PlaceholderFunc` mettent à jour les libellés sur place. `Emoji.WithSkinTone` applique le teint choisi aux deux personnes d'une paire, et le teint par défaut restaure sa forme neutre ; la recherche par un emoji préserve sa séquence exacte.

Utilisez `ui.EmojiPicker` pour le sélecteur thématisé avec icônes de catégories Lucide, champ de recherche effaçable, aperçu de l'emoji surligné et pastilles de teint de peau.
