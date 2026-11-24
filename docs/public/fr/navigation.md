---
title: Navigation
description: Piles de navigation natives, en-têtes système, fenêtres modales, menus et barres d’onglets.
group: Concepts clés
order: 56
---

## Une pile

`base.NewNav` maintient une pile de routes d’un même type : `Push` ajoute une page, `Pop` retire la page du dessus, `PopToRoot` revient à la première et `Replace` remplace celle du dessus. Son `Stack` restitue chaque route sous forme de page, en gardant les pages inférieures montées avec leur état pendant que celle du dessus est affichée. `ui.NavPage` attribue à chaque page l’arrière-plan de `ui` et un `ui.NavHeader` comportant son titre, son bouton retour et ses boutons.

watchOS ne dispose ni de barre de navigation système, ni de barre d’onglets, ni d’interrupteur système ; `ui.NavPage` y dessine donc son `ui.NavHeader` au-dessus de la page, bouton retour inclus, `ui.TabShell` dessine sa propre barre d’onglets et `ui.SwitchSystem` correspond à l’interrupteur propre à `ui`.

```go
nav := base.NewNav(cx, "inbox")

return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, route string, depth func() int) base.NavPage {
	if route == "inbox" {
		return ui.NavPage(cx, base.NavPage{Title: "Inbox", Children: []core.Node{
			ui.Button{Text: "Open a message", OnPress: func() { nav.Push("message") }},
		}}, ui.NavHeader{})
	}
	return ui.NavPage(cx, base.NavPage{Title: "Message"}, ui.NavHeader{
		OnBack: func() { nav.Pop(1) }, BackLabel: "Inbox",
		Trailing: []core.Node{ui.IconButton{Icon: lucide.Archive, Label: "Archive", Variant: ui.IconButtonGhost, OnPress: archive}},
	})
})
```

Sur iOS et Android, la plateforme dessine l’en-tête système à partir du titre de la page et des boutons de l’en-tête, avec son bouton retour et sa transition ; sur macOS, Windows, Linux et le Web, c’est `ui` qui le dessine.

Android utilise une barre d’application supérieure Material 3. Une barre standard mesure 64 dp avec un titre de 22 sp. `HeaderLargeTitle` fournit une grande barre de 152 dp avec un titre de 28 sp qui se replie en barre standard lorsque la vue défilante en haut de la page fait défiler son contenu dessous, et se déploie lorsque le contenu revient en haut.

Définissez `HeaderTitleStyle` et `HeaderLargeTitleStyle` pour styliser les polices natives des titres compacts et déployés sur iOS et Android : `FontSize`, `FontWeight`, `FontFamily` et `FontStyle`. Les champs non définis conservent les valeurs par défaut de chaque plateforme ; `HeaderTitleStyleFunc` et `HeaderLargeTitleStyleFunc` suivent les changements. `HeaderStyle` continue de fournir l’arrière-plan, la couleur du titre et la teinte des boutons. En TSX, il s’agit des objets ou tableaux de style `headerTitleStyle` et `headerLargeTitleStyle`.

Un `HeaderTitle` personnalisé remplace uniquement le titre compact sur Android ; `Title` reste le grand titre déployé. Définissez `Android: core.AndroidScreenProps{HeaderRightWithLargeTitle: true}` pour placer `HeaderRight` à côté de ce grand titre. Ses contrôles se déplacent dans la barre d’outils épinglée lorsque le titre se replie et restent activables à tout moment. En TSX, utilisez `android={{ headerRightWithLargeTitle: true }}`. La grande bande s’agrandit lorsque sa police a besoin de plus d’espace. Les compilations Android autonomes activent la mise en page RTL native ; un hôte intégré doit définir `android:supportsRtl="true"` sur son application afin que les en-têtes natifs suivent le sens de lecture de l’application.

