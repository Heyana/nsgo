---
title: App TSX
description: Scrivere un'app in TSX con classi Tailwind, compilata in anticipo senza Node.
group: Concetti chiave
order: 63
---

## Un'app TSX

Un'app TSX è un pacchetto Go che incorpora i propri sorgenti ed esegue il proprio punto d'ingresso:

```go
//go:embed *.tsx nsgo.yaml
var src embed.FS

var Root = compiler.App(src, "App.tsx")

func main() { nsgo.Run(Root) }
```

```tsx
import { mount, signal, Text, View } from "nsgo";

function App() {
  const count = signal(0);
  return (
    <View class="gap-3 p-6" onPress={() => count.update((n) => n + 1)}>
      <Text class="text-lg font-bold">Pressed {count()} times</Text>
    </View>
  );
}

mount(App);
```

`go tool nsgo init -template tsx` ne crea una, e `examples/tsx` è un altro esempio. `nsgo dev` e `nsgo build` compilano il TSX in anticipo, quindi i pacchetti non includono alcun compilatore; `go run` e `go test` lo compilano all'avvio dell'app. In nessuno dei due casi serve Node.

## Componenti e moduli

Il TSX importa le primitive da `nsgo`, i componenti di `base` da `nsgo/base`, i controller di gioco da `nsgo/gamepad` (con la capability `gamepad` nei pacchetti firmati), i suoni da `nsgo/audio` (con `audio`) e quelli di `ui` da `nsgo/ui`, una volta che l'app importa `tsx/ui`; `examples/gallery-tsx` li mostra.

Per i diagrammi Mermaid, importa `github.com/nsgo-dev/nsgo/tsx/diagram` per il suo effetto nell'host Go, insieme a `tsx/textview` o `tsx/ui`. Questo aggiunge `Diagram` a `nsgo/base` senza collegare il parser dei diagrammi alle app che non lo usano. Accetta `source`, `fontSize`, un `theme` parziale, `accessibilityLabel` e le consuete prop di stile. Usalo direttamente oppure fornisci un renderer per TextView:

```tsx
import { Diagram, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderDiagram={(code) => <Diagram source={code.code} errorClass="font-mono" />} />
```

La stessa prop `renderDiagram` funziona sulla TextView di `nsgo/ui`. Un blocco di codice non Mermaid mantiene il suo renderer consueto; un diagramma non valido mostra la propria sorgente.

Per la composizione tipografica della matematica, importa `github.com/nsgo-dev/nsgo/tsx/formula` per il suo effetto nell'host Go, insieme a `tsx/textview` o `tsx/ui`. Questo aggiunge `Formula` a `nsgo/base`; le app che omettono il pacchetto tengono il motore matematico e il font fuori dal proprio binario. Accetta `source`, `dialect` (`tex` per impostazione predefinita, oppure `typst`), `display`, `accessibilityLabel` e le prop di stile. La dimensione del font e il colore seguono il testo circostante. Usalo direttamente oppure fornisci un renderer per TextView:

```tsx
import { Formula, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderMath={(math) => <Formula source={math.tex} display={math.display} />} />
```

Lo stesso `renderMath` funziona sulla TextView di `nsgo/ui`. Le formule mantengono il loro TeX quando vengono copiate. Il `Renderer` Go resta una configurazione dell'host.

`UrlScheme.url()` legge l'indirizzo dell'applicazione tracciato; `UrlScheme.setUrl(url, { history: "push" })` aggiunge una voce di indirizzo, oppure la sostituisce quando le opzioni sono omesse. `UrlScheme.on("change", fn)` osserva i comandi indietro e avanti del browser e le modifiche dell'hash, e restituisce una funzione per annullare la sottoscrizione. Consulta [Indirizzi dell'applicazione](addresses.md) per il routing, la validazione della stessa origine e l'hosting dei deep link.

## Go da TSX

Registra il codice Go che il TSX deve importare:

