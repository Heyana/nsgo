---
title: Immagini e icone
description: Mostra un'immagine da un URL, dalle risorse dell'app, da byte o da pixel disegnati dall'app, e disegna icone Lucide e SVG.
group: Concetti chiave
order: 54
---

## Un'immagine

`core.Image` mostra una bitmap o un SVG: un URL, un file, un percorso `asset:` tra le risorse dell'app, oppure byte in possesso dell'app. La piattaforma la carica, la decodifica e la mette in cache fuori dal thread principale, e il thread principale disegna il risultato.

```go
core.Image{
	Source: core.Src("https://example.com/cover.jpg"),
	ResizeMode: core.Cover,
	Style: core.Style{Width: core.Pt(160), Height: core.Pt(100), CornerRadius: 12},
}
```

`core.Cover` riempie il riquadro e ritaglia ciò che sborda; `core.Contain` fa stare l'intera immagine, `core.Center` ne mantiene le dimensioni naturali e `core.ScaleDown` le mantiene a meno che l'immagine non sia più grande del riquadro. `BlurRadius` sfoca l'immagine in punti, `TintColor` ricolora i suoi pixel opachi e `ObjectPosition` la posiziona nel riquadro.

Le bitmap ridimensionate usano per impostazione predefinita un campionamento morbido. Imposta `Sampling: core.ImageSamplingNearest` in Go o `sampling="nearest"` in TSX per mantenere i bordi netti della pixel art e dei buffer di pixel. Le piattaforme Apple, Windows, Linux, Android e il web supportano questa impostazione; anche HarmonyOS e i mini program la applicano, sebbene il loro risultato non sia ancora verificato sui dispositivi. Per bordi di pixel esatti in un browser che usa il CSS `pixelated`, ridimensiona a un numero intero di pixel del dispositivo per pixel sorgente.

`Source` dichiara le dimensioni usate dal layout: una dimensione ricava l'altra, e un'immagine che non ne ha è di 0 per 0 finché il suo stile non la dimensiona. `core.ImageSource{Data: data, Size: core.Size{Width: 24, Height: 24}}` mostra byte incorporati con `go:embed`, e `Size` assegna a un SVG le sue dimensioni intrinseche.

Con [`desktop.WindowPointerPainted`](services.md), `HitTestAlpha: 0.5` lascia passare il puntatore attraverso i pixel opachi per meno della metà su macOS e Windows. TSX imposta la stessa soglia con `hitTestAlpha`.

## Frame disegnati dall'app

`Pixels` mostra un `*image.RGBA` disegnato dall'app, come i frame di un lettore video o di un emulatore, senza nulla da codificare, caricare o decodificare. Scrivi ogni frame nello stesso buffer e incrementa `Version`: la vista lo mostra senza eventi di caricamento e senza dissolvenza, mantenendone l'alfa, e su macOS, iOS, tvOS, watchOS, Android, Windows e Linux lo sostituisce sul posto senza alcun frame vuoto intermedio. Senza `Size` misura i suoi pixel in punti.

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

NSGo copia i pixel quando viene eseguito il commit che porta una nuova `Version`, quindi l'app può scrivere il frame successivo dalla sua callback `RequestAnimationFrame` seguente; un decoder su una goroutine propria scrive in un secondo buffer e pubblica lo scambio. La stessa `Version` non viene disegnata di nuovo. `cx.ImageSize` risponde subito per una sorgente di pixel con le dimensioni del suo buffer. Un `core.Canvas` disegna pixel con `DrawImage`, copiati allo stesso modo, su ogni piattaforma; i mini program richiedono un canvas 2D di WeChat. Le `Images` di un `core.Shader` accettano pixel allo stesso modo, così uno shader può filtrare un frame scritto dall'app, come lo schermo di un emulatore, su ogni piattaforma che disegna shader.

