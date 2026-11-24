---
title: Configurazione
description: Installa ciò che serve a NSGo, crea un'app ed eseguila su ogni target.
group: Per iniziare
order: 10
---

## Requisiti

- [Go](https://go.dev/dl/) 1.27.3 o successivo.
- Per compilare per iOS, tvOS e watchOS, un Mac con Xcode. Tutti gli altri target si compilano da macOS con il solo Go; quello per l'emulatore Android x86_64 richiede anche un Android NDK.
- Per eseguire su un iPhone, un Apple ID negli Account di Xcode e l'iPhone autorizzato come attendibile, con la Modalità sviluppatore attiva.
- Per eseguire su Android, `adb` dai platform-tools dell'Android SDK.
- Per eseguire su Windows, il solo Go, che compila anche i pacchetti per Windows e per il web.
- Su Linux, GTK 4.14 o successivo, Pango e Cairo. Le immagini di rete usano libsoup 3, le web view usano WebKitGTK 6.0, mentre audio e video usano i plug-in di GStreamer. Sui sistemi con restrizioni AppArmor sui namespace utente, [WebKitGTK richiede un profilo bubblewrap](debugging.md#linux-web-views).

## Creare un'app

Crea un modulo, aggiungi NSGo e il suo strumento, poi lascia che lo strumento scriva un'app:

```sh
mkdir hello && cd hello
go mod init example.com/hello
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
```

`go get -tool` aggiunge `nsgo` al modulo una volta sola, così `go tool nsgo` lo esegue senza installare nulla. `init` scrive:

| File | Che cos'è |
| --- | --- |
| `app.go` | L'app, un contatore: un componente `Root` e `main`, che chiama `nsgo.Run(Root)` |
| `app_test.go` | Un test che preme il pulsante |
| `nsgo.yaml` | Il nome, l'id, la versione e i target dell'app |

`go tool nsgo init -template tsx` scrive invece un'app TSX, e `-template mixed` un'app Go il cui punto di ingresso TSX usa componenti Go propri.

### Template da moduli Go

Passa il percorso del modulo del template con una versione facoltativa. Se la versione è omessa, viene risolta nell'ultima versione che il proxy dei moduli di Go riesce a trovare.

```sh
go tool nsgo init -template example.com/my-template@v1.2.0 -module example.com/hello hello
```

Pubblica lo starter nella directory `template/` del modulo. Includi `nsgo.yaml` e `main.go`. Lo strumento elabora entrambi, più ogni file che termina in `.tmpl`, con il `text/template` di Go, e rimuove `.tmpl` dai nomi in uscita. Gli altri file, comprese le risorse binarie, vengono copiati senza modifiche.

I template ricevono `.Name` (il nome visualizzato dell'app), `.ID` (l'ID dell'app generato), `.Slug` (il nome in minuscolo con trattini) e `.Module` (il valore di `-module`, oppure lo slug). Includi un `go.mod.tmpl` per dichiarare il modulo e le dipendenze dell'app generata, poiché Go esclude i file `go.mod` annidati dai download dei moduli. In sua assenza, `-module` scrive lo stesso `go.mod` minimale che scrive per un template integrato.

## Eseguirla

```sh
go tool nsgo dev
```

`dev` compila l'app per questa macchina e la apre. A ogni salvataggio la ricompila e la riavvia, e una modifica al solo TSX aggiorna l'app in esecuzione senza riavviarla. Una compilazione non riuscita viene mostrata sopra l'app, con pulsanti per copiare l'errore e per compilare di nuovo.

Eseguila altrove con `-target`:

```sh
go tool nsgo dev -target web                # in the browser
go tool nsgo dev -target ios_simulator      # in the iOS simulator
go tool nsgo dev -target tvos_simulator     # in the Apple TV simulator
go tool nsgo dev -target watchos_simulator  # in the Apple Watch simulator
go tool nsgo dev -target android            # on a connected device or emulator
```

`go tool nsgo doctor` elenca ogni target indicando se questa macchina lo compila e lo esegue e, per quelli che non può gestire, cosa le manca, ad esempio Xcode, `adb`, un dispositivo collegato o GTK.

[Esecuzione e debug](debugging.md) tratta i dispositivi, il tracciamento degli aggiornamenti, il monitor delle prestazioni e le altre lingue.

## Con un agente di programmazione

La skill di NSGo insegna agli agenti di programmazione, come Claude Code, Codex e Cursor, come si scrive, si testa e si distribuisce un'app NSGo. Aggiungila alla directory dell'app, oppure con `-g` per tutti i progetti:

```sh
bunx skills add nsgo-dev/nsgo
```

Gli agenti che leggono da sé la documentazione partono da <https://nsgo.dev/llms.txt>, che elenca ogni pagina in Markdown all'indirizzo `https://nsgo.dev/docs/<page>.md`; <https://nsgo.dev/llms-full.txt> le contiene tutte. Un agente può anche usare l'app in esecuzione ([Esecuzione e debug](debugging.md#agents)).

## Prossimi passi

- [Esempio: un'app di attività](tutorials/todo.md): stato, una lista e un test, in una decina di minuti.
- [Esempio: un'app di note](tutorials/notes.md): uno stack di navigazione e note conservate sul dispositivo.
- [Esempio: repository GitHub popolari](tutorials/repos.md): caricamento dalla rete con gestione degli errori e un nuovo tentativo.
