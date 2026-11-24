---
title: Canevas et shaders
description: Dessiner des graphismes 2D sur le moteur et remplir une vue avec un shader de fragment GPU.
group: Concepts clés
order: 59
---

## Un canevas

`core.Canvas` enregistre des dessins 2D sur le moteur et les fait rejouer par la plateforme, comme le fait la balise `<canvas>` d'une page web. `Draw` s'exécute à nouveau lorsqu'un signal qu'elle a lu, la taille du canevas ou les polices disponibles changent, et la plateforme dessine l'enregistrement sur le fil principal. Une police chargée via `cx.RegisterFont` relance `Draw` dès qu'elle est disponible, mettant à jour à la fois le texte et la géométrie fondée sur `TextMetrics`.

Utilisez `Em` ou `EmFunc` pour un canevas dimensionné en `em` de sa police héritée. Pour un contenu dont la hauteur dépend de la largeur, `MeasureEm(width, fontSize)` renvoie un `EmBox` pour la largeur disponible en points (`core.Auto` lorsqu'elle est illimitée) et la taille de police résolue. Ses lectures sont suivies et elle s'exécute également pendant la mise en page ; évitez donc d'y écrire dans des signaux. `Ref.ContentBox()` suit la boîte de mise en page d'une vue montée à l'intérieur des marges internes et bordures, dans ses propres coordonnées.

```go
core.Canvas{
	Style: core.Style{Height: core.Pt(220)},
	Draw: func(d *core.Drawing, size core.Size) {
		d.SetFillColor(core.RGB(0xf1f5f9)).FillRect(0, 0, float32(size.Width), float32(size.Height))
		d.SetStrokeColor(core.RGB(0x007aff)).SetLineWidth(2).SetLineCap(core.CapRound)
		d.BeginPath().MoveTo(24, 160).LineTo(90, 80).LineTo(160, 120).LineTo(230, 40).Stroke()
	},
}
```

L'enregistrement suit le `CanvasRenderingContext2D` du web : `Save` et `Restore`, transformations, tracés de lignes, courbes, rectangles et arcs, remplissages et contours avec couleurs et dégradés, découpage (*clipping*), pointillés, ombres, texte et images. `core.ParsePath` crée un `core.Path` à partir de données de chemin SVG, et `FillFromView` et `StrokeFromView` reprennent les styles hérités `Fill` et `Stroke` de la vue canevas, afin qu'un graphique suive le thème de l'application.

Sur le web, les canevas placés dans un `core.Scroll` zoomé ou un `Style.Transform` / `AnimatedStyle` mis à l'échelle sont rejoués à l'échelle cumulée dès que le mouvement s'immobilise, jusqu'à quatre fois le ratio de pixels de l'écran. Un canevas dont l'image zoomée dépasserait 4096 × 4096 pixels ne dessine que sa partie visible et se met à jour au fil du défilement ou du panoramique, gardant traits et glyphes nets sans relancer `Draw`. Apple, Android, Windows et Linux conservent également la netteté des canevas zoomés et du texte sous zoom de défilement comme sous transformations de style.

```go
d.SetFillGradient(core.CanvasLinear(0, 0, float32(size.Width), 0,
	core.GradientStop{Offset: 0, Color: core.RGB(0x4da3ff)},
	core.GradientStop{Offset: 1, Color: core.RGB(0x007aff)},
)).FillRect(0, 0, float32(size.Width), 40)
```

## Texte, images et parties

`MeasureText`, `TextMetrics` et `SetTextAlign` mettent le texte en page via le moteur de texte de la plateforme, afin que les libellés utilisent les mêmes polices que le reste de l'application. `DrawImage`, `DrawImageIn` et `DrawImageFrom` dessinent des images, `DrawSVG` et `DrawSVGIn` dessinent des documents `*core.SVG` analysés (`core.ParseSVG`) à leur taille intrinsèque ou ajustés dans un rectangle — les éléments `<text>` et `<tspan>` étant mesurés et rendus par le moteur de texte propre à la plateforme et `currentColor` provenant de la couleur de remplissage du dessin — et `OnPress` peut tester le dernier dessin avec `IsPointInPath` et `IsPointInStroke`. Les moteurs natifs mettent en cache les 64 sources d'images de canevas non épinglées les plus récemment dessinées ; appelez `release := cx.KeepImage(src)` pour exclure des atlas de sprites ou des textures durables de cette limite d'éviction jusqu'à ce que `release()` s'exécute ou que `cx` soit détruit.

```go
badge := core.MustParseSVG([]byte(`<svg viewBox="0 0 120 40">
	<rect width="120" height="40" rx="8" fill="#007aff"/>
	<text x="60" y="25" text-anchor="middle" font-size="14" fill="#fff">Live</text>
</svg>`))

d.DrawSVGIn(badge, 12, 12, 180, 60)
```

Les images bitmaps redimensionnées utilisent par défaut un échantillonnage lissé. Pour du pixel art, appelez `d.SetImageSmoothing(false)` avant `DrawImage`, `DrawImageFrom` ou `DrawAtlas`, ou définissez `ctx.imageSmoothingEnabled = false` en TSX. `Save` et `Restore` conservent ce réglage avec le reste de l'état de dessin. L'échantillonnage au plus proche voisin est disponible sur les plateformes Apple, Windows, Linux, Android et le web ; HarmonyOS et les mini-programmes l'appliquent aussi, bien que leur rendu n'ait pas encore été vérifié sur appareil.

`d.DrawVertices(image, vertices)` dessine un maillage texturé. Chaque groupe de trois valeurs `core.CanvasVertex` consécutives forme un triangle. Définissez `Position` en points du canevas et `TexCoord` en points de l'image, comme dans `DrawImageFrom`. La couleur `Color` de chaque sommet multiplie la texture et s'interpole sur le triangle. Sa valeur zéro laisse la texture inchangée. Mettez à jour les points et enregistrez à nouveau le dessin pour déformer le maillage. L'image peut être n'importe quel `ImageSource`, y compris un tampon de pixels ou le `Source()` d'une `OffscreenImage` ; les canevas Android et les mini-programmes ne dessinent pas encore d'images hors écran. `SetImageSmoothing(false)` s'applique également aux maillages. Le web exige des images intégrées (*inline*) ou de même origine pour les maillages arbitraires. Les mini-programmes dessinent les maillages avec le lecteur web, ce qui n'a pas encore été vérifié dans WeChat DevTools ni sur appareil. HarmonyOS découpe chaque triangle et dessine l'image via sa transformation affine ; comme pour `DrawImage`, il dessine les images encodées et tampons de pixels mais pas les images hors écran, et applique la transparence moyenne des sommets de chaque triangle mais aucune teinte de couleur. Il ne masque les jointures qu'entre triangles opaques et n'a pas encore été vérifié sur appareil.

`DrawTintedImage`, `DrawTintedImageIn` et `DrawTintedImageFrom` multiplient les canaux sRGB et alpha d'une image par une couleur avant la composition. Avec le mode source-over par défaut, les pixels transparents laissent l'arrière-plan visible. Utilisez `drawTintedImage(image, tint, ...args)` en TSX avec les mêmes formes de coordonnées que `drawImage`. Sur le web, une source d'origine croisée doit autoriser les requêtes CORS anonymes pour que ses pixels puissent être teintés. HarmonyOS teinte les images provenant d'octets, des ressources de l'application et de tampons de pixels, bien que cela n'ait pas encore été vérifié sur appareil ; les images hors écran n'y sont pas encore dessinées.

Utilisez `d.DrawAtlas(source, rects, xforms, alphas)` pour dessiner de nombreux sprites à partir d'une seule image dans l'ordre. Chaque `core.Bounds` sélectionne une tuile dans les propres points de l'image. Le `core.RSXform` correspondant associe le coin supérieur gauche de la tuile à `TX, TY`, avec `SCos` et `SSin` égaux au facteur d'échelle multiplié respectivement par le cosinus et le sinus de la rotation. Par exemple, `core.RSXform{SCos: 0.5, TX: 40, TY: 20}` dessine une tuile de 32 points à demi-taille en (40, 20). Passez `nil` pour une opacité complète des sprites, ou fournissez des facteurs qui multiplient l'alpha de dessin courant.

`Drawing.Node` nomme une partie d'un dessin, telle qu'une barre de graphique ou une région de carte, avec des limites, un rôle, un libellé et un état sélectionné. Les technologies d'assistance lisent ces parties comme les enfants du canevas, et les tests les trouvent par libellé, texte ou identifiant de test ; un appui sur l'une d'elles équivaut à un appui en son centre.

```go
d.Node(core.CanvasNode{
	Bounds: core.Bounds{Origin: core.Point{X: x, Y: y}, Size: core.Size{Width: w, Height: h}},
	Role:   core.RoleButton,
	Label:  "Open the report",
	TestID: "report",
})
```

## Images hors écran

`cx.NewOffscreenImage(size, scale, draw)` effectue le rendu d'un `core.Drawing` une seule fois dans une image hors écran conservée que les images suivantes dessinent avec `DrawImage`, `DrawImageIn`, `DrawImageFrom`, `DrawAtlas` ou `core.Image`. Passer `scale <= 0` utilise le ratio de pixels de l'écran courant. Le dessin n'est recalculé que lorsque `Invalidate()` ou `Resize(size, scale)` est appelé (ou lors de l'enregistrement de nouvelles polices), et est automatiquement libéré à la destruction de `cx` ou lors de l'appel à `Release()`.

