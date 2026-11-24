---
title: Componenti e stato
description: Elementi, componenti, signal e stili.
group: Concetti chiave
order: 50
---

## Elementi

La schermata è un albero di elementi, che sono valori di struct Go. `core` contiene le primitive: `View`, `Text`, `Image`, `Scroll`, `Input` e altre ancora. `ui` offre i controlli usati dalle app, come `Button`, `Select`, `DatePicker` ed `EmojiPicker`, con un aspetto che segue il tema (`base` raccoglie gli stessi controlli privi di stile, per gli [autori di componenti](authoring/index.md)).

```go
core.View{
	Style: core.Style{Padding: core.All(24), Gap: 12},
	Children: []core.Node{
		core.Text{Content: "Hello"},
	},
}
```

`core.WebView` mostra contenuti web; una `WebSource` vuota carica `about:blank` e segnala i consueti eventi di caricamento e navigazione. Imposta `SourceFunc` su una sorgente vuota oppure chiama `Ref.Load(core.WebSource{})` per cancellare la pagina precedente. Imposta `InjectedJavaScriptBeforeContentFunc` quando lo script eseguito prima degli script della pagina segue un signal: quando cambia, la vista ricarica la propria sorgente con il nuovo script, al posto della pagina mostrata al momento, anche se raggiunta dall'utente o tramite `Ref.Load`. Imposta `UserAgentSuffix` (TSX `userAgentSuffix`) per aggiungere un token, come `Version/26.0 Safari/605.1.15`, dopo lo user agent della piattaforma o dopo `UserAgent` se impostato; su macOS e iOS un suffisso aggiunto allo user agent della piattaforma ha effetto solo alla creazione iniziale della vista, mentre HarmonyOS e il web mantengono il proprio user agent. Collega un `core.Ref` per chiamare `Reload`, `GoBack` e `GoForward`. Sui target nativi questi comandi mantengono l'HTML dell'app e le sue sorgenti dati esenti da `core.AccessWebSchemes`; le pagine che avviano nuove navigazioni seguono comunque la policy di accesso.

Su tvOS, `cx.Env().HasWebView()` è falso. Il montaggio di una `WebView` segnala un `*core.WebError` tramite `OnError` senza caricare WebKit; fornisci contenuti nativi dell'app per quel target.

Su macOS, iOS, Windows e Linux, `OnNavigationStateChange` segnala anche le variazioni di URL nello stesso documento, della cronologia e del titolo.

Imposta `WebView.Media` su `core.WebMicrophone`, `core.WebCamera` o entrambi per consentire le richieste `getUserMedia` di una pagina. Dichiara le descrizioni di utilizzo in `apple.info` e `RECORD_AUDIO` o `CAMERA` in `android.permissions`; il sistema chiede comunque il consenso all'utente. Su Android, una richiesta in sospeso viene rifiutata se l'app rimuove uno dei dispositivi richiesti, se la pagina viene abbandonata o se la vista si chiude mentre il prompt di sistema è aperto. La pagina dovrà richiedere nuovamente l'accesso dopo una modifica di questo tipo.

`WebView.Muted` e `MutedFunc` (TSX `muted`) silenziano tutto l'audio della pagina su Windows e Linux, inclusi Web Audio e i frame incorporati. macOS 12 e iOS 15 o successivi sospendono la riproduzione multimediale mentre l'audio è disattivato. Gli altri host mantengono la riproduzione consueta. Gli aggiornamenti si applicano alla pagina corrente e permangono attraverso le navigazioni.

In TSX, una modifica di `injectedJavaScriptBeforeContentLoaded` ricarica la sorgente con il nuovo script nella stessa vista web, come fa `InjectedJavaScriptBeforeContentFunc`; una modifica di `injectedJavaScript` crea una nuova vista web che carica nuovamente la sorgente da zero, così come avviene cambiando `userAgent`, `userAgentSuffix` o `mobile`. Su HarmonyOS, la modifica di uno qualsiasi dei due script iniettati crea una nuova vista web e avvia una nuova cronologia nativa. Su Windows e HarmonyOS ciascuno script iniettato viene eseguito nel frame principale all'interno di un blocco, perciò le sue dichiarazioni `let`, `const` e `class` di primo livello rimangono locali ad esso; assegna a `window` per esporre un oggetto globale visibile dalla pagina.

