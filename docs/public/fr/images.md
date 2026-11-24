---
title: Images et icônes
description: Afficher une image depuis une URL, les ressources de l’application, des octets ou des pixels dessinés par l’application, et dessiner des icônes Lucide et des SVG.
group: Concepts clés
order: 54
---

## Une image

`core.Image` affiche une image matricielle (*bitmap*) ou un SVG : une URL, un fichier, un chemin `asset:` dans les ressources de l’application, ou des octets détenus par l’application. La plateforme la charge, la décode et la met en cache hors du thread principal, et le thread principal dessine le résultat.

```go
core.Image{
	Source: core.Src("https://example.com/cover.jpg"),
	ResizeMode: core.Cover,
	Style: core.Style{Width: core.Pt(160), Height: core.Pt(100), CornerRadius: 12},
}
```

`core.Cover` remplit la boîte et rogne ce qui dépasse ; `core.Contain` ajuste l’image entière, `core.Center` conserve sa taille naturelle, et `core.ScaleDown` la conserve sauf si elle est plus grande que la boîte. `BlurRadius` floute l’image en points, `TintColor` recolore ses pixels opaques, et `ObjectPosition` la positionne dans la boîte.

Les images matricielles mises à l’échelle utilisent par défaut un échantillonnage lissé. Définissez `Sampling: core.ImageSamplingNearest` en Go ou `sampling="nearest"` en TSX pour conserver les bords nets du pixel art et des tampons de pixels. Les plateformes Apple, Windows, Linux, Android et le web prennent en charge ce réglage ; HarmonyOS et les mini-programmes l’appliquent également, bien que leur rendu n’ait pas encore été vérifié sur appareil. Pour obtenir des bords de pixels exacts dans un navigateur utilisant la règle CSS `pixelated`, mettez à l’échelle selon un nombre entier de pixels d’appareil par pixel source.

`Source` déclare la taille utilisée par la mise en page : une dimension permet de déduire l’autre, et une image qui n’en possède pas mesure 0 sur 0 tant que son style ne la dimensionne pas. `core.ImageSource{Data: data, Size: core.Size{Width: 24, Height: 24}}` affiche des octets embarqués avec `go:embed`, et `Size` donne à un SVG sa taille intrinsèque.

Avec [`desktop.WindowPointerPainted`](services.md), `HitTestAlpha: 0.5` laisse passer le pointeur à travers les pixels dont l’opacité est inférieure à la moitié sur macOS et Windows. TSX définit le même seuil avec `hitTestAlpha`.

## Images dessinées par l’application

`Pixels` affiche un `*image.RGBA` dessiné par l’application, comme les images d’un lecteur vidéo ou d’un émulateur, sans rien encoder, charger ni décoder. Écrivez chaque image dans le même tampon et incrémentez `Version` : la vue l’affiche sans événements de chargement ni fondu, en conservant son canal alpha, et la remplace sur place sans image vide intermédiaire sur macOS, iOS, tvOS, watchOS, Android, Windows et Linux. Sans `Size`, ses pixels sont mesurés en points.

```go
frame := image.NewRGBA(image.Rect(0, 0, 960, 540))
version := core.NewSignal(cx, uint64(0))
var tick func(time.Time)
tick = func(at time.Time) {
	if decoder.WriteDue(at, frame) { // écrit l’image suivante dans frame.Pix
		version.Set(version.Get() + 1)
	}
	cx.RequestAnimationFrame(tick)
}
cx.RequestAnimationFrame(tick)

core.Image{ResizeMode: core.Contain, SourceFunc: func() core.ImageSource {
	return core.ImageSource{Pixels: frame, Version: version.Get()}
}}
```

NSGo copie les pixels lors de l’exécution du commit portant une nouvelle `Version`, afin que l’application puisse écrire l’image suivante dès son prochain rappel `RequestAnimationFrame` ; un décodeur dans sa propre goroutine écrit dans un second tampon et soumet l’échange. Une même `Version` n’est pas redessinée. `cx.ImageSize` répond immédiatement à une source de pixels avec la taille de son tampon. Un `core.Canvas` dessine des pixels avec `DrawImage`, copiés de la même manière, sur toutes les plateformes ; les mini-programmes nécessitent un canevas 2D WeChat. Les `Images` d’un `core.Shader` acceptent les pixels de la même façon, permettant ainsi à un shader de filtrer une image écrite par l’application, comme l’écran d’un émulateur, sur toute plateforme prenant en charge les shaders.

