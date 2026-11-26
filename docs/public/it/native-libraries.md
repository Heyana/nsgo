---
title: Librerie native
description: Distribuisci librerie condivise precompilate e framework Apple con un pacchetto Go attraverso i normali passaggi di pacchettizzazione, licenza e firma di NSGo, e indica i pacchetti Linux di cui hanno bisogno.
group: Strumenti
order: 23
---

Un pacchetto Go può distribuire le librerie condivise che carica tramite purego inserendo `nsgo-natives.yaml` nella directory del pacchetto e, dalla versione 2, i suoi framework per iOS e tvOS, e indicare i pacchetti Linux di cui ha bisogno. Quando un'app importa il pacchetto, NSGo seleziona le sue dichiarazioni per il target, verifica i file e li include prima della firma e della creazione degli installer. L'app non ha bisogno di configurazioni aggiuntive né di un comando di pacchettizzazione.

Le build delle app desktop e i test sui target disattivano cgo per il codice dell'app. Il codice nativo deve usare purego e una libreria dichiarata in `nsgo-natives.yaml`. NSGo rifiuta i pacchetti dell'app che contengono file cgo prima che la build cominci.

## Dichiarare le librerie

```yaml
version: 2
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
  ios:
    - name: Example.framework
      url: https://downloads.example.org/v1.0/ios/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  ios_simulator:
    - name: Example.framework
      path: lib/ios-simulator/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  tvos: []
  tvos_simulator: []
```

La versione 2 accetta le chiavi `macos_arm64`, `macos_x64`, `windows_arm64`, `windows_x64`, `linux_arm64`, `linux_x64`, `ios`, `ios_simulator`, `tvos` e `tvos_simulator`. La versione 1 accetta solo le sei chiavi desktop; un file viene letto solo per le chiavi della sua versione, quindi le build iOS e tvOS non pacchettizzano nulla da un file della versione 1. Dichiara ogni target su cui il pacchetto Go viene selezionato; `[]` indica esplicitamente che quel target non ha bisogno di alcuna libreria inclusa. In alternativa, i build tag di Go possono escludere il pacchetto dai target che non supporta. `macos_universal` usa le due voci macOS, e un target da terminale quelle del proprio desktop: `tui_macos_arm64` usa `macos_arm64`.

Ogni libreria ha:

