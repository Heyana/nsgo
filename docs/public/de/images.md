---
title: Bilder und Icons
description: Ein Bild aus einer URL, den App-Ressourcen, Bytes oder selbst gezeichneten Pixeln anzeigen sowie Lucide-Icons und SVGs zeichnen.
group: Kernkonzepte
order: 54
---

## Ein Bild

`core.Image` zeigt ein Bitmap oder ein SVG an: eine URL, eine Datei, einen `asset:`-Pfad in den App-Ressourcen oder Bytes, die die App vorhält. Die Plattform lädt, dekodiert und cacht es außerhalb des Hauptthreads, und der Hauptthread zeichnet das Ergebnis.

```go
core.Image{
	Source: core.Src("https://example.com/cover.jpg"),
	ResizeMode: core.Cover,
	Style: core.Style{Width: core.Pt(160), Height: core.Pt(100), CornerRadius: 12},
}
```

`core.Cover` füllt die Box aus und schneidet ab, was übersteht; `core.Contain` passt das gesamte Bild ein, `core.Center` behält seine natürliche Größe bei und `core.ScaleDown` behält sie bei, sofern es nicht größer als die Box ist. `BlurRadius` zeichnet das Bild in Punkten weich, `TintColor` färbt seine undurchsichtigen Pixel neu ein und `ObjectPosition` platziert es in der Box.

Skalierte Bitmaps verwenden standardmäßig weiches Sampling. Setzen Sie `Sampling: core.ImageSamplingNearest` in Go oder `sampling="nearest"` in TSX, um die harten Kanten von Pixel-Art und Pixelpuffern zu erhalten. Apple-Plattformen, Windows, Linux, Android und das Web unterstützen diese Einstellung; HarmonyOS und Mini-Programme wenden sie ebenfalls an, allerdings wurde ihre Ausgabe noch nicht auf Geräten geprüft. Für exakte Pixelkanten in einem Browser mit CSS `pixelated` skalieren Sie auf ein ganzzahliges Vielfaches von Gerätepixeln pro Quellpixel.

`Source` deklariert die Größe, die das Layout verwendet: eine Dimension leitet die andere ab, und ein Bild ohne Größenangabe ist 0 mal 0, bis sein Style es bemisst. `core.ImageSource{Data: data, Size: core.Size{Width: 24, Height: 24}}` zeigt mit `go:embed` eingebettete Bytes an, und `Size` gibt einem SVG seine intrinsische Größe.

Mit [`desktop.WindowPointerPainted`](services.md) lässt `HitTestAlpha: 0.5` den Zeiger unter macOS und Windows durch Pixel hindurch, die weniger als halb deckend sind. TSX setzt denselben Schwellenwert mit `hitTestAlpha`.

## Frames, die die App zeichnet

`Pixels` zeigt ein `*image.RGBA` an, das die App zeichnet – etwa die Frames eines Videoplayers oder eines Emulators –, ohne dass etwas kodiert, geladen oder dekodiert wird. Schreiben Sie jeden Frame in denselben Puffer und erhöhen Sie `Version`: Die View zeigt ihn ohne Ladeereignisse und ohne Überblendung an, sein Alpha bleibt erhalten, und unter macOS, iOS, tvOS, watchOS, Android, Windows und Linux wird er an Ort und Stelle ohne leeren Zwischenframe ausgetauscht. Ohne `Size` bemisst es seine Pixel in Punkten.

```go
frame := image.NewRGBA(image.Rect(0, 0, 960, 540))
version := core.NewSignal(cx, uint64(0))
var tick func(time.Time)
tick = func(at time.Time) {
	if decoder.WriteDue(at, frame) { // writes the next frame into frame.Pix
		version.Set(version.Get() + 1)
	}
	cx.RequestAnimationFrame(tick)
}
cx.RequestAnimationFrame(tick)

core.Image{ResizeMode: core.Contain, SourceFunc: func() core.ImageSource {
	return core.ImageSource{Pixels: frame, Version: version.Get()}
}}
```

NSGo kopiert die Pixel, wenn der Commit läuft, der eine neue `Version` trägt, sodass die App den nächsten Frame aus ihrem nächsten `RequestAnimationFrame`-Callback schreiben darf; ein Decoder auf einer eigenen Goroutine schreibt in einen zweiten Puffer und stößt den Tausch an. Dieselbe `Version` wird nicht erneut gezeichnet. `cx.ImageSize` beantwortet eine Pixelquelle sofort mit der Größe ihres Puffers. Ein `core.Canvas` zeichnet Pixel mit `DrawImage`, auf dieselbe Weise kopiert, auf jeder Plattform; Mini-Programme erfordern ein WeChat-2D-Canvas. Die `Images` eines `core.Shader` nehmen Pixel auf dieselbe Weise entgegen, sodass ein Shader einen Frame filtern kann, den die App schreibt – etwa den Bildschirm eines Emulators –, auf jeder Plattform, die Shader zeichnet.

