---
title: Esecuzione e debug
description: Eseguire un'app su qualsiasi target con hot reload e visualizzare errori, aggiornamenti, layout, prestazioni e altre lingue.
group: Strumenti
order: 20
---

## Esecuzione

```sh
go tool nsgo dev
```

`dev` compila l'app per questa macchina e la avvia, oppure per il target indicato da `dev.target` in `nsgo.yaml`. A ogni salvataggio di un file Go ricompila e riavvia l'app, eseguendo prima `go mod tidy` quando l'app importa un pacchetto il cui modulo manca in `go.mod` o `go.sum`; una modifica solo a file TSX aggiorna l'app in esecuzione sul posto, mantenendone lo stato. L'esecuzione termina insieme all'app.

Su macOS e Windows, una ricompilazione dev nativa riapre la finestra della vista principale nella sua ultima posizione e dimensione regolari all'interno della sessione corrente. Windows ripristina anche la modalità a schermo intero. Su Linux, GTK ripristina la dimensione regolare e richiede l'ultima modalità a schermo intero. La posizione viene scelta dal window manager.

`-target` la esegue altrove, e `-device` seleziona l'iPhone, Apple TV, Apple Watch, simulatore o dispositivo Android per nome, UDID o seriale:

```sh
go tool nsgo dev -target tui                # terminal cells; diagnostics in .nsgo/tui.log
go tool nsgo dev -target web                # in the browser, at the address it prints, on -port (8080 by default)
go tool nsgo dev -target ios_simulator      # in the iOS simulator
go tool nsgo dev -target ios                # on the connected iPhone, signed by Xcode
go tool nsgo dev -target tvos_simulator     # in the Apple TV simulator
go tool nsgo dev -target watchos_simulator  # in the Apple Watch simulator
go tool nsgo dev -target android            # on a connected device or emulator, over adb
go tool nsgo dev -target miniprogram_wechat # in WeChat DevTools (also _qq, _alipay, _douyin)
```

Il server dev ascolta su tutte le interfacce per impostazione predefinita. Usa `-host 127.0.0.1` per limitarlo al loopback, anche quando Android si collega tramite `adb reverse`, oppure indica l'indirizzo IP di un'interfaccia locale per ascoltare solo lì. Usa un indirizzo raggiungibile sulla LAN per le funzionalità dev su dispositivi Apple fisici.

`-h` elenca ogni flag.

## Debugger Go

