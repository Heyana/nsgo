---
title: Texte
description: Afficher du texte avec des spans, polices, décorations, sélection et documents.
group: Concepts clés
order: 53
---

## Un texte

`core.Text` représente un paragraphe, composé par le moteur de texte propre à la plateforme côté moteur et dessiné par le thread principal. Sa taille provient de sa police et de sa largeur, et il passe à la ligne à l’intérieur de sa boîte.

```go
core.Text{Content: "Hello from NSGo", Style: core.Style{
	FontSize:   15,
	LineHeight: 22,
	FontWeight: core.FontWeightSemibold,
}}
```

`Content` est fixe et `ContentFunc` suit un signal, de sorte qu’un texte qui change le lit à l’intérieur de la fonction. `MaxLines` limite le nombre de lignes affichées, `EllipsizeMode` coupe le reste au début, au milieu ou à la fin, et `WhiteSpaceNoWrap` maintient un paragraphe sur une seule ligne. `AdjustsFontSizeToFit` réduit le texte jusqu’à ce qu’il tienne dans sa boîte et sa limite de lignes, jusqu’à `MinimumFontScale` (au moins 0,1). Les formules et autres pièces jointes relatives à la police rétrécissent avec lui et retrouvent leur taille naturelle lorsque l’espace le permet ; les tailles explicites en points et les marges internes conservent leurs valeurs d’origine.

Les champs de texte héritent comme en CSS : un texte à l’intérieur d’une vue dotée de `FontSize`, `Color` ou `LineHeight` les reprend à moins de définir les siens. `TextAlign`, `TextTransform`, `LetterSpacing`, `TextDecoration`, `TextDecorationStyle`, `TextDecorationColor` ainsi que `TextShadowColor`, `TextShadowOffset` et `TextShadowRadius` mettent en forme les lignes et les glyphes.

## Segments et liens

`Spans` compose un texte de segments, chacun doté de son propre style. Un span avec `OnPress` est un lien, et un appui sur un autre span déclenche le `OnPress` du texte lui-même.

```go
core.Text{Style: core.Style{FontSize: 15}, Spans: []core.Span{
	{Content: "By continuing you agree to the "},
	{Content: "terms", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openTerms},
	{Content: " and the "},
	{Content: "privacy policy", Style: core.Style{TextDecoration: core.TextDecorationUnderline}, OnPress: openPrivacy},
	{Content: "."},
}}
```

`SpansFunc` fournit de nouveau les spans lorsque ce qu’elle lit change, et seules les parties qui diffèrent sont préparées à nouveau, de sorte que les lignes d’un éditeur de code se recolorent à mesure que la coloration syntaxique arrive. `Background` et `CornerRadius` d’un span dessinent une boîte derrière ses caractères sur chaque ligne, et son `Cursor` indique l’apparence du pointeur au-dessus d’eux avant celle du texte. `Visibility: core.VisibilityHidden` masque un span sur place : ses mots conservent leur place, si bien que les lignes ne bougent pas lorsqu’il apparaît, et la copie ainsi que les lecteurs d’écran l’omettent, utile pour des mots qui apparaissent en fondu un par un.

Définissez `BackgroundInsetTop: core.Pct(62)` sur un span ayant un `Background` pour surligner les 38 % inférieurs de sa boîte sur chaque ligne. `BackgroundInsetBottom` rogne le bord inférieur. Les deux marges internes acceptent des points ou des pourcentages de la hauteur de la boîte, de l’ascendante à la descendante de la police. Elles laissent la mise en page du texte inchangée. En TSX, définissez `backgroundInsetTop: "62%"` et `backgroundInsetBottom` dans le style d’un `<Text>` imbriqué. Les arrière-plans arrondis et les marges verticales s’affichent également sur HarmonyOS et les quatre cibles mini-programmes.

`Attachment` insère une vue à l’intérieur d’un texte, à la place du contenu du span. Elle est disposée à sa propre taille, suit le retour à la ligne avec les mots qui l’entourent et reçoit ses propres appuis, afin qu’une icône, une image ou un petit bouton puisse prendre place dans une ligne. Une sélection copie le `Content` du span à sa place, ou l’omet s’il est vide. Par défaut, sa première ligne de texte s’aligne sur la ligne de base environnante, en utilisant les métriques de sa police réelle. Un canevas `Em` fournit sa propre ligne de base, et une vue sans ligne de base repose sur son bord inférieur. Définissez `Style.VerticalAlign` pour choisir un autre alignement.

