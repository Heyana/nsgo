---
title: Widget
description: Mostra le viste dell'app nella schermata Home e nella schermata di blocco di iPhone e iPad, e sulla scrivania del Mac.
group: Funzionalità di piattaforma
order: 47
---

## Un tipo di widget

I widget mostrano uno sguardo d'insieme dell'app, che sia in esecuzione o meno: nella schermata Home e nella schermata di blocco di iPhone e iPad, sulla scrivania del Mac e nel Centro Notifiche, e nel launcher di Android. Un tipo (kind) è un widget offerto dalla galleria del sistema, con le famiglie in cui è disponibile:

```yaml
widgets:
  - kind: counter
    name: Counter                 # shown in the system's gallery of widgets
    description: Shows the count.
    families: [small, medium, circular]  # also large, extra_large, rectangular and inline
# apple: { app_group: group.dev.example.hello }  # the shared App Group, group.<id> by default
```

Sulle piattaforme Apple lo disegna un'estensione per widget, che `build` e `dev` compilano con il compilatore Swift di Xcode ([Compilazione e distribuzione](building.md#widget-e-attività-in-tempo-reale)). Su Android, il runtime Java precompilato disegna la stessa timeline come App Widget. Entrambi continuano a mostrare il contenuto pubblicato quando l'app si ferma.

Per una build sandbox per il Mac App Store, imposta `apple.app_group` e fornisci i profili di provisioning macOS tramite `sign.macos_profile` e `sign.widgets_profile`. Sia l'app sia `<id>.widgets` devono appartenere allo stesso team e avere l'entitlement per quel gruppo. Timeline e immagini vanno nel suo contenitore condiviso; le build macOS dirette e di sviluppo mantengono la posizione esistente nella directory home. [Firma per gli store](building.md#pacchetti-per-gli-store) descrive le opzioni di build.

`name` e `description` possono indicare messaggi nei cataloghi dell'app. In tal caso, la galleria dei widget e il segnaposto usano la lingua dell'utente, ricorrendo al testo del catalogo sorgente quando manca una traduzione. Un valore che non è una chiave di catalogo viene mostrato alla lettera.

## Una timeline

`widget.Update` scrive ciò che i widget di un tipo mostrano da quel momento in poi, come una timeline di voci: il sistema mostra la voce la cui data è trascorsa per ultima, poi ciascuna voce successiva alla sua data, senza l'app. Una `Date` zero corrisponde ad adesso. Senza voci, i widget mostrano il nome e la descrizione del tipo. Sulle piattaforme Apple, un'app in primo piano aggiorna i propri widget tutte le volte che vuole, e un'app in background qualche decina di volte al giorno.

```go
err := widget.Update("counter", widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Count", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Text{Content: strconv.Itoa(n), Style: widget.Style{FontSize: 44, FontWeight: core.FontWeightBold}},
	}},
})
```

`Entry.Families` mostra a una famiglia una vista propria al posto di `Entry.View`, `Entry.Background` riempie il widget dietro di essa, oltre i margini mantenuti dal sistema, e `Entry.URL` si apre con l'app quando si tocca il widget. `widget.Update` segnala `widget.ErrNoWidgets` in un'app compilata senza widget, `widget.ErrUnsupported` altrove che su macOS, iOS e Android, e `widget.Available` indica quale dei due prima di una scrittura. Può essere eseguito su qualsiasi goroutine, anche prima di `nsgo.Run`.

## Viste

Le viste di un widget non sono gli elementi di core: si dispongono come in SwiftUI, con una vista grande quanto il suo contenuto a meno che `Style` non la dimensioni o le faccia riempire lo spazio, e un contenitore che centra ciò che contiene a meno che `AlignItems` e `Justify` non indichino altrimenti. `Text`, `Symbol` e `Image`, da byte o da un file che `Update` legge, mostrano contenuti; `Column`, `Row`, `Stack`, `Spacer` e `Divider` li dispongono; `Rectangle`, `Progress` e `Gauge` disegnano; e `Date` mostra un orario con i formati di sistema, che `Relative`, `Offset` e `Timer` continuano ad aggiornare da soli, senza una voce per ogni secondo.

```go
widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Next hour in", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Date{Time: hour, Format: widget.Timer, Style: widget.Style{TextStyle: widget.Title2, FontVariant: core.FontTabularNums}},
		widget.Progress{Value: 0.4, Style: widget.Style{Color: widget.Accent}},
	}},
}
```

`Style` accetta una spaziatura, un padding, una dimensione o un riempimento, un allineamento, uno sfondo con angoli arrotondati, gli stili di testo di sistema oppure dimensione, peso, design e variante del font, limiti di righe, un `Link` che si apre con l'app e `Accented`, che inserisce una vista tra quelle colorate dal sistema. `widget.Primary`, `Secondary` e `Accent` seguono l'aspetto, e `RGB` e `RGBA` accettano un altro colore in modalità scura con `Dark`.

## Pulsanti

`widget.Button` esegue `widget.OnAction` a partire da iOS 17 e macOS 14, senza aprire l'app. I sistemi precedenti mostrano soltanto i suoi figli. Registra il gestore in `main`, prima di `nsgo.Run`: il sistema può avviare l'app in background senza le sue viste. Una pressione su un widget ha `Activity == nil`; la sua `Action` è il valore del pulsante.

```go
widget.Button{Action: "refresh", Children: []widget.View{
    widget.Text{Content: "Refresh"},
}}
```

Gestisci l'azione e chiama `widget.Update` prima che il gestore ritorni, così il ricaricamento automatico della timeline da parte del sistema vede il risultato. Il gestore viene eseguito fuori dalla goroutine delle viste; usa `core.Scope.Post` per modificare le viste. Usa lo stesso gestore e lo stesso intent delle [Live Activities](live-activities.md), e richiede soltanto `widgets` in `nsgo.yaml`, senza `live_activities`.

## Android

Aggiungi il widget dal selettore dei widget del launcher e ridimensionalo lì. Il renderer sceglie una famiglia consentita in base ai suoi limiti: small sotto i 240 punti in entrambe le dimensioni, medium da 240 in larghezza, large da 240 in altezza ed extra-large da 500 in larghezza; se quella famiglia non è elencata, usa la prima famiglia elencata. `Entry.URL` e `Style.Link` aprono l'app tramite uno schema che dichiara in `url_schemes`. Un link su una vista annidata ha la precedenza sul link del suo contenitore.

Le voci future della timeline usano gli allarmi inesatti di Android, che il sistema può ritardare mentre l'app è in background. Le viste timer usano cronometri nativi e avanzano senza l'app; al momento rispettano dimensione e colore, mentre peso del font personalizzato, design e stile corsivo restano obiettivi. Le date relative si aggiornano secondo la pianificazione del sistema. Il renderer generico supporta testo, immagini, layout, forme, barre, anelli e un insieme di glifi di simboli comuni; usa `Image` per la grafica che deve corrispondere su entrambe le piattaforme. I nomi arbitrari di SF Symbol restano un obiettivo aperto su Android. I provider di widget Android vengono generati per APK e AAB autonomi; gli host integrati non ne ricevono.

## Live Activities

La stessa estensione disegna le Live Activities dell'app sulla schermata di blocco e nella Dynamic Island ([Live Activities](live-activities.md)).

## Prossimi passi

- [Live Activities](live-activities.md): ciò che l'app segue mentre accade.
- [Notifiche](notifications.md): le notifiche e i badge accanto a un widget.
- [Compilazione e distribuzione](building.md): l'app group condiviso dall'estensione.
