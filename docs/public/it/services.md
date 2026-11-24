---
title: Servizi di piattaforma
description: Avvisi, appunti, file, foto, condivisione, archiviazione, segreti, riconoscimento vocale, sintesi vocale, rete, accesso, acquisti in-app e ciclo di vita dell'app.
group: Funzionalità di piattaforma
order: 44
---

## Avvisi e appunti

`cx.ShowAlert` mostra l'avviso o l'action sheet della piattaforma. Le sue `Actions` vengono restituite per indice, e un ruolo disegna i pulsanti di annullamento e distruttivi secondo le convenzioni della piattaforma. Su macOS, avvisi e selettori di file usano un foglio (sheet) finché la loro finestra è visibile. Una finestra nascosta, ridotta a icona o fuori schermo, oppure un'app nella barra di sistema senza finestra, apre invece la finestra di dialogo separatamente. Chiudere la finestra richiedente annulla le sue finestre di dialogo.

```go
cx.ShowAlert(core.AlertConfig{
	Title:   "Delete the file?",
	Message: "This cannot be undone.",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(index int) {
	if index == 1 {
		remove()
	}
})
```

`SetClipboard` scrive testo e `SetClipboardImage` un'immagine. `ReadClipboard`, `ReadClipboardImage` e `ReadClipboardFiles` rispondono in modo asincrono, e `HasClipboardText` e `HasClipboardImage` leggono cosa contengono gli appunti senza mostrare un avviso di incolla. `core.FileFromURL` apre come `core.File` gli URL forniti da `ReadClipboardFiles` e `View.OnFileDrop`: URL `file:` e, sul web, URL `blob:`.

tvOS non ha appunti pubblici. Le scritture negli appunti vengono ignorate, le letture non restituiscono contenuto e i controlli di presenza restituiscono false.

```go
cx.ReadClipboard(func(text string) { pasted.Set(text) })
cx.SetClipboard("Hello")
```

Su macOS e Windows, `ReadClipboardItems` fornisce un'istantanea di tutte le rappresentazioni in byte disponibili, fino a 48 MiB. Ogni elemento porta valori `ClipboardRepresentation` ordinati con un `Type` nativo e i suoi `Data` grezzi, inclusi i formati di sola presenza con dati vuoti. `ClipboardFormats` elenca i nomi dei formati nativi senza copiarne il contenuto. Per ripristinare un'istantanea, passala a `WriteClipboardItems` insieme a un callback facoltativo che riceve l'errore di scrittura. Una slice vuota svuota gli appunti. Confronta per uguaglianza i valori di `ClipboardChangeCount` per rilevare una nuova copia, anche quando il testo copiato è identico.

macOS conserva i confini degli elementi e i nomi dei tipi del pasteboard. Windows usa un solo elemento e preserva i nomi Win32 basati su byte come `CF_HDROP` e i formati registrati come `Rich Text Format`. In scrittura combina gli elementi con formati distinti e usa un byte zero per un contenuto vuoto. Handle GDI, formati owner-display e ID numerici di formati privati vengono omessi. Sulle altre piattaforme, i callback di lettura e scrittura ricevono `CodeUnavailable` e il callback del conteggio riceve `-1`.

## Aprire link e file

`cx.OpenURL` apre un URL nel browser o nell'app del sistema, e `core.CheckURL` indica cosa rifiuta: nessuno schema, oltre 16 KiB, spazi, un indirizzo web senza host, e `javascript:`, `data:`, `blob:`, `file:`, `about:` e `asset:`.

```go
if err := core.CheckURL(url); err == nil {
	cx.OpenURL(url, func(opened bool) {
		if !opened {
			status.Set("No app opens that link.")
		}
	})
}
```

`cx.OpenFile` apre un file dell'app nell'app associata al suo tipo, e `cx.Share` mostra il foglio di condivisione del sistema per file, testo o entrambi su macOS, iOS, Android, Windows, Linux (tramite il portale desktop o il file manager) e il web (`navigator.share`). Un `core.SharedFile` è un percorso su disco — oppure, per `Share` sul web e ovunque un file sia tenuto in memoria, i suoi byte `Data` — con il nome e il tipo che vede l'altra app. Sui desktop, `desktop.RevealFile` mostra un file o una cartella selezionati nel Finder, in Esplora file o nel file manager, e `desktop.TrashFile` lo sposta nel Cestino; non elimina mai in modo permanente, e risponde `core.ErrNoTrash` lasciando l'elemento al suo posto quando il volume non ha un cestino o il cestino non lo conserverebbe, come un file più grande del Cestino di Windows.

```go
cx.Share(core.ShareOptions{
	Files: []core.SharedFile{{Path: path, Name: "Notes.txt"}},
	Text:  "A note from NSGo",
}, func(shown bool, err error) {})
```