Donnez à une page son propre `Header` pour remplacer `ui.NavHeader`. Sur iOS et Android, il devient la vue de titre de l’en-tête système, à la place de `HeaderTitle`, à côté de `HeaderLeft`, de `HeaderRight` et du bouton retour de la page ; il occupe l’espace entre ces extrémités lorsque ses vues s’étendent ou prennent une largeur en pourcentage, comme un champ de recherche, et adopte sinon la taille de son contenu. Ailleurs, il est dessiné au-dessus du contenu. `ui.NavHeader.Scroll` continue de relier le défilement d’une page iOS ou Android au flou de son en-tête.

`NoPopOnBack` conserve une page lorsque le bouton retour est pressé, `OnBackPress` intercepte l’appui et `NoPopOnSwipe` désactive le retour par balayage sur iOS. `cx.OnBackPress(func() bool)` donne à l’application un gestionnaire avant la pile, par exemple pour les modifications non enregistrées. `OnDismiss` signale que l’utilisateur a fermé la page, et la pile la retire.

## Plein écran immersif

Appelez `mobile.Immersive(cx, true)` pour demander le plein écran pour la fenêtre et `mobile.Immersive(cx, false)` pour revenir à ses réglages normaux de barres système. Pour une page ou une boîte de dialogue, utilisez `StatusBar: core.StatusBarSetting{Immersive: core.ToggleOn}` pour le demander tant que cette couche est affichée. En TSX, utilisez `statusBar={{ immersive: true }}` sur une page ou une boîte de dialogue, ou `status_bar: { immersive: true }` dans `nsgo.yaml` pour la valeur par défaut de la fenêtre.

Sur Android, un balayage depuis un bord affiche brièvement les barres d’état et de navigation. Sur HarmonyOS, la fenêtre demande au système de masquer la barre d’état, la barre de navigation à trois boutons et l’indicateur de navigation. Sur iOS, la barre d’état se masque et UIKit décide quand masquer ou révéler l’indicateur d’accueil. `cx.Env().SafeArea()` continue de signaler les marges de sécurité fournies par le système, y compris les découpes d’écran. Sur HarmonyOS avec la navigation à trois boutons en orientation paysage, le système signale la zone des boutons le long du bord inférieur tout en les dessinant sur un côté ; la zone de sécurité peut donc laisser de la place en bas tandis que les boutons recouvrent le côté.

## Fenêtres modales

`ui.Dialog` affiche une surface au-dessus d’une application assombrie, ou une feuille depuis le bas sur les écrans tactiles, et `ui.Sheet` un panneau depuis le bas. Chacun se ferme avec Échap et lors d’un appui sur son arrière-plan, sauf indication contraire.

```go
ui.Dialog{
	Open:         dialog.Get,
	OnOpenChange: func(open bool, _ base.Reason) { dialog.Set(open) },
	Title:        "Rename",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { dialog.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

`ui.AlertDialog` demande confirmation à l’utilisateur, et `cx.ShowAlert` affiche l’alerte ou la feuille d’actions propre au système ([Boîte de dialogue](ui/dialog.md)).

## Menus

`ui.Menu` ouvre des `ui.MenuItem` depuis un bouton : à côté de celui-ci, ou sous forme de feuille depuis le bas lorsque son `Presentation` le demande, sur n’importe quel appareil. `ui.ContextMenu` fait de même lors d’un clic secondaire ou d’un appui long, et `ui.Popover`, `ui.HoverCard` et `ui.Tooltip` affichent d’autres contenus ancrés.

```go
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", OnSelect: duplicate},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## Onglets

