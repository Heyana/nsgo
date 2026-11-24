---
title: Stili e temi
description: Stili tipizzati, classi Tailwind, stili per stato e temi chiaro e scuro.
group: Concetti chiave
order: 52
---

## Uno stile

`core.Style` è un unico valore Go per il layout, l'aspetto e il testo di un box. Il valore zero di ogni campo significa non impostato, perciò gli stili si compongono: `Merge` sostituisce solo ciò che lo stile successivo imposta, e un figlio eredita i campi di testo impostati dai genitori. Per azzerare una lunghezza sopra uno stile che ne fornisce una, usa `core.Zero` in `Padding`, `Margin`, `Borders`, `BorderWidth`, `CornerRadius` o in un gap, come fa `core.Transparent` per un colore: `Padding: core.All(core.Zero)`.

```go
card := core.Style{
	Background: core.RGB(0xffffff),
	CornerRadius: 12,
	Padding: core.All(16),
	Gap: 12,
	BorderWidth: 1,
	BorderColor: core.RGBA(0x000000, 0.1),
}
```

Gli stili accettano i colori, gli angoli, i bordi, i contorni, le ombre, i gradienti, i filtri, le sfocature dello sfondo, le maschere, le modalità di fusione, le trasformazioni 3D e le transizioni di CSS. `core.RGB`, `core.RGBA` e `core.ParseColor` creano colori, `core.LinearGradient` e le funzioni affini creano gradienti, `core.Ring` crea il ring di Tailwind e `core.AllCorners` assegna a ogni angolo il suo raggio. `BorderCurveContinuous` raccorda gli angoli ai lati come fa iOS. Le ombre e il contorno di un box restano fuori dal ritaglio del suo `Overflow`, salvo dove indicato in [Layout](layout.md#position). I filtri di sfondo modificano ciò che sta dietro una vista come fa `backdrop-filter` di CSS: un `Blur` lo sfoca secondo il suo raggio e i filtri di colore ne modificano i colori. iOS disegna il materiale di sistema indicato da `View.IOS.BackdropMaterial` per una sfocatura priva di filtri di colore; una sfocatura che indica un materiale diverso da `core.BackdropUltraThin` disegna quel materiale su iOS e il materiale AppKit più vicino su macOS, tralasciando i filtri di colore. Linux e Android non disegnano filtri di sfondo. `Style.GlassEffect` (`core.GlassRegular` o `core.GlassClear`, `glassEffect` in TSX) disegna Liquid Glass al posto di una sfocatura dello sfondo su macOS 26 e iOS 26 (e lo shader glass di NSGo su Windows; assegna a una vista glass una sfocatura da usare come ripiego altrove). Impostare `Style.GlassInteractive` (`glassInteractive` in TSX) abilita la risposta interattiva alla pressione del sistema per Liquid Glass su macOS 27 (`NSGlassEffectView.effectIsInteractive`) e iOS 26 (`UIGlassEffect.isInteractive`), mantenendo funzionanti `OnPress` e i figli premibili.

`Transform` sposta il bersaglio del puntatore insieme alla vista: pressioni, hover e gesti iniziano dove la vista è disegnata, e le posizioni del puntatore riportate annullano la trasformazione. Un figlio trasformato al di fuori del genitore resta raggiungibile, a meno che un ritaglio o il viewport di uno scroll lo nasconda. I mini program non annullano ancora i keyframe eseguiti dal loro livello di rendering ([roadmap](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md#mini-programs)).

```go
core.View{Style: core.Style{
	CornerRadii: core.Corners{TopLeft: 28, BottomRight: 28},
	BorderCurve: core.BorderCurveContinuous,
	Shadows:     []core.BoxShadow{core.Ring(1, core.RGBA(0x000000, 0.08)), ui.Shadow(0, 8, 24, -6, ui.Black(0.18))},
}}
```

## Stati

L'aspetto di una vista può cambiare in base al suo stato. `Hover`, `Focus`, `FocusWithin`, `Active`, `Selected`, `Checked`, `Disabled` e gli stati `aria-*` sono valori `*Style` che si fondono sopra il resto finché lo stato è attivo. Un'app imposta `Selected`, `Checked`, `Disabled` ed `Expanded` tramite funzioni, mentre la piattaforma segnala le pressioni e il focus.

`Active` e `group-active:` seguono una vista mentre è premuta, il che richiede un gestore di pressione o un `Responder` che trattenga il tocco. Applicare `Active`, `active:` o `group` a una vista semplice non la rende premibile né la trasforma in un punto di tabulazione da tastiera; imposta `Focusable` quando deve ricevere il focus essa stessa.

```go
core.View{
	OnPress: func(core.PressEvent) { open() },
	Style: core.Style{
		Background: core.RGB(0x007aff),
		Active:     &core.Style{Opacity: core.Alpha(0.85)},
		Hover:      &core.Style{Background: core.RGB(0x006be0)},
	},
	Disabled: func() bool { return saving },
}
```

Una parte di un componente può assumere gli stati del controllo che la circonda senza segnalarli essa stessa: `PartStates` fornisce `Checked`, `Selected`, `Disabled` e gli altri, così il segno di spunta di un checkbox segue il checkbox anche per gli screen reader.

## Classi

`Class` su `View`, `Text`, `Span`, `Input`, `Image`, `Scroll` e `Canvas` accetta classi Tailwind CSS 4, e ogni parte di un componente `base` ha una classe accanto al suo stile. Utility, varianti e valori arbitrari hanno lo stesso significato che hanno in Tailwind, con il suo tema e la sua palette predefiniti.

```go
core.View{Class: "flex items-center gap-3 rounded-xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"}
```

Una classe si fonde al di sotto dello stile, quindi prevalgono i campi dello stile. Le classi possono seguire ciò che cambia con `ClassFunc`, e una variante di stato (`hover:bg-zinc-100`, `checked:opacity-100`) diventa lo stato dello stile. `group` e `group-hover:` applicano stili ai figli in base allo stato di un genitore, `first:` e `last:` seguono la posizione di un figlio, e `aria-checked:`, `data-open:` e `focus-within:` seguono gli stati mostrati da un controllo. `pointer-fine:` e `pointer-coarse:` applicano stili a un sottoalbero per un mouse o per uno schermo touch, corrispondendo alla media feature `pointer` di CSS tramite `cx.Env().Pointer()` (la query `pointer` del browser sul web, fine su desktop e Mac, coarse su telefoni, tablet e orologi, e nessuna delle due sulle TV), e si combinano con altre varianti come `dark:` e `hover:`, così è l'app a scegliere esplicitamente il dimensionamento per tocco o per puntatore, anziché lasciare che `core` lo modifichi implicitamente.

Contrassegna un controllo con `peer` per applicare stili ai fratelli successivi in base al suo stato, ad esempio un checkbox con `Class: "peer"` seguito da un'etichetta con `Class: "peer-checked:text-accent peer-disabled:opacity-50"`. Un peer applica stili solo ai fratelli che lo seguono sotto lo stesso genitore, compresi `Show` ed `Each`, mai ai fratelli precedenti né alle viste al suo interno. Usa `peer/field` con `peer-checked/field:` per dare un nome a un insieme, e `peer-data-invalid:` per lo stato dati booleano `invalid` di un controllo. Contrassegni e stati si aggiornano in modo reattivo. Contrassegnare una vista non la rende un punto di tabulazione né le fa ricevere pressioni, quindi `peer-active:` segue solo un peer che riceve pressioni da sé, come un pulsante.

`has-[>svg]:px-3` modifica il padding di un genitore quando ha un'icona come figlio diretto; `[&_svg]:size-4 [&_svg]:shrink-0` dimensiona le sue icone discendenti. `svg` seleziona `core.Icon` e `SVGView`, anche attraverso le regioni condizionali. Questi selettori si aggiornano quando cambiano figli, classi o stati. `hover:[&_svg]` segue l'hover del genitore; `[&_svg]:hover` segue quello di ciascuna icona. L'`active` di un selettore segue solo le viste che ricevono pressioni, come un pulsante, così come fa `peer-active:`. I selettori corrispondenti competono per specificità prima che per ordine delle varianti; gli stili inline prevalgono, a meno che la classe sia important.

I selettori corrispondono agli elementi NSGo su ogni target. Supportano liste, i combinatori di figlio (`>`), discendente (`_`), fratello adiacente (`+`) e fratello successivo (`~`), classi, presenza/uguaglianza di `data-*`, ruoli, gli stati di accessibilità booleani supportati e `:is`, `:where`, `:not`, `:has` e le posizioni dei figli. I tipi sono `view`/`div`, `text`/`span` (nodi di testo interi), `scroll`, `input`, `textarea`, `image`/`img`, `canvas`, `svg`, `video`, `webview` e `native`. Usa un solo `&` di primo livello in ogni alternativa di un selettore arbitrario; senza di esso, il selettore verifica l'elemento stesso. Selettori solo per browser, pseudo-elementi, at-rule, escape e `&` espliciti all'interno di una funzione non sono riconosciuti; questi selettori non raggiungono le viste di implementazione del backend né i singoli span di testo.

Le varianti `peer-*` in `Span.Class` o `Screen.HeaderClass` non hanno fratelli da seguire. Vengono ignorate e registrate nel log una volta per classe e contesto; metti la variante peer di uno span sul `Text` che lo contiene.

Il campo `Tailwind` del tema estende il tema predefinito con colori, breakpoint e utility, e un colore assente dalla palette indica un ruolo della `Palette` (`bg-accent`, `text-text-muted/70`) o un colore del tema, letto dal tema del nodo nell'aspetto che mostra.

## Temi

Un `core.Theme` è un unico stile visivo in entrambi gli aspetti: una `Palette` per il chiaro e una per lo scuro, un raggio degli angoli e famiglie di font. I ruoli della palette sono `Background`, `Surface`, `Text`, `TextMuted`, `Border`, `Accent`, `AccentText`, `Danger`, `Warning`, `Success`, `Selection`, `Focus` e `Scrollbar`; `Colors` contiene quelli aggiunti da una libreria, come `inset` e `raised`. `core.DefaultTheme()`, che un'app mostra finché non ne imposta un altro, è il tema neutro con un accento blu e i ruoli di `ui`, e `ui.WarmDuskTheme()` è una palette calda.

```go
theme := core.DefaultTheme()
theme.Light.Accent, theme.Dark.Accent = core.RGB(0xbd481b), core.RGB(0xed986e)
theme.Radius = 10
cx.SetTheme(theme)
```

`cx.SetTheme` assegna un tema allo scope e a tutto ciò che viene costruito al suo interno, così una barra laterale o un pannello possono avere un tema proprio; un tema nil adotta quello superiore. `core.ParseTheme` legge un file di tema YAML o JSON, oppure le variabili CSS di un file di tema shadcn/ui, con i colori dei ruoli, un raggio e famiglie di font. `core.DefaultTheme`, il tema che un'app mostra finché non ne imposta uno, e `ui.WarmDuskTheme` restituiscono temi pronti da modificare.

`cx.SetAppearance` forza l'intera app in chiaro o in scuro, `cx.SetSubtreeAppearance` fa lo stesso per un sottoalbero, e `core.AppearanceSystem` torna a seguire il sistema. `cx.Dark()` indica quale aspetto mostra un sottoalbero, e `cx.Palette()` ne fornisce i colori con applicate le sostituzioni dell'host che incorpora l'app.

Quando nessun antenato imposta `Style.Color`, il testo e le icone `currentColor` usano la `Palette.Text` del sottoalbero, seguendo sul posto le modifiche di tema, aspetto e token dell'host. Un colore esplicito, compreso `core.Transparent`, continua a essere ereditato.

## Colori nei componenti

Un componente legge i propri colori dove viene costruito e nelle sue `StyleFunc`, così un cambio di tema o di aspetto viene seguito sul posto, senza ricostruire il componente. `ui.ColorsOf(cx)` nomina i ruoli usati da `ui`, `ui.TintsOf(cx)` fornisce i loro riempimenti traslucidi e `ui.FinOf(cx)` fornisce le finiture a strati di `ui`.

```go
colors := ui.ColorsOf(cx)
core.Text{Content: "Hello", StyleFunc: func() core.Style {
	return core.Style{Color: colors.Text(), FontSize: 15}
}}
```

`StyleFunc` viene rieseguita quando cambia ciò che ha letto, quindi `cx.Dark()` e `cx.Palette()` al suo interno seguono il tema. Lo stesso vale per `ClassFunc`, per la `StyleFunc` di uno span e per i campi `StyleFunc` di un componente.

## Cursori

`Style.Cursor` imposta un cursore del puntatore con nome (`core.CursorText`, `core.CursorColResize` e così via); lascia gli elementi premibili su `core.CursorAuto`, così mantengono la freccia della piattaforma. Per strumenti di disegno, giochi o puntatori personalizzati su canvas, su desktop e sul web, `Style.CursorImage` accetta un `*core.CustomCursor` con un'`Image` 1x, un'`Image2x` HiDPI facoltativa (scelta automaticamente con `PixelRatio >= 1.5`), un `Hotspot` in punti logici limitato ai bordi dell'immagine e un cursore con nome come `Fallback`. Verifica `cx.Env().SupportsCustomCursor()` per sapere se il target corrente mostra immagini di cursore personalizzate; le piattaforme touch le ignorano.

## Prossimi passi

- [Layout](layout.md): dove vanno i box.
- [Testo](text.md): font, span e decorazioni.
- [Animazione](animation.md): transizioni tra stili.