- `name`: il nome del file installato, che termina in `.dylib` su macOS, `.dll` su Windows, oppure `.so` o un `.so.*` con versione su Linux. Su iOS e tvOS è quello di un framework, `<Name>.framework`, dove `Name` è composto da lettere ASCII, cifre, `.` e `-`, e inizia e finisce con una lettera o una cifra.
- Esattamente uno tra `path`, un file relativo a questa directory del pacchetto, e `url`, un URL HTTP(S) della libreria stessa. La versione 2 accetta solo URL https, e un download fallisce quando un reindirizzamento abbandona https. Pubblica le librerie già estratte anziché URL di archivi e, per un framework, solo il suo binario. Gli URL non contengono nomi utente e password incorporati.
- `sha256`: lo SHA-256 del file originale, obbligatorio per entrambe le sorgenti. Per esempio, `shasum -a 256 lib/macos-arm64/libexample.dylib` lo stampa su macOS.
- `licenses`: percorsi non vuoti ai file originali di licenza e di avviso, relativi a questa directory del pacchetto. Questi testi vengono distribuiti in `THIRD_PARTY_NOTICES.txt` e arrivano a `core.ThirdPartyNotices()` nelle build di release. La [policy sulle licenze](building.md#note-sulle-licenze) già esistente dell'app, comprese le eccezioni per moduli specifici, si applica in modo indipendente alle licenze native.

I percorsi locali e i link simbolici restano all'interno del pacchetto che li dichiara. NSGo rifiuta campi sconosciuti, checksum non corrispondenti, file per un altro sistema operativo o un'altra architettura e nomi di installazione in conflitto. Due pacchetti possono condividere un file con lo stesso nome e lo stesso checksum. Un file è limitato a 1 GiB.

## Pacchettizzazione e caricamento

| Target | Directory delle librerie |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows, Linux e target da terminale | Accanto all'eseguibile |
| iOS e tvOS | `<app>.app/Frameworks/<Name>.framework/<Name>` |

Il pacchetto carica la propria libreria da quella directory, usando la posizione dell'eseguibile anziché la directory di lavoro. Precompila le dipendenze in modo che le librerie correlate vengano caricate dalla directory in cui sono distribuite. Dichiara ogni file correlato necessario e ogni nome di file di cui il loader ha bisogno.

NSGo verifica di nuovo i download in cache prima di riutilizzarli e coordina le build concorrenti tramite un lock su file. La cache dei contenuti si trova nella directory della cache utente del sistema operativo, in `nsgo/natives`. I file locali vengono verificati mentre vengono copiati.

Un pacchetto macOS universale richiede gli stessi nomi di libreria per entrambe le architetture. NSGo unisce le slice con `lipo`; una libreria già universale fornita dall'editore può servire per entrambe le voci. Le librerie vengono firmate prima dell'app, quindi passano attraverso i normali passaggi di immagine disco, firma e notarizzazione dell'app. La firma su Windows include le DLL dichiarate anche con `build.installers: false`. Gli installer Linux includono gli stessi file e avvisi della directory dell'eseguibile. Su un Mac i target da terminale firmano ad hoc le librerie macOS; altrove le includono così come pubblicate, una universale per intero, e rifiutano una libreria arm64 non ancora firmata.

Lo sviluppo, `-run` e i test sui target usano gli stessi passaggi di selezione dei pacchetti e di copia, sui desktop come su iOS e tvOS. Le dichiarazioni vengono selezionate con il target, i tag e l'overlay Go generato della build, incluse le sostituzioni locali e i pacchetti in vendor. Android, HarmonyOS, watchOS, il web e i mini programmi non pacchettizzano nulla da questo file.

## Framework per iOS e tvOS

NSGo controlla il binario di ogni framework e interrompe la build indicandone il motivo, a meno che non sia:

- una libreria dinamica Mach-O arm64 compilata per la piattaforma del target, dispositivi o simulatore, come indica il suo `LC_BUILD_VERSION`; un file universale fornisce la propria slice arm64; arm64e viene rifiutato;
- per un sistema operativo minimo non più recente di quello dell'app, `apple.info.MinimumOSVersion` oppure iOS 15.0 e tvOS 17.0 ([Compilazione e distribuzione](building.md#nsgoyaml));
- con il nome di installazione `@rpath/<Name>.framework/<Name>`;
- e che carichi tramite `@rpath` solo framework che i pacchetti dell'app dichiarano per lo stesso target.

NSGo scrive l'`Info.plist` del framework, il cui `CFBundleIdentifier` è l'ID dell'app, un punto e `Name` in minuscolo, con la piattaforma e il sistema operativo minimo dell'app. Le build per il simulatore firmano il framework ad hoc. Una build per dispositivi lo lascia alla firma dell'app: `sign.profile` lo firma con l'identità dell'app prima dell'app stessa, come fa Xcode per `nsgo build -run` e `nsgo dev` su un dispositivo. Un framework che carica i framework correlati ha bisogno del percorso di esecuzione `@loader_path/..` o `@executable_path/Frameworks` per trovarli in `Frameworks`, dato che l'eseguibile dell'app non ne ha, e non carica altro tramite `@rpath` a meno che non abbia un percorso di esecuzione di sistema come `/usr/lib/swift`. Un framework il cui bundle ID appartiene già a un altro bundle dell'app, senza distinzione tra maiuscole e minuscole, come `StoreKit.framework` accanto al `NSGoStoreKit.framework` di NSGo, interrompe la build.

`nsgo build -embed` si ferma indicando il pacchetto quando l'app collega un pacchetto che dichiara framework per iOS: l'XCFramework per un'app host non può ancora portarli con sé.

## Pacchetti Linux

Dalla versione 2, `linux` indica, per ogni architettura Linux, i pacchetti di sistema da cui dipendono e che raccomandano i pacchetti Debian e Arch di un'app che collega il pacchetto Go:

```yaml
version: 2
libraries:
  # ...
linux:
  linux_x64:
    depends:
      deb: [libva2, libva-drm2, libpulse0]
      arch: [libva, libpulse]
    recommends:
      deb: [va-driver-all]
      arch: ["intel-media-driver: hardware decoding on Intel GPUs"]
  linux_arm64:
    depends:
      deb: [libavcodec61, libavformat61, libpulse0]
      arch: [ffmpeg, libpulse]
```

- `linux` è facoltativo. Le sue chiavi sono `linux_x64` e `linux_arm64`, così un pacchetto che include una libreria su un'architettura e carica quella di sistema sull'altra indica ciò di cui ha bisogno ciascuna.
- `depends` e `recommends` accettano ciascuno le liste `deb` e `arch`, come `linux.depends` e `linux.recommends` dell'app in [nsgo.yaml](building.md#nsgoyaml): ogni voce di `deb` come la scrive dpkg, ad esempio `libsoup-3.0-0 | libsoup3` o `libavcodec61 (>= 7:7.1)`, e ogni voce di `arch` come la scrive pacman, ad esempio `ffmpeg>=7` o `libpulse: audio output`.
- I campi `Depends` e `Recommends` del `.deb` e le righe `depend` e `optdepend` del pacchetto Arch elencano i pacchetti propri di NSGo, poi quelli dell'app, poi quelli di ogni pacchetto Go che la build collega, nell'ordine in cui Go li elenca, omettendo le voci già elencate e la raccomandazione di un pacchetto Go che una voce di `depends` nomina già. Un pacchetto Go che il target o i build tag escludono non aggiunge nulla.
- Una voce vuota o che contiene un a capo ferma la build indicando il pacchetto Go, così come `linux` in un file della versione 1 o una chiave diversa da queste due.
- L'AppImage e il Flatpak di una build per gli store non prendono nulla da `linux`.

## Preservare i file nel vendoring

L'operazione di vendor di Go richiede dati incorporati per conservare i file sotto la directory di un pacchetto. Un pacchetto può contrassegnare il proprio manifest e le licenze per l'inclusione senza usarne i byte a runtime:

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

Quando usi `path`, includi in quella direttiva embed anche i percorsi delle librerie locali, per esempio `lib/*`. I download da URL richiedono solo il manifest e i file di licenza nel pacchetto Go.
