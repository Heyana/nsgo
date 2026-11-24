---
title: Librerie native
description: Distribuisci librerie condivise precompilate con un pacchetto Go attraverso i normali passaggi di pacchettizzazione, licenza e firma di NSGo.
group: Strumenti
order: 23
---

Un pacchetto Go può distribuire le librerie condivise che carica tramite purego inserendo `nsgo-natives.yaml` nella directory del pacchetto. Quando un'app importa il pacchetto, NSGo seleziona le sue dichiarazioni per il target, verifica i file e li include prima della firma e della creazione degli installer. L'app non ha bisogno di configurazioni aggiuntive né di un comando di pacchettizzazione.

## Dichiarare le librerie

```yaml
version: 1
libraries:
  macos_arm64:
    - name: libexample.dylib
      path: lib/macos-arm64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  macos_x64:
    - name: libexample.dylib
      url: https://downloads.example.org/v1.0/macos-x64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  windows_arm64: []
  windows_x64: []
  linux_arm64: []
  linux_x64: []
```

Le chiavi supportate sono `macos_arm64`, `macos_x64`, `windows_arm64`, `windows_x64`, `linux_arm64` e `linux_x64`. Dichiara ogni target su cui il pacchetto Go viene selezionato; `[]` indica esplicitamente che quel target non ha bisogno di alcuna libreria inclusa. In alternativa, i build tag di Go possono escludere il pacchetto dai target che non supporta. `macos_universal` usa le due voci macOS.

Ogni libreria ha:

- `name`: il nome del file installato, che termina in `.dylib` su macOS, `.dll` su Windows, oppure `.so` o un `.so.*` con versione su Linux.
- Esattamente uno tra `path`, un file relativo a questa directory del pacchetto, e `url`, un URL HTTP(S) della libreria stessa. Pubblica le librerie già estratte anziché URL di archivi. Gli URL non contengono nomi utente e password incorporati.
- `sha256`: lo SHA-256 del file originale, obbligatorio per entrambe le sorgenti. Per esempio, `shasum -a 256 lib/macos-arm64/libexample.dylib` lo stampa su macOS.
- `licenses`: percorsi non vuoti ai file originali di licenza e di avviso, relativi a questa directory del pacchetto. Questi testi vengono distribuiti in `THIRD_PARTY_NOTICES.txt` e arrivano a `core.ThirdPartyNotices()` nelle build di release. La [policy sulle licenze](building.md#license-notices) già esistente dell'app, comprese le eccezioni per moduli specifici, si applica in modo indipendente alle licenze native.

I percorsi locali e i link simbolici restano all'interno del pacchetto che li dichiara. NSGo rifiuta campi sconosciuti, checksum non corrispondenti, file per un altro sistema operativo o un'altra architettura e nomi di installazione in conflitto. Due pacchetti possono condividere un file con lo stesso nome e lo stesso checksum. Un file è limitato a 1 GiB.

## Pacchettizzazione e caricamento

| Target | Directory delle librerie |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows e Linux | Accanto all'eseguibile |

Il pacchetto carica la propria libreria da quella directory, usando la posizione dell'eseguibile anziché la directory di lavoro. Precompila le dipendenze in modo che le librerie correlate vengano caricate dalla directory in cui sono distribuite. Dichiara ogni file correlato necessario e ogni nome di file di cui il loader ha bisogno.

NSGo verifica di nuovo i download in cache prima di riutilizzarli e coordina le build concorrenti tramite un lock su file. La cache dei contenuti si trova nella directory della cache utente del sistema operativo, in `nsgo/natives`. I file locali vengono verificati mentre vengono copiati.

Un pacchetto macOS universale richiede gli stessi nomi di libreria per entrambe le architetture. NSGo unisce le slice con `lipo`; una libreria già universale fornita dall'editore può servire per entrambe le voci. Le librerie vengono firmate prima dell'app, quindi passano attraverso i normali passaggi di immagine disco, firma e notarizzazione dell'app. La firma su Windows include le DLL dichiarate anche con `build.installers: false`. Gli installer Linux includono gli stessi file e avvisi della directory dell'eseguibile.

Lo sviluppo e i test sui target desktop nativi usano gli stessi passaggi di selezione dei pacchetti e di copia. Le dichiarazioni vengono selezionate con il target, i tag e l'overlay Go generato della build, incluse le sostituzioni locali e i pacchetti in vendor. Gli altri target al momento non pacchettizzano alcuna libreria da questo file.

## Preservare i file nel vendoring

L'operazione di vendor di Go richiede dati incorporati per conservare i file sotto la directory di un pacchetto. Un pacchetto può contrassegnare il proprio manifest e le licenze per l'inclusione senza usarne i byte a runtime:

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

Quando usi `path`, includi in quella direttiva embed anche i percorsi delle librerie locali, per esempio `lib/*`. I download da URL richiedono solo il manifest e i file di licenza nel pacchetto Go.
