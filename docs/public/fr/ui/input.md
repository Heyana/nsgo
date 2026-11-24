---
title: Champ texte
description: Le champ texte de la plateforme, contrôlé ou non, avec le focus et le clavier virtuel.
group: Composants
order: 33
---

`ui.Input` est le champ texte de la plateforme dans l’apparence de `ui`, sur une ligne sauf si `Multiline` est défini. Il possède les champs de `base.Input`, comme tout champ stylisé ; `ui.Field` est le même champ sans taille propre, dans un contour qui ne suit ni le pointeur ni le focus ; `core.Transparent` dans son `Background` et `core.Zero` dans son `BorderWidth` désactivent le cadre, pour un texte qui se lit comme du texte brut mais reste éditable.

```go preview=input/basic
name := core.NewSignal(cx, "")
ui.Input{
	Value:       name.Get,
	OnChange:    name.Set,
	Placeholder: "Your name",
	TestID:      "name",
}
```

## Contrôlé et non contrôlé

Avec `Value`, le champ est contrôlé : l’application détient la valeur et la met à jour dans `OnChange`. Avec `DefaultValue`, le champ conserve la sienne. Un champ contrôlé peut être vidé en définissant sa valeur sur `""`.

```go preview=input/submit
draft := core.NewSignal(cx, "")
ui.Input{Value: draft.Get, OnChange: draft.Set, Placeholder: "New task",
	OnSubmit: func(string) { add(); draft.Set("") }}
```

## Champs

`Multiline` passe à la ligne, démarre avec une hauteur de trois lignes et grandit avec son texte à moins que son style ne définisse `Height` ou `MinHeight`. `Masked` masque les caractères, `Readonly` empêche les modifications tout en laissant le texte sélectionnable et copiable, `Disabled` désactive le champ, et `MaxLength` limite le nombre d’unités UTF-16 saisies par l’utilisateur. `Placeholder` peut être une fonction avec `PlaceholderFunc`.

```go
ui.Input{Masked: true, Value: password.Get, OnChange: password.Set,
	AutoComplete: core.AutoCompletePassword}
```

`KeyboardType` choisit un clavier virtuel (`KeyboardEmail`, `KeyboardNumber`, `KeyboardURL` et les autres), `ReturnKeyType` définit le libellé de sa touche Entrée, et `SubmitBehavior` indique si Entrée valide la saisie ou insère un saut de ligne dans un champ multiligne. `AutoCapitalize`, `NoAutoCorrect` et `AutoComplete` ajustent l’assistance du système.

`InputSource` sélectionne une source de saisie clavier tant que le champ a le focus et rétablit celle de l’utilisateur lorsqu’il le perd, sauf si `KeepInputSource` est défini : `"com.apple.keylayout.ABC"` maintient en anglais un champ destiné aux codes ou aux commandes sous macOS. L’identifiant est celui de la plateforme, un identifiant de disposition clavier tel que `"00000409"` sous Windows et une méthode de saisie Fcitx 5 ou IBus sous Linux ; une source qui n’est pas activée, les téléphones et le web ne changent rien.

```go
ui.Input{Placeholder: "Bundle ID", InputSource: "com.apple.keylayout.ABC"}
```

`Style` fusionne par-dessus l’apparence du champ, et `StyleFunc` par-dessus celui-ci : ce qu’ils définissent, comme la hauteur, la marge interne, la police ou une couleur, remplace ce que définit `ui`, et le reste demeure. Le contour s’assombrit sous le pointeur et prend la couleur d’accentuation lorsqu’il a le focus, tant que ni l’un ni l’autre ne définit `BorderColor` ou `Hover`. Un `Padding` nul conserve celui de `ui` ; `core.Zero`, une longueur définie, le retire (`Padding: core.All(core.Zero)`).

```go
ui.Input{Placeholder: "Filter", Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
ui.Field{DefaultValue: "Untitled", Style: core.Style{FontSize: 24, FontWeight: ui.Bold,
	Background: core.Transparent, BorderWidth: core.Zero}}
```

## Focus et événements

`Ref` donne le focus au champ et le lui retire. `OnFocus` et `OnBlur` suivent le focus, `OnSelectionChange` et `OnCaretMove` la sélection et le curseur, `OnKeyPress` chaque touche saisie, `OnPaste` ce qui est collé et `OnCompositionChange` la composition d’une méthode de saisie.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

Le clavier virtuel recouvre le bas d’un téléphone. Placez `core.EnvKeyboard` sur la marge interne ou externe inférieure du contenu pour qu’il reste au-dessus du clavier ; le déplacement s’anime avec lui. `cx.DismissKeyboard()` ferme le clavier, et `cx.Env().KeyboardHeight()` lit sa hauteur.

## Validation

L’application détient les règles. Un signal conserve l’erreur, `OnBlur` ou `OnSubmit` la définit, et un message sous le champ l’affiche ([Formulaires](../forms.md#validation)).

```go preview=input/validation
err := core.NewSignal(cx, "")
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{Value: email.Get, OnChange: email.Set, Label: "Email",
		OnSubmit: func(string) {
			if !strings.Contains(email.Get(), "@") {
				err.Set("Enter an email address.")
			} else {
				err.Set("")
			}
		}},
	ui.Textf(err.Get, 12, ui.Regular, colors.Danger),
}}
```

## Accessibilité

Le champ signale `RoleTextField`, ou `RoleSearchField` pour une zone de recherche lorsque son `Role` l’indique, et lit son `Label` ; `Placeholder` n’est pas un libellé, donnez donc un `Label` à un champ dépourvu de libellé visible. `Readonly` et `Disabled` sont signalés, et `OnSubmit` constitue l’action du clavier. Les champs monolignes, masqués et multilignes exposent le `Placeholder` courant en tant que texte indicatif ou conseil d’accessibilité natif sur macOS, iOS et Linux, en se mettant à jour et en s’effaçant sans second arrêt de focus pour le libellé indicatif dessiné d’un champ multiligne.

## Plateforme

iOS, Android, Windows et Linux utilisent leurs champs natifs ; le web utilise un `<input>` ou `<textarea>`. Le clavier virtuel, le remplissage automatique, les méthodes de saisie et le sens du texte sont ceux de la plateforme. Le focus et la saisie sont testés de la même manière sur chaque cible.

## Voir aussi

- [Formulaires](../forms.md) : champs, validation, choix et clavier.
- [Accessibilité](../accessibility.md) : libellés et focus.
- [Mise en page](../layout.md) : garder le contenu au-dessus du clavier.
- Les pages Input et Form de la galerie.