Aggiungi al `PATH`, con il nome `dlv`, una versione di [Delve](https://github.com/go-delve/delve/blob/master/Documentation/installation/README.md) compatibile con la tua versione di Go. Avvia un'app desktop nativa con le informazioni di debug e con le ottimizzazioni di Go disabilitate.

```sh
go tool nsgo dev -target native -debug
```

Copia la porta dalla riga `API server listening at` di Delve. In VS Code con l'estensione Go, aggiungi una configurazione di attach e sostituisci `40000` con quella porta.

```json
{
  "name": "Attach to NSGo",
  "type": "go",
  "request": "attach",
  "mode": "remote",
  "debugAdapter": "dlv-dap",
  "host": "127.0.0.1",
  "port": 40000
}
```

In GoLand, aggiungi una configurazione di esecuzione **Go Remote** con host `127.0.0.1` e la stessa porta. Imposta un breakpoint nel codice Go della tua app, collegati al processo ed esegui l'azione che lo raggiunge. Dopo una ricompilazione Go riuscita, ricollega il debugger allo stesso indirizzo affinché invii i breakpoint al nuovo processo. Le modifiche TSX continuano a usare l'hot reload.

`-debug` funziona su macOS, Windows e Linux con `-target native`. Usa una build dev per il debug e riserva `-release` per misurare una build di rilascio.

## Anteprime dei componenti

Inserisci una o più funzioni `PreviewXxx` accanto ai componenti in un pacchetto Go importabile. Salvale in un file Go protetto dal build tag per le anteprime.

```go
//go:build nsgopreview

package components

import "github.com/nsgo-dev/nsgo/core"

func PreviewEmpty(cx *core.Scope) core.Node {
    return core.Text{Content: "No items"}
}
```

```sh
go tool nsgo dev -preview ./components
go tool nsgo dev -target web -preview ./components
```

Seleziona le fixture da visualizzare insieme e scegli lingua, tema, aspetto chiaro o scuro, dimensioni del viewport e direzione del layout. L'anteprima utilizza il file `nsgo.yaml` dell'app più vicina, i suoi cataloghi di traduzione, il `theme` configurato e le risorse. Passa la directory di un'app dopo i flag per selezionare una configurazione diversa nello stesso modulo. In tal caso, il percorso dell'anteprima è relativo alla directory dell'app.

Il salvataggio di un file ricompila l'anteprima e mantiene la selezione e le impostazioni di visualizzazione. Il tag `nsgopreview` esclude le fixture dalle normali build di rilascio. Usa la stessa fixture in un file `_test.go` con tag utilizzando `nsgotest.Start(t, PreviewEmpty)` ed eseguilo con `GOFLAGS=-tags=nsgopreview go tool nsgo test -target macos ./components` per acquisire uno screenshot nativo.

Dopo la prima installazione dell'app principale con `-target ios_simulator`, una ricompilazione solo Go sostituisce l'eseguibile installato e riavvia l'app. La modifica di `nsgo.yaml`, delle risorse impacchettate o dell'icona dell'app attiva una reinstallazione.

Usa `go tool nsgo dev -target android -android-host` per lavorare sull'interfaccia Android con un ciclo di ricompilazione Go più rapido. Lo strumento installa il runtime nativo una sola volta ed esegue Go sulla macchina di sviluppo. Ogni ricompilazione Go si ricollega alla stessa Activity tramite adb. Le modifiche a risorse impacchettate, classi precompilate, icone di avvio o `nsgo.yaml` reinstallano l'APK di runtime. Il processo Go usa i vincoli di build, i file e la rete dell'host. `Scope.DiskSpace` segnala il volume che contiene `DataDir` dell'host. Usa il normale comando dev per Android per esercitare i file `_android.go` e il comportamento Go su dispositivo.

## Errori

Una compilazione fallita viene mostrata sopra l'app in esecuzione, con pulsanti per copiare l'errore e compilare di nuovo; correggi il file e il salvataggio successivo la sostituirà. Un componente che va in panic durante il rendering viene mostrato allo stesso modo. Command+R nell'app, oppure Control+R fuori dalle piattaforme Apple, ricompila e riavvia in qualsiasi momento, anche sopra la schermata di errore, a meno che l'app non associ il tasto stesso con `cx.OnKey` o come voce abilitata del menu applicativo; nel browser il tasto ricarica normalmente la pagina. L'output dell'app stessa, come `log.Print`, compare nel terminale sotto le righe dello strumento.

Prima dell'esecuzione, `go tool nsgo check` verifica l'app e controlla i tipi del codice TSX; `-target all` effettua la verifica per ogni target, così anche il codice dietro i build tag di altre piattaforme viene controllato da un Mac.

### Log delle app Apple impacchettate

Su macOS, iOS, tvOS e watchOS, il logger Go predefinito scrive nello unified log di Apple, inclusi `log.Print`, la diagnostica di NSGo e l'output della console TSX. In Console, filtra utilizzando il bundle identifier dell'app in esecuzione come subsystem e `nsgo` come category. Su macOS puoi anche interrogare lo store:

```sh
log show --last 10m --predicate 'subsystem == "dev.example.hello"'
```

I record usano il tipo di log predefinito e mantengono i prefissi di avviso o errore. Il loro testo è pubblico, quindi tieni credenziali e altri segreti fuori dai log. I messaggi lunghi diventano più voci distinte. L'output su terminale rimane disponibile durante lo sviluppo; un log writer personalizzato o un handler `slog` gestisce autonomamente il proprio output.

### Viste web su Linux

Ubuntu 24.04 e versioni successive possono limitare i namespace utente non privilegiati tramite AppArmor. Quando bubblewrap non riesce ad avviare la sandbox di WebKitGTK, NSGo registra l'errore e mostra un segnaposto. Alcune versioni di WebKitGTK terminano invece con `SIGTRAP` all'apertura della prima vista web. [Documentazione AppArmor di Ubuntu](https://documentation.ubuntu.com/security/security-features/privilege-restriction/apparmor/)

Usa il profilo AppArmor per bubblewrap della tua distribuzione se ne fornisce uno, ad esempio `bwrap-userns-restrict`. Altrimenti, un amministratore può caricare un profilo che conceda `userns` a `/usr/bin/bwrap` prendendo come punto di partenza [il profilo della CI Linux di NSGo](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/test.yml). L'amministratore gestisce il profilo perché si applica a ogni app che utilizza il bubblewrap di sistema.

`WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1` disattiva la sandbox dei sottoprocessi di WebKit. Usalo soltanto per lo sviluppo locale con contenuti attendibili.

## DevTools

```sh
go tool nsgo dev -devtools
```

`-devtools` apre DevTools nel browser, servito da `nsgo dev` per ogni target, incluse le app su telefono. Mostra l'app in esecuzione mentre gira:

- **Elements**: le viste in un albero. Ciascuna mostra il componente che l'ha costruita, un collegamento alla sua riga nell'editor, il selettore con cui un test la individua, le sue classi, i suoi limiti (bounds), margine, bordo e padding calcolati dal layout, e lo stile impostato. Passando il mouse su una riga vengono disegnati margine, padding e contenuto nell'app, come nei DevTools del browser, mentre **Pick** consente di toccare o cliccare una vista in qualsiasi finestra, pannello, sheet o vista incorporata per trovarla nell'albero.
- **State**: i signal creati dal tuo codice, sotto i componenti e le computazioni che li possiedono, con i relativi valori, i valori dei memo e quante computazioni leggono ciascuno. Un valore non letto da alcuna computazione non cambia nulla sullo schermo. Stringhe, booleani e numeri sono modificabili, permettendo di provare uno stato senza cambiare codice. I valori creati internamente da NSGo sono raccolti sotto **NSGo's own**.
- **Profiler**: il tempo di ogni commit, suddiviso tra il tuo codice prima del commit, il commit con il suo layout e l'applicazione sul thread principale della piattaforma. Mostra inoltre le righe le cui scritture hanno provocato il commit e si apre sulle computazioni eseguite, dalla più lenta alla più veloce. Un secondo elenco nomina le computazioni eseguite più volte nell'ultimo secondo, ordinate per la riga che le ha create. Il runtime di Android applica i batch nel proprio codice Java, quindi i commit Android non riportano un tempo di piattaforma.
- **Recorder**: ciò che premi, digiti, cancelli e scorri nell'app, scritto come file di test `nsgotest` da copiare in un file `_test.go`. In Elements, **Assert shown** e **Assert text** aggiungono una verifica della vista selezionata. Una verifica del testo memorizza il testo nella lingua in cui l'app lo stava mostrando, quindi registra nella lingua in cui girano i tuoi test.
- **Highlight updates** evidenzia con un contorno ogni vista modificata da un commit, come fa `-highlight`.
- **Layout** mostra e nasconde la [verifica del layout](#layout) sul dispositivo, come fa `-layout` all'avvio.

Stato, computazioni e scritture di un'app TSX sono identificati tramite le rispettive righe TSX, attraverso le source map dei bundle, e collegano direttamente ai file della tua app.

DevTools legge l'app attraverso il server di `nsgo dev`, che ascolta sulla rete affinché i telefoni possano raggiungerlo. Ogni esecuzione genera un token, che gli indirizzi stampati e aperti da `nsgo dev` per DevTools e il target web portano dopo `#`, e il server risponde solo alle richieste che lo contengono: apri quindi DevTools e il target web da quegli indirizzi. Chiunque riceva uno di questi indirizzi può modificare lo stato dell'app mentre gira e aggiungere passaggi al test scritto dal Recorder, perciò rileggi un test registrato prima di eseguirlo. Gli altri dispositivi sulla rete e le pagine web di altri siti visitati non possono accedere a nulla di tutto ciò.

## Aggiornamenti

```sh
go tool nsgo dev -highlight
```

`-highlight` evidenzia per mezzo secondo con un contorno ogni vista modificata da un commit, così un aggiornamento che ridisegna più del dovuto diventa visibile sullo schermo mentre usi l'app. I contorni vengono disegnati sopra ogni finestra, pannello, modale, sheet iOS e vista incorporata (`nsgo.NewView`). DevTools consente di attivarli e disattivarli mentre l'app è in esecuzione.

```sh
go tool nsgo dev -trace
```

`-trace` stampa ogni commit dell'app in esecuzione: il numero, la durata e gli elementi di mount inviati, così vedi quali viste un aggiornamento ha creato, modificato o rimosso. Sopra gli elementi indica da cosa ha avuto origine il commit: le righe del tuo codice che hanno scritto nei signal dall'ultimo commit, a partire da quelle con più scritture, e quanti binding, effect e memo sono stati eseguiti.

```text
── commit 12: 40 items, commit 1.65 ms
   after app/market.go:473 ×4, app/market.go:487 ×4, app/market.go:384
   ran 118 bindings, 18 effects, 5 memos
```

Una riga che scrive molte volte, o molti binding eseguiti per una sola scrittura, indica dove raggruppare le scritture (`cx.Batch`) o restringere ciò che un binding legge. `go tool nsgo test -trace` stampa le stesse informazioni sotto ogni passaggio di un test.

## Profili a runtime

Acquisisci l'attività CPU Go, l'heap o la traccia di esecuzione di un'app in esecuzione con `nsgo dev`. L'app invia la cattura al server dev, che la salva sulla macchina che esegue il comando.

```sh
go tool nsgo dev -target android -profile cpu -profile-duration 10s
go tool pprof -top .nsgo/profiles/cpu.pprof
```

L'esempio CPU funziona anche su desktop, su iPhone o su un simulatore iOS. Su un iPhone, consenti l'accesso alla Rete Locale se il sistema lo richiede. L'heap è un'istantanea e funziona anche sul web. Per gli eventi dello scheduler e delle goroutine, esegui `go tool nsgo dev -profile trace` e apri `.nsgo/profiles/execution.trace` con `go tool trace`. Le tracce CPU ed execution hanno una durata predefinita di 10 secondi. `-profile-output` imposta il percorso del file locale. Per le anteprime dei componenti, il percorso predefinito e i percorsi relativi usano la directory dell'app. Un'esecuzione dev salva la prima cattura completa, quindi continua a far girare l'app.

```sh
go tool nsgo dev -target web -profile heap
go tool pprof -top .nsgo/profiles/heap.pprof
```

Il runtime Go per browser supporta profili heap e tracce di esecuzione. Per il campionamento CPU nel browser, usa gli strumenti Performance del browser. Scegli `-profile` oppure `-release`. La distribuzione con `nsgo build` esclude il client di cattura dev. [Diagnostica del runtime Go](https://go.dev/doc/diagnostics)

## Layout

```sh
go tool nsgo dev -layout
go tool nsgo dev -layout=bounds,grid=4
```

`-layout` avvia l'app con la verifica del layout di DevTools sopra ogni finestra, pannello, sheet iOS e vista incorporata, tracciando il contorno di ogni vista: il box del bordo in blu, il bordo interno del padding in verde e il bordo esterno del margine in arancione, ciascuno tagliato dove una vista scroll o una vista con clipping lo nasconde. `grid` disegna una griglia di 8 punti, e `grid=N` una di N punti, minimo 4. Specifica il valore dopo `=`: il flag da solo equivale a `bounds`.

Un riquadro informativo nell'angolo inferiore mostra le dimensioni della finestra in punti, con pulsanti per attivare e disattivare riquadri, griglia e righello. Su Apple TV e altri dispositivi con solo telecomando, il focus raggiunge i pulsanti del riquadro così puoi attivarli con il telecomando senza uscire dall'app. Mentre il righello è attivo, i clic o tocchi selezionano due punti invece di raggiungere l'app, e il riquadro mostra la distanza orizzontale e verticale tra essi; un terzo tocco ricomincia. Su Apple TV o con una tastiera, le frecce direzionali (o lo scorrimento sul touchpad del Siri Remote) muovono un mirino sullo schermo e Select, Invio o Spazio fissano i punti, aggiornando la distanza in tempo reale finché il secondo punto non la blocca.

| Tasto | Effetto |
| --- | --- |
| `Cmd+Option+Shift+L`, `Ctrl+Alt+Shift+L` | Mostra o nasconde la verifica, in una build con `-layout` |
| Frecce, D-pad del telecomando / swipe | Muove il mirino del righello del passo della griglia (8 pt per impostazione predefinita) |
| `Shift` + Frecce | Muove il mirino del righello di 1 pt |
| Select, `Return`, `Space` | Seleziona il punto del mirino durante la misurazione |
| `Esc`, indietro, Menu | Interrompe la misurazione |

Se la tua app associa lo stesso tasto, prevale l'associazione dell'app. Senza il tasto, ad esempio su un telefono, avvia anche con `-devtools` e mostra o nascondi la verifica tramite **Layout** di DevTools; nascondila per raggiungere un controllo sotto il riquadro informativo. I backend nativi e il web leggono le matrici di presentazione così che bounds, Pick e la verifica seguano trasformazioni visualizzate, animazioni e scorrimento; watchOS segue il proprio disegno affine. HarmonyOS e i mini program mantengono il tracker lato Go finché i rispettivi layer di rendering non segnalano la geometria. Ridisegnarli richiede tempo a ogni aggiornamento, quindi misura la velocità senza di essi.

## Velocità

```sh
go tool nsgo dev -fps
go tool nsgo dev -release -fps
```

`-fps` mostra il monitor delle prestazioni sopra l'app su macOS, iOS, Windows, Linux e Android. Misura frame e commit, e su Android mostra anche il tempo di andata e ritorno del testo nativo. Le build dev includono il client dev; misura con `-release`, che compila come fa `nsgo build`.

## Lingue e aspetto

```sh
go tool nsgo dev -lang zh-Hans
go tool nsgo dev -lang en-XA   # accented and longer, to find untranslated and cut text
go tool nsgo dev -lang ar-XB   # right to left
```

`-lang` avvia l'app in una specifica locale, inclusa una pseudo-locale ([Localizzazione](localization.md)).

## Agenti

```sh
go tool nsgo dev -automation
go tool nsgo dev -target android -automation
go tool nsgo dev -target web -automation
```

`-automation` consente a un agente IA o a un runner end-to-end di usare l'app in esecuzione come farebbe una persona, finora su macOS, Windows, Linux (`-target native`), Android (`-target android`), web (`-target web`), mini program nei simulatori dei rispettivi strumenti di sviluppo (`-target miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay` o `miniprogram_douyin`) e dispositivi e simulatori Apple (`-target ios`, `tvos`, `watchos`, `ios_simulator`, `tvos_simulator` o `watchos_simulator`): elencare le finestre, descrivere ciò che mostrano, premere, digitare, inviare tasti, scorrere, trascinare, passare con il cursore, scegliere voci di menu, rispondere agli avvisi, attendere ciò che appare e acquisire screenshot. Gli agenti accedono tramite MCP attraverso `go tool nsgo mcp`, che usa l'istanza `dev -automation` in esecuzione per la directory, oppure ne avvia una con gli stessi flag (come `-target android`) arrestandola alla chiusura del client:

```sh
claude mcp add nsgo -- go tool nsgo mcp
```

Altri client accettano lo stesso comando, `go` con gli argomenti `tool nsgo mcp`, eseguito nella directory dell'app. Le viste sono identificate tramite i selettori dei [test](testing.md#un-test), oppure tramite i riferimenti `view:` stampati da uno snapshot, che un salvataggio rende obsoleti riavviando l'app.

L'agente opera sui dati reali dell'app, così come la esegue `dev`. Solo le build `dev -automation` includono questa funzionalità, e solo gli strumenti su questa macchina in possesso del token in `.nsgo/automation.yaml`, leggibile solo da te, possono raggiungerla.

### Strumenti MCP definiti dall'app

Chiama `automation.Register` da un'app o libreria Go sulla goroutine del motore. Ad esempio, questa funzione espone agli agenti l'ID della scena attualmente mostrata.

```go
import (
    "context"
    "encoding/json"

    "github.com/nsgo-dev/nsgo/automation"
    "github.com/nsgo-dev/nsgo/core"
)

func registerSceneTool(cx *core.Scope, sceneID *core.Signal[string]) error {
    return automation.Register(cx, automation.Tool{
        Name:        "current_scene",
        Description: "Read the ID of the active scene",
        InputSchema: json.RawMessage(`{"type":"object"}`),
        Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
            return map[string]string{"id": sceneID.Get()}, nil
        },
    })
}
```

Il nome deve essere univoco nel grafo dell'app e distinto dai nomi integrati. `nsgo mcp` elenca lo strumento finché lo scope non viene ripulito. L'handler viene eseguito sulla goroutine del motore, quindi può leggere o impostare signal. Riceve un oggetto JSON, verifica gli argomenti necessari e restituisce un valore serializzabile in JSON o un errore.

In un test, acquisisci lo scope all'avvio dell'app, chiama `automation.Call(cx, ctx, name, args)` dentro `app.Post`, quindi usa `app.Settle()` prima di verificare il risultato o la vista. La registrazione è attiva in `dev -automation` e nei test, ed è un no-op nelle build di rilascio.

## Flag di ogni build

Ogni build di un'app, inclusi i pacchetti distribuiti, accetta all'avvio i flag propri di NSGo, che sovrascrivono `nsgo.yaml` e il codice dell'app e non raggiungono mai i flag dell'app stessa:

| Flag | Effetto |
| --- | --- |
| `--nsgo-fps` | Mostra il monitor delle prestazioni su macOS, iOS, Windows, Linux e Android |
| `--nsgo-lang=zh-Hans` | Avvia l'app in una locale, anche una pseudo-locale |
| `--nsgo-appearance=dark` | Avvia l'app in modalità `light`, `dark` o seguendo `system` |
| `--nsgo-os=ios` | Esegue una build web come `macos`, `windows`, `linux`, `ios` o `android`, con `web` per sé stessa |

```sh
./Hello --nsgo-fps                                   # Windows and Linux
open Hello.app --args --nsgo-fps                     # macOS
xcrun simctl launch booted dev.example.hello --nsgo-fps
adb shell am start -n dev.example.hello/dev.oneo.nsgo.NsgoActivity --ez nsgo-fps true --es nsgo-lang de
```

Sul web vanno nella query della pagina: `?nsgo-fps&nsgo-lang=de`.

`--nsgo-os` mostra l'anteprima del layout di un'altra piattaforma in un browser: l'app legge quel sistema operativo e il relativo idioma visivo, un telefono per iOS e Android e un desktop altrimenti, e `Env.Simulated` restituisce vero. Il browser non dispone di alcuna parte nativa di quella piattaforma: `NavHeader` e la barra delle schede disegnata prendono il posto di quelli di sistema, non ci sono menu applicativo, menu popup nativo (`desktop.PopupMenu`) né funzioni di finestra, e `mobile.IsAvailable` e `desktop.IsAvailable` sono falsi. I menu disegnati come `ui.ContextMenu` e `ui.Menu` continuano a funzionare. Le scorciatoie seguono la tastiera dell'utente. Le altre build ignorano il flag e registrano un avviso.