`ui.Tabs[T]` est un contrôle segmenté ou une liste d’onglets dotée d’un indicateur qui glisse vers l’option choisie ; `TabStyle` fusionne par-dessus le style de chaque onglet, son `Selected` par-dessus celui de l’onglet choisi, et `IndicatorStyle` par-dessus celui de l’indicateur. Les onglets soulignés peuvent définir `IndicatorColor` et ajuster le trait à leur contenu avec `IndicatorFit: TabsIndicatorContent`. `ui.Sidebar` contient des sections et des lignes pour une barre latérale de bureau, et `ui.TabShell` maintient des sections de premier niveau à côté d’une barre d’onglets (en bas par défaut, ou verticale sur le bord de début ou de fin avec `Edge` / `EdgeFunc`, y compris `mobile.TabBarAutomatic` pour suivre `cx.Env().VerticalBarEdge()`) quelle que soit la largeur de la fenêtre. Il utilise la barre système lorsque chaque onglet possède un `SystemIcon` ou une `Icon` SVG, sur n’importe quel bord sur iOS et sur le bord inférieur sur Android, en affichant le SVG partout où `SystemIcon` ne désigne aucun SF Symbol sur iOS ni aucun drawable sur Android ; sinon, il dessine la barre avec les icônes SVG. Définissez `BarHidden` lorsqu’une page de détail poussée est visible pour masquer l’une ou l’autre barre et restaurer la zone de sécurité de la section ; `TabBarInset` vaut 0 tant qu’elle est masquée ou placée verticalement.

`mobile.TabBar` est la barre d’onglets système sur iOS et Android, avec son apparence Liquid Glass ou Material ; elle prend des `Items` de type `mobile.TabBarItem{Title, Icon, SVG}`, l’index sélectionné, `OnChange`, ainsi qu’un paramètre optionnel `Edge` (`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing` ou `mobile.TabBarAutomatic`, qui suit `cx.Env().VerticalBarEdge()` sur iOS 27.1 et revient au bord inférieur ailleurs) ou un `EdgeFunc` réactif. Les rôles de palette Android `tabBarBackground`, `tabBarIdle`, `tabBarSelected` et `tabBarIndicator` définissent l’arrière-plan de la barre, les icônes et libellés inactifs, l’icône et le libellé sélectionnés, ainsi que la pilule de sélection. Les palettes personnalisées se rabattent respectivement sur `Surface`, `TextMuted`, `Accent` et `accentContainer`, avec une pilule d’accent à 12 % lorsque `accentContainer` n’est pas défini. La palette par défaut conserve les couleurs d’arrière-plan et d’état inactif de Material, avec l’onglet sélectionné teinté par l’accent. Ailleurs, elle n’occupe aucun espace, si bien qu’une application y affiche `ui.Tabs`. `mobile.IsAvailable(cx)` indique si la barre s’affiche. `Icon` nomme une icône de plateforme ; `SVG`, tel que `lucide.House()`, s’affiche sur iOS et Android partout où `Icon` ne désigne aucun SF Symbol ni drawable, teinté par la barre comme l’est une icône système.

Définissez `TabBarItem.Badge` sur `mobile.TabBadge{Text: "3"}` pour un texte ou `mobile.TabBadge{Dot: true}` pour un point. Mettez-le à jour via `ItemsFunc` et renvoyez la valeur zéro pour l’effacer. `ui.ShellTab.Badge` accepte `func() mobile.TabBadge` aussi bien pour la barre système que pour la barre dessinée. Le texte prime lorsque les deux champs sont définis. UIKit affiche un badge natif vierge pour un point. Les technologies d’assistance lisent le texte du badge ou le mot localisé désignant un nouveau contenu. TSX accepte `badge: { text: "3" }` ou `badge: { dot: true }` sur un élément natif, et une fonction renvoyant cette valeur sur un élément `TabShell`.

```go
mobile.TabBar{
	Items: []mobile.TabBarItem{{Title: "Home", Icon: "house"}, {Title: "Search", Icon: "magnifyingglass"}},
	Selected: func() int { return tab.Get() },
	OnChange: func(i int) { tab.Set(i) },
}
```

## Suite

- [Formulaires](forms.md) : ce que contient une page d’une pile.
- [Animation](animation.md) : comment les pages et les fenêtres modales se déplacent.
- [Mise en page](layout.md) : la zone de sécurité autour d’un en-tête.