```go
tiles := cx.NewOffscreenImage(core.Size{Width: 512, Height: 512}, 0, func(d *core.Drawing) {
	// Drawn once into the offscreen image until tiles.Invalidate() is called.
	for y := range 16 {
		for x := range 16 {
			d.SetFillColor(tileColor(x, y)).FillRect(float32(x*32), float32(y*32), 32, 32)
		}
	}
})

core.Canvas{
	Style: core.Style{Width: core.Pt(320), Height: core.Pt(240)},
	Draw: func(d *core.Drawing, _ core.Size) {
		// Reuses the cached surface in a single DrawImage operation per frame.
		d.DrawImage(tiles.Source(), -cameraX.Get(), -cameraY.Get())
	},
}
```

## Arguments animés

`Drawing.Bind` lie un argument de la dernière opération à un nœud animé, et la plateforme ne redessine que cette partie sur son horloge d'affichage, sans *commit*. `core.ArgX`, `ArgY`, `ArgR`, `ArgA` et `ArgColor` nomment les arguments ; macOS, iOS et Linux ne redessinent que la zone modifiée, tandis que Windows l'enregistre à nouveau et qu'Android et le web redessinent tout le canevas.

```go
needle := core.NewAnimatedValue(cx, 0)
d.Save().Translate(cx, cy).Rotate(0).Bind(core.ArgA, needle).
	BeginPath().MoveTo(-14, -3).LineTo(80, 0).LineTo(-14, 3).ClosePath().Fill().Restore()
```