Les plateformes Apple, Android, Windows, Linux et le web mettent à jour directement les images de pixels, et les mini-programmes WeChat utilisent un canevas 2D. HarmonyOS crée une `PixelMap` pour chaque version, ce qui a été vérifié sur appareil, et ses copies traversent un pont de chaînes, de sorte que les grands tampons réécrits à chaque image tournent nettement en dessous de 60 ips sur cette plateforme ([tampons de pixels](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/images.md#pixel-buffers)).

## Capturer une vue

Appelez `cx.Capture(&ref, done)` avec la `core.Ref` d’une vue montée, ou `cx.Capture(nil, done)` pour la surface courante. Le rappel reçoit les octets PNG à l’échelle de pixels de la surface ou une erreur sur la goroutine du moteur. Utilisez `core.ImageSource{Data: png}` pour afficher le résultat. Pour le partager, écrivez les octets dans un fichier `.png` dans le répertoire de cache de l’application et passez son chemin dans `core.ShareOptions.Files`. [Partager des fichiers](services.md#ouvrir-des-liens-et-des-fichiers)

La capture prélève les pixels visibles de la surface dans les limites de la vue. Elle est disponible sur macOS, iOS, tvOS, Windows, Linux, Android et le web. Android capture la fenêtre de l’activité. La capture web peint le DOM de l’application dans un canevas, y compris la sortie des shaders NSGo, et omet les cadres intégrés, les images qu’elle ne peut pas lire ainsi que certains effets CSS.

## Ressources et échelles

Les fichiers situés sous `resources/` sont empaquetés avec l’application. `core.Src("asset:images/cover.png")` en charge un, et `Scales: core.ScalesOf(1, 2, 3)` choisit `cover.png`, `cover@2x.png` ou `cover@3x.png` selon le ratio de pixels de l’écran.

```go
core.Image{Source: core.ImageSource{
	URI:    "asset:images/cover.png",
	Scales: core.ScalesOf(1, 2, 3),
}}
```

`Placeholder` affiche une source pendant le chargement de l’image, et `FadeDuration` fait apparaître l’image chargée en fondu par-dessus ; 0 laisse la plateforme choisir, et une nanoseconde l’affiche immédiatement. `OnLoadStart`, `OnProgress`, `OnLoad`, `OnError` et `OnLoadEnd` suivent le chargement. `cx.ImageSize` lit la taille en pixels d’une source sans l’afficher, `cx.PrefetchImage` en télécharge une dans le cache, `cx.SetImageCacheLimit` fixe le budget en mémoire en octets pour les images encodées (par défaut `core.DefaultImageCacheLimit`, 32 Mio ; les valeurs négatives sont ramenées à 0), et `cx.KeepImage` épingle un atlas ou une texture pour l’exclure de l’éviction du cache jusqu’à l’exécution du `release` retourné ou la destruction de son scope. La limite s’applique à chaque cache, hors éléments épinglés, chargements en cours et passe de dessin courante ; elle ne plafonne ni les pixels décodés, ni la mémoire du processus, ni le cache du navigateur. Zéro désactive la rétention des éléments non épinglés. Les backends natifs conservent les images distantes dans leur propre cache disque.

## Images animées et vectorielles

Les animations GIF et WebP sont lues dans la vue, chaque image pendant son propre délai, et bouclent selon les indications du fichier. Les documents SVG se chargent comme des images et restent nets à toute taille ; un SVG monochrome peut être teinté avec `TintColor`.

## Icônes

`core.Icon` dessine un SVG dans la couleur de texte dont il hérite, aussi haut que la taille de police sauf si son style le dimensionne. Sa boîte conserve le ratio d’aspect du SVG, de sorte qu’une boîte carrée ne l’étire pas.

```go
core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 20}, Label: "Search"}
```

`icons/lucide` dispose d’un accesseur par icône Lucide, nommé en PascalCase (`lucide.ArrowUpRight`). Une icône est considérée comme décorative pour les technologies d’assistance à moins que `Label` ne la nomme. `ui.Icon` fonctionne de la même façon avec les valeurs par défaut d’`ui`, et `core.SVG` ainsi que `core.SVGView` dessinent des tracés construits par l’application elle-même.

## L’image stylisée

`ui.Image` est une image dans un cadre arrondi, avec un emplacement réservé pendant son chargement et une discrète icône d’image en cas d’échec. Elle conserve le ratio d’aspect de la source, ou 3:2 à défaut, sauf si son style dimensionne les deux côtés.

```go
ui.Image{
	Source:      core.Src("asset:images/cover.png"),
	Label:       "Cover",
	ResizeMode:  core.Cover,
	AspectRatio: 16.0 / 9.0,
	Placeholder: ui.Skeleton{Style: core.Style{Height: core.Pt(180)}},
}
```

## Suite

- [Texte](text.md) : des images dans une ligne, sous forme de spans.
- [Styles et thèmes](styles.md) : coins, ombres et teintes.
- [Listes et défilement](lists.md) : des rangées d’images recyclées.