Le piattaforme Apple, Android, Windows, Linux e il web aggiornano direttamente le immagini di pixel, e i mini program di WeChat usano un canvas 2D. HarmonyOS crea una `PixelMap` per ogni versione, verificato su un dispositivo, e le sue copie attraversano un bridge di stringhe, per cui lì i buffer grandi riscritti a ogni frame girano ben al di sotto dei 60 fps ([buffer di pixel](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/images.md#pixel-buffers)).

## Catturare una vista

Chiama `cx.Capture(&ref, done)` con il `core.Ref` di una vista montata, oppure `cx.Capture(nil, done)` per la superficie corrente. La callback riceve, sulla goroutine del motore, i byte PNG alla scala di pixel della superficie oppure un errore. Usa `core.ImageSource{Data: png}` per mostrare il risultato. Per condividerlo, scrivi i byte in un file `.png` nella directory della cache dell'app e passa il suo percorso in `core.ShareOptions.Files`. [Condividere file](services.md#aprire-link-e-file)

La cattura prende i pixel visibili della superficie entro i limiti della vista. È disponibile su macOS, iOS, tvOS, Windows, Linux, Android e il web. Android cattura la finestra dell'activity. La cattura sul web dipinge il DOM dell'app in un canvas, incluso l'output degli shader di NSGo, e omette i frame incorporati, le immagini che non riesce a leggere e alcuni effetti CSS.

## Risorse e scale

I file in `resources/` vengono distribuiti con l'app. `core.Src("asset:images/cover.png")` ne carica uno, e `Scales: core.ScalesOf(1, 2, 3)` sceglie `cover.png`, `cover@2x.png` o `cover@3x.png` in base al rapporto di pixel dello schermo.

```go
core.Image{Source: core.ImageSource{
	URI:    "asset:images/cover.png",
	Scales: core.ScalesOf(1, 2, 3),
}}
```

`Placeholder` mostra una sorgente mentre l'immagine si carica, e `FadeDuration` fa comparire in dissolvenza l'immagine caricata sopra di essa; 0 lascia scegliere alla piattaforma, e un nanosecondo la mostra subito. `OnLoadStart`, `OnProgress`, `OnLoad`, `OnError` e `OnLoadEnd` seguono il caricamento. `cx.ImageSize` legge le dimensioni in pixel di una sorgente senza mostrarla, `cx.PrefetchImage` ne scarica una nella cache, `cx.SetImageCacheLimit` imposta il budget in byte in memoria per le immagini codificate (predefinito `core.DefaultImageCacheLimit`, 32 MiB; i valori negativi vengono portati a 0), e `cx.KeepImage` blocca un atlas o una texture escludendoli dall'espulsione dalla cache finché non viene eseguito il `release` restituito o il suo scope viene eliminato. Il limite si applica a ogni cache, escludendo gli elementi bloccati, i caricamenti in corso e il passaggio di disegno corrente; non limita i pixel decodificati, la memoria del processo né la cache del browser. Zero disabilita la conservazione degli elementi non bloccati. I backend nativi conservano le immagini remote in una propria cache su disco.

## Immagini animate e vettoriali

Le animazioni GIF e WebP vengono riprodotte nella vista, ciascun frame per il proprio ritardo, e si ripetono come indicato dal file. I documenti SVG si caricano come immagini e restano nitidi a qualsiasi dimensione; un SVG monocromatico può essere colorato con `TintColor`.

## Icone

`core.Icon` disegna un SVG nel colore del testo che eredita, alto quanto la dimensione del font a meno che il suo stile non lo dimensioni. Il suo riquadro mantiene le proporzioni dell'SVG, così un riquadro quadrato non lo deforma.

```go
core.Icon{SVG: lucide.Search, Style: core.Style{FontSize: 20}, Label: "Search"}
```

`icons/lucide` ha un accessor per ogni icona Lucide, con nome in PascalCase (`lucide.ArrowUpRight`). Per la tecnologia assistiva un'icona è decorativa a meno che `Label` non le dia un nome. `ui.Icon` è la stessa cosa con i valori predefiniti di `ui`, e `core.SVG` e `core.SVGView` disegnano tracciati costruiti dall'app stessa.

## L'immagine con stile

`ui.Image` è un'immagine in un riquadro incassato con angoli arrotondati, con un segnaposto durante il caricamento e una tenue icona di immagine quando il caricamento non riesce. Mantiene le proporzioni della sorgente, o 3:2 se non ne ha, a meno che il suo stile non ne dimensioni entrambi i lati.

```go
ui.Image{
	Source:      core.Src("asset:images/cover.png"),
	Label:       "Cover",
	ResizeMode:  core.Cover,
	AspectRatio: 16.0 / 9.0,
	Placeholder: ui.Skeleton{Style: core.Style{Height: core.Pt(180)}},
}
```

## Prossimi passi

- [Testo](text.md): immagini all'interno di una riga, come span.
- [Stili e temi](styles.md): angoli, ombre e tinte.
- [Liste e scorrimento](lists.md): righe di immagini che vengono riciclate.