`mobile.OnShared(cx, func(items []mobile.SharedItem) { ... })` riceve testo e file per [`android.share_types`](building.md#incoming-shares-on-android) o [`apple.share_types`](building.md#incoming-shares-on-ios). Le condivisioni Android possono avviare l'app; iOS le mette in coda in un App Group finché l'app non è attiva o l'utente non la apre la volta successiva. Le condivisioni in sospeso attendono il primo handler. iOS conserva il lotto della casella in arrivo finché il callback non ritorna; la consegna avviene almeno una volta anche attraverso l'uscita del processo. Il testo arriva in `Text`, e i file arrivano come copie leggibili nella cache con `Path`, `Name` e `MIME`. Sposta o copia un file in un archivio persistente se deve sopravvivere alla cache.

`desktop.LookUp` mostra il pannello del dizionario di sistema per un testo, come fa un tocco con tre dita su macOS, disegnato sopra un riquadro in una vista: il testo e il riquadro di una selezione con il `Ref` della sua vista `Selectable`. `desktop.CanLookUp` indica se è possibile, in una finestra su macOS; Windows, Linux e il web non hanno un pannello del genere. Nei test, `nsgotest.App.LookUps` elenca i pannelli mostrati quando `nsgotest.Environment` imposta `CanLookUp`.

```go
var area core.Ref
var selected core.TextSelection
core.View{Ref: &area, Selectable: true, OnTextSelection: func(s core.TextSelection) { selected = s }, Children: subtitles}
// From a menu item or a shortcut:
if desktop.CanLookUp(cx) {
	desktop.LookUp(cx, selected.Text, &area, selected.Bounds)
}
```

`desktop.QuickLook` richiede le anteprime dei file, a partire da un indice, e aggiorna i file quando viene richiamato mentre il pannello è aperto. `desktop.CanQuickLook` indica la disponibilità in una finestra propria dell'app su macOS e Windows, e su Linux quando la sessione espone il servizio D-Bus di GNOME Sushi in esecuzione o attivabile. Windows usa il gestore di anteprima registrato per il tipo di file; i formati supportati dipendono dai gestori o dal visualizzatore installati. Un risultato true significa che la piattaforma è stata interpellata, non che abbia mostrato il file.

Spazio o Esc chiude il pannello. I tasti freccia selezionano i file della richiesta su Windows e nelle versioni di Sushi che riportano selezione e proprietà su X11. Su Wayland, Linux apre attualmente un'anteprima senza finestra genitore; la chiusura automatica con il proprietario e la selezione con le frecce non sono disponibili. I visualizzatori specifici di KDE e il web non sono supportati.

```go
if desktop.CanQuickLook(cx) {
	desktop.QuickLook(cx, []string{reportPath, sourcePath}, 0)
}
```

## Icone nei menu nativi

`desktop.MenuItem.Icon` mostra un'icona accanto a una voce nei menu dell'applicazione, del Dock, della barra di sistema e nei menu a comparsa su macOS, Windows e Linux. Un `desktop.MenuIcon` accetta un'icona vettoriale già analizzata (`SVG`, come `lucide.Folder()`), byte di immagine codificati (`Data`, PNG, JPEG, GIF, WebP o SVG) oppure una `core.ImageSource` locale, asset o in pixel (`Source`). Impostare `Template` colora un'immagine monocromatica con il colore del testo del menu, seguendo l'aspetto chiaro e scuro (e lo stato di evidenziazione su macOS); le icone `SVG` monocromatiche sono trattate come template per impostazione predefinita.

```go
desktop.SetApplicationMenu(cx, []desktop.MenuItem{{
	Label: "File",
	Items: []desktop.MenuItem{
		{Label: "Open Folder…", Icon: desktop.MenuIcon{SVG: lucide.Folder()}, OnSelect: openFolder},
		{Label: "Export", Icon: desktop.MenuIcon{Data: exportPNG, Template: true}, OnSelect: exportFile},
	},
}})
```

Le icone vengono caricate in modo sincrono da sorgenti locali; URL remoti e immagini non valide lasciano la voce senza icona. Etichette dei menu, scorciatoie e indicatori di spunta e di opzione mantengono il loro comportamento nativo.

Nei test, `nsgotest.MenuItem.Icon` registra l'icona di ogni voce in `app.Menu`, `app.DockMenu` e `app.PopupMenu`.

## Foto e file

`cx.PickImages` mostra il selettore di foto del sistema e risponde con dei `core.PickedFile`, di cui l'app legge byte, nome, tipo e dimensioni in pixel. `cx.TakePhoto` mostra la fotocamera dove la piattaforma ne ha una; `cx.Env().CanTakePhoto` indica se c'è. Su macOS, Windows e Linux mostra l'anteprima della fotocamera dello scanner con un pulsante Scatta foto. `cx.ScanCode` apre un'anteprima della fotocamera e risponde con il primo codice che legge, un codice QR a meno che `ScanOptions.Kinds` non ne chieda altri, oppure nil quando l'utente la chiude; `cx.Env().CanScanCode` indica se la piattaforma ha uno scanner, su dispositivi macOS, iOS e Android con fotocamera, macchine Windows che ne hanno una, desktop Linux con il portale Camera e `pipewiresrc` di GStreamer, e browser con `getUserMedia` e `BarcodeDetector`. macOS presenta un foglio quando la finestra richiedente è visibile, Windows e Linux una finestra modale sopra la finestra, e il web un overlay sopra la pagina. Inserisci `NSCameraUsageDescription` in `apple.info` per macOS e iOS, e `CAMERA` in `android.permissions` per Android. Android, Windows e Linux leggono tutti i tipi tranne PDF417. Tratta il testo del codice come input non attendibile.

Su tvOS, `PickImages` risponde senza immagini e `TakePhoto` restituisce `core.ErrNoCamera`; NSGo non fornisce il selettore di Continuity Camera.

Nei test, `nsgotest.App.PickImages`, `nsgotest.App.TakePhoto` e `nsgotest.App.ScanCode` ricevono record `nsgotest.ImagePick` per selezioni e scatti e record `nsgotest.CodeScan` per le scansioni, e rispondono al posto dell'interfaccia di sistema. Gli hook funzionano in modalità headless e con gli host `nsgo test -target` supportati. `nsgotest.Environment(core.Environment{Camera: true, ScanCode: true})` dà una fotocamera a un target che non ne ha.

```go
cx.PickImages(core.PickOptions{Limit: 1, MaxDimension: 2048}, func(files []core.PickedFile) {
	if len(files) == 1 {
		avatar.Set(files[0].Data)
	}
})
```

`cx.PickFiles` seleziona file di qualsiasi tipo, `cx.PickDirectories` seleziona cartelle come `fs.FS`, e `cx.SaveFile` salva un file o dei byte tramite la finestra di salvataggio del sistema. Entrambi i selettori accettano un `Title`, un'etichetta `Button` e una cartella `Start`, e `PickFilesOptions.TypesName` dà il nome al menu dei tipi, così un'app può mostrarli nella propria lingua; sono suggerimenti, che macOS, Windows e Linux seguono, iOS solo per `Start`, e Android e il web per niente. `cx.PickScreenColor` preleva un colore in qualsiasi punto dello schermo dove `cx.Env().CanPickScreenColor()` è true: macOS, Windows, Linux con il portale desktop Screenshot e browser con `EyeDropper`.

Imposta `core.SaveOptions.Start` su una cartella assoluta esistente per aprire lì la finestra di salvataggio su macOS, Windows, Linux e iOS. Con un suggerimento vuoto o inutilizzabile, o su altri target, è il sistema a scegliere la cartella.

Imposta `core.SaveOptions.Accept` per convalidare la destinazione scelta prima di scrivere. Viene eseguito sulla goroutine del motore. Restituire un errore lascia intatta la destinazione. Su macOS, Windows e nel selettore GTK in-process su Linux, la finestra di dialogo mostra il motivo e permette all'utente di scegliere un altro percorso. Annullare dopo un rifiuto riporta `saved == false`, il percorso rifiutato e l'errore. Su Android e nel portale desktop di Linux, un rifiuto completa il salvataggio con quel risultato. Android permette all'utente di scegliere una cartella e fornisce un URI di albero con il nome di file richiesto come parametro di query `name`. Crea il documento dopo l'approvazione e, in caso di successo, riporta l'URI del nuovo documento. La piattaforma abbandona la scrittura se il controllo non risponde entro cinque secondi. iOS e HarmonyOS riportano un errore per i salvataggi che richiedono un controllo. Il web non ha un percorso di destinazione e usa il suo normale flusso di salvataggio.

Nei test headless, `App.PickFiles`, `App.PickDirectories` e `App.SaveFile` ricevono record `nsgotest.FilePick`, `nsgotest.DirectoryPick` e `nsgotest.FileSave`. Gli stessi record si trovano in `app.FilePicks`, `app.DirectoryPicks` e `app.Saves`.

### Asset inclusi

Metti i file nella cartella `resources/` dell'app e usa `cx.ReadAsset("asset:maps/level.bin", done)` per leggerli da Go. Il callback riceve `([]byte, error)` sulla goroutine del motore dopo la lettura e viene scartato se il suo proprietario scompare o viene eseguito di nuovo. Usa `errors.Is(err, core.ErrAssetNotFound)` per riconoscere i file mancanti. La lettura carica l'intero file in memoria.

## Audio PCM in modalità pull

Usa `audio.NewOutput(cx, audio.OutputConfig{SampleRate: 48000, Channels: 2, Render: render})` per riprodurre PCM Float32 interleaved che la tua app decodifica o sintetizza. `Start` inizia a prelevare i campioni, `Pause` interrompe il prelievo e lo scope proprietario chiude l'output quando termina. macOS, iOS, Android, Windows, Linux e il web supportano questo flusso. Gli altri target restituiscono `audio.ErrUnsupported`.

`Render` restituisce il numero di frame che ha riempito. Viene eseguito sul thread audio e deve evitare blocchi, allocazioni e accessi al grafo. Un riempimento parziale riproduce silenzio per il resto e incrementa `Underruns`. `Position` restituisce il conteggio monotono dei frame e l'ora dell'host in quel frame. Usa `Flush` per scartare i frame in attesa e `Latency` per leggere la latenza del buffer. Cambi di instradamento e interruzioni mettono in pausa l'output e chiamano `OnInterrupt` sul motore. L'app può riprenderlo con `Start`.

## Archiviazione

`cx.Storage()` è l'archivio chiave-valore dell'app, conservato sul dispositivo e condiviso da tutti i grafi dell'app. Le letture sono sincrone, e una lettura all'interno di un calcolo lo sottoscrive alle modifiche; `SetItem` restituisce un `core.StorageFullError` oltre la quota (`core.StorageQuota`, 5 MiB per impostazione predefinita). È un semplice file che qualsiasi programma dell'utente può leggere, quindi conserva token e password nei [segreti](#secrets).

`cx.Store()` restituisce l'archivio sottostante, per un host che crea un'altra radice con `core.RootOptions.Storage`, come uno che esegue un plugin su una goroutine propria: entrambe le radici condividono allora le voci dell'app e ciò che è conservato sul dispositivo. È l'intero archivio, non la partizione che `cx.ProvideStorage` dà al codice non scritto dall'app.

Un host che fornisce la propria persistenza chiama `core.NewStore(backend)` con un `core.StorageBackend`: `Load() (map[string]string, error)` carica una volta, e `Write(core.StorageChange)` riceve le modifiche in ordine. Fai uno switch su `change.Op` (`core.StorageSet`, `core.StorageRemove`, `core.StorageClear`) e usa `change.Key` e `change.Value`. Passa `nil` per un archivio tenuto solo in memoria.

```go
store := cx.Storage()
if tab, ok := store.GetItem("tab"); ok {
	selected.Set(tab)
}
store.SetItem("tab", "library")
```

`store.Keys()` elenca le chiavi, `RemoveItem` ne elimina una e `Clear` svuota l'archivio. `cx.DataDir()` e `cx.CacheDir()` forniscono le cartelle dell'app per i file per cui l'archivio chiave-valore è troppo piccolo; `cx.LogsDir()` fornisce la cartella della piattaforma per i file di log, e uno `Scope.ProvideStorage` dà a un sottoalbero una partizione propria.

Un App Clip iOS e la sua app completa condividono questi percorsi di dati e archiviazione quando sono configurati `app_clip` e un `apple.app_group` esplicito; le loro cache restano separate. Abilitare il gruppo non migra i dati esistenti della sandbox. [Archiviazione degli App Clip](building.md#app-clips) tratta configurazione e firma.

Su HarmonyOS, le operazioni sui file di Go come `os.ReadFile`, `os.WriteFile` e `os.Rename` funzionano in queste cartelle e in `os.TempDir()`. I percorsi relativi partono dalla cartella dei file. I lock consultivi sui file e i collegamenti simbolici non sono disponibili.

### Le cartelle dell'utente

`desktop.UserDir(cx, desktop.DownloadsDir)` restituisce il percorso di una delle cartelle dell'utente: `DocumentsDir`, `DownloadsDir`, `DesktopDir`, `PicturesDir`, `MusicDir` o `VideosDir`. Lo chiede al sistema ogni volta, così segue una cartella che l'utente ha spostato: le cartelle nella directory home su macOS (`Movies` per i video), le cartelle note su Windows, incluse quelle di OneDrive, e le directory utente XDG su Linux. La cartella potrebbe non esistere ancora. I telefoni, il web e HarmonyOS non hanno cartelle di questo tipo raggiungibili da un'app tramite percorso, né le ha Linux per una cartella che `user-dirs.dirs` tralascia: `UserDir` restituisce `core.ErrUserDirUnavailable`, e l'app chiede invece all'utente con `cx.PickDirectories` o `cx.SaveFile`.

```go
dir, err := desktop.UserDir(cx, desktop.DownloadsDir)
if err != nil {
	dir = cx.DataDir()
}
```

I test non ricevono mai le cartelle dell'utente: ciascuna è una directory vuota all'interno di quella temporanea del test, e `nsgotest.UserDirs(map[core.StandardDir]string{…})` le fissa, mentre una cartella tralasciata risulta non disponibile.

### Rapporti di crash

Le build native di release conservano le tracce dei crash di Go in `cx.LogsDir()/crashes`. Apple registra le eccezioni Objective-C non gestite, Android registra le eccezioni Java non gestite, e Windows registra le eccezioni SEH che raggiungono il suo filtro di primo livello. Elenca i rapporti in sospeso all'avvio successivo con `cx.CrashReports()`. Ogni rapporto identifica la versione, la build e la piattaforma che lo hanno prodotto. Chiama `Open()` per leggere il testo diagnostico, chiudi il reader, poi chiama `Acknowledge()` per eliminare il rapporto quando l'app lo ha elaborato. Esegui queste operazioni sui file su una goroutine.

## Spazio su disco

`cx.DiskSpace(func(space core.DiskSpace, err error) { … })` interroga in modo asincrono il volume che contiene `cx.DataDir()` e richiama il callback sulla goroutine del motore. `Total` è la capacità riportata dal sistema, `Available` è ciò che l'app può usare per un'operazione richiesta dall'utente, e `Opportunistic` è ciò che può usare per cache e prefetch. Tutti e tre sono in byte. `Available` include lo spazio che il sistema recupera su richiesta, dove riporta quel valore. Le piattaforme con un solo valore di spazio libero lo restituiscono in entrambi i campi. Sul web, `Total` è la quota di archiviazione stimata dal browser per l'origine e `Available` è la parte non usata. Quando il servizio non è disponibile restituisce `core.ErrDiskSpaceUnavailable`.

Le build Apple richiedono che `apple.privacy.accessed_apis` in `nsgo.yaml` dichiari `NSPrivacyAccessedAPICategoryDiskSpace`. Dichiara `E174.1` per controllare lo spazio prima di scrivere o per liberare file quando lo spazio è scarso, con una modifica che l'utente può osservare. Dichiara `85F4.1` per mostrare all'utente lo spazio disponibile. L'app deve rispettare le restrizioni di Apple sull'uso e la condivisione delle letture. Una build senza la categoria restituisce `core.ErrNotDeclared`. [Le motivazioni approvate da Apple](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype)

Imposta la risposta di un test con `nsgotest.DiskSpace(space, err)`.

## Archiviazione cloud

Il package `cloud` sincronizza tra dispositivi piccole voci chiave-valore (`cloud.Get`, `cloud.Set`, `cloud.Delete`, `cloud.Keys`) e file più grandi o istantanee di salvataggio (`cloud.ReadFile`, `cloud.WriteFile`, `cloud.DeleteFile`, `cloud.ListFiles`): iCloud Key-Value Store e container di documenti ubiquity su macOS e iOS, e Saved Games di Google Play Games su Android.

Ogni operazione consegna il suo callback sulla goroutine del motore e lo salta se `cx` è stato eliminato. `cx.Env().CanUseCloudStorage()` indica se l'archiviazione cloud è disponibile; quando l'utente non ha effettuato l'accesso, mancano gli entitlement o la piattaforma non ha un backend cloud nativo (Windows, Linux e il web), ogni chiamata restituisce `cloud.ErrUnavailable` e non scarta mai le scritture in silenzio.

```go
if cx.Env().CanUseCloudStorage() {
	cloud.WriteFile(cx, "saves/slot1.sav", saveBytes, func(err error) {
		if err != nil {
			status.Set(err.Error())
		}
	})
}

cloud.WatchChanges(cx, func(ev cloud.ChangeEvent) {
	reload(ev.Keys, ev.Files)
})

cloud.OnConflict(cx, func(c cloud.Conflict) {
	_ = c.Resolve(mergeSaves(c.Local, c.Remote))
})
```

`cloud.WatchChanges` e `cloud.OnConflict` consegnano sulla goroutine del motore le modifiche esterne da più dispositivi e i conflitti di versione, e si ripuliscono automaticamente quando `cx` viene eliminato. Leggere un file in conflitto restituisce un `*cloud.ConflictError` (che soddisfa `errors.Is(err, cloud.ErrConflict)`).

Per abilitare gli entitlement iCloud di Apple in `nsgo build`, aggiungi una sezione `cloud:` (o `icloud:`) a `nsgo.yaml`:

```yaml
cloud:
  kv_store: true
  containers:
    - iCloud.com.example.myapp
```

Il profilo di firma deve concedere le capacità iCloud che richiedi. Su macOS solo le build per il Mac App Store (`sign.macos_profile`) includono iCloud, poiché macOS lo concede solo con un provisioning profile; una build Developer ID lo omette con un avviso. Su Android, l'archiviazione cloud richiede l'SDK Play Games Services v2 nell'app e un giocatore che abbia effettuato l'accesso.

Sugli store desktop come Steam, Steam Auto-Cloud sincronizza direttamente `cx.DataDir()` senza codice aggiuntivo — continua a scrivere lì i file di salvataggio locali e lascia che Steam sincronizzi la cartella.

Nei test, `nsgotest` dà a ogni app un archivio cloud isolato in memoria: attiva o disattiva la disponibilità con `nsgotest.CloudAvailable` o `app.SetCloudAvailable`, simula aggiornamenti esterni da più dispositivi con `app.SimulateCloudChange`, e verifica la risoluzione dei conflitti con `app.SimulateCloudConflict`.

## Segreti

`cx.Secrets()` è l'archivio sicuro dell'app, per refresh token, chiavi API e password: `Get`, `Set` e `Delete` di una stringa per account, che la piattaforma protegge. Un account mancante si legge come `("", false, nil)`, e dove la piattaforma non ha un archivio sicuro, come sul web, ogni chiamata restituisce `core.ErrSecretsUnavailable`; NSGo non ripiega mai su un archivio in chiaro. Il sistema può far attendere una chiamata, come fa un portachiavi che interpella l'utente, quindi chiamale fuori dalla goroutine del motore e riporta il risultato con `cx.Post`.

```go
secrets := cx.Secrets()
go func() {
	token, ok, err := secrets.Get("refresh-token")
	cx.Post(func() {
		if err == nil && ok {
			login(token)
		}
	})
}()
```

| Piattaforma | Dove viene conservato un segreto |
| --- | --- |
| macOS | Una password generica del portachiavi di login: `kSecAttrService` è il servizio, `kSecAttrAccount` l'account |
| iOS, tvOS, watchOS | Una password generica del portachiavi, allo stesso modo |
| Windows | Una credenziale generica di Gestione credenziali il cui target è `service/account`, al massimo 2560 byte |
| Linux | Un elemento del Secret Service (GNOME Keyring, KWallet) con attributi `service` e `account`; non disponibile dove la sessione non ne esegue nessuno |
| Android | Cifrato con una chiave AES dell'Android Keystore, nei file privati dell'app, fuori dai backup; le letture ripiegano anche sui valori che `expo-secure-store` ha cifrato nelle sue `SharedPreferences` `SecureStore` |
| Mini programmi WeChat | Archiviazione cifrata dalla piattaforma (`wx.setStorage` / `wx.getStorage` con `encrypt: true`, libreria base 2.21.3+); non disponibile con librerie base precedenti |
| Web, altri mini programmi | Non disponibile (`core.ErrSecretsUnavailable`): i browser non hanno un archivio di segreti lato client protetto dal sistema operativo (conserva le sessioni web in cookie `HttpOnly` impostati dal server), e i mini programmi QQ, Alipay e Douyin non dispongono di archiviazione cifrata |

Il servizio è per impostazione predefinita l'ID dell'app (`core.App().ID`). Un'app che proviene da un altro framework mantiene l'accesso dei suoi utenti indicando gli elementi scritti da quel framework: `Service` e, sulle piattaforme Apple, `AccessGroup` e `Accessible`. Su Android, imposta `Service` sul `keychainService` di `expo-secure-store` (`"key_v1"` per impostazione predefinita) per leggere i valori che ha cifrato in `SharedPreferences`; scrivere o eliminare cancella la voce migrata. Su iOS, `core.SecretAfterFirstUnlock` permette al lavoro in background di leggere un elemento mentre il telefono è bloccato, e un'accessibilità `ThisDeviceOnly` lo tiene fuori dai backup. Su macOS un `AccessGroup` sposta gli elementi nel portachiavi con protezione dei dati, che richiede una build firmata da un team il cui entitlement `keychain-access-groups` elenca il gruppo, configurato sotto `macos.entitlements` in `nsgo.yaml` sia per le build Mac App Store sia per quelle Developer ID.

```go
s := cx.Secrets()
s.Service = "app:no-auth" // the service an earlier build used
s.Accessible = core.SecretAfterFirstUnlockThisDeviceOnly
err := s.Set("refresh-token", token)
```

`nsgotest` conserva i segreti di un'app in memoria, in `App.Secrets`, e `nsgotest.Secrets` avvia un test con alcuni segreti già presenti, così un test non tocca mai il portachiavi della macchina. Un package TSX raggiunge l'archivio con la capability `secrets`, sotto un servizio proprio.

## Rete

`cx.Env().Network()` indica se il sistema ha un percorso utilizzabile (`Online`) e se considera quel percorso a consumo o costoso (`Expensive`). I binding lo seguono come qualsiasi altro valore dell'ambiente. `cx.OnNetworkChange(func(core.Network))` chiama l'handler sulla goroutine del motore a ogni modifica finché il suo scope è attivo. Il percorso del sistema non dice nulla sul fatto che un determinato server risponda. Un target che non sa riportare la connettività è considerato online per impostazione predefinita, e un costo sconosciuto vale false per impostazione predefinita.

Imposta `core.Environment{Network: &core.Network{Online: false}}` tramite `nsgotest.Environment` o `App.SetEnvironment` per testare il comportamento offline.

Il `net/http` di Go effettua richieste su ogni target, con le impostazioni TLS e proxy del sistema. I segnali appartengono alla goroutine del motore, quindi una risposta da un'altra goroutine li raggiunge tramite `cx.Post`.

```go
go func() {
	resp, err := http.Get("https://api.example.com/items")
	if err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	defer resp.Body.Close()
	var items []Item
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	cx.Post(func() { list.Set(items) })
}()
```

`cx.Post` esegue la sua funzione sulla goroutine del grafo, dove i segnali possono essere impostati. Su Android il manifest richiede il permesso internet, e i download delle immagini vengono messi in cache su disco.

`core.NewResource` fa lo stesso per un valore caricato da una sorgente. Il suo fetcher viene eseguito su una goroutine propria e ricarica quando la sorgente cambia:

```go
items := core.NewResource(cx, func() (string, bool) { return query.Get(), query.Get() != "" },
	func(ctx context.Context, q string, _ core.ResourceInfo) ([]Item, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com/items?q="+url.QueryEscape(q), nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var list []Item
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return nil, err
		}
		return list, nil
	})
```

`items.Get()`, `items.Err()` e `items.Loading()` sono segnali che i binding seguono. Una nuova query annulla tramite `ctx` la richiesta ancora in corso, così una risposta in ritardo non sostituisce mai una più recente. `items.Refetch()` riprova, mantenendo l'ultima lista durante il caricamento, e `items.Mutate` imposta subito un valore per un aggiornamento ottimistico.

`core.Suspense` mostra un fallback mentre le risorse lette dal suo contenuto caricano il loro primo valore, così una schermata non ha bisogno di un proprio flag di caricamento:

```go
core.Suspense{
	Content:  func(cx *core.Scope) core.Node { return itemList(cx, items) },
	Fallback: func(*core.Scope) core.Node { return ui.Spinner{Label: "Loading"} },
}
```

Il contenuto resta montato ma nascosto durante l'attesa, così compare subito quando arrivano i valori. Un nuovo caricamento lo mantiene visibile con l'ultimo valore.

### Scoprire servizi locali

Su macOS, iOS, Android, Windows e Linux, `cx.AdvertiseService` pubblica una porta in ascolto e degli attributi TXT. `cx.BrowseServices` riporta i servizi risolti con il loro host, la porta e gli attributi TXT. `Host` può essere un nome host DNS o un indirizzo IP. Quando un servizio se ne va, il callback lo riceve di nuovo con `Removed` impostato. Controlla `cx.Env().CanDiscoverServices()` per il supporto della piattaforma. Entrambe le chiamate restituiscono una funzione di arresto e terminano quando il loro scope viene eliminato. Un errore della piattaforma viene riportato in modo asincrono sulla goroutine del motore. Linux usa Avahi e riporta un errore se il demone non è disponibile.

Per Bonjour sulle piattaforme Apple, dichiara i tipi di servizio e spiega l'accesso alla rete locale in `nsgo.yaml`. Il sistema gestisce la richiesta di permesso e ricorda la scelta dell'utente.

```yaml
apple:
  info:
    NSLocalNetworkUsageDescription: Finds nearby devices to sync with.
    NSBonjourServices:
      - _example._tcp
```

Android usa `NsdManager`. Android 12 e versioni precedenti, e Android 13 prima di T Extensions 7, richiedono `CHANGE_WIFI_MULTICAST_STATE` per la scoperta via Wi-Fi. Le build standalone dichiarano quel permesso. Se incorpori NSGo in un host Android, aggiungilo al manifest dell'host per quei dispositivi. I valori TXT pubblicizzati devono essere UTF-8 valido sui dispositivi senza l'API TXT grezza di Android (API 37 o T Extensions 22). La ricerca conserva i valori TXT grezzi. Se un host Android ha come target l'API 37 o superiore, deve dichiarare e richiedere `ACCESS_LOCAL_NETWORK` su Android 17 o versioni successive.

Usa `nsgotest.LocalServices` per fornire risultati di ricerca in un test headless.

## In riproduzione

Usa `cx.SetNowPlaying(core.NowPlaying{...})` per mostrare il contenuto multimediale corrente dell'app nel Centro di Controllo e nella barra dei menu di macOS. Fornisci `Title`, `Artist`, `Album` e un `Artwork` facoltativo come `*core.ImageSource`. Una `Duration` pari a zero indica uno stream dal vivo. Indica la `Position` corrente e il `Rate` di riproduzione (zero in pausa, uno a velocità normale). Pubblica di nuovo quando cambiano la traccia, la posizione o la velocità. Il sistema fa avanzare la posizione tra una chiamata e l'altra.

Elenca in `Commands` i controlli di trasporto supportati, come `core.MediaPlay`, `core.MediaPause`, `core.MediaTogglePlayPause` e `core.MediaSeek`. Gestisci le richieste con `cx.OnMediaCommand`. Il callback riceve il `Kind` del comando e, per uno spostamento, la `Position` richiesta. L'handler attivo più recente riceve i comandi. Chiama `cx.ClearNowPlaying()` quando l'app ha finito di riprodurre. Mentre l'app pubblica manualmente, i video controllati su macOS sospendono la propria pubblicazione. Controlla `cx.Env().CanShowNowPlaying()` per il supporto della piattaforma.

## Download in background

Usa `cx.Download` e `cx.OnDownloadUpdate` nelle app standalone iOS e macOS, e su Android. Le viste Apple incorporate non hanno un host per i trasferimenti. Le app Android abilitano `android.background_downloads: true` in `nsgo.yaml`. Avvia un download da un'azione dell'utente mentre l'app è visibile. Usa una destinazione assoluta con una cartella genitore esistente in cui l'app può scrivere.

```go
cx.OnDownloadUpdate(func(p core.DownloadProgress) {
    progress.Set(p)
})
task, err := cx.Download(core.DownloadRequest{URL: url}, path, core.DownloadOptions{
    CertificatePEM: serverCertificate,
})
```

Il trasferimento usa GET e accetta header di richiesta facoltativi. Usa il normale archivio di fiducia HTTPS, a meno che `CertificatePEM` non fissi il certificato foglia del server. Il pinning richiede anche un nome host corrispondente e un certificato valido. Usa l'URL finale, poiché il servizio richiede una risposta con esito positivo da quell'URL.

Su Apple, un certificato autofirmato su un nome host pubblico richiede un'eccezione App Transport Security per quell'host in `apple.info`. Il trasferimento controlla comunque il pin esatto del certificato, il nome host e la validità. Una connessione a un NAS locale richiede anche un `NSLocalNetworkUsageDescription` che spieghi l'accesso, e gli host non qualificati o `.local` usano `NSAllowsLocalNetworking`. [Le impostazioni ATS di Apple](https://developer.apple.com/documentation/security/preventing-insecure-network-connections)

```yaml
apple:
  info:
    NSAppTransportSecurity:
      NSExceptionDomains:
        files.example.com:
          NSExceptionAllowsInsecureHTTPLoads: true
```

Il servizio in primo piano di Android continua dopo che l'activity lascia lo schermo. Pubblica una notifica di avanzamento, soggetta al permesso di notifica di Android. L'avanzamento usa conteggi in byte e imposta `Total` a -1 finché non è noto. Il completamento sostituisce la destinazione in modo atomico. `task.Pause()` conserva il file parziale, `task.Resume()` usa un intervallo HTTP quando il server fornisce un validatore, e `task.Cancel()` rimuove il file parziale.

iOS usa una URLSession di sistema in background che continua i trasferimenti mentre l'app è sospesa. L'app ricollega la sua sessione all'avvio e pubblica i file prima di confermare il completamento in background. Su macOS i trasferimenti vengono eseguiti nel processo. Le piattaforme Apple usano i dati di ripresa nativi per i download in pausa e conservano i metadati della richiesta nell'archivio privato dell'app finché non viene salvato un risultato definitivo. Con un server che non supporta la ripresa, il trasferimento riparte da capo.

Registra di nuovo `OnDownloadUpdate` all'avvio dell'app per vedere le attività in corso e i risultati salvati. Usa `p.Task.Forget()` dopo aver gestito un risultato definitivo. Android può arrestare il servizio, per esempio quando scade il tempo di trasferimento in primo piano. L'app riceve allora `DownloadInterrupted`. Su Windows, Linux, tvOS, watchOS e il web, `Download` restituisce `ErrDownloadUnavailable`.

## Riconoscimento vocale e ciclo di vita

`cx.StartSpeech` riconosce il parlato con il riconoscitore del sistema dove `cx.Env().CanRecognizeSpeech()` indica che esiste (macOS, iOS, Android, Windows e browser che espongono `SpeechRecognition` o `webkitSpeechRecognition`), e risponde con la trascrizione man mano che cambia. Sulle piattaforme Apple dichiara `NSMicrophoneUsageDescription` e `NSSpeechRecognitionUsageDescription` in `apple.info`; su Android aggiungi `RECORD_AUDIO` a `android.permissions`. In un test headless, `app.Speech` contiene una `nsgotest.SpeechSession` mentre il riconoscimento è attivo. Usa `app.HearSpeech` e `app.EndSpeech` per consegnare i risultati.

```go
session, err := cx.StartSpeech(core.SpeechOptions{
	Lang:    "en-US",
	Interim: true,
	OnResult: func(text string, final bool) {
		transcript.Set(text)
	},
})
if err == nil {
	defer session.Stop()
}
```

`cx.Env().Lifecycle()` indica se l'app è attiva, inattiva o in background: sui desktop, la finestra in cui si trova `cx`, così il contenuto di una finestra ridotta a icona può andare in pausa mentre un'altra finestra è visibile, e nello scope proprio dell'app tutte le sue finestre insieme. `desktop.Minimized(cx)` indica se quella finestra è ridotta a icona, e `desktop.FrontmostApp(cx)` quale applicazione è in primo piano sul desktop, inclusa l'app stessa, e se la sua finestra è a schermo intero, come la segue un animaletto da scrivania o un time tracker; `desktop.CanReadFrontmostApp(cx)` indica se può essere seguita, su macOS, Windows, X11 e sui compositor Wayland supportati. `cx.OnQuit` fa pulizia quando l'app esce, in qualunque modo lo faccia, con 200 ms a disposizione per il suo lavoro. `desktop.OnQuitRequest` può annullare o rimandare una voce di menu Esci del desktop o `desktop.RequestQuit(cx)`. Su Windows e Linux viene eseguito anche dopo `OnClosing` quando chiudere una finestra terminerebbe l'app. `desktop.RequestRestart(cx)` interpella gli stessi handler prima di riavviare; `desktop.Restart(cx)` resta immediato. Anche i riavvii dell'updater su Linux e Windows chiedono, così un salvataggio può annullarli o rimandarli. Quando il sistema segnala logout, riavvio o spegnimento, `q.Forced()` è true e l'app prosegue attraverso `OnQuit`. Controlla `Forced()` prima di chiedere una decisione all'utente. `cx.SetAppearance` forza il tema chiaro o scuro, `cx.SetLocale` sceglie la lingua, `cx.OnKey` associa una scorciatoia da tastiera, che resta inattiva mentre ciò che mostra il suo proprietario è nascosto o sotto una finestra di dialogo a meno che `core.KeyOptions{OverModals: true}` non la mantenga, che prende il suo tasto prima del menu dell'applicazione a meno che `YieldsToMenu` non lo lasci a una voce di menu abilitata con la stessa scorciatoia nel punto in cui si trova il focus (mentre un campo di testo o una web view ha il focus, una voce `MenuNotEditing` lascia il tasto al binding e lo sottrae alle voci successive con la stessa scorciatoia, e il menu di Linux non prende nessun tasto premuto senza Control, Alt o un tasto logo, quindi lì il binding mantiene un tasto del genere), che funziona nella finestra in cui si trova `cx` a meno che `AllSurfaces` non la associ in ogni finestra e pannello dell'app, anche quelli aperti in seguito, per una scorciatoia valida in tutta l'app, che resta inattiva in tutti mentre ciò che mostra il suo proprietario è nascosto nella finestra in cui è associata, e che lascia a un controllo con il focus i tasti che usa, a un pulsante Invio e Spazio e a uno slider, una barra di schede o un gruppo di opzioni le frecce, a meno che non sia associata con `core.KeyAlways` o `core.KeyNotEditing`; `cx.OnOpenFiles` riceve i documenti aperti su macOS, Windows, Linux e iOS. iOS importa i file normali in un archivio temporaneo di proprietà dell'app; spostali in `cx.DataDir()` per conservarli. Su macOS, chiudere la finestra che forniva il menu dell'applicazione ripristina il menu della finestra superstite più recente, o quello predefinito quando non ne resta nessuna.

Su Wayland, il rilevamento dell'app in primo piano richiede la versione 2 o successiva del protocollo wlr foreign-toplevel del compositor; gli altri compositor riportano `CanReadFrontmostApp` false. Il suo ID è l'ID applicazione Wayland, il nome proviene dalla voce desktop quando disponibile, e il PID è 0. Le app Windows pacchettizzate usano il loro AUMID al posto del percorso di un frame host. macOS segue lo stato di presentazione a schermo intero di AppKit invece di dedurlo dalle dimensioni della finestra.

Un ruolo di modifica nel menu dell'applicazione si rivolge all'editor con il focus prima del suo fallback `OnSelect`. Su macOS, Annulla e Ripeti seguono la cronologia di quell'editor, e Taglia segue la sua selezione, anche mentre l'app è inattiva; senza un editor, il fallback resta abilitato.

Su iOS, passa a `cx.OnKeyWith` una funzione `core.KeyOptions.Title` per offrire la scorciatoia nel menu della tastiera del sistema finché il binding può attivarsi. `Group` dà il nome al suo menu su iPadOS 26 e versioni successive. Le funzioni possono leggere il catalogo dell'app, così i titoli delle scorciatoie seguono la lingua. Usa `app.KeyCommands()` nei test per esaminare l'elenco corrente.

```go
cx.OnKey("mod+k", func() { palette.Set(true) })
cx.SetLocale("de")
cx.OnQuit(func(ctx context.Context) { saveDraft(ctx) })
```

`cx.Env().DeviceName()` è il nome che l'utente ha dato al dispositivo, e `cx.Env().DeviceModel()` il suo modello hardware, come `Mac15,7`, `iPhone15,2` o `Google Pixel 8`, così un'app può distinguere due dispositivi abbinati. macOS, Android, Windows e Linux forniscono entrambi; iOS fornisce il modello, e il nome solo a un'app con l'entitlement di Apple per il nome del dispositivo assegnato dall'utente. watchOS fornisce il suo identificatore hardware, HarmonyOS il nome commerciale o il modello del prodotto, e i mini programmi la marca e il modello della piattaforma; questi target lasciano vuoto il nome assegnato dall'utente. Il web non fornisce né l'uno né l'altro. I test li impostano con `nsgotest.Environment`.

`cx.Env().ThermalState()` riporta `core.ThermalNominal`, `ThermalFair`, `ThermalSerious` o `ThermalCritical` sulle piattaforme Apple e su Android API 29 o successive. `cx.Env().LowPowerMode()` riporta `core.LowPowerOn` o `LowPowerOff` sulle piattaforme Apple, su Android, su Windows e sulle sessioni Linux con power-profiles-daemon. Dove il sistema non fornisce un valore, le letture restituiscono `ThermalUnknown` e `LowPowerUnknown`. Ogni lettura segue le modifiche in modo indipendente, così un'app può adattare il proprio carico di lavoro durante l'esecuzione. Nei test imposta `ThermalState` e `LowPowerMode` su `core.Environment` tramite `nsgotest.Environment` o `app.SetEnvironment`.

`desktop.CursorPoint(cx, func(p core.Point, available bool) { ... })` legge il puntatore anche fuori dalle finestre dell'app su macOS, Windows e X11. Prende un campione per chiamata. Il punto è nelle stesse coordinate dello schermo di `desktop.Monitors(cx)` e `Window.ShowAt`. Su Wayland e sugli altri target non supportati, o se allo scope manca `core.AccessCursorPoint`, `available` è false.

`desktop.SetWindowConstraints` limita le dimensioni del contenuto e controlla il ridimensionamento, la riduzione a icona e la massimizzazione; imposta `WindowOptions.Constraints` quando apri una finestra o usa `nsgo.WindowConstraints` per la finestra principale. Linux applica i massimi non fissi su X11 e rimuove i pulsanti di riduzione a icona e massimizzazione vietati dalla barra del titolo nativa di GTK su X11 e Wayland. Wayland supporta attualmente le dimensioni fisse solo come limite superiore.

`desktop.SetWindowLevel` fa fluttuare una finestra sopra le altre, e `desktop.SetWindowWorkspaces(cx, desktop.WindowWorkspacesAll)` o `WindowOptions.Workspaces` la mantiene su ogni spazio di lavoro, come un animaletto da scrivania, un HUD o un timer: su macOS su ogni Spazio e sopra gli spazi a schermo intero delle altre app, fuori da Cmd-`, e su Linux su ogni desktop virtuale di un window manager X11. Windows e Wayland la lasciano su quello in cui è stata aperta, e `desktop.CanSetWindowWorkspaces` indica se ha effetto: true su macOS e con un window manager X11 che supporta `_NET_WM_DESKTOP`. Una finestra principale va su ogni spazio di lavoro chiamando `desktop.SetWindowWorkspaces` dal suo contenuto. Nei test, `nsgotest.App.Window` elenca le richieste, e `nsgotest.Environment` imposta `CanSetWindowWorkspaces`.

`desktop.RegisterHotKey` associa una scorciatoia globale, che funziona qualunque app sia in primo piano, su macOS, su Linux e in un'app Windows eseguita con `nsgo.Run(App)`. Lo `State` del suo handle indica se il sistema l'ha accettata: `HotKeyPending` finché non risponde, poi `HotKeyRegistered`, `HotKeyTaken` quando un'altra registrazione occupa la combinazione, oppure `HotKeyUnsupported` per un tasto o una piattaforma senza scorciatoie globali, così un registratore di scorciatoie può chiedere un'altra combinazione. Su macOS una combinazione risulta occupata solo quando l'app la detiene già o un'altra app l'ha registrata in modo esclusivo. Su Wayland il compositor chiede all'utente di associare la scorciatoia: è registrata una volta che l'utente l'ha fatto, e occupata quando l'utente o il compositor la tralasciano. Nei test, `nsgotest.HotKeyTaken` e `nsgotest.HotKeyUnsupported` fanno rispondere così le combinazioni.

Con `desktop.TitleBarHidden`, Windows e Linux offrono il menu dell'applicazione come popup quando si preme e si rilascia Alt da solo o F10. Alt con la prima lettera o cifra dell'etichetta di primo livello apre quel sottomenu o sceglie quel comando, nella scrittura dell'etichetta. I binding dell'app mantengono i loro tasti; Maiusc+F10 resta il tasto del menu contestuale, e gli altri accordi con Alt restano al sistema. Esc chiude il popup e ripristina il focus della tastiera.

```go
shortcut := desktop.RegisterHotKey(cx, "alt+space", launcher.Toggle)
warning := core.Text{ContentFunc: func() string {
	if shortcut.State() == desktop.HotKeyTaken {
		return "Another app uses Alt+Space"
	}
	return ""
}}
```

I ridimensionamenti sul desktop su macOS, Windows e Linux attendono fino a 16 ms il nuovo layout prima di disegnare. Mantieni rapidi i calcoli reattivi e gli handler degli eventi così che il layout possa rispettare quella scadenza; esegui l'I/O lento su una goroutine e invia i risultati tramite `cx.Post`. Un'app occupata continua a ridimensionarsi e mostra il suo layout quando si rimette in pari.

`desktop.SetWindowFrame` sposta una finestra quando `WindowFrame.Move` è true, usando `X` e `Y` in punti dall'angolo superiore sinistro dello schermo principale. Le dimensioni non impostate mantengono i valori attuali. Controlla `desktop.CanPlaceWindows`: macOS, Windows e Linux su X11 supportano il posizionamento; una normale finestra toplevel di Wayland mantiene la posizione decisa dal compositor. `desktop.Frame` segue la posizione della finestra nativa su questi desktop.

Su macOS e Windows, `WindowFrame{Animated: true}` porta con un'animazione morbida un pannello visibile al suo nuovo frame, incluse le dimensioni di un pannello agganciato. I pannelli nascosti e il movimento ridotto applicano subito il nuovo frame. Le normali finestre macOS usano l'animazione di AppKit.

Imposta `desktop.WindowOptions.Parent` su una finestra aperta dell'app per chiudere entrambe quando si chiude la finestra genitore. Sui desktop la finestra figlia resta sopra la genitore e la segue quando viene nascosta o ridotta a icona; una finestra figlia nascosta esplicitamente resta nascosta quando la genitore ritorna. Su macOS si centra anche sopra la genitore quando `Frame` non è impostato e la segue a schermo intero.

`WindowOptions.NoShadow` e `desktop.SetWindowShadow(cx, false)` tolgono l'ombra anche a una finestra Windows con la barra del titolo nascosta, che disegna la propria cornice; una finestra con la barra del titolo di sistema mantiene la sua cornice e la sua ombra, come una finestra Linux decorata dal suo window manager, dove `NoShadow` toglie solo l'ombra delle decorazioni proprie di GTK.

`desktop.SetWindowPointerEvents` lascia passare il puntatore attraverso una finestra verso le finestre sottostanti, di qualsiasi app: `WindowPointerNone` ovunque, come fa un heads-up display, e `WindowPointerPainted` ovunque nessuna vista disegni qualcosa o ascolti il puntatore, come attorno a un animaletto da scrivania che riceve comunque pressioni, trascinamenti e clic destri su sé stesso; la barra del titolo disegnata dal sistema conta come nulla di disegnato. `WindowOptions.PointerEvents` e `PanelOptions.PointerEvents` aprono una finestra in questo modo. Una regione `Desktop.WindowDrag` continua a trascinare all'interno di un antenato premibile; un controllo premibile o di modifica più vicino mantiene il proprio input invece di trascinare la finestra. Una finestra che non riceve il puntatore non sente pressioni, quindi l'app lo restituisce tramite una scorciatoia, un'icona nella barra di sistema o un'altra finestra. Il codice controllato da un `Access` ha bisogno di `core.AccessOverlay`.

```go
desktop.OpenPanel(cx, desktop.PanelOptions{Width: 600, Height: 400, PointerEvents: desktop.WindowPointerPainted}, Pet)
```

Su macOS e Windows, un'immagine in una finestra in modalità painted può impostare `Image.HitTestAlpha` (TSX `hitTestAlpha`) a un valore maggiore di 0 e al massimo 1. Per esempio, 0.5 riceve il puntatore sui pixel dell'immagine opachi almeno per metà. La soglia si applica anche all'`OnPress` dell'immagine. Uno sfondo o un bordo della vista contano comunque come disegnati. Zero usa l'intera vista dell'immagine, così come le immagini su Linux.

In una finestra macOS in modalità painted, `Video.HitTestAlpha` (TSX `hitTestAlpha`) applica la stessa soglia ai frame video decodificati. Funziona con `Controls` disattivato e una traccia video non trasformata, come HEVC con alfa per un animaletto da scrivania. Zero, un frame non disponibile e gli altri host desktop usano i limiti della vista video.

`desktop.NewTray` può disegnare un'icona dinamica con `TrayOptions.View` su macOS e Windows. Windows adatta il rendering trasparente della vista alla dimensione delle icone dell'area di notifica e lo aggiorna quando la vista cambia o un'immagine finisce di caricarsi; usa `TrayOptions.OnClick`, `Panel` e `Menu` per le sue azioni. Linux mostra attualmente icone PNG; disegnare la sua vista nella barra di sistema resta un obiettivo.

`desktop.OpenPanel` apre un pannello come quelli che mostrano una scorciatoia del genere o un'icona nella barra di sistema: una finestra senza bordi sopra le altre. Come `desktop.OpenWindow`, `desktop.NewTray`, `mobile.OpenOverlay`, `audio.NewPlayer`, `audio.LoadSound` e `core.NewHolder`, appartiene direttamente al `cx` che passi anziché al proprietario dell'handler in esecuzione, quindi passare lo scope dell'app dall'`OnPress` di un pulsante mantiene viva la finestra, il pannello o il player dopo che lo scope di quel pulsante scompare; le primitive reattive (`Effect`, `Cleanup`, `AfterFunc`, `OnKey`) appartengono invece al proprietario in esecuzione, che `cx.Run` cambia. Una scheda popup si disegna su un pannello con `Transparent`, che non ha sfondo, cornice né ombra propri, così si vedono solo gli angoli arrotondati e il `BoxShadow` della scheda, con spazio per l'ombra all'interno del pannello; `FitContent` dimensiona il pannello in base al contenuto, fino a `Width` e `Height`, prima che venga mostrato la prima volta e quando il contenuto cambia. `desktop.Transparent` indica se in questo momento il pannello non disegna nulla di proprio, come avviene per una finestra trasparente, mentre `desktop.CanMakeWindowsTransparent` indica solo se una finestra potrebbe esserlo: dove è false, come su Linux senza compositor, dai uno sfondo al contenuto.

Su macOS, un pannello senza `Transparent` usa angoli squadrati quando una delle due dimensioni è inferiore a 24 punti e ripristina gli angoli arrotondati alle dimensioni normali.

```go
lookup := desktop.OpenPanel(cx, desktop.PanelOptions{Width: 380, Height: 520, Transparent: true, FitContent: true, HideOnBlur: true}, Card)
desktop.RegisterHotKey(cx, "alt+space", lookup.Toggle)
```

`desktop.NewTray` mostra `TrayOptions.Title` accanto alla sua immagine o vista disegnata su macOS. Per un popup personalizzato, leggi `tray.ClickBounds()` dentro `OnClick` o `Menu` e passa il rettangolo disponibile a `panel.ShowAt(bounds, core.PlaceBottom)`. Usa gli stessi punti dello schermo di `Env.Displays` e include il titolo. I limiti non sono disponibili prima del primo clic, dopo la rimozione, su Linux e quando la shell di Windows non riesce a fornirli. Ogni clic sostituisce i limiti precedenti, così un risultato non disponibile non riutilizza mai un vecchio rettangolo. Impostare `TrayOptions.Panel` mantiene invece il comportamento predefinito di apertura e chiusura al clic.

`PanelOptions.Dock` mantiene un pannello centrato lungo il bordo utilizzabile di uno schermo durante `Show`, `Focus`, `Toggle` e il ridimensionamento del contenuto. Imposta `Edge` su `core.PlaceTop`, `PlaceBottom`, `PlaceLeft` o `PlaceRight`, `Display` su un ID di `Env.Displays` (vuoto per lo schermo principale), e `Margin` sulla distanza in punti da quel bordo. Un pannello agganciato non può usare `ShowAt` o `MoveTo`; `SetWindowFrame` ne cambia solo le dimensioni. Un'icona nella barra di sistema lo mostra nella sua posizione di aggancio anziché sotto l'icona.

```go
panel := desktop.OpenPanel(cx, desktop.PanelOptions{
	Width: 320, Height: 80, NoActivate: true,
	Dock: desktop.PanelDock{Edge: core.PlaceBottom, Margin: 12},
}, Candidates)
panel.OnPlacementError(cx, func(err error) { /* offer another presentation */ })
panel.Show()
```

`desktop.CanDockPanels(cx)` riporta il supporto all'aggancio separatamente dal posizionamento arbitrario delle finestre. macOS, Windows e X11 lo supportano; Wayland richiede la libreria di runtime facoltativa `libgtk4-layer-shell.so.0` e un compositor che implementi `zwlr_layer_shell_v1`. NSGo carica quella libreria prima di GTK; un host incorporante che ha già caricato Wayland deve precaricare gtk4-layer-shell prima di avviare l'host. Una libreria, un protocollo o uno schermo selezionato mancanti lasciano il pannello nascosto e chiamano `OnPlacementError`. Un aggancio su Wayland usa il livello superiore senza riservare spazio di lavoro: `NoActivate` lascia la tastiera alla finestra corrente, mentre un pannello senza di esso prende il focus esclusivo della tastiera finché non viene nascosto. Le normali finestre Wayland mantengono il consueto posizionamento del compositor.

Un registratore di scorciatoie permette all'utente di scegliere un tasto del genere. L'`OnKeyDown` di una vista `Focusable` sente ogni tasto premuto mentre ha il focus, prima dei binding dell'app, con i modificatori tenuti premuti, incluso un modificatore premuto da solo; restituire `true` sottrae il tasto ai binding, così registrare Comando+S non salva. `KeyEvent.Shortcut` scrive la pressione nel formato accettato da `OnKey` e `RegisterHotKey`, ogni modificatore per nome ("ctrl+shift+k", "alt+space", "shift+meta+k" per Comando+Maiusc+K), e "" per un modificatore da solo. Il suo `OnKeyUp` sente il rilascio di ciascuno di quei tasti, anche dopo che `OnKeyDown` lo ha consumato o il focus si è spostato, con i modificatori ancora premuti. Entrambi funzionano su macOS, Windows, Linux, il web, dove il browser mantiene le scorciatoie che non concede a nessuna pagina, Apple TV e in `nsgotest`; Android consegna gli eventi della tastiera fisica alle viste con il focus in modo asincrono, quindi il valore restituito dal loro `OnKeyDown` non può consumare il tasto. iPhone e iPad supportano l'interrogazione periodica dei tasti fisici. Una scorciatoia che l'app ha registrato a livello globale prende il suo tasto prima della vista, quindi un registratore la annulla mentre registra.

```go
core.View{
	Focusable: func() bool { return true },
	OnKeyDown: func(e core.KeyEvent) bool {
		if s := e.Shortcut(); s != "" {
			shortcut.Set(s)
		}
		return true
	},
	OnKeyUp: func(core.KeyEvent) { recording.Set(false) },
	Children: []core.Node{core.Text{ContentFunc: shortcut.Get}},
}
```

Un gioco o un canvas interroga invece i tasti. `core.KeysDown(cx)` elenca i tasti tenuti premuti sulla finestra e `core.KeyHeld(cx, "left")` indica se uno lo è: letture tracciate, che legge anche un ciclo `RequestAnimationFrame`. La prima lettura fa sì che la piattaforma riporti ogni tasto da quel momento in poi, quindi un tasto già premuto resta sconosciuto finché non viene premuto di nuovo; i tasti digitati in un campo o in una web view, o mentre un metodo di input compone, non risultano mai premuti, e un tasto ancora premuto quando la finestra perde il focus viene rilasciato in quel momento. Funzionano dove funziona `OnKeyUp` e su iPhone e iPad con una tastiera fisica.

Per i pulsanti del mouse, `View.OnMouseDown` e `View.OnMouseUp` consegnano un `core.MouseButtonEvent` per ogni pulsante (`core.MouseButtonLeft`, `core.MouseButtonRight`, `core.MouseButtonMiddle`, `core.MouseButtonBack`, `core.MouseButtonForward`) su macOS, Windows, Linux, il web e `nsgotest`, senza rendere la vista una tappa del focus della tastiera. `core.MouseButtonsDown(cx)` e `core.MouseButtonHeld(cx, core.MouseButtonRight)` sono letture tracciate dei pulsanti tenuti premuti sulla finestra, che si azzerano automaticamente quando la finestra perde il focus.

Per un controllo legato a una posizione fisica, leggi `KeyEvent.Code`, `core.KeyCodesDown(cx)` o `core.KeyCodeHeld(cx, "KeyW")`. Il codice usa i nomi di `KeyboardEvent.code`: su AZERTY la posizione W di QWERTY ha `Code: "KeyW"` e `Key: "z"`. `core.KeyCodeLabel(cx, "KeyW")` fornisce l'etichetta di visualizzazione del layout attivo per un messaggio. Su iOS, Android, HarmonyOS e nei browser senza una mappa del layout, usa l'ultimo nome di tasto riportato in quella posizione e restituisce una stringa vuota finché non vede un tasto. HarmonyOS nomina sia la posizione sia il tasto a partire dal `KeyCode` di InputKit, poiché ArkUI non riporta alcuno scan code, quindi lì il nome del tasto non segue ancora un layout diverso da QWERTY.

```go
var loop func(time.Time)
loop = func(time.Time) {
	if core.KeyHeld(cx, "left") {
		x.Update(func(x float32) float32 { return x - 4 })
	}
	if core.KeyHeld(cx, "right") {
		x.Update(func(x float32) float32 { return x + 4 })
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

## Mantenere lo schermo acceso

`cx.KeepAwake("Playing video", core.KeepAwakeOptions{Display: true})` mantiene acceso lo schermo finché non viene eseguita la funzione di rilascio restituita o lo scope corrente non viene eliminato. Su Android il blocco si applica mentre l'app è visibile. Sul web funziona in un contesto sicuro con il supporto di Screen Wake Lock, e la pagina richiede di nuovo il blocco quando torna visibile. Safari può richiedere una pressione o un tasto prima di concedere la prima richiesta. I blocchi riservati al computer non hanno effetto su queste due piattaforme.

## Blocco del puntatore

In una telecamera di gioco o in un trascinamento che deve continuare al bordo dello schermo, chiama `ref.LockPointer()` da una pressione sulla vista che detiene il ref. Il puntatore resta fermo ed è nascosto mentre la vista riceve il movimento relativo in `OnPointerDelta(dx, dy)`. I valori positivi spostano verso destra e verso il basso.

Chiama la funzione restituita per rilasciare il blocco. Lo rilasciano anche Esc, la perdita del focus da parte della finestra, oppure nascondere o rimuovere la vista. Controlla `cx.Env().CanLockPointer()` per sapere se la superficie lo supporta. Il web richiede un gesto dell'utente come una pressione. Linux su Wayland richiede un compositor che supporti i vincoli del puntatore e il movimento relativo.

## Limiti dei suoni

Usa `audio.SetSoundLimits(cx, audio.SoundLimits{MaxSounds: 200, MaxDuration: 20 * time.Second})` prima di caricare i suoni per cambiare i limiti del motore. Si possono impostare anche `MaxEncodedBytes`, `MaxDecodedBytes` e `MaxPlaybacks`. I campi a zero usano i valori predefiniti di 8 MiB codificati e 5 secondi per suono, 64 sorgenti, 16 MiB decodificati in totale tra i suoni, e 32 riproduzioni sovrapposte per output. I suoni esistenti restano caricati e contano per i nuovi limiti. Gli scope con restrizioni mantengono i limiti predefiniti entro il budget del motore: i suoni `nsgo/audio` di un package TSX firmato li raggiungono qualunque cosa abbia impostato l'app Go, mentre il TSX proprio dell'app adotta i limiti dell'app Go. Il SoundPool di Android riproduce al massimo 32 suoni contemporaneamente e decodifica al massimo 1 MiB di ciascuno, qualunque siano i limiti.

## Schermo intero

`desktop.SetFullScreen(cx, true, displayID)` riempie lo schermo indicato da `cx.Env().Displays()` su macOS, Windows e Linux. Ometti l'ID per usare lo schermo corrente della finestra. Un ID sconosciuto lascia la finestra dov'è. Ripetere la stessa richiesta non ha effetto. `desktop.SetFullScreen(cx, false)` esce dallo schermo intero e ripristina il frame precedente della finestra. Sul web, entra da un handler di pressione. Il browser sceglie lo schermo, e `cx.Env().FullScreen()` segue l'entrata e l'uscita, incluso Esc.

## Schermi

`cx.Env().Display()` restituisce il `core.DisplayInfo` corrente della finestra e aggiorna i binding quando la finestra si sposta su un altro schermo. `cx.Env().Displays()` elenca gli schermi riportati dalla piattaforma. `RefreshRate` e `MaxRefreshRate` sono in Hz, con zero per una frequenza non disponibile. iOS riporta il massimo del pannello. HarmonyOS riporta lo schermo della finestra, la sua frequenza corrente e la frequenza massima supportata. Il web riporta lo schermo corrente e contrassegna la misurazione basata sui callback dei frame con `RefreshRateEstimated`. Nei test, imposta `Displays` e `DisplayID` tramite `nsgotest.Environment`.

## Sintesi vocale

`cx.Speak` legge ad alta voce un testo con la voce di sistema per una lingua, dove `cx.Env().CanSpeak()` indica che la piattaforma parla: macOS, iOS, tvOS, Android, Windows, il web e Linux con speech-dispatcher. Viene pronunciato un enunciato alla volta: un altro `Speak` interrompe quello in corso, e lo stesso fanno `cx.StopSpeaking` e la fine dello scope che lo ha pronunciato. `done` riceve una sola volta come è terminato: concluso, interrotto, oppure fallito con un `core.SpeakError`, come `core.SpeakLanguageUnavailable` quando nessuna voce installata parla quella lingua.

```go
cx.Speak("Guten Morgen", core.Voice{Lang: "de-DE", Rate: 0.8}, func(stopped bool, err error) {
	speaking.Set(false)
})
```

`Rate` e `Pitch` vanno da 0.5 a 2, dove 1 è il valore proprio della voce. `cx.Voices("ja", ...)` elenca le voci installate per una lingua, così l'app può chiedere all'utente di installarne una che manca o lasciargliene scegliere una, il cui `ID` va in `core.Voice`. I browser parlano solo dopo che l'utente ha premuto qualcosa nella pagina, e prima di allora falliscono con `core.SpeakNotAllowed`. I componenti Go di un package TSX parlano solo con la capability `text_to_speech`.

## Audio in loop

Imposta `audio.Voice.Loop` per ripetere la riproduzione di un suono fino a `Stop` o `Close`. Per la musica in streaming, imposta `audio.PlayerOptions.LoopStart` e chiama `SetLoop(true)`. Il primo passaggio include l'introduzione. I passaggi successivi partono dall'offset. Un offset pari a zero ripete l'intera traccia.

Su macOS 14 e iOS 17 o versioni successive, una traccia finita usa la coda nativa per preparare ogni ripetizione prima che termini quella corrente. Le versioni Apple precedenti saltano all'inizio del loop quando termina il passaggio corrente, con una breve pausa. Su Linux e sul web, un player in loop decodifica in memoria una traccia fino a 32 MiB e 5 minuti, circa 23 MB al minuto, e la ripete senza pause. Su Linux una traccia più grande o più lunga viene trasmessa in streaming tramite il proprio decoder e si ripete anch'essa senza pause; sul web torna indietro alla fine, con una breve pausa. Su Windows e Android, un loop con un punto di inizio torna a quel punto alla fine, con una breve pausa. Sul web anche un player in loop a una velocità diversa da 1 torna indietro alla fine, poiché la riproduzione senza pause del browser sposterebbe l'intonazione con la velocità. Sul web un player in loop, come un suono, riproduce solo dopo che l'utente ha premuto qualcosa nella pagina, e prima di allora fallisce con `NotAllowed`.

## Biometria

`cx.BiometricAvailability` indica se sono registrati Face ID, Touch ID o un'impronta digitale del dispositivo; Android non dice quale tipo è registrato, quindi lì riporta un'impronta digitale quando ne è registrata una e altrimenti il volto quando il dispositivo ha l'hardware per il riconoscimento facciale. `cx.AuthenticateBiometric` chiede al sistema di confermare l'utente con una motivazione, che non deve essere vuota, e riporta successo, fallimento, annullamento o indisponibilità. Non sceglie un ripiego su codice di accesso per l'app.

```go
cx.BiometricAvailability(func(kind core.BiometricKind) {
	if kind != core.BiometricNone {
		cx.AuthenticateBiometric("Confirm deleting this account", func(result core.BiometricResult) {
			if result == core.BiometricSuccess {
				deleteAccount()
			}
		})
	}
})
```

Le app con Face ID dichiarano `NSFaceIDUsageDescription` sotto `apple.info`; le app Android elencano `USE_BIOMETRIC` sotto `android.permissions`.

## Accesso

`cx.AuthSession` fa accedere l'utente con un provider OAuth nella sessione di autenticazione del sistema anziché in una web view: `ASWebAuthenticationSession` su macOS, iOS e watchOS, una Custom Tab o il browser su Android, il browser predefinito su Windows, Linux e su un Mac senza schema, e un popup sul web. Sul web, la pagina di reindirizzamento invia il suo callback anche tramite un `BroadcastChannel` della stessa origine, così un accesso si completa quando la `Cross-Origin-Opener-Policy` del provider separa il popup dalla pagina. Un popup del genere risulta chiuso alla pagina, quindi chiudere un popup dopo che ha mostrato la pagina del provider termina la sessione come annullata solo dopo 5 minuti; dai alla sessione un pulsante di annullamento. L'app gestisce il protocollo: crea il verifier PKCE e lo state, costruisce l'URL di autorizzazione per il reindirizzamento che le viene fornito, e controlla il callback prima di scambiare il suo codice. Annullare non è fallire: `errors.Is(err, core.AuthCancelled)` indica che l'utente ha chiuso la sessione, o che l'ha terminata la funzione `cancel` restituita.

```go
verifier, challenge, state := newPKCE() // the app's own
cx.AuthSession(func(redirect string) string {
	q := url.Values{"client_id": {clientID(redirect)}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {"openid email"},
		"state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	return "https://accounts.example.com/authorize?" + q.Encode()
}, core.AuthSessionOptions{Scheme: "com.example.app", Path: "/oauth2redirect", Timeout: 5 * time.Minute}, func(callback string, err error) {
	if errors.Is(err, core.AuthCancelled) {
		return
	}
	if err != nil {
		showError(err)
		return
	}
	u, _ := url.Parse(callback)
	if u.Query().Get("state") == state {
		exchangeCode(u.Query().Get("code"), verifier)
	}
})
```

Il reindirizzamento è `com.example.app:/oauth2redirect` dove è impostato uno schema (Windows, Linux e Android ne hanno bisogno in `url_schemes` di `nsgo.yaml` affinché il sistema operativo consegni il callback all'app in esecuzione; la sessione di sistema di macOS, iOS e watchOS lo intercetta senza quella dichiarazione), e `http://127.0.0.1:<port>/oauth2redirect` sul loopback dei desktop, quindi registrali entrambi presso il provider, spesso come client distinti. Lascia `Scheme` vuoto per il solo loopback, oppure impostalo su `https` con `Host` per il dominio associato dell'app, che iOS 17.4, macOS 14.4 e watchOS 10.4 intercettano. watchOS richiede uno schema o un dominio HTTPS associato; non ha un ripiego sul loopback, e resta valido il requisito minimo di watchOS 26 già esistente in NSGo. Una scheda del browser chiusa non dice nulla a una sessione loopback, quindi dalle un `Timeout` o un pulsante di annullamento. `Ephemeral` chiede a macOS, iOS e watchOS di non condividere i cookie del browser, e su Android richiede una Custom Tab privata dove il browser la supporta; non dare per scontato l'isolamento dei cookie su Android. tvOS e i mini programmi rispondono per ora `core.AuthUnavailable`. In TSX, `AuthSession.start(url, options)` richiede la capability `auth_session` e fornisce una sessione il cui `result` si risolve con il callback, o con null una volta annullata.

Android consuma un callback corrispondente prima del normale `OnOpenLinks`, e tornare senza un callback termina la sessione come annullata. Il suo callback deve usare uno schema privato di proprietà dell'app; uno schema vuoto, un callback HTTPS o uno schema non dichiarato restituiscono `core.AuthUnavailable`, così come un runtime Android compilato prima delle sessioni di accesso.

Una sessione nel browser consuma solo il proprio link di callback corrispondente; i link non correlati o in ritardo raggiungono comunque `OnOpenLinks`. Avviare un'altra sessione nel browser per lo stesso URI di callback annulla quella precedente; URI distinti possono restare in attesa in finestre diverse.

Usa esattamente il reindirizzamento passato alla funzione dell'URL di autorizzazione. I reindirizzamenti web e loopback del desktop codificano i percorsi Unicode e risolvono i segmenti con punti prima che la funzione li riceva; un `Path` loopback è testo decodificato, quindi anche un `%` letterale viene sottoposto a escape.

I mini programmi usano invece `cx.SignInMiniProgram(core.MiniProgramSignInOptions{}, done)`. Il callback riceve una `core.MiniProgramCredential` con la sua `Platform` (`MiniProgramWeChat`, `MiniProgramQQ`, `MiniProgramDouyin` o `MiniProgramAlipay`) e un `Code` non vuoto e di breve durata. Invialo al tuo server per lo scambio del codice dell'host; tieni lì il segreto dell'app. Si tratta di `wx.login`/`qq.login`/`tt.login`, o dell'autorizzazione di base `auth_base` di Alipay, senza richiedere informazioni del profilo. Non apre un URL né esegue un costruttore di URL OAuth.

`cx.Env().CanSignInMiniProgram()` riporta la disponibilità dell'API dell'host. Gli altri target rispondono `core.MiniProgramSignInUnavailable`; il codice senza `core.AccessMiniProgramSignIn` riceve `AccessDenied`. Douyin chiede in modo silenzioso per impostazione predefinita: un host senza accesso restituisce `MiniProgramSignInNotSignedIn`, e `MiniProgramSignInOptions{Prompt: true}` consente la sua finestra di accesso con l'account dell'host. Chiudere quella finestra restituisce `MiniProgramSignInCancelled`. Un codice anonimo del dispositivo non conta mai come utente che ha effettuato l'accesso. I test impostano `nsgotest.Environment(core.Environment{MiniProgramSignIn: true})` e `app.MiniProgramSignIn`; `app.MiniProgramSignIns` registra le richieste. TSX non viene eseguito nei mini programmi.

Un'app le cui web view devono risultare con l'accesso già effettuato dall'utente altrove, come nel suo browser, scrive i cookie di quell'accesso con `cx.SetWebCookies`, inclusi quelli HttpOnly, che gli script della pagina non possono impostare, poi carica le pagine: done viene eseguito una volta che l'archivio li contiene. L'archivio è quello persistente condiviso da tutte le web view dell'app, quindi i cookie con un `Expires` sopravvivono all'app; le viste in incognito non li vedono, tranne su Android, che mantiene un solo archivio per app. HarmonyOS, il web, tvOS e watchOS rispondono `Unavailable`.

```go
cx.SetWebCookies([]core.WebCookie{
	{Name: "SID", Value: sid, Domain: ".example.com", Expires: expires, Secure: true, HTTPOnly: true},
}, func(err error) {
	if err == nil {
		page.Set(core.WebSrc("https://www.example.com/"))
	}
})
```

Per rimuovere un cookie, scrivilo di nuovo con lo stesso nome, dominio e percorso e con `Expires` nel passato; una scadenza pari a zero crea un cookie di sessione. Attendi il callback prima di caricare una pagina che deve rilevare la rimozione.

Al contrario, `cx.WebCookies` elenca i cookie dell'archivio, inclusi quelli HttpOnly con il loro dominio, percorso, scadenza e flag, che `document.cookie` non può leggere, che sia visibile o meno una pagina del sito: un `core.WebCookieFilter` sceglie quelli che una richiesta a un `URL` invierebbe, o quelli di un `Domain` e di ogni host sottostante, e il filtro zero li sceglie tutti. Un'app che chiama il sito da Go con l'accesso dell'utente, o che mostra dove l'utente ha effettuato l'accesso, li legge in questo modo. `cx.ClearWebsiteData` disconnette l'utente da ogni sito: rimuove cookie, local e session storage, IndexedDB, service worker e cache dell'archivio, e done viene eseguito una volta che sono stati eliminati. Il `CookieManager` di Android risponde solo con i nomi e i valori che un URL invierebbe, quindi lì elenca solo un filtro `URL`, che riempie `Domain` con il suo host e lascia vuoti gli altri campi, mentre un filtro `Domain` o il filtro zero rispondono `Unavailable`; la pulizia lì lascia la cache HTTP. HarmonyOS, il web, tvOS e watchOS rispondono `Unavailable` a entrambi.

```go
cx.WebCookies(core.WebCookieFilter{Domain: "example.com"}, func(cookies []core.WebCookie, err error) {
	signedIn.Set(err == nil && slices.ContainsFunc(cookies, func(c core.WebCookie) bool { return c.Name == "SID" }))
})
```

## Accedi con Apple

`cx.SignInWithApple` mostra il foglio di sistema Accedi con Apple dal pulsante proprio dell'app su macOS, iOS e tvOS, dove `cx.Env().CanSignInWithApple()` è true. Risponde una volta con la credenziale, oppure con un errore: `core.AppleSignInCancelled` quando l'utente chiude il foglio, che non è un fallimento, altrimenti `AppleSignInFailed` o, sulle altre piattaforme, `AppleSignInUnavailable`. Apple fornisce l'email e il nome solo al primo accesso dell'utente all'app, quindi conservali in quel momento. `Nonce` entra nella richiesta come il suo SHA-256 in esadecimale minuscolo, contenuto nel claim `nonce` dell'identity token: il server calcola l'hash del nonce che ha dato all'app e confronta. `cx.AppleCredentialState` indica se l'utente l'ha revocata nel frattempo. Nei test headless, `app.AppleSignIn` riceve una `nsgotest.AppleSignInRequest` e fornisce la credenziale o l'errore.

```go
nonce := rand.Text() // also sent to the server, which checks the token's claim against its SHA-256
cx.SignInWithApple(core.AppleSignInOptions{FullName: true, Email: true, Nonce: nonce}, func(c core.AppleCredential, err error) {
	switch {
	case errors.Is(err, core.AppleSignInCancelled):
	case err != nil:
		showError(err)
	default:
		signInOnServer(c.IdentityToken, c.AuthorizationCode, nonce, c.GivenName, c.FamilyName, c.Email)
	}
})
cx.AppleCredentialState(savedUser, func(s core.AppleCredentialState, err error) {
	if err == nil && s == core.AppleCredentialRevoked {
		signOut()
	}
})
```

L'app deve essere firmata con l'entitlement Accedi con Apple, che `build` aggiunge quando `nsgo.yaml` lo richiede; senza di esso il foglio fallisce:

```yaml
apple:
  sign_in_with_apple: true  # com.apple.developer.applesignin, in iOS and tvOS packages and Mac App Store builds
```

Xcode registra la capability per le build su dispositivo da `nsgo dev`; un provisioning profile in `sign.profile` o `sign.macos_profile` deve provenire da un App ID con Accedi con Apple attivato. macOS la concede solo con un profilo, quindi un'immagine disco o un'esecuzione di `nsgo dev` su un Mac non possono effettuare l'accesso, e nemmeno il simulatore, le cui build non dichiarano alcun entitlement da sviluppatore.

## Acquisti in-app

`cx.Products`, `cx.Purchase` e le chiamate affini vendono tramite l'App Store su macOS, iOS e tvOS, dove `cx.Env().CanPurchase()` è true, in una build il cui `nsgo.yaml` richiede StoreKit:

```yaml
apple:
  purchases: true  # StoreKit's library, in macOS, iOS and tvOS packages; no entitlement is needed
```

Mostra i prezzi forniti dallo store, `DisplayPrice`, mai prezzi stabiliti dall'app: lo store fissa da sé i prezzi di ogni vetrina. Un acquisto risponde una volta, con la transazione o un errore: `core.PurchaseCancelled` quando l'utente chiude il foglio, `core.PurchasePending` mentre attende un'approvazione, come fa Chiedi di acquistare, e altrimenti `core.PurchaseFailed`, con il messaggio dello store, oppure `PurchasesUnavailable` altrove. Invia il `Signed` della transazione, il JWS dello store, al server dell'app per verificarlo, consegna ciò che è stato acquistato, poi concludila; lo store ripresenta all'app una transazione non conclusa a ogni avvio.

`cx.ManageSubscriptions` apre l'interfaccia di gestione degli abbonamenti di Apple (il foglio di sistema su iOS, la pagina degli abbonamenti dell'App Store su macOS e tvOS). `cx.RequestRefund` avvia una richiesta di rimborso per una transazione e risponde `core.RefundSuccess` o `core.RefundCancelled`.

```go
cx.OnPurchaseUpdate(func(t core.Transaction) { // renewals, approved purchases, other devices', refunds, and unfinished ones at launch
	verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
})
cx.Products([]string{"com.example.pro.monthly"}, func(products []core.Product, err error) {
	if err == nil && len(products) > 0 {
		price.Set(products[0].DisplayPrice)
	}
})
cx.Purchase("com.example.pro.monthly", core.PurchaseOptions{AccountToken: userUUID}, func(t core.Transaction, err error) {
	switch {
	case errors.Is(err, core.PurchaseCancelled):
	case errors.Is(err, core.PurchasePending):
		showWaiting() // its transaction comes through OnPurchaseUpdate
	case err != nil:
		showError(err)
	default:
		verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
	}
})
```

`FinishTransaction` restituisce `PurchaseNotFound` per una transazione già conclusa, inclusa una restituita da `CurrentEntitlements`; un articolo non consumabile concluso resta di proprietà dell'utente.

Registra `OnPurchaseUpdate` all'avvio dell'app: gli aggiornamenti che arrivano prima di qualsiasi handler attendono il primo. `cx.CurrentEntitlements` risponde con ciò che l'utente possiede ora, per sbloccarlo all'avvio, e un pulsante Ripristina acquisti chiama `cx.SyncPurchases`, che può chiedere all'utente di effettuare l'accesso. `AccountToken` è un UUID proprio dell'app che la transazione e le notifiche del server dell'App Store riportano. macOS vende solo in una build per il Mac App Store, e un App Clip non vende nulla; Android, watchOS, Windows, Linux, il web e i mini programmi rispondono per ora `PurchasesUnavailable`. Testa gli acquisti con `nsgotest`, i cui `Products`, `PurchaseAnswer`, `UpdatePurchase` e `UnfinishedTransactions` fanno le veci dello store. `app.PurchaseRequests` registra le operazioni dello store come valori `nsgotest.PurchaseRequest`. In TSX, il modulo `Purchases` richiede la capability `purchases`: `purchase` si risolve con `{ status: "purchased", transaction }`, `"pending"` o `"cancelled"`, e `on("update", listener)` riceve gli aggiornamenti.

## Orientamento dell'interfaccia

Chiama `cx.RequestOrientation(core.OrientationLandscape)` nello scope di un player a schermo intero per chiedere a iOS, Android, HarmonyOS o al browser l'orientamento orizzontale. Ha effetto la richiesta più recente il cui scope è ancora attivo. Quando quello scope termina, riprende la richiesta precedente oppure l'app torna alla sua consueta politica di orientamento.

Su iOS 16 e versioni successive la richiesta può ruotare l'interfaccia immediatamente. Sulle versioni di iOS precedenti limita la successiva rotazione del sistema. Le `IOS.SupportedOrientations` di una finestra modale presentata hanno la precedenza. Su Android e HarmonyOS, una combinazione di direzioni verticali e orizzontali che il sistema non può rappresentare viene ridotta a una sola direzione consentita, preferendo quella già mostrata. Android può ignorare le richieste di orientamento nelle modalità a schermo grande o multi-finestra.

In un'app web, chiama `desktop.ToggleFullScreen(cx)` da un handler di pressione che apre lo scope del player. Il browser entra a schermo intero a partire dalla pressione, poi NSGo applica la richiesta di orientamento del player. Uscire dallo schermo intero rilascia il blocco. La console del browser riporta le operazioni non supportate e i rifiuti.

`nsgotest.App.Orientation` registra l'ultima maschera richiesta, con zero per la consueta politica di orientamento.

## Recensioni sugli store

`cx.RequestReview()` richiede un invito a valutare l'app all'interno dell'app stessa dall'App Store su macOS e iOS, o da Google Play su Android. `cx.Env().CanRequestReview()` indica se il servizio è disponibile. Lo store decide se e quando mostrare l'invito e non riporta alcun risultato. Usa una build di sviluppo per testare l'invito di Apple. Le build TestFlight lo sopprimono.

I package Android standalone includono l'SDK Play Review, la sua activity di dialogo e la query di visibilità del Play Store; per compilarli non servono né l'Android SDK né un JDK. Un dispositivo senza un Play Store abilitato riporta il servizio come non disponibile. Testa un package pubblicato usando la [Condivisione interna delle app](https://developer.android.com/guide/playcore/in-app-review/test).

Quando incorpori NSGo, aggiungi `com.google.android.play:review:2.0.2` all'host Gradle. Nel manifest dell'host, aggiungi `<package android:name="com.android.vending" />` sotto `<queries>` così che l'app possa vedere il Play Store. Se l'host riduce il proprio codice, preserva le API Play Review e Tasks con le regole seguenti, poiché NSGo le carica per nome.

```proguard
-keep class com.google.android.play.core.review.ReviewManagerFactory { *; }
-keep interface com.google.android.play.core.review.ReviewManager { *; }
-keep class com.google.android.play.core.review.ReviewInfo { *; }
-keep class com.google.android.gms.tasks.Task { *; }
-keep interface com.google.android.gms.tasks.OnCompleteListener { *; }
```

Nei test, imposta `CanRequestReview` in `nsgotest.Environment` e controlla `nsgotest.App.ReviewRequests`.

## Gesti di sistema dai bordi

Un gioco con controlli touch al bordo dello schermo può chiamare `mobile.DeferSystemGestures` mentre i controlli sono attivi. Usa la funzione restituita per rilasciare la richiesta in anticipo. La richiesta termina anche quando il suo scope viene ripulito. Le richieste di più scope nella stessa finestra si combinano.

```go
release := mobile.DeferSystemGestures(cx, mobile.SystemGestureOptions{
	IOSEdges: mobile.GestureEdgeBottom,
	AndroidRects: []core.Bounds{{
		Origin: core.Point{Y: 120},
		Size:   core.Size{Width: 24, Height: 96},
	}},
})
```

iOS usa i bordi fisici dello schermo. Android 10 e versioni successive usano rettangoli in punti della superficie misurati dall'angolo superiore sinistro. Mantieni i rettangoli attorno ai controlli che ne hanno bisogno. Android si riserva il gesto Home e limita l'area che un'app può escludere dal gesto Indietro. `nsgotest.App.SystemGestures` fornisce ai test la richiesta combinata corrente sulla loro superficie principale. Usa `Surface.SystemGestures()` per un'altra finestra.

## Overlay su Android

`mobile.OpenOverlay` mostra una vista dell'app sopra le altre app su Android, un albero proprio che condivide i segnali dell'app, e un servizio in primo piano mantiene l'app in esecuzione mentre è visibile. L'app elenca `SYSTEM_ALERT_WINDOW` sotto `android.permissions`, e l'utente lo concede nelle impostazioni del sistema, che `mobile.RequestOverlayPermission` apre; senza di esso l'overlay termina subito e `OnEnd` ne riceve il motivo. Riceve i tocchi ma lascia i tasti e i tocchi attorno all'app in primo piano, a meno che non sia `Focusable` o `Passive`, e si sposta con `desktop.SetWindowFrame` dal suo contenuto. Le altre piattaforme non ne aprono nessuno.

```go
mobile.RequestOverlayPermission(cx, func(p core.OverlayPermission) {
	if p == mobile.OverlayGranted {
		mobile.OpenOverlay(cx, mobile.OverlayOptions{Width: 200, Height: 80}, Score)
	}
})
```

## Posizione e movimento

`cx.Locate` ottiene una posizione, `cx.WatchLocation` trasmette un flusso di posizioni e `cx.WatchMotion` trasmette un flusso di campioni di accelerometro, giroscopio, magnetometro e movimento del dispositivo combinato. Ciascuno restituisce uno `*core.SensorStream` il cui `Stop` lo annulla, come fa la fine dello scope che lo ha avviato; i callback vengono eseguiti sulla goroutine del motore. I permessi sono `core.PermissionLocation`, `PermissionBackgroundLocation` e `PermissionMotion` ([permessi](permissions.md)): avviare un flusso non chiede mai nulla, e un flusso senza il suo permesso fallisce con `core.SensorNotAllowed`.

```go
cx.RequestPermission(core.PermissionLocation, func(s core.PermissionStatus) {
	if !s.Allowed() {
		return
	}
	cx.Locate(core.LocationOptions{}, func(f core.LocationFix, err error) {
		if err == nil {
			showPosition(f.Latitude, f.Longitude, f.Approximate)
		}
	})
})
```

`Locate` attende una nuova posizione, 30 secondi per impostazione predefinita (`Timeout`); solo `MaxAge` gli permette di prenderne una in cache, che lo segnala con `Cached` e `Age`. `Approximate` indica una posizione che l'utente ha consentito solo in modo approssimativo; Android aggiorna una posizione del genere al massimo ogni dieci minuti, quindi lì dai a `Locate` un `MaxAge`. Le coordinate sono WGS 84; una misura che manca al provider ha `Valid` false, e `Altitude` è ellissoidale mentre `MSLAltitude` è sopra il livello medio del mare. Il movimento usa gli assi naturali del dispositivo, l'accelerazione in m/s² (+9.81 su z a faccia in su), la rotazione in rad/s e il campo magnetico in µT, ogni 20 ms per impostazione predefinita, entro 10 ms–1 s. Sotto carico un flusso mantiene il campione più recente per sensore e conta quelli sostituiti in `Dropped`.

I flussi proseguono mentre l'app è inattiva e si mettono in pausa in background, contrassegnando con `Gap` il primo campione successivo; un `Locate` in sospeso mantiene il resto del suo timeout. `LocationOptions.Background` mantiene attivo un flusso di posizione in background sotto `PermissionBackgroundLocation`: le app Apple aggiungono la modalità in background `location`, e su Android un servizio in primo piano mostra una notifica con `NotificationTitle` e `NotificationBody` e mantiene l'app in esecuzione dopo la fine della sua activity. Niente riavvia un processo che il sistema ha terminato.

```yaml
apple:
  info:
    NSLocationWhenInUseUsageDescription: Shows where you are on the map.
    NSLocationUsageDescription: Shows where you are on the map.  # macOS
    NSMotionUsageDescription: Counts your reps.
    UIBackgroundModes: [location]                                # background streams
android:
  permissions: [ACCESS_COARSE_LOCATION, ACCESS_FINE_LOCATION, FOREGROUND_SERVICE, FOREGROUND_SERVICE_LOCATION]
```

iOS e watchOS forniscono posizione e movimento, macOS la posizione, tvOS una posizione alla volta e Android entrambi tramite i provider propri del framework. Linux ottiene posizioni e flussi da GeoClue 2 sul bus di sistema, il cui agente interpella l'utente quando inizia la prima richiesta dell'app; poiché a GeoClue non si può chiedere senza mostrare una richiesta, il permesso risulta non ancora richiesto finché l'app non lo ha chiesto in quell'esecuzione, e la posizione in background non è disponibile. Windows le ottiene dal `Geolocator` di WinRT; il permesso risulta negato mentre la posizione è disattivata per l'app, concesso una volta che l'app lo ha chiesto in quell'esecuzione ed è stata autorizzata, e altrimenti non ancora richiesto, e anche lì la posizione in background non è disponibile. Un browser in contesto sicuro fornisce posizioni di geolocalizzazione e, su mobile, letture di accelerometro, giroscopio e movimento combinato. Chiama `cx.RequestPermission(core.PermissionMotion, done)` da un handler di pressione su Safari. Il browser riporta la posizione come non disponibile senza `navigator.geolocation`, e il movimento come non disponibile sui desktop e per i flussi del magnetometro. In TSX, `Sensors` ha `permission`, `requestPermission`, `locate`, `watchLocation` e `watchMotion`, sotto le capability del package `location`, `backgroundLocation` e `motion`, in millisecondi.

## Audio

Il package `audio` riproduce brevi suoni decodificati una volta in memoria (`audio.LoadSound`) e audio lungo in streaming (`audio.NewPlayer`) dove `cx.Env().CanPlayAudio()` è true; altrove le chiamate falliscono con `audio.Unavailable`.

```go
audio.LoadSound(cx, audio.Source{URI: "asset:sounds/click.wav"}, func(click *audio.Sound, err error) {
	if err != nil {
		return
	}
	click.Play(audio.Voice{})                                  // center, full volume, normal rate
	click.Play(audio.Voice{Volume: 0.5, Rate: 1.2, Pan: -0.5}) // overlaps, panned left
})
```

`audio.Voice` imposta per ogni riproduzione `Volume` (da 0 a 1, 0 vale 1), `Rate` (da 0.5 a 2, 0 vale 1) e `Pan` (da -1 tutto a sinistra a 1 tutto a destra, 0 al centro); i valori fuori da questi intervalli vengono limitati e NaN assume il valore predefinito. `Sound.Stop` interrompe tutte le riproduzioni del suono e `Close` lo libera, come fa il suo scope proprietario quando viene eliminato. `Player` supporta `Play`, `Preload`, `Pause`, `Seek`, `SetVolume`, `SetRate`, `SetLoop` e `Close`. macOS, iOS, Windows, Linux, Android e il web riproducono audio già oggi. I backend Apple, Windows e browser supportano WAV, AAC e MP3. Linux usa i codec di GStreamer, e il suo player in streaming riproduce a 1×. Android mette in pausa un player quando la sua vista lascia lo schermo e limita ogni suono decodificato a 1 MiB. tvOS, watchOS, HarmonyOS e i mini programmi riportano ancora `CanPlayAudio` false.

I browser richiedono un gesto dell'utente per la riproduzione udibile. Carica in anticipo i suoni brevi, poi chiama `Sound.Play` dopo un clic, un tocco o la pressione di un tasto. Un suono riprodotto prima di un gesto è muto. Il callback di `Play` di un player riceve `audio.NotAllowed` se il browser blocca la riproduzione. Riprova a partire da un gesto dell'utente.

`Sound.Play` restituisce un `audio.Playback`, che su macOS, iOS, Windows, Linux e il web può impostare il proprio volume e la propria velocità o interrompersi in modo indipendente. `Ended()` è reattivo e diventa true quando quella riproduzione termina, si interrompe, viene sostituita a causa del limite di voci o non riesce ad avviarsi. I backend senza controlli individuali restituiscono un handle già terminato.

Su iOS, un motore audio proprio di una libreria che riproduce al di fuori di `audio` e `Video`, come una `AudioQueue`, detiene l'unica sessione audio dell'app con `audio.HoldSession(core.AudioSessionPlayback)` mentre riproduce e chiama la funzione di rilascio restituita quando si ferma. Accetta gli stessi livelli dell'`AudioSession` di un player, accanto ai video, ai player e alle registrazioni dell'app, quindi `AudioSessionPlayback` e `AudioSessionMixWithOthers` continuano a riprodurre con l'interruttore Suoneria/Silenzioso su silenzioso. Altrove non detiene nulla.

Usa `audio.Voice{Muted: true}` per avviare un effetto in silenzio e `playback.SetVolume(0)` per silenziare un effetto attivo. Per l'audio lungo, `audio.NewPlayer` trasmette una sorgente in streaming e `player.SetMuted(true)` silenzia l'output mentre la riproduzione continua. Riattivare l'audio ripristina il volume configurato del player. `Voice.Volume` e `Player.SetVolume` usano zero per il volume pieno predefinito.

In TSX, `nsgo/audio` espone `supported()` e `loadSound(uri)`, condizionati dalla capability del package `audio`. `loadSound` si risolve con un suono il cui `play({ volume, rate, pan, loop, muted })` restituisce una riproduzione con `setVolume`, `setRate`, `stop` e un `ended` reattivo; lì, un volume pari a 0 è silenzio. Un caricamento rifiutato viene respinto con un `Error` il cui `reason` è il nome dell'errore audio, come `quota-exceeded`, e il cui `code` è `ERR_ACCESS_DENIED` senza la capability, `ERR_NSH_QUOTA_EXCEEDED` oltre un limite e `ERR_FEATURE_UNAVAILABLE_ON_PLATFORM` dove nulla riproduce audio. Un suono viene rilasciato solo da `close()` o quando la sua vista scompare; in una sandbox ogni caricamento conta per `Limits.Sounds` (256 per impostazione predefinita) finché non fallisce o il suo suono non viene chiuso.

## Gamepad

Il package `gamepad` legge i controller di gioco collegati nel layout W3C Standard Gamepad (da `ButtonSouth`/`ButtonA` a `ButtonHome`, `AxisLeftX`/`AxisLeftY`/`AxisRightX`/`AxisRightY`). `cx.Env().SupportsGamepads()` indica se il target supporta i controller (macOS, iOS, Windows, Linux, Android e browser con la Gamepad API).

`gamepad.Pads(cx)` è una lettura reattiva tracciata che fa rieseguire il rendering degli scope quando i controller si collegano o si scollegano, senza rieseguirlo ai cambiamenti di pulsanti o levette. Ogni `gamepad.Pad` espone la sua `Family` (`FamilyXbox`, `FamilyPlayStation`, `FamilyNintendo` o `FamilyGeneric`, classificata anche tramite `gamepad.DetectFamily`), il `VendorID` e il `ProductID` USB dove la piattaforma li riporta, e `CanRumble` per il supporto dei motori aptici. All'interno di un ciclo `RequestAnimationFrame`, `gamepad.State(cx, id)` restituisce i pulsanti del controller (`[0, 1]`, con `Pad.Pressed` true a `>= 0.5`) e gli assi (`[-1, 1]`, negativi a sinistra/in alto e positivi a destra/in basso) fotografati una volta per frame dello schermo.

`gamepad.Rumble(cx, id, lowFreq, highFreq, duration)` aziona i motori di vibrazione a bassa frequenza (pesante/sinistro) e ad alta frequenza (leggero/destro) con intensità limitate a `[0, 1]`, sostituendo qualsiasi vibrazione in corso su quel controller e fermandosi automaticamente dopo `duration`, quando l'app va in background o perde il focus (`cx.Env().Lifecycle() != core.LifecycleActive`), quando `cx` viene eliminato o quando il controller si scollega. `gamepad.StopRumble(cx, id)` interrompe la vibrazione in anticipo; i controller con `CanRumble == false` ignorano senza problemi le chiamate di vibrazione.

```go
var loop func(time.Time)
loop = func(time.Time) {
	if pad, ok := gamepad.State(cx, 0); ok {
		move(pad.Axes[gamepad.AxisLeftX], pad.Axes[gamepad.AxisLeftY], pad.Pressed(gamepad.ButtonA))
		if hit {
			gamepad.Rumble(cx, pad.ID, 0.8, 0.3, 180*time.Millisecond)
		}
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

Chiama `gamepad.EnableFocusNavigation(cx)` per abilitare la navigazione tra le tappe del focus nativo con un controller (il D-pad e la levetta sinistra spostano il focus, `ButtonSouth` attiva il controllo con il focus e `ButtonEast` attiva Indietro). Le viste di gioco attive chiamano `gamepad.CaptureInput(cx)` per sospendere la navigazione del focus finché sono montate. In TSX, `nsgo/gamepad` espone `supported()`, `pads()`, `state(id)`, `Button` e `Axis`, condizionati dalla capability del package `gamepad`.

## Temporizzazione audio

Usa `audio.Now(cx)` come orologio per la riproduzione temporizzata. Imposta `audio.Voice.StartAt` a un istante futuro quando `audio.CanSchedule(cx)` è true. Chiama `audio.OutputLatency(cx, func(latency time.Duration, known bool) { ... })` per il percorso di output attivo. macOS, iOS, Linux e il web supportano gli avvii temporizzati; su Windows e Android `CanSchedule` è false e uno `StartAt` riproduce subito. Il valore `known` del callback indica se il percorso ha riportato una latenza, cosa che Windows e Android non fanno.

## Servizi di gioco

Il package `gameservices` fornisce un'API indipendente dal provider per l'accesso del giocatore, gli obiettivi, le classifiche e gli overlay nativi di Game Center / Play Games dove `cx.Env().CanUseGameServices()` è true. Ogni callback viene eseguito sulla goroutine del motore, quindi può impostare direttamente i segnali senza bloccare i frame.

```go
if cx.Env().CanUseGameServices() {
	gameservices.Authenticate(cx, func(p gameservices.Player, err error) {
		if err == nil && p.Authenticated {
			playerName.Set(p.DisplayName)
			gameservices.ReportAchievement(cx, "first-run", 100, nil)
			gameservices.SubmitScore(cx, "high-scores", 1250, nil)
		}
	})
}
```

`gameservices.ReportAchievement` limita l'avanzamento a `0..100` (`100` sblocca l'obiettivo). `gameservices.LoadScores` legge le righe classificate `gameservices.ScoreEntry` per `Global` o `Friends` su `AllTime`, `Week` o `Today`, e `gameservices.ShowAchievements` e `gameservices.ShowLeaderboard` aprono gli overlay nativi della piattaforma. macOS, iOS e tvOS raggiungono GameKit tramite purego; Android raggiungerà Play Games Services tramite un hook del runtime Java che non è ancora presente nel runtime, quindi per ora riporta `CanUseGameServices() == false`; Windows, Linux, il web, watchOS e i mini programmi riportano `CanUseGameServices() == false` e rispondono `gameservices.ErrUnsupported`.

Nei test, abilita i servizi di gioco con `nsgotest.Environment(core.Environment{CanUseGameServices: true})` e pilota il finto servizio isolato di ogni app con `App.SetGameServicesPlayer`, `App.SetGameServicesFriends`, `App.SeedLeaderboardScores`, `App.SetGameServicesError`, `App.GameServicesAchievements`, `App.GameServicesScores`, `App.GameServicesShownAchievements` e `App.GameServicesShownLeaderboards`.

## Prossimi passi

- [Notifiche](notifications.md): permesso, badge e pressioni.
- [Moduli](forms.md): campi di testo e tastiera.
- [Incorporamento](embedding.md): i servizi che un'app host fornisce alla sua app incorporata.
- [Compilazione e distribuzione](building.md): i permessi e i file di cui un package ha bisogno.

### Fitness, regioni e movimento a durata limitata

`cx.WatchSteps`, `cx.WatchActivity` e `cx.WatchPressure` restituiscono flussi legati allo scope. Chiedi `core.PermissionActivityRecognition` prima di passi e attività, e prima della pressione su Apple. Apple fornisce tutti e tre tramite Core Motion; Android fornisce i contapassi e la pressione, e riporta come non disponibile la classificazione dell'attività. I conteggi appartengono alla finestra `Since` del campione; le finestre riprese portano `Gap`. La pressione è in pascal, con un'altitudine relativa facoltativa in metri. Distanze, piani e altitudine mancanti hanno `Valid` false.

`cx.WatchGeofences` accetta delle `core.GeofenceOptions` con da uno a venti cerchi `GeofenceRegion` distinti (coordinate WGS 84 e raggio in metri). Riporta `GeofenceInside`, `GeofenceOutside` o `GeofenceUnknown` per ogni regione. `Background` richiede l'accesso e le dichiarazioni per la posizione in background e l'autorizzazione Sempre di Apple; Android richiede anche un titolo per la notifica. L'arresto o l'eliminazione rimuove le sue registrazioni; nessuna sottoscrizione sopravvive alla morte del processo.

`cx.RequestPreciseLocation("route", done)` chiede esplicitamente l'autorizzazione alla posizione precisa e risponde con un booleano, mai con una posizione. In `apple.info`, dichiara `NSLocationTemporaryUsageDescriptionDictionary: {route: Explains why this feature needs precision.}`. Android richiede sia il permesso approssimativo sia quello preciso. Una risposta false può lasciare autorizzata la posizione approssimativa.

`cx.WatchBackgroundMotion(core.BackgroundMotionOptions{Motion: core.MotionOptions{}, Duration: 20 * time.Second, NotificationTitle: "Recording motion"}, sample, failed)` richiede un flusso dal vivo di durata finita. Richiede `AccessMotion` e `AccessBackgroundMotion`; i package TSX dichiarano `motion` e `backgroundMotion`. iOS può far scadere prima la sua concessione di esecuzione. Le app Android impostano `android.background_motion: true` e dichiarano `FOREGROUND_SERVICE`; il servizio non persistente si ferma entro tre minuti anche se la durata richiesta è maggiore. La durata o la scadenza imposta dal sistema operativo riportano `SensorTimedOut`. L'arresto o l'eliminazione termina il provider e la concessione; nulla viene registrato dopo la terminazione. Gli altri target riportano l'indisponibilità.

`Sensors` in TSX rispecchia questi sei metodi e usa i millisecondi. I test headless forniscono dati con `FeedSteps`, `FeedActivity`, `FeedPressure` e `FeedGeofence`, e impostano `PreciseLocationAnswer` per le richieste di precisione. Il comportamento reale di fitness, il risveglio per regione e il comportamento in background sospeso richiedono ancora un collaudo su dispositivo.
