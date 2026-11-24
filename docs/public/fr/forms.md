---
title: Formulaires
description: Champs texte, focus, clavier virtuel, interrupteurs, sélecteurs et validation.
group: Concepts clés
order: 57
---

## Champs texte

`ui.Input` est le champ texte de la plateforme avec l'apparence de `ui` et les champs de `base.Input`. Avec `Value`, il est contrôlé : l'application détient la valeur et la met à jour dans `OnChange` ; avec `DefaultValue`, il conserve la sienne.

```go
name, email := core.NewSignal(cx, ""), &core.Ref{}

ui.Input{
	Value:         name.Get,
	OnChange:      name.Set,
	Placeholder:   "Your name",
	ReturnKeyType: core.ReturnKeyNext,
	OnSubmit:      func(string) { email.Focus() },
}
```

`Masked` masque les caractères, `Readonly` empêche les modifications tout en laissant le texte sélectionnable et copiable (dans les mini-programmes, uniquement dans un champ monoligne WeChat : un champ multiligne WeChat et les champs Alipay se désactivent comme le fait `Disabled`, car ces hôtes ne peuvent pas annuler une saisie), `Disabled` désactive le champ, et `Multiline` passe le texte à la ligne et grandit avec lui sauf si son style fixe sa taille. Les lignes d'un champ multiligne, ainsi que celles de son texte indicatif, ont la hauteur définie par `LineHeight` dans son style, la police étant centrée dans chacune comme dans un `core.Text` ; un champ monoligne garde sa ligne centrée dans sa hauteur. `KeyboardType` choisit le clavier virtuel (`core.KeyboardEmail`, `core.KeyboardNumber`, `core.KeyboardURL`, etc.), `AutoComplete` indique au système ce que le champ contient pour le remplissage automatique, et `MaxLength` limite le nombre d'unités UTF-16 saisies par l'utilisateur. `AutoCapitalize` et `NoAutoCorrect` ajustent le comportement du clavier.

`OnSelectionChange` et `OnCaretMove` signalent la sélection et la position du curseur, par exemple pour un menu qui s'ouvre à cet endroit. `Suggest`, un `core.Suggest`, suit une requête après un déclencheur tel que `@`, et un `ui.SuggestList` à côté du champ propose des lignes au niveau du curseur, parmi lesquelles Haut, Bas, Entrée et un appui permettent de choisir, comme le fait `Suggest` de `chat.Composer` pour les mentions. `OnKeyPress` écoute chaque touche, `OnPaste` ce que l'utilisateur colle, et `OnCompositionChange` suit une méthode de saisie.

Sur iOS et Android, `cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways}, handler)` intercepte la touche Entrée du clavier virtuel avant que le champ ne valide, ne perde le focus ou n'insère un saut de ligne. `KeyAuto` et `KeyNotEditing` la laissent à `SubmitBehavior` ; sur iOS, une touche Entrée qui valide du texte en cours de composition reste gérée par la méthode de saisie.

## Focus et clavier

`Ref.Focus()` donne le focus à un champ ou, sur ordinateur de bureau, le web et Apple TV, à une vue cliquable ou `Focusable` activée. `Ref.Blur()` ne retire le focus que de sa propre vue, et `cx.DismissKeyboard()` ferme le clavier virtuel. `OnFocus` et `OnBlur` suivent le focus, et `View.OnFocus`/`OnBlur` font de même pour toute vue pouvant recevoir le focus. Une vue ayant le focus qui quitte l'arbre, lorsque son `Show` se ferme ou que sa ligne disparaît, reçoit `OnBlur` une fois, après le commit qui l'a retirée ou, lorsque la plateforme signale d'abord le focus suivant, avant le `OnFocus` de cette vue, et les vues englobantes qui restent en place reçoivent `OnFocusOut`.

Les demandes de focus, de retrait du focus, de défilement et de zoom d'une `Ref` sont sans effet tant que son élément se trouve dans le contenu d'un `Holder` qu'aucun emplacement n'affiche, et celles encore en file d'attente lorsque l'élément quitte l'arbre sont abandonnées. Réafficher le contenu ne les rejoue pas : demandez à nouveau une fois qu'il est placé.

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

Les claviers virtuels recouvrent le bas d'un téléphone. `core.EnvKeyboard` utilisé comme marge interne ou externe inférieure maintient le contenu au-dessus, et son déplacement est animé avec le clavier :

```go
core.View{Style: core.Style{
	Padding:    core.All(16),
	PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard},
	Gap:        12,
}, Children: form}
```

Une vue défilante autour d'un formulaire peut fermer le clavier lorsqu'elle est glissée et décider de l'effet d'un appui pendant que le clavier est affiché :

```go
core.Scroll{Behavior: core.ScrollBehavior{
	KeyboardDismissMode:       core.KeyboardDismissOnDrag,
	KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
}, Children: []core.Node{form}}
```

`cx.Env().KeyboardHeight()` et `cx.Env().KeyboardDuration()` lisent dans le code la taille et la durée d'animation du clavier, et `cx.Env().SafeArea()` les marges de sécurité autour de l'écran.

## Choix

