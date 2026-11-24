---
title: Texte
description: Paragraphes, segments, liens, polices et documents.
group: Composants
order: 32
---

`core.Text` est un paragraphe composé par le moteur de texte de la plateforme. `ui.Text` et `ui.Textf` sont des fonctions d’aide pour une ligne dans la typographie d’`ui` ; l’élément reste un `core.Text`, il accepte donc aussi `Style` et `Spans`.

```go preview=text/basic
ui.Text("Settings", 20, ui.Bold, colors.Text)
ui.Textf(func() string { return fmt.Sprintf("%d files", n.Get()) }, 13, ui.Regular, colors.Muted)
```

## Contenu

`Content` est fixe et `ContentFunc` suit un signal. `MaxLines` limite le nombre de lignes et `EllipsizeMode` tronque le reste au début, au milieu ou à la fin ; `Style.WhiteSpaceNoWrap` maintient un paragraphe sur une seule ligne. `TextAlign`, `TextTransform`, `LineHeight` et `LetterSpacing` mettent en forme le paragraphe, et `OnTextLayout` signale ses lignes après la mise en page.

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) },
	MaxLines: 2, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 15}}
```

## Spans et liens

`Spans` compose un texte de segments, chacun doté de son propre style. Un span muni de `OnPress` est un lien, et ses `Background` et `CornerRadius` se dessinent derrière ses caractères sur chaque ligne. `SpansFunc` fournit de nouveau les segments lorsque ce qu’il lit change, pour du code coloré ou une phrase dynamique.

```go preview=text/spans
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "Read the "},
	{Content: "guide", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openGuide},
	{Content: " before you start."},
}}
```

`Attachment` insère une vue dans une ligne à la place du contenu du span : une icône, une image ou un petit bouton. Elle est mise en page à sa propre taille, passe à la ligne avec les mots et reçoit ses propres appuis.

## Polices et style

`FontFamily` désigne une famille ou une liste CSS. `FontSize`, `FontWeight`, `FontStyle`, `FontVariant`, `LineHeight`, `LetterSpacing`, `TextDecoration` ainsi que `TextShadowColor` avec son décalage et son rayon mettent le texte en forme. `core.FontMonospace` est la famille à chasse fixe de la plateforme.

```go
core.Text{Content: "fn main() {}", Style: core.Style{FontFamily: core.FontMonospace, FontSize: 13}}
```

Enregistrez les polices empaquetées avec `nsgo.Fonts` avant la première vue, ou enregistrez une fonte pendant l’exécution de l’application avec `cx.RegisterFont`. Un texte préparé avant l’arrivée d’une police est préparé à nouveau.

## Documents

`base/textview.TextView` affiche du Markdown ou du HTML avec les mêmes primitives : titres, paragraphes, listes, tableaux, code, notes de bas de page, alertes, images, formules et liens. `textview.Parse` lit le Markdown et `textview.ParseHTML` le HTML, et `Options.Streaming` répare un document encore en cours de réception.

```go preview=text/document height=240
textview.TextView{Markdown: "## Notes\n\nA **native** document.", Styles: ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) { open(e.URL) }}
```

`textview.NewSearch` recherche dans le texte affiché par un `TextView` et en surligne les correspondances. Partagez la même recherche entre plusieurs vues avec `textview.ProvideSearch`, puis appelez `Open`, `SetQuery`, `Next` et `Previous` ; `Count` et `Selected` signalent le résultat. Les éléments de recherche sont mis en forme avec `textview.PartSearch` et `textview.PartSearchSelected`.

## Sélection et accessibilité

`SelectableOn` permet à l’utilisateur de sélectionner et de copier un texte. Une `core.View` dotée de `Selectable` permet de sélectionner ses textes ensemble dans l’ordre du document. Un `Role` tel que `RoleHeader` indique aux technologies d’assistance la nature du texte, et `Label` le remplace lorsque les mots ne constituent pas le libellé.

## Plateforme

Le moteur de texte de la plateforme met le texte en page, de sorte que les sauts de ligne, les polices et les méthodes de saisie le suivent. La césure CJK, les émojis et le texte de droite à gauche suivent le système. L’application peut lire la taille du texte avec `cx.Env().FontScale()`, et un texte la suit sauf si `NoFontScaling` est défini.

## Voir aussi

- [Texte](../text.md) : le guide complet sur le texte, les spans, les polices et les documents.
- [Images et icônes](../images.md) : images et icônes en ligne.
- [Accessibilité](../accessibility.md) : titres, libellés et sélection.
- Les pages Text et TextView de la galerie.