```go
core.Text{Style: core.Style{FontSize: 17, LineHeight: 24}, Spans: []core.Span{
	{Content: "Search with "},
	{Attachment: core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 17}}},
	{Content: " NSGo"},
}}
```

## Polices

`FontFamily` nomme une famille ou une liste CSS (`Inter, "Noto Sans SC", sans-serif`), chacune utilisée pour les caractères qui manquent aux précédentes. Chaque famille de la liste reçoit la graisse et le style. Sur macOS et iOS, une ligne tire sa hauteur et sa ligne de base des fontes dans lesquelles ses caractères sont composés, de sorte qu’une ligne entièrement composée dans une famille ultérieure adopte celles de cette famille, là où un navigateur conserve celles de la première famille comme plancher. `FontWeight`, `FontStyle`, `FontVariant`, `LineHeight`, `LineHeightMultiple`, `LetterSpacing` et `LetterSpacingEm` définissent la fonte et l’espacement, et `core.FontMonospace` désigne la famille à chasse fixe de la plateforme.

`Language` est l’étiquette BCP 47 du texte, héritée comme les champs ci-dessus sur les textes, les spans et les champs de saisie. Les caractères han sont dessinés selon ses formes, si bien que `直` défini en `ja` prend sa forme japonaise même là où le système préfère le chinois, et le coréen (`ko`) coupe les lignes entre les mots comme le fait `word-break: keep-all` en CSS. Les textes et champs de saisie qui n’en définissent aucune adoptent la langue affichée par l’application et la suivent lorsqu’elle change. Définissez-la sur une vue pour un sous-arbre, ou sur un `Span` pour un seul segment :

```go
core.Text{Spans: []core.Span{
	{Content: "日本語: "},
	{Content: "直 骨 角", Style: core.Style{Language: "ja"}},
}}
```

Empaquetez les polices de l’application sous `resources/` et enregistrez-les avant la première vue, ou enregistrez les octets d’une police pendant l’exécution de l’application :

```go
func main() { nsgo.Run(Root, nsgo.Fonts("fonts/Inter-Regular.ttf", "fonts/Inter-Bold.ttf")) }

cx.RegisterFont(data) // une fonte TrueType ou OpenType
```

Un texte mis en page avant l’arrivée d’une police est préparé à nouveau, et les canevas exécutent `Draw` à nouveau afin que leurs glyphes et mesures de texte utilisent la police chargée. `cx.Env().FontScale()` est la taille de texte de l’utilisateur, que les textes suivent sauf si `NoFontScaling` est défini ; `MaxFontSizeMultiplier` la plafonne.

## Sélection et mesure