`ui.Switch` est un réglage qui prend effet immédiatement, `ui.Checkbox` est coché, décoché ou indéterminé, et `ui.Radio` est une option au sein d'un `ui.RadioGroup`. `ui.Select[T]` ouvre une liste d'options sous son déclencheur, ou sous forme de feuille lorsque sa propriété `Presentation` le demande, et `ui.Tabs[T]` en affiche plusieurs à la fois.

```go
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}

size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options:  []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:    size.Get,
	OnChange: size.Set,
	Label:    "Size",
}
```

`ui.NumberInput` et `base.NumberInput` acceptent `LabelFunc` pour un nom accessible dynamique. Un `Format` personnalisé suit les signaux qu'il lit tant que le champ n'est pas en cours d'édition ; un changement de langue préserve la saisie en cours jusqu'à la fin de l'édition. `ui.NumberInput` incrémente et ajuste un nombre avec des bornes, `ui.DatePicker` et `ui.TimeField` éditent des dates et des heures, `ui.ColorPicker` édite une couleur, et `ui.TagInput` édite une liste d'étiquettes.

`ui.OtpInput` saisit un code à usage unique dans des cellules. Avec `base.OtpInput`, il accepte `LabelFunc` sans remplacer le champ ayant le focus ni son code. `Size` définit la hauteur et la largeur par défaut, 36 points sur ordinateur de bureau et TV et 44 sur appareils tactiles. `CellWidth` et `CellHeight` surchargent indépendamment chaque dimension. `TextStyle` surcharge la typographie des cellules, `CornerRadius` modifie leurs coins, et `CaretHeight` surcharge la hauteur déduite de la hauteur de ligne.

```go
quantity := core.NewSignal(cx, 1.0)
ui.NumberInput{Value: quantity.Get, OnChange: quantity.Set, Min: 1, Max: 99, Step: 1, Label: "Quantity"}
```

## Lignes groupées

`ui.FormSection` place ses lignes sur une carte, avec le titre au-dessus et le pied de section en dessous. Définissez `CardStyle` pour l'arrière-plan, la bordure ou les coins de la carte. `CardStyleFunc` fusionne par-dessus et suit les signaux qu'il lit. `DividerInset` reçoit chaque ligne enfant, y compris celles produites par `core.Each` : retournez un retrait et `true` pour surcharger son séparateur, ou `false` pour conserver l'alignement automatique sur le titre (12 points pour les autres nœuds). Un retrait de zéro produit un séparateur sur toute la largeur.

`ui.FormRow` place `Leading` ou une tuile d'icône avant le titre. Définissez `LeadingWidth` pour réserver l'espace de votre nœud et aligner le séparateur situé en dessous avec le titre. L'emplacement est de 24 points par défaut. Définissez `TileSize` pour mettre à l'échelle ensemble la tuile d'icône, le glyphe et les coins. `TitleStyle` et `DetailStyle` surchargent la typographie du titre et du détail. `TitleMaxLines` limite le nombre de lignes du titre (zéro est illimité), `MinHeight` surcharge la hauteur minimale de la ligne en points, et `Below` place un nœud après le titre et le détail dans leur colonne de texte. Un `MinHeight` à zéro conserve la valeur par défaut : 40 points, ou 48 sur appareils tactiles.

Pour un groupe à choix unique, enveloppez la section dans `base.RadioGroup` et définissez `Role: core.RoleRadio`, `Checked` et `OnPress` sur chaque ligne. La ligne utilise `base.Radio` pour ses états d'accessibilité coché et sélectionné, un seul arrêt Tab et la sélection aux touches fléchées ; appuyer sur une ligne déjà cochée ne fait rien. L'application détient la valeur choisie.

`ui.FormSection.TitleFunc` et `FooterFunc`, ainsi que `ui.FormRow.TitleFunc`, `DetailFunc` et `LabelFunc`, mettent à jour leurs libellés sans reconstruire la ligne. Un détail, un titre de section ou un pied de section vide supprime ce texte optionnel ; un libellé de ligne vide utilise son titre courant. Un `ui.Select` avec recherche lie `SearchPlaceholderFunc` et `EmptyFunc` tout en conservant sa recherche ouverte et sa sélection.

## Validation

L'application gère les règles : un signal contient l'erreur, `OnBlur` ou `OnSubmit` la définit, et un `core.Show` ajoute un message sous le champ tant qu'il y en a une.

```go
address := core.NewSignal(cx, "")
err := core.NewSignal(cx, "")
colors := ui.ColorsOf(cx)
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{
		Value:    address.Get,
		OnChange: func(v string) { address.Set(v); err.Set("") },
		Label:    "Email",
		OnSubmit: func(string) {
			if !strings.Contains(address.Get(), "@") {
				err.Set("Enter an email address.")
			}
		},
	},
	core.Show{
		When: func() bool { return err.Get() != "" },
		Then: func(*core.Scope) core.Node { return ui.Textf(err.Get, 12, ui.Regular, colors.Danger) },
	},
}}
```

`cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` anime un message qui apparaît, et `OnSubmit` avec `ReturnKeyNext` déplace le clavier vers le champ suivant.

## Suite

- [Navigation](navigation.md) : où vit un formulaire.
- [Texte](text.md) : comment le texte d'un champ est stylé.
- [Accessibilité](accessibility.md) : les libellés et valeurs dont les lecteurs d'écran ont besoin.