```go
tsx.Register("app/native", map[string]core.Node{"Badge": Badge{}})
tsx.RegisterFunctions("app/native", map[string]any{
	"greet": func(name string) string { return "Hello, " + name },
})
```

`import { Badge, greet } from "app/native"` riceve quindi i campi di `Badge` come prop e chiama `greet`. Le funzioni che accettano un `context.Context` diventano promise, e `tsx.RegisterClass` trasforma i tipi Go in classi che il TSX crea con `new`.

Una prop opzionale inoltrata come `undefined` usa il proprio valore predefinito. Quando registri un componente Go con un getter il cui valore predefinito differisce dal valore zero del suo tipo, aggiungi un'opzione come `tsx.Default("Max", float64(100))`; lo stesso valore predefinito si applica mentre una prop reattiva restituisce undefined, senza sostituire il componente.

Anche i campi dati con valori predefiniti Go documentati possono essere omessi: `width` e `height` di un nodo `Flow` misurano il suo contenuto JSX quando assenti, la `width` omessa di un arco usa il tratto predefinito, e il `pace` omesso di una riga di quota lascia nascosto il relativo indicatore di ritmo.

La registrazione di componenti e classi Go trova i loro campi e metodi esportati tramite reflection mentre l'app è in esecuzione. Questa ricerca impedisce al linker Go di eliminare i metodi esportati che nulla chiama, nell'intera app e non solo in ciò che il TSX registra, quindi collegare `tsx` aggiunge circa 20 MB a un binario desktop privo di simboli, tanto più quanti più pacchetti l'app collega.

## Pacchetti firmati

Un'app può eseguire TSX che scarica, come le proprie schermate distribuite di nuovo o dei plugin, dopo aver verificato il pacchetto. Dichiara ciò che il codice del pacchetto può raggiungere nel suo `nsgo.yaml`; tutto il resto gli viene negato:

```yaml
capabilities:
  network:
    hosts: [api.example.com, "*.cdn.example.com"]
  open_url: true # or { schemes: [https] }
  host: [search, "player.*"] # the app's handlers it may call
  # webview: { hosts: [...], input: true, media: [microphone, camera] }, clipboard, picker, speech, text_to_speech, status_bar, notifications, biometrics, sign_in_with_apple, purchases, secrets, audio, gamepad, auth_session
```

Conserva una chiave radice offline e usala per certificare le chiavi di rilascio, poi firma l'app in quella directory. `go tool nsgo keygen` stampa una coppia di chiavi e non scrive alcun file; eseguilo una volta per la radice e una volta per ciascuna chiave di rilascio. Le chiavi pubbliche vanno nei flag e nel codice dell'app; le chiavi segrete restano fuori dal repository e raggiungono lo strumento solo tramite `NSGO_ROOT_KEY` e `NSGO_RELEASE_KEY`:

```sh
go tool nsgo keygen # once for the root, once for the release key
NSGO_ROOT_KEY=<root secret> go tool nsgo certify -key <release public> -apps 'com.example.*'
NSGO_RELEASE_KEY=<release secret> go tool nsgo publish -cert release.cert plugin
go tool nsgo verify -root <root public> plugin/dist/Plugin-1.0.0.nsgopkg
```

L'app si fida della chiave pubblica radice, scritta nel suo codice, verifica il pacchetto fuori dalla goroutine dell'engine e lo mostra in `tsx.View`, in sandbox accanto alle proprie viste. Il pacchetto raggiunge l'app solo tramite il canale host che la vista gli fornisce, e mantiene una propria porzione dello storage e dei segreti dell'app:

```go
trust, err := pkg.OpenTrust(filepath.Join(cx.DataDir(), "trust"))
trust.AddRoot(root)
p, err := trust.Verify(data) // refuses tampered, revoked and older builds
channel := core.NewHost()
channel.Handle("search", func(c *core.HostCall) { c.Resolve(results(c.Data)) })
view := tsx.View{Package: p, Sandbox: tsx.Sandbox{Host: channel}}
core.Catch{Try: view.Render, Fallback: failed}
trust.Accept(p) // once it ran, older builds are refused
```