`SelectableOn` permet à l’utilisateur de sélectionner et de copier un texte, et une `View` avec `Selectable` permet de sélectionner ses textes ensemble, dans l’ordre du document, comme sur une page web. `SelectionColor` colore le surlignage. `OnTextSelection` reçoit un `core.TextSelection` à mesure que l’utilisateur déplace la sélection : son texte brut `Text`, son `Markdown` et son `HTML` (préservant la structure en ligne et en bloc analysée des éventuels `TextView` et textes stylisés de la zone, y compris les spans partiels et les liens), ses `Ranges` par texte et les `Bounds` des textes qu’elle touche, dans la vue, pour une copie en Markdown ou HTML, une liste de mots ou une traduction des mots sélectionnés ; `desktop.LookUp` les affiche dans le dictionnaire du système ([services](services.md#ouvrir-des-liens-et-des-fichiers)). `cx.MeasureText` mesure une chaîne dans une police, pour une colonne ajustée à son texte ; les fonctions réactives qui l’appellent se mettent à jour lorsque les polices disponibles changent, et `OnTextLayout` signale les lignes après chaque mise en page.

## Documents

`base/textview` affiche du Markdown ou du HTML avec les mêmes primitives : titres, paragraphes, listes, tableaux, code, notes de bas de page, alertes, images, formules et liens.

```go
textview.TextView{
	Markdown: "## NSGo\n\nNative views from **Go**.",
	Styles:   ui.TextViewStyles(cx),
	OnLinkPress: func(e textview.LinkPressEvent) {
		if strings.HasPrefix(e.URL, "#") {
			return
		}
		cx.OpenURL(e.URL, nil)
	},
}
```

Les formules (`$…$`, `$$…$$`) affichent leur source TeX tant que l’application ne passe pas `RenderMath: formula.RenderMath` depuis `base/formula`, qui les compose dans la taille et la couleur du texte environnant, lues à haute voix dans la langue de l’application et copiées en TeX ; cela ajoute environ 6,5 Mo à une application de bureau et 3 Mo compressés par gzip à une application web, police incluse. `formula.View` affiche une formule n’importe où ailleurs, en TeX ou en Typst ; son rappel `OnHit` signale la partie source Untie, le curseur le plus proche et les diagnostics qui se chevauchent pour les éditeurs de formules. Sur watchOS, les formules utilisent le même chemin de canevas ; les lier ajoute environ 6,1 Mo d’exécutable, y compris la police de 1,4 Mo. Une exécution sur simulateur Watch a mesuré environ 3,9 Mo de tas Go conservé pour le chargement et l’analyse de cette police, y compris sa copie d’octets détenue. Les mesures sur simulateur n’établissent ni la limite de mémoire d’une montre physique ni sa marge disponible ; mesurez celles-ci sur la montre qui exécute l’application. Les segments de texte (`\text{}` ou chaînes Typst) que ses polices configurées ne peuvent pas couvrir utilisent les polices système, y compris CJK et d’autres écritures ; les valeurs explicites de `formula.Fonts.Fallbacks` ont la priorité.

Pour une source arrivant par morceaux, passez `SourceFunc` à `formula.View` et définissez `Streaming: true`, ou liez `StreamingFunc` à l’état de réception de l’application. Cela masque l’encre des erreurs récupérables et des espaces réservés tandis que le contenu valide continue d’être mis en page. Désactivez l’indicateur une fois la réception terminée pour afficher les éventuelles erreurs restantes. Dans un `TextView`, fournissez une fonction `RenderMath` renvoyant ce `formula.View` avec les champs `Math.TeX` et `Math.Display` fournis ; son `StreamingFunc` peut lire le même état de réception.

Les formules en mode bloc passent à la ligne aux relations et opérateurs sûrs selon la largeur du bloc à l’intérieur de ses marges internes et bordures, en conservant la taille de police du texte. Le redimensionnement ou le changement de la taille du texte réajuste les lignes. Les parties insécables défilent toujours latéralement ; les formules en ligne conservent leur taille naturelle.

Un bloc de code `mermaid` affiche sa source tant que l’application ne passe pas `RenderDiagram: diagram.RenderDiagram` depuis `base/diagram`, qui le dessine comme un diagramme natif, avec limu : organigrammes, diagrammes de séquence, de classes, d’états et entité-association, diagrammes de Gantt, graphiques circulaires, cartes mentales, chronologies et plus encore, dans la police de la plateforme, en clair ou en sombre selon l’affichage de l’application. Les éléments de diagramme dotés de liens les ouvrent via le service d’URL de l’application lors d’un appui, et l’infobulle Mermaid d’un élément apparaît lorsque le pointeur reste immobile dessus. Le diagramme s’adapte à la largeur disponible du texte, faisant passer un large organigramme à la verticale tout en conservant la taille de ses libellés ; les diagrammes qui ne peuvent pas tenir défilent toujours latéralement. Une copie reprend sa source, et un bloc encore en cours de diffusion s’affiche sous forme de code jusqu’à ce qu’il soit complet. `diagram.View` affiche un diagramme n’importe où ailleurs et suit la largeur de son conteneur lorsqu’elle change. Son champ `Icons` associe des noms Mermaid tels que `"fa:fa-car"` à des documents `*core.SVG`, y compris ceux issus d’`icons/lucide` ; les noms absents ne laissent aucun espace vide en ligne. Les nœuds d’image chargent des sources HTTP(S), fichier ou `asset:` via le chargeur d’images partagé du canevas, en ajustant et centrant lorsque `constraint: "on"` est défini et en étirant sinon. Mermaid filtre les adresses d’images avant le chargement (y compris en rejetant les URL `data:`). Pour fournir des icônes dans une vue de texte, définissez `RenderDiagram` sur un rappel renvoyant `diagram.View{Source: code.Code, Icons: icons}`.

Pour donner à vos diagrammes votre palette `ui`, renvoyez un `diagram.View` avec `ThemeFunc` depuis votre rappel `RenderDiagram`. La fonction lit les couleurs au moment où le canevas dessine, de sorte qu’un changement de palette ou d’apparence d’un sous-arbre recolore la vue existante. Les couleurs laissées à zéro conservent les valeurs par défaut du diagramme.

```go
func drawDiagram(cx *core.Scope, code textview.CodeBlock) core.Node {
	colors := ui.ColorsOf(cx)
	return diagram.View{Source: code.Code, ThemeFunc: func() *diagram.Theme {
		return &diagram.Theme{
			Text: colors.Text(), Surface: colors.Surface(), Border: colors.Border(),
			Line: colors.Muted(), Accent: colors.Accent(), Critical: colors.Danger(),
		}
	}}
}
// ui.TextView{Markdown: source, RenderDiagram: drawDiagram}
```

`textview.Parse` lit le Markdown et `textview.ParseHTML` lit le HTML en blocs que l’application peut aussi construire. `Options.Streaming` répare un document qui arrive par morceaux, en fermant ce que sa fin laisse ouvert et en retenant une dernière ligne dont le sens dépend des caractères suivants, comme un marqueur de liste seul, afin qu’une réponse n’affiche jamais brièvement de marqueurs comme du texte, et `StreamingCaret` dessine un curseur à son extrémité, pour une réponse rédigée au fil du flux. `FadeIn` fait apparaître en fondu chaque caractère que le document reçoit, l’un après l’autre et jamais loin derrière son arrivée, pour qu’une réponse arrivant par rafales se lise avec fluidité.

Un `textview.Search` recherche du texte à travers les vues qui s’y rattachent, via leur champ `Search` ou `textview.ProvideSearch` au-dessus d’elles. `SetQuery`, `Open` et `Close` le pilotent depuis une barre de recherche, `Count` et `Selected` indiquent sa position, et `Next` et `Previous` parcourent ses correspondances dans l’ordre de lecture. Les vues marquent chaque correspondance avec `PartSearch` et celle sélectionnée avec `PartSearchSelected`, afin que les styles de l’application la colorent, la fassent défiler dans la zone visible et laissent intacte la sélection de l’utilisateur.

```go
search := textview.NewSearch(cx)
textview.ProvideSearch(cx, search)
search.SetQuery("readme")
search.Open()
```

## Suite

- [Images et icônes](images.md) : images et SVG, également dans un texte.
- [Styles et thèmes](styles.md) : couleurs et champs de texte qui héritent.
- [Mise en page](layout.md) : comment un texte prend sa largeur.

## Édition de texte enrichi

`ui/richeditor.View` affiche une `base/richeditor.Session` avec l’apparence du thème ; `base/richeditor.View` fournit le comportement avec les styles fournis par l’application. Sur iOS et Android, un appui long sélectionne un mot et un double toucher fait de même ; faites glisser les poignées de sélection pour étendre la plage à travers les blocs. Le menu d’édition natif propose Copier et Tout sélectionner, ainsi que Couper et Coller lorsque la session est modifiable, selon ce qui s’applique à la sélection et au presse-papiers. `Session.SetEditable` actualise également un menu ouvert. Stylisez les poignées avec `Styles.SelectionHandle` ou `Classes.SelectionHandle` sur la vue de base.

Pour un contrôle de texte personnalisé, donnez le focus à un `core.Input` et appelez `Ref.OpenEditMenu(bounds, actions)` avec des limites en points de surface et un masque de bits composé de `core.EditCut`, `EditCopy`, `EditPaste` et `EditSelectAll`. Sur iOS et Android, `Input.OnEditAction` reçoit Couper, Copier et Tout sélectionner ; Coller utilise `Input.OnPaste`. Appelez `Ref.CloseEditMenu` pour le fermer.