Imposta `Mobile` (TSX `mobile`) per mostrare una pagina come viene vista su un telefono su Windows, macOS e Linux, utile per siti come YouTube Shorts che cambiano video con scorrimenti verticali solo sui telefoni: la pagina riceve lo user agent di un telefono, a meno che `UserAgent` non sia impostato, e uno schermo touch senza hover, mentre un trascinamento con il mouse o uno scorrimento con rotellina o trackpad su ciò che non scorre arriva alla pagina come lo scorrimento di un dito. Windows emula un telefono come fa la modalità dispositivo di Chrome. macOS e Linux lo fanno tramite uno script, quindi i fogli di stile della pagina vedono comunque un mouse e gli eventi touch risultano non attendibili (untrusted), che una pagina potrebbe ignorare. iOS, Android, HarmonyOS, il web e i mini program mostrano le pagine così come sono.

Su macOS, iOS, Windows e Linux, `OnOpenWindow` riceve le richieste di apertura di una nuova finestra provenienti da una pagina web. La vista non crea popup. Usa `URL` e `UserInitiated` della richiesta per decidere se navigare nella stessa vista con `Ref.Load`, aprire l'URL altrove o ignorarlo. Una vista con restrizioni invia le richieste rifiutate a `OnError`.

Imposta `OnDownload` per salvare i file scaricati dalle pagine di una vista web su macOS, iOS, Linux, Windows e Android. I download vengono salvati in `downloads/<id>/<file name>` all'interno della directory dei dati dell'app e segnalano avvio, completamento con numero di byte o errore; le viste prive di `OnDownload`, o le viste con restrizioni, annullano i download.

`core.Video` riproduce un file o un URL nel player della piattaforma. Su macOS, iOS e tvOS, imposta `Video.AudioSource` su un file o URL separato contenente l'audio per un clip video di durata finita. L'audio del player proviene da quella sorgente. Immagine e suono partono insieme e `Ref.Seek` sposta entrambi sulla stessa timeline. Il video determina la durata della riproduzione e un audio più corto tace al termine.