`Sandbox.Grant` gli concede meno di quanto dichiara, `Sandbox.Limits` altri budget di tempo e quote, e `Sandbox.Modules` funzioni e componenti Go oltre a quelli di NSGo. Le sue scorciatoie da tastiera, i gestori del tasto indietro e gli stack di navigazione hanno effetto solo mentre il focus della tastiera è nella sua vista, e sposta il focus da sé (`focus()`, `autoFocus`, `Keyboard.dismiss`, `setAccessibilityFocus`) solo mentre il focus è già lì o in risposta a una pressione dell'utente al suo interno. Viene mostrato in una vista propria che riempie lo spazio concesso dal genitore, non di più, e viene ritagliato al suo interno, menu, popover e toast compresi; in una scroll view, inseriscilo in una vista per consentirgli di assumere l'altezza del proprio contenuto. Una sua finestra di dialogo, un suo avviso, un suo menu contestuale o un suo annuncio vengono mostrati solo allo stesso modo, e il tasto indietro chiude una sua finestra di dialogo anche se il pacchetto non la rimuove. `View.Export` indica un componente esportato dal modulo d'ingresso del pacchetto, mostrato al posto della radice montata dal punto d'ingresso, il cui `mount` allora non rende né esegue il proprio componente, e a cui viene passato `View.Props`, una mappa codificabile in JSON copiata per ogni vista, così un pacchetto fornisce più viste, ciascuna con il proprio stato, con le stesse capability e gli stessi limiti; un export mancante genera un panic con `tsx.ErrNoViewExport`, e uno che non è una funzione con `tsx.ErrViewExportNotComponent`. Un pacchetto il cui punto d'ingresso esporta funzioni è un plugin di funzioni: `tsx.Load(cx, p, tsx.Sandbox{})` lo esegue senza mostrarlo e in modo sincrono sulla goroutine del chiamante, con un `cx` che appartiene a un albero di viste (senza di esso restituisce `tsx.ErrNoTree`), oppure restituisce ciò che il suo modulo d'ingresso ha lanciato durante il caricamento, e `Plugin.Call` le chiama. `go tool nsgo dev -limits` esegue un'app come farebbe una vista di questo tipo, e `go tool nsgo revoke` firma un elenco di chiavi di rilascio e pacchetti revocati per `Trust.UpdateRevocations`. `examples/plugins` chiede all'utente di concedere le capability di un pacchetto, lo installa e lo esegue. `Limits.Memory`, 64 MiB per impostazione predefinita, stabilisce il budget dello storage nativo misurato per array, stringhe, buffer e typed array. Ciò include le copie prodotte da `concat`, `flat`, `slice`, spread di array, concatenazione di stringhe, template, `replace`, `replaceAll`, `TextEncoder` e `Blob`. `memory=<bytes>` in `-limits` modifica il budget, e `memory=0` lo disattiva. I controlli vengono eseguiti prima dell'allocazione. Un pacchetto che supera il budget si ferma con `ERR_NSH_QUOTA_EXCEEDED` e la risorsa `memory`, dopo che Go ha verificato la presenza di storage recuperabile dal garbage collector. Anche oggetti ordinari, closure, stack, piccole allocazioni e built-in e API host non misurati possono allocare memoria. Il browser mantiene un proprio heap. Un'app che firma da sé i pacchetti con `pkg.Build`, oppure con `go tool nsgo publish -extra <file>` con i byte di quel file, può inserire metadati propri in `pkg.Contents.Extra`, come ciò che un plugin aggiunge ai suoi menu: vengono firmati insieme al pacchetto, e `Verify` li restituisce invariati come `p.Extra` affinché l'app li analizzi, così l'app li legge senza eseguire il codice del pacchetto.

