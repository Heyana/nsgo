---
title: Widgets
description: Afficher les vues de l’application sur l’écran d’accueil et l’écran verrouillé des iPhone et iPad, ainsi que sur le bureau du Mac.
group: Fonctionnalités plateforme
order: 47
---

## Un type de widget

Les widgets offrent un aperçu de l’application qu’elle soit en cours d’exécution ou non : sur l’écran d’accueil et l’écran verrouillé des iPhone et iPad, sur le bureau du Mac et dans le Centre de notifications, ainsi que dans le lanceur d’Android. Un type (*kind*) correspond à un widget proposé par la galerie du système, avec les familles dans lesquelles il est disponible :

```yaml
widgets:
  - kind: counter
    name: Counter                 # affiché dans la galerie de widgets du système
    description: Shows the count.
    families: [small, medium, circular]  # également large, extra_large, rectangular et inline
# apple: { app_group: group.dev.example.hello }  # l’App Group partagé, group.<id> par défaut
```

Sur les plateformes Apple, une extension de widget le dessine, que `build` et `dev` compilent avec le compilateur Swift de Xcode ([Compilation et distribution](building.md#widgets-et-activités-en-direct)). Sur Android, le runtime Java précompilé dessine la même chronologie sous forme d’App Widgets. Les deux continuent d’afficher le contenu publié lorsque l’application s’arrête.

Pour une compilation Mac App Store en bac à sable (*sandbox*), définissez `apple.app_group` et fournissez les profils d’approvisionnement macOS via `sign.macos_profile` et `sign.widgets_profile`. L’application et `<id>.widgets` doivent toutes deux appartenir à la même équipe et disposer de l’habilitation pour ce groupe. Les chronologies et les images sont placées dans son conteneur partagé ; les compilations macOS directes et de développement conservent leur emplacement actuel dans le répertoire personnel. [Signature pour les boutiques](building.md#paquets-de-boutique) décrit les options de compilation.

`name` et `description` peuvent désigner des messages dans les catalogues de l’application. Dans ce cas, la galerie de widgets et l’espace réservé utilisent la langue de l’utilisateur, en se rabattant sur le texte du catalogue source lorsqu’une traduction manque. Une valeur qui n’est pas une clé de catalogue est affichée telle quelle.

## Une chronologie

`widget.Update` écrit ce que les widgets d’un type affichent à partir de maintenant, sous la forme d’une chronologie d’entrées : le système affiche l’entrée dont la date est passée le plus récemment, puis chaque entrée suivante à sa date, sans l’application. Un `Date` nul correspond à maintenant. Sans entrées, les widgets affichent le nom et la description du type. Sur les plateformes Apple, une application au premier plan met à jour ses widgets aussi souvent qu’elle le souhaite, et une application en arrière-plan quelques dizaines de fois par jour.

```go
err := widget.Update("counter", widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Count", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Text{Content: strconv.Itoa(n), Style: widget.Style{FontSize: 44, FontWeight: core.FontWeightBold}},
	}},
})
```

`Entry.Families` affiche à une famille une vue qui lui est propre à la place de `Entry.View`, `Entry.Background` remplit l’arrière-plan du widget, au-delà des marges conservées par le système, et `Entry.URL` s’ouvre avec l’application lorsqu’on appuie sur le widget. `widget.Update` signale `widget.ErrNoWidgets` dans une application compilée sans widgets, `widget.ErrUnsupported` ailleurs que sur macOS, iOS et Android, et `widget.Available` indique ce qu’il en est avant toute écriture. Elle peut s’exécuter sur n’importe quelle goroutine, y compris avant `nsgo.Run`.

## Vues

Les vues d’un widget ne sont pas les éléments de `core` : elles se disposent comme dans SwiftUI, une vue étant aussi grande que son contenu sauf si `Style` la dimensionne ou la fait remplir l’espace, et un conteneur centrant ce qu’il contient sauf indication contraire de `AlignItems` et `Justify`. `Text`, `Symbol` et `Image` (à partir d’octets ou d’un fichier lu par `Update`) affichent du contenu ; `Column`, `Row`, `Stack`, `Spacer` et `Divider` l’organisent ; `Rectangle`, `Progress` et `Gauge` dessinent ; et `Date` affiche une heure selon les formats du système, que `Relative`, `Offset` et `Timer` continuent de décompter d’eux-mêmes, sans entrée pour chaque seconde.

```go
widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Next hour in", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Date{Time: hour, Format: widget.Timer, Style: widget.Style{TextStyle: widget.Title2, FontVariant: core.FontTabularNums}},
		widget.Progress{Value: 0.4, Style: widget.Style{Color: widget.Accent}},
	}},
}
```

`Style` accepte un espacement, des marges intérieures, une taille ou un remplissage, un alignement, un arrière-plan à coins arrondis, les styles de texte du système ou une taille, graisse, famille typographique et variante de police, des limites de lignes, un `Link` qui s’ouvre avec l’application, et `Accented`, qui place une vue parmi celles que le système teinte. `widget.Primary`, `Secondary` et `Accent` suivent l’apparence, et `RGB` et `RGBA` acceptent une autre couleur en mode sombre via `Dark`.

## Boutons

`widget.Button` exécute `widget.OnAction` à partir d’iOS 17 et de macOS 14, sans ouvrir l’application. Les systèmes plus anciens affichent uniquement ses enfants. Enregistrez le gestionnaire dans `main`, avant `nsgo.Run` : le système peut démarrer l’application en arrière-plan sans ses vues. Un appui sur un widget a `Activity == nil` ; son `Action` est la valeur du bouton.

```go
widget.Button{Action: "refresh", Children: []widget.View{
    widget.Text{Content: "Refresh"},
}}
```

Traitez l’action et appelez `widget.Update` avant que le gestionnaire ne retourne, afin que le rechargement automatique de la chronologie par le système voie le résultat. Le gestionnaire s’exécute en dehors de la goroutine des vues ; utilisez `core.Scope.Post` pour modifier les vues. Cela utilise le même gestionnaire et le même intent que les [Activités en direct](live-activities.md), et ne nécessite que `widgets` dans `nsgo.yaml`, sans `live_activities`.

## Android

Ajoutez le widget depuis le sélecteur de widgets du lanceur et redimensionnez-le à cet endroit. Le moteur de rendu choisit une famille autorisée d’après ses dimensions : small en dessous de 240 points dans les deux dimensions, medium à partir de 240 en largeur, large à partir de 240 en hauteur, et extra-large à partir de 500 en largeur ; si cette famille ne figure pas dans la liste, il utilise la première famille répertoriée. `Entry.URL` et `Style.Link` ouvrent l’application via un schéma qu’elle déclare dans `url_schemes`. Un lien sur une vue imbriquée prime sur le lien de son conteneur.

Les entrées futures de la chronologie utilisent les alarmes inexactes d’Android, que le système peut retarder lorsque l’application est en arrière-plan. Les vues de type minuteur utilisent des chronomètres natifs et décomptent sans l’application ; elles respectent actuellement la taille et la couleur, tandis que la graisse de police personnalisée, la famille typographique et le style italique restent des objectifs à venir. Les dates relatives se rafraîchissent selon le calendrier du système. Le moteur de rendu générique prend en charge le texte, les images, la mise en page, les formes, les barres, les anneaux et un ensemble de glyphes de symboles courants ; utilisez `Image` pour les illustrations devant être identiques sur les deux plateformes. Les noms arbitraires de SF Symbols restent un objectif ouvert sur Android. Les fournisseurs de widgets Android sont générés pour les APK et AAB autonomes ; les hôtes embarqués n’en reçoivent aucun.

## Activités en direct

La même extension dessine les activités en direct de l’application sur l’écran verrouillé et dans la Dynamic Island ([Activités en direct](live-activities.md)).

## Suite

- [Activités en direct](live-activities.md) : ce que l’application suit en temps réel.
- [Notifications](notifications.md) : les notifications et les badges à côté d’un widget.
- [Compilation et distribution](building.md) : le groupe d’applications que l’extension partage.