Un clip trasparente mostra ciò che si trova dietro di esso come HEVC con canale alfa in un file `.mov` su macOS (su iOS non è ancora stato verificato) e come VP9 con canale alfa in WebM su Linux con il plug-in `codecalpha` di GStreamer (`gstreamer1.0-plugins-bad`, che un pacchetto Linux può raccomandare in `nsgo.yaml`). Su macOS, iOS, Windows e Linux, un clip locale il cui canale alfa non può essere mantenuto (WebM su piattaforme Apple, HEVC su Linux e qualsiasi formato su Windows finché l'output alfa di Media Foundation non sarà verificato su una macchina Windows) fa sì che `OnError` lo segnali invece di mostrare il video in un riquadro opaco: l'errore è un `*core.MediaError` con `Reason` pari a `core.MediaErrorAlphaUnsupported`, così l'app può decodificare autonomamente quel clip. Android, il web e gli URL remoti su ogni piattaforma riproducono tali clip così come il rispettivo decoder li emette, risultando potenzialmente opachi.

`OnPause` e `OnPlay` seguono i controlli integrati su Apple, Android, Windows, Linux e web. `OnPause` passa `VideoPauseControls` per una pausa attivata dai controlli. `Paused` si applica all'apertura di una sorgente o alla modifica del suo valore. Le variazioni di volume, velocità e layout mantengono lo stato nativo di pausa o riproduzione, inclusa la riproduzione avviata dai controlli mentre `Paused` rimane vero. Associa `PausedFunc` al tuo stato e aggiornalo in queste callback quando l'app mostra un proprio pulsante di riproduzione.

`Rate` imposta la velocità di riproduzione e `RateFunc` può modificarla durante la riproduzione. Usa `VideoInfo.RateChangeable` ricevuto da `OnLoad` per mostrare un controllo della velocità. Linux può variare la velocità dei video ricercabili quando sono installati GstPlay e il filtro `scaletempo` di GStreamer. Il player di fallback usa 1×.

Imposta `Video.Headers` per inviare header di richiesta HTTP come `Referer`, `User-Agent` o `Cookie` su Android e per file HTTP diretti come MP4 o MOV su Linux, macOS, iOS e tvOS.

```go
core.Video{
    Source:  "https://video.example/clip.mp4",
    Headers: map[string]string{"Referer": "https://video.example/player"},
}
```

Il video copia la mappa al momento del montaggio e utilizza gli stessi header se `SourceFunc` fornisce un nuovo URL. Sulle piattaforme Apple, usa una sorgente con lunghezza in byte nota e supporto agli intervalli (range) per la ricerca nella timeline. Il player invia gli header configurati attraverso i redirect sulla stessa origine. Gli stessi header si applicano anche a un file `AudioSource` HTTP diretto sulle piattaforme Apple. Gli URL di playlist che terminano con `.m3u` o `.m3u8` usano il loader nativo.

Su Linux, installa GstPlay e i plug-in `scaletempo` e `souphttpsrc` di GStreamer. Usa l'URL finale del file multimediale se imposti `User-Agent` o se il tuo `souphttpsrc` non dispone della proprietà `location-trusted`. Con tale proprietà e lo user agent predefinito, GStreamer mantiene gli header dell'app sui redirect della stessa origine. Gli header dell'app si applicano al file sorgente, mentre i segmenti dei flussi adattivi usano richieste proprie.

## Componenti

Un componente è una funzione che riceve uno scope e restituisce un elemento. Viene eseguito una sola volta; ciò che cambia in seguito è guidato dai signal.

```go
func Greeting(cx *core.Scope) core.Node {
	return core.Text{Content: "Hello from NSGo"}
}
```

Un componente con proprietà (props) è un tipo dotato di un metodo `Render`:

```go
type Badge struct{ Text string }

func (b Badge) Render(*core.Scope) core.Node {
	return core.Text{Content: b.Text}
}
```

`ui.ToastContent` può omettere il pulsante di chiusura con `NoClose`, aggiungere un nodo `Trailing` e chiudere il toast tramite `OnPress`. `Toasts.NoClose` nasconde i pulsanti di chiusura su tutto lo stack.

`ui.Toasts.Style` e `StyleFunc` personalizzano ogni superficie del toast, inclusi sfondo, bordo, angoli e padding. Lo stile dello stack si fonde sopra il tema, seguito da `ToastContent.Style` e `StyleFunc` di ciascun toast. Ogni `StyleFunc` viene eseguita dopo il rispettivo `Style` e si aggiorna quando i suoi signal cambiano. Un campo `Color` in questi stili colora il titolo e la descrizione con opacità ridotta, così il testo rimane leggibile su uno sfondo scuro o colorato. Le fasi di ingresso, uscita e swipe applicano comunque i propri stili di movimento. In TSX, imposta la prop `style` di `Toasts` e passa `style` o `styleFunc` a `show` per il singolo toast.

## Signal

Un signal contiene un valore. Leggerlo all'interno di una funzione chiamata da un elemento, come `ContentFunc`, sottoscrive quella parte della schermata e solo quella parte si aggiorna al variare del valore.

```go
count := core.NewSignal(cx, 0)

core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }}

ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }}
```

Un signal su un'intera slice aggiorna ogni riga che lo legge. `core.NewCollection` conserva gli elementi per chiave, ciascuno in un proprio signal, in modo che ogni riga segua il proprio elemento e la modifica di un elemento riesegua una sola riga:

```go
todos := core.NewCollection(cx, func(t Todo) int { return t.ID }, nil)

core.List{Count: todos.Len, Key: func(i int) any { return todos.KeyAt(i) },
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		return core.Text{ContentFunc: func() string { t, _ := todos.At(row.Index()); return t.Title }}
	}}

todos.Update(id, func(t Todo) Todo { t.Done = true; return t })
```

Altre forme di stato sono costruite sui signal allo stesso modo:

- `core.NewSelector` indica se una chiave è quella selezionata ed esegue soltanto le righe che la selezione lascia o raggiunge.
- `core.NewHistory` mantiene un valore con annulla e ripeti, con i relativi signal `CanUndo` e `CanRedo` per un menu Modifica.
- `core.NewPersisted` conserva un valore tipizzato nello storage dell'app come JSON versionato, migrando quanto scritto dalle versioni precedenti.
- `core.From` segue una sorgente esterna al grafo, come una sottoscrizione che consegna dati su un'altra goroutine.

## Stili

`core.Style` segue il modello CSS: flexbox e grid, padding e margini, angoli, bordi, ombre, gradienti, trasformazioni e stati come `Active`. Gli elementi accettano anche classi Tailwind tramite `Class`:

```go
core.Text{Content: "Done", Class: "self-start rounded-full bg-zinc-100 px-3 py-1 text-sm"}
```

I temi conferiscono ai componenti di `ui` il loro aspetto in modalità chiara e scura.

## App desktop

`nsgo.Run(Root)` mostra un'unica vista principale. Un'app desktop può invece aprire autonomamente ciò che mostra: `nsgo.Run(App)` esegue `func App(app *core.Scope)` una volta sola, e questa apre finestre, pannelli, icone nella tray e scorciatoie globali con `desktop`, che ne condividono i signal. Funziona su macOS, Windows e Linux; su macOS `examples/tray` resta fuori dal Dock e vive nella barra dei menu. `nsgo.Run(App, nsgo.StartHidden())` apre la finestra principale senza mostrarla, così un'app nella tray, o avviata dal sistema al login (`desktop.LaunchedAtLogin`), non lampeggia sullo schermo; l'app la mostra poi con `app.MainWindow().Show()`. Telefoni e web mostrano l'app subito all'avvio.

`desktop.SetWindowLevel` mantiene una finestra sopra le altre, come fa un timer o un desktop pet, oppure sotto di esse, mentre `desktop.SetWindowFrame` la sposta e la ridimensiona. `desktop.CanSetWindowLevel` e `desktop.CanPlaceWindows` indicano nello scope di una finestra se queste operazioni hanno effetto: entrambe funzionano su macOS, Windows e Linux sotto X11, e nessuna delle due su una normale finestra toplevel di Wayland, il cui compositore ne gestisce impilamento e posizionamento.

Una finestra aperta con `desktop.WindowOptions{Transparent: true}`, oppure la finestra principale con `nsgo.WindowTransparent()`, non ha sfondo né cornice, lasciando intravedere il desktop ovunque l'app non disegni nulla, come per un desktop pet o un widget; una vista con `Desktop.WindowDrag` permette di trascinarla. `NoShadow` rimuove l'ombra di sistema e `SkipTaskbar` esclude la finestra dalla barra delle applicazioni e da Alt+Tab su Windows nonché dall'elenco finestre su X11, mentre `desktop.SetWindowTransparent`, `SetWindowShadow` e `SetSkipTaskbar` modificano queste opzioni in seguito. Windows consente di cambiare trasparenza solo in una finestra aperta come trasparente, priva di ombra, e Wayland non dispone di un hint per nascondere una finestra dall'elenco applicazioni. `desktop.CanMakeWindowsTransparent` e `desktop.CanSkipTaskbar` indicano nello scope di una finestra se queste opzioni hanno effetto: la trasparenza richiede DirectComposition su Windows e un compositore su Linux, poiché X11 senza compositore non la supporta; su macOS `desktop.SetDockVisible` rimuove l'app dal Dock.

## Terminale

Importa `base/terminal` e crea una sessione con `terminal.New(terminal.Options{Conn: conn})` per un flusso remoto, oppure imposta `Command` per un programma locale su macOS, Linux o Windows. Renderizza `terminal.View{Terminal: session}` e chiama `Close` al termine della sessione. `Options.ImageLimit` limita in byte la memoria per la grafica Kitty: zero usa 64 MiB, un valore negativo disabilita le immagini. I programmi possono inviare immagini RGB, RGBA o PNG e posizionamenti tramite placeholder Unicode.

Chiama `Search(text)` per evidenziare le corrispondenze nello schermo e nello scrollback, `SearchNext` e `SearchPrevious` per selezionarle ed `EndSearch` per ripulirle. `Options.OnSearch` segnala `SearchStatus`; `Theme.Search`, `SearchText`, `SearchSelected` e `SearchSelectedText` impostano i colori delle corrispondenze. Le shell che emettono marcatori OSC 133 supportano Alt/Opzione+clic all'interno dell'input del comando attivo per spostare il cursore. NSGo non installa script di integrazione nella shell.