Un'app può firmare i propri documenti JSON, come un catalogo dei suoi plugin, con `pkg.SignDocument` e verificarli con `Signed.Open`, usando le chiavi stampate da `nsgo keygen`. Assegna a ogni tipo di documento un contesto `app:` proprio con una versione, come `app:catalog-v1`: un documento non viene mai scambiato per un altro tipo, né per un pacchetto, un certificato o un elenco di revoche. `Open` si fida esattamente delle chiavi pubbliche che riceve, senza certificati né elenchi di revoche, e rifiuta i campi del corpo assenti nel tipo in cui decodifica. L'involucro non contiene alcun orario né numero di sequenza, quindi un documento più vecchio viene verificato tanto quanto uno più recente: inserisci una versione o una scadenza nel corpo e controllala.
In una sandbox nativa, `JSON.stringify`, `JSON.parse` con un reviver, `Array.prototype.join`, `toString`, `toLocaleString` e `flat`, e quindi `String()` di un array, lanciano un `RangeError` intercettabile una volta superati i 1.000 livelli di profondità, contando un livello dentro l'altro, dato che il `toString` di un array e il `join` che esso chiama contano ciascuno; dati più profondi bloccherebbero la chiamata o terminerebbero l'app. Il web usa i built-in del browser.

Sui target nativi, `DefaultLimits.Stack` è di 1.000 frame. Imposta `Limits.Stack` o `-limits stack=<frames>` per scegliere una profondità diversa, fino a 10.000 frame. I valori Go pari o inferiori a zero e gli input oltre il limite massimo usano 10.000. Il web usa il limite dello stack del browser.

Le funzioni native che si chiamano a vicenda contano su `Limits.Stack` come le chiamate JS: un getter, `toString` o `valueOf` che sia un built-in, un'espressione regolare il cui `exec` è un altro dei suoi metodi, `Reflect.apply` e `Reflect.construct` l'uno dentro l'altro, e catene di funzioni bound o di proxy lanciano un `RangeError` intercettabile oltre quel limite anziché terminare l'app, e un valore di un plugin annidato più in profondità del limite fa fallire con un `RangeError` la chiamata che lo converte in Go. Questo deriva dal fork di goja richiesto da NSGo, `github.com/nsgo-dev/goja`, che la tua app ottiene insieme a NSGo senza alcuna modifica al suo `go.mod`.

Sui target nativi, `Limits.Call` limita ogni porzione di JavaScript, inclusi i callback dei timer e le continuazioni delle promise dopo `await`. Quando una porzione viene interrotta, un `Plugin.Call` in sospeso riporta `tsx.ErrBudget` una sola volta, ferma i timer rimanenti e ignora le risposte tardive dell'host. Le chiamate indipendenti continuano a essere eseguite, incluse le reazioni delle promise accodate nella porzione interrotta. Una chiamata che risolve la promise di un'altra riporta comunque il proprio risultato o errore. Le espressioni regolari con backtracking usano come timeout di corrispondenza il più breve tra il budget di caricamento e quello di chiamata; anche un timeout non gestito diventa `tsx.ErrBudget`.

`Limits.CallTotal` aggiunge una scadenza in tempo reale per ogni chiamata a un plugin e per ogni evento di una vista in sandbox. Include il tempo trascorso in attesa e si applica ai timer, agli intervalli, ai frame, alle continuazioni delle promise e alle risposte dell'host della chiamata, anche dopo che la chiamata si è conclusa. Alla scadenza il lavoro in sospeso viene scartato e una chiamata ancora in attesa fallisce con `tsx.ErrBudget`. Il lavoro del modulo d'ingresso è soggetto a `Limits.Load`, che usa `CallTotal` se `Load` è zero. Gli eventi host autonomi usano `CallTotal` a partire dalla consegna. Usa `Plugin.CallWithBudget` per ottenere un handle alla scadenza di una chiamata in attesa. Nello `Scope.Post` di un host, chiama `budget.Extend(duration)` prima di quella scadenza per concedere più tempo solo a questa chiamata.