## Shaders

`core.Shader` remplit sa boîte à l'aide d'un shader de fragment que le GPU exécute pour chaque pixel. `core.CompileShader` prend un programme écrit en syntaxe Go avec une fonction `Fragment`, et `core.CompileWGSL` prend du WGSL. `Uniforms` définit les variables exportées du programme, et `Images` lui fournit jusqu'à quatre textures.

```go
var program = core.MustCompileShader(`
package main

var Tint vec4

func Fragment(dst vec4, pos vec2) vec4 {
	v := 0.5 + 0.5*sin(pos.x/40)
	return Tint * vec4(v, v, v, 1)
}
`)

core.Shader{
	Program:  program,
	Uniforms: core.Uniforms{"Tint": core.RGB(0x007aff)},
	Style:    core.Style{Height: core.Pt(220), CornerRadius: 16},
}
```

Un shader Go reçoit le centre du pixel dans `dst` et le même point en points dans `pos`, et dispose des fonctions intégrées de GLSL ; `rectSize`, `pixelScale`, `image0At` à `image3At` et leurs tailles sont les ajouts propres à NSGo. Un programme WGSL écrit `fn shade(uv: vec2f) -> vec4f` et lit `nsh.time` et `nsh.size` ; `Paused` arrête son horloge. Une plateforme sans chemin GPU pour les shaders n'y dessine rien.

`core.ShaderEffect` dessine ses enfants à travers WGSL et `core.ScreenEffect` recouvre la surface. `core.ShaderTransition` et `core.ScreenTransition` échantillonnent les images avant et après le changement de `Value`. `core.TransitionShader` fait de même pour la navigation et les boîtes de dialogue. Sous Android, capturer des vues pour les effets requiert Android 10 ou ultérieur et OpenGL ES 3.0 avec prise en charge des textures externes. `core.Canvas` et `core.Shader` acceptent tous deux `Label` et leurs enfants se dessinent par-dessus.

## Suite

- [Images et icônes](images.md) : les images qu'un canevas dessine.
- [Animation](animation.md) : les valeurs qu'un canevas lie.
- [Accessibilité](accessibility.md) : les parties qu'un canevas expose.