Apple-Plattformen, Android, Windows, Linux und das Web aktualisieren Pixelbilder direkt, und WeChat-Mini-Programme verwenden ein 2D-Canvas. HarmonyOS erstellt aus jeder Version eine `PixelMap`, was auf einem Gerät geprüft wurde, und seine Kopien überqueren eine String-Bridge, sodass große Puffer, die jeden Frame neu geschrieben werden, dort deutlich unter 60 fps laufen ([Pixelpuffer](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/images.md#pixel-buffers)).

## Eine View aufnehmen

Rufen Sie `cx.Capture(&ref, done)` mit dem `core.Ref` einer eingehängten View auf, oder `cx.Capture(nil, done)` für die aktuelle Oberfläche. Der Callback erhält PNG-Bytes in der Pixelskalierung der Oberfläche oder einen Fehler auf der Engine-Goroutine. Verwenden Sie `core.ImageSource{Data: png}`, um das Ergebnis anzuzeigen. Um es zu teilen, schreiben Sie die Bytes in eine `.png`-Datei im Cache-Verzeichnis der App und übergeben Sie deren Pfad in `core.ShareOptions.Files`. [Dateien teilen](services.md#links-und-dateien-öffnen)

Capture erfasst die sichtbaren Oberflächenpixel innerhalb der Grenzen der View. Es ist unter macOS, iOS, tvOS, Windows, Linux, Android und im Web verfügbar. Android erfasst das Fenster der Activity. Die Web-Erfassung zeichnet das DOM der App in ein Canvas, einschließlich NSGo-Shader-Ausgaben, und lässt eingebettete Frames, nicht lesbare Bilder sowie einige CSS-Effekte aus.

## Ressourcen und Skalierungen

Dateien unter `resources/` werden mit der App ausgeliefert. `core.Src("asset:images/cover.png")` lädt eine davon, und `Scales: core.ScalesOf(1, 2, 3)` wählt passend zum Pixelverhältnis des Bildschirms `cover.png`, `cover@2x.png` oder `cover@3x.png`.

```go
core.Image{Source: core.ImageSource{
	URI:    "asset:images/cover.png",
	Scales: core.ScalesOf(1, 2, 3),
}}
```

`Placeholder` zeigt eine Quelle an, während das Bild lädt, und `FadeDuration` blendet das geladene Bild darüber ein; 0 überlässt die Wahl der Plattform, und eine Nanosekunde zeigt es sofort an. `OnLoadStart`, `OnProgress`, `OnLoad`, `OnError` und `OnLoadEnd` verfolgen den Ladevorgang. `cx.ImageSize` liest die Größe einer Quelle in Pixeln, ohne sie anzuzeigen, `cx.PrefetchImage` lädt eine Quelle in den Cache herunter, `cx.SetImageCacheLimit` legt das Budget für kodierte Bilder im Arbeitsspeicher in Bytes fest (Standard `core.DefaultImageCacheLimit`, 32 MiB; negative Werte werden auf 0 begrenzt), und `cx.KeepImage` pinnt einen Atlas oder eine Textur gegen die Cache-Verdrängung fest, bis die zurückgegebene `release`-Funktion ausgeführt oder ihr Scope verworfen wird. Das Limit gilt für jeden Cache, ausgenommen gepinnte Einträge, laufende Ladevorgänge und den aktuellen Zeichenvorgang; es begrenzt weder dekodierte Pixel noch den Prozessspeicher oder das Browser-Caching. Null deaktiviert das Behalten nicht gepinnter Einträge. Native Backends führen für entfernte Bilder einen eigenen Festplatten-Cache.

## Animierte und vektorielle Bilder

GIF- und WebP-Animationen spielen in der View ab, jeder Frame für seine eigene Verzögerungszeit, und wiederholen sich gemäß den Angaben in der Datei. SVG-Dokumente laden als Bilder und bleiben in jeder Größe scharf; ein einfarbiges SVG lässt sich mit `TintColor` einfärben.

## Icons

`core.Icon` zeichnet ein SVG in der Textfarbe, die es erbt, so hoch wie die Schriftgröße, sofern sein Style keine Größe festlegt. Seine Box behält das Seitenverhältnis des SVG bei, sodass eine quadratische Box es nicht verzerrt.

```go
core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 20}, Label: "Search"}
```

`icons/lucide` hat für jedes Lucide-Icon einen Accessor, benannt in PascalCase (`lucide.ArrowUpRight`). Ein Icon gilt für assistive Technologien als Dekoration, sofern `Label` es nicht benennt. `ui.Icon` ist dasselbe mit den Standardwerten von `ui`, und `core.SVG` sowie `core.SVGView` zeichnen Pfade, die die App selbst erstellt.

## Das gestaltete Bild

`ui.Image` ist ein Bild in einer abgerundeten Vertiefung, mit einem Platzhalter während des Ladens und einem blassen Bild-Icon bei einem Fehlschlag. Es behält das Seitenverhältnis der Quelle bei – oder 3:2 ohne eines –, sofern sein Style nicht beide Seiten bemisst.

```go
ui.Image{
	Source:      core.Src("asset:images/cover.png"),
	Label:       "Cover",
	ResizeMode:  core.Cover,
	AspectRatio: 16.0 / 9.0,
	Placeholder: ui.Skeleton{Style: core.Style{Height: core.Pt(180)}},
}
```

## Weiter

- [Text](text.md): Bilder innerhalb einer Zeile, als Spans.
- [Styles und Themes](styles.md): Ecken, Schatten und Einfärbungen.
- [Listen und Scrollen](lists.md): Zeilen von Bildern, die wiederverwendet werden.