`Plugin.Interrupt` ferma il runtime da qualsiasi goroutine, scarta i timer in sospeso e fa fallire le chiamate correnti e successive. Anche `Plugin.Stopped()` e `Plugin.Err()` possono essere letti da qualsiasi goroutine. `Err()` è nil mentre il runtime è in esecuzione e restituisce il primo motivo di arresto una volta fermato. La chiusura del plugin o l'eliminazione del suo proprietario normalmente esegue le sue operazioni di pulizia e lo ferma con `tsx.ErrPluginClosed`. Imposta `Sandbox.OnStop` quando chiami `tsx.Load` per ricevere quel motivo una sola volta sulla goroutine dell'engine, dopo che le chiamate in attesa sono fallite.

Imposta `Sandbox.OnError` quando carichi un plugin di funzioni per ricevere gli errori non intercettati di timer, frame di animazione, `queueMicrotask` e `reportError` al di fuori di un `Plugin.Call` in attesa. Il gestore di ciascun plugin viene eseguito sulla goroutine dell'engine, così un host può attribuire gli errori e chiamare `Plugin.Interrupt` quando il suo interruttore scatta. Anche le interruzioni native per il budget di porzione riportano `tsx.ErrBudget`, incluso un job di promise la cui chiamata si è già conclusa. I normali errori dei callback lasciano il plugin in esecuzione. Gli errori di caricamento e del risultato delle chiamate continuano a essere restituiti ai rispettivi chiamanti, e le viste continuano a usare `core.Catch`; senza un gestore, resta invariato il comportamento esistente dei boundary e delle segnalazioni dell'app.

Imposta `Sandbox.OnViewRuntime` per ricevere il `*tsx.ViewRuntime` di ciascuna vista sulla goroutine dell'engine prima che venga eseguito il suo modulo d'ingresso. Chiama `Interrupt` da qualsiasi goroutine per fermarne il lavoro e lasciare al suo posto l'ultima vista resa. `Stopped` ed `Err` riportano il suo stato terminale, con `tsx.ErrViewClosed` in caso di eliminazione normale. Anche `Sandbox.OnStop` riporta una sola volta il motivo terminale di una vista.

`Plugin.JSTime` e `ViewRuntime.JSTime` riportano il tempo trascorso negli ingressi JavaScript del runtime nativo, incluso l'ingresso corrente. L'attesa tra un ingresso e l'altro e il tempo trascorso nelle funzioni Go sincrone registrate sono esclusi. `ViewRuntime.EngineTime` include anche le chiamate Go sincrone e il lavoro di reattività, montaggio, commit e layout di sua proprietà, compreso quello ancora in corso. Esclude l'attesa tra un ingresso nell'engine e l'altro, il lavoro che appartiene all'host o a un'altra vista, la contabilità condivisa dei commit e il disegno nativo. Entrambi i contatori possono essere letti in sicurezza da qualsiasi goroutine. I turni JavaScript del browser al di fuori di NSGo non possono essere misurati né interrotti in modo preventivo. Lì entrambi i contatori risultano non disponibili e `Interrupt` ferma i futuri callback di NSGo quando l'host riprende il controllo.

Il web non può ancora isolare un pacchetto dalla pagina: il suo codice raggiungerebbe la rete, gli appunti e lo storage della pagina stessa, compreso quello del trust store, al di là delle sue capability. Lì `tsx.View` e `tsx.Load` rifiutano un pacchetto con `tsx.ErrUnisolated`, a meno che tu non imposti `Sandbox.WebUnisolated` per eseguirlo con l'autorità della pagina, il che è adatto solo a pacchetti a cui affideresti quella dell'app stessa.

## Editor e test

Un `tsconfig.json` che estende `.nsgo/tsconfig.json` fornisce agli editor i tipi del TSX, che vengono scritti da `go tool nsgo dev` e `go tool nsgo check`. I file `*.test.tsx` accanto all'app, scritti con `nsgo/test`, vengono eseguiti insieme ai suoi test Go ([Test](testing.md)).
