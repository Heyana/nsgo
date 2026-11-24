---
title: Compilazione e distribuzione
description: Impacchetta un'app per ogni piattaforma, crea gli installer, firmala e notarizzala, pubblicala su GitHub e incorporala in altre app.
group: Strumenti
order: 22
---

## Build

```sh
go tool nsgo build
```

`build` scrive un pacchetto di rilascio stripped per ciascun target elencato da `nsgo.yaml` in `dist/<name>-<version>-<target>`: un `.app`, un `.ipa`, un `.apk`, le directory per Windows e Linux oppure un sito web, la cui pagina recupera ciascun modulo TSX nominato da un `import()` quando l'importazione viene eseguita. `-target` sostituisce l'elenco, e `all` indica ogni target che richiede soltanto Go e Xcode; aggiungi `android_x64` quando è installato un NDK Android, e `miniprogram`, oppure singolarmente `miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay` o `miniprogram_douyin`, per i [mini program](#mini-program), e `harmonyos` per un'anteprima [HarmonyOS](#harmonyos). `-run` avvia il pacchetto compilato per questa macchina oppure lo installa sull'iPhone, sul simulatore o sul dispositivo Android scelto da `-device`.

Tutto ciò che va distribuito finisce in `dist/releases`, l'unica directory in cui guardare:

| In `dist/releases` | Contenuto |
| --- | --- |
| `*.dmg`, `*-setup.exe`, `*.deb`, `*.pkg.tar.gz`, `*.AppImage` | [Installer](#installer) per macOS, Windows e Linux |
| `*.AppImage.zsync` | Controlli per aggiornamenti delta per AppImageUpdate e launcher compatibili |
| `*-ios.ipa`, `*-tvos.ipa`, `*.apk` | I pacchetti iOS, tvOS e Android, per gli store o i dispositivi |
| `*.pkg`, `*.msix`, `*.aab`, `*.flatpak`, `*.tar.gz` e `flatpak/` | I [pacchetti per gli store](#pacchetti-per-gli-store) di una build `store` |
| `install.sh`, `install.ps1` | Script che installano l'app dalle [release di GitHub](#pubblicare-su-github) |
| `appcast-<target>.xml` | I feed di [aggiornamento](#aggiornamenti) letti dalle app |
| `homebrew/<name>.rb`, `winget/` | [Cask Homebrew e manifest winget](#canali) per il rilascio diretto |
| `*-web.zip` | Il sito, da scompattare e servire così com'è |
| `miniprogram/<platform>/` | Il progetto di ciascun mini program, da caricare dal rispettivo strumento di sviluppo |

Ogni file è chiamato `<name>-<version>-<system>-<arch>`, ad esempio `hello-1.2.0-windows-x64-setup.exe`, così gli utenti trovano quello del proprio sistema in una pagina di rilascio, e ha accanto il proprio `.sha256`. Una build di un'altra versione svuota prima la directory; le build della stessa versione, anche su altre macchine, vi si aggiungono. I pacchetti dei simulatori e di watchOS restano nelle rispettive directory `dist/<name>-<version>-<target>`.

Il sito web include `.wasm.br` e `.wasm.gz` quando sono più piccoli del modulo originale. Abilita i file precompressi sugli host che li supportano. Servili all'URL originale `.wasm` con `Content-Type: application/wasm`, il corrispondente `Content-Encoding` e `Vary: Accept-Encoding`. Il modulo originale funziona su qualsiasi host statico. [Pubblicare sul web](web.md) tratta intestazioni semantiche, link del browser, metadati per pagina e caricamento da CDN.

### Pacchetti web offline

Imposta `web.offline: true` in `nsgo.yaml` per memorizzare nella cache una release web al suo primo avvio online.

```yaml
web:
  offline: true
```

Servi la directory pubblicata su HTTPS, oppure usa `go tool nsgo build -target web -run` su localhost. Il service worker memorizza nella cache la pagina, il modulo WebAssembly, gli script e le risorse incluse nel pacchetto. Le visite online caricano la versione pubblicata. Dopo che l'installazione è riuscita e le schede della release precedente sono state chiuse, l'ultima release può avviarsi dalla propria cache. Le richieste verso i servizi online di un'app necessitano comunque di una connessione.

## Terminale

Esegui l'app in un terminale interattivo con `go tool nsgo dev -target tui`. `go tool nsgo test -target tui` esegue lì i suoi test tramite `nsgotest.Main`, una singola applicazione per processo di test. Il terminale controlla la dimensione: una cella rappresenta 8 × 16 punti di layout. Scegli esplicitamente gli stili della tua applicazione; i componenti mantengono le loro forme e dimensioni ordinarie. `examples/tui` mostra campi, pulsanti, scorciatoie e ridimensionamento.

Compila un eseguibile da console con `go tool nsgo build -target tui_macos_arm64`, oppure con i corrispondenti `tui_macos_x64`, `tui_windows_arm64`, `tui_windows_x64`, `tui_linux_arm64` o `tui_linux_x64`. Il gruppo di compilazione/verifica `tui` li include tutti e sei, e li include anche `all`. Ogni directory del pacchetto contiene l'eseguibile e le risorse; non dispone di un installer grafico. Tutti e sei si compilano con il solo Go.

Il backend supporta testo nelle celle, sfondi, bordi continui, ritaglio, scorrimento, focus e modifica del testo. Usa il font e i colori del terminale; Control funge da `mod`, anche su Mac. Ctrl+C copia all'interno di un editor ed esce quando nessuna associazione o editor lo intercetta. Il bracketed paste inserisce il testo senza eseguire scorciatoie e gli appunti sono locali al processo. Canvas, immagini, contenuti multimediali, componenti nativi ospitati, menu/avvisi di sistema, animazioni, zoom, modellazione bidirezionale, ellissi iniziale/centrale e un bridge verso gli appunti di sistema non sono ancora implementati. `base/terminal` rimane il controllo terminale incorporato in un'applicazione grafica.

Mentre `nsgo dev` controlla il terminale, la diagnostica dello strumento e dell'applicazione viene scritta in `.nsgo/tui.log`. All'uscita vengono ripristinati schermo, cursore e modalità del terminale prima di tornare alla shell. Gli screenshot dei test su terminale rasterizzano le celle con un font portabile fisso anziché catturare l'emulatore; usa asserzioni sul testo per i glifi Unicode assenti da quel carattere.

## nsgo.yaml

```yaml
name: Hello
id: dev.example.hello   # the bundle ID; without it, dev.nsgo. and the name's letters and digits
version: 0.1.0
build_number: 1         # CFBundleVersion and Android's versionCode; else from version
# copyright: © 2026 Example  # the Windows executable's details and installer, and the macOS About panel
# appearance: dark      # light or dark whatever the system's
# theme: theme.json     # a TSX app's theme: NSGo's or a shadcn/ui one
# i18n: { dir: locales, source: en }  # the app's catalogs and the language they are written in
icon:
  apple: assets/AppIcon.icon  # macOS, iOS and watchOS, or an .icns for macOS
  windows: assets/IconWindows.png  # optional Windows artwork
  linux: assets/IconLinux.png      # optional Linux artwork
  png: assets/Icon.png        # every other target
  tvos: assets/AppIconTV.brandassets  # the Apple TV's layered icon and top shelf art, or a wide PNG
  watchos: assets/AppIconWatch.icon   # the Apple Watch's own, drawn in circles, or a square PNG
  android:
    png: assets/IconAndroid.png  # optional bitmap; valid without adaptive layers
    foreground: assets/IconForeground.png  # Android's adaptive icon: 432 px, the logo within the center 264 px
    background: "#1F1F1F"                 # a color or a square PNG
    monochrome: assets/IconMonochrome.png  # optional: the shape Android 13's themed icons tint
dev:
  target: native        # native, web, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android, harmonyos or miniprogram_<platform>
  hot_reload: true      # false builds once, watching no files
  # icon: { png: assets/Icon-dev.png }  # replaces icon's files in dev's builds; defaults to assets/*-dev.*
build:
  output: dist
  tags: []              # Go build tags every build, test and check of the app takes, such as timetzdata
  ldflags: []           # the app's own linker flags, such as -X main.releaseDate=2026-11-17
  open: false           # true opens the output directory after building
  installers: true      # false leaves out the disk image, the Windows installer and the Linux packages
  jobs: 0               # how many targets build at once; 0 is half the CPUs, 1 one after another
  targets: [macos_arm64, macos_x64, windows_arm64, windows_x64, linux_arm64, linux_x64, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android_arm64, web]
# apple: { sign_in_with_apple: true }  # the Sign in with Apple entitlement, for iOS, tvOS and Mac App Store builds
# apple: { purchases: true }  # StoreKit's library, which in-app purchases go through, in macOS, iOS and tvOS packages
# apple: { privacy: { accessed_apis: { NSPrivacyAccessedAPICategoryDiskSpace: [E174.1] }, tracking: false } }  # add required-reason APIs and tracking declarations to PrivacyInfo.xcprivacy
# apple: { requires_full_screen: false }  # let an iPad run the app in Split View, Slide Over and Stage Manager, in all four orientations
# apple: { orientations: [portrait, landscape_left] }  # the orientations a full-screen app takes on an iPad
# apple: { multiple_scenes: true }  # let an iPad open more than one window of the app, each showing the main view with its own state
# apple: { info: { UIBackgroundModes: [audio] } }  # background playback for iOS Picture in Picture
# android: { permissions: [REQUEST_INSTALL_PACKAGES] }  # manifest permissions beyond the runtime's
# android: { recording: true }  # the microphone service microphone.RecordWAV records in, with RECORD_AUDIO, the foreground-service permissions and POST_NOTIFICATIONS, which RecordWAV asks for so its notification shows from Android 13; a refusal still records
# android: { uses_cleartext_traffic: true }  # allow plain HTTP to any host in the APK and App Bundle
# android: { background_downloads: true }  # dataSync foreground service for cx.Download
# android: { background_motion: true }  # explicit finite motion shortService; streams also need a notification title
# android: { loopback_cleartext: true }  # allow plain HTTP to loopback in the APK and App Bundle
# commands: [{ name: hello-cli, package: ./cmd/hello-cli }]  # command-line programs the desktop packages ship beside the app: see Commands
# windows: { command_line: true }  # add <name>.com, a console companion that forwards arguments and streams to <name>.exe; not in store builds
# windows: { path: true }  # the installer puts the app's folder on the user's Path, so consoles find its commands
# windows: { publisher: Example Ltd }  # the company the Windows executable's details, its installer and the uninstall entry name; else windows.store.publisher_name
# linux: { maintainer: "Jane Doe <jane@example.com>" }  # the Linux packages' maintainer, else the app's name
# linux: { categories: [Office, Dictionary] }  # the freedesktop menu categories of the app's desktop entry, at least one a main category such as Office, Utility or Game, else menus file it under Other; your own start with X-
# linux: { share: packaging/share }  # files the .deb and Arch package install under /usr/share and the AppImage in its usr/share, such as metainfo/<id>.metainfo.xml or bash-completion/completions/<command>
# linux: { depends: { deb: [gstreamer1.0-plugins-good], arch: [gst-plugins-good] }, recommends: { deb: [gstreamer1.0-libav], arch: ["gst-libav: H.264 and AAC video"] } }  # packages the .deb and Arch package also depend on and recommend, after GTK 4 and WebKitGTK 6, each as dpkg or pacman writes it
# release: { github: me/hello }  # the GitHub repository the install scripts and updates install from, else the Git remote origin's; tag_prefix: v by default
# updates: { key: <public key> }  # how the app updates, by platform and channel: see Updates
# store: { dir: store }           # store listings directory (YAML and Markdown per language)
# miniprogram: { appid: { wechat: wx0123456789abcdef, alipay: "2021000000000000" }, custom_navigation: false }  # the mini programs'
```

Un semplice nome del sistema operativo in `targets` — o in `nsgo build -target` e `nsgo check -target` — indica le sue architetture (`windows` equivale a `windows_arm64` e `windows_x64`; `android` è solo `android_arm64`, poiché la build x86_64 richiede un NDK). `macos_universal`, che `macos` e `all` tralasciano, compila entrambe le architetture macOS e le unisce in un'unica app, incluse le sue librerie e i suoi widget. Il canale Steam usa il bundle universale dell'app. Le build con installer lo racchiudono in un'unica immagine disco, con un solo feed Sparkle sul canale diretto. `nsgo test -target` accetta anche il nome dell'architettura per cui vengono eseguiti i suoi test: quella della macchina corrente per un sistema operativo desktop e `arm64` per Android, quindi `windows_x64` esegue il target `windows` su una macchina Windows x64, mentre un'architettura per cui il target non può essere eseguito viene rifiutata indicando il nome da usare.

I flag sostituiscono i valori che nominano, come `-target`, `-version` o `-output`; `-h` li elenca tutti.

`build.ldflags` contiene i flag del linker propri dell'app, ciascuno scritto nella forma accettata da `-ldflags` di `go build`, come `-X main.releaseDate=2026-11-17` o `-X 'main.channel=beta builds'`. Ogni build che effettua il link dell'app li utilizza: `dev` e i suoi ricaricamenti, `build` anche con `-embed` e `test` anche con `-target`, su ogni target. `-ldflags` su `build` e `test` si aggiunge a essi anziché sostituirli, e può essere ripetuto per aggiungerne altri. Vengono inseriti prima dei flag `-X` dello strumento stesso e il linker conserva l'ultimo valore per ciascuna variabile, così nome, ID e versione in `core.App` restano quelli di `nsgo.yaml`. Lo strumento passa a Go il proprio `-ldflags`, che sostituisce quello eventualmente presente in `GOFLAGS`, quindi i flag del linker vanno inseriti qui; in un `test` headless il flag `-ldflags` dopo la directory appartiene a `go test` e li sostituisce tutti.

Ogni comando `go` eseguito dallo strumento per l'app utilizza `build.tags`, in `dev` e nei suoi ricaricamenti, in `build` anche con `-embed`, in `test` anche con `-target` e in `check`, su ogni target. I tag propri di NSGo, come `nsgodev` in `dev` o `nsgotest` nei test sui target, vengono aggiunti a essi, così come i tag di `-tags` in `GOFLAGS`, che altrimenti verrebbero sostituiti; il resto di `GOFLAGS` si applica così com'è.

`apple.info.LSMinimumSystemVersion` innalza la versione minima del pacchetto macOS, e `apple.info.MinimumOSVersion` innalza quella del pacchetto iOS, tvOS o watchOS selezionato. Scrivi stringhe di versione tra virgolette, come `"14.2"` o `"18.0"`, con da una a tre componenti numeriche. I limiti minimi del runtime sono macOS 13.0 (Go 1.27), iOS 15.0, tvOS 17.0 e watchOS 26.0; valori inferiori o malformati fanno fallire la build. La chiave mobile deve soddisfare la soglia minima di ogni target mobile selezionato. Le App Clip mantengono la propria chiave `app_clip.minimum`. I valori raggiungono i metadati del pacchetto, la compilazione degli asset, i progetti di firma e il linker dell'app; anche i framework iOS incorporati li utilizzano, con Mac Catalyst almeno alla versione 16.0 per il runtime macOS 13 di Go.

`icon.windows` e `icon.linux` selezionano PNG quadrati per i rispettivi pacchetti dell'app, la grafica degli installer e i loghi negli store. Come valore predefinito usano `assets/IconWindows.png` e `assets/IconLinux.png`, poi ripiegano su `icon.png`; `dev.icon` accetta le stesse impostazioni e file predefiniti `-dev`. `icon.android.png` seleziona una bitmap Android in modo indipendente, anche quando non sono configurati livelli adattivi. Gli altri target mantengono il fallback condiviso `icon.png`.

L'icona di Apple TV è una scheda a livelli, non un quadrato: `tvos` accetta il formato `.brandassets` di Xcode, i suoi stack `App Icon` e le immagini top shelf, oppure un'immagine larga ad esempio 1280×768, che lo strumento disegna adattandola alla scheda sopra il colore dei bordi e centra nelle immagini top shelf. In assenza di entrambi, il PNG occupa tre quarti dell'altezza della scheda. Apple Watch maschera la propria icona in un cerchio, quindi `watchos` accetta un `.icon` disegnato per i cerchi di watchOS o un PNG quadrato; senza di esso l'app usa `apple`, poi `png`. Entrambi ripiegano su `assets/AppIconTV.*` e `assets/AppIconWatch.*`, come fanno `apple` e `png`.

Su Android, `icon.android.foreground` e `background` rendono adattiva l'icona del launcher, così i launcher la mascherano nella propria forma e la spostano a piacere anziché rimpicciolire `png` su una base propria. Ogni livello è un quadrato di 108 dp, un PNG scalato a 432 px; il launcher mostra i 72 dp centrali, 288 px, e li maschera secondo una forma che può tagliare tutto ciò che si trova fuori dai 66 dp centrali, 264 px, quindi mantieni il logo all'interno di quel cerchio e lascia che lo sfondo riempia l'intero quadrato. `background` è un PNG o un colore come `"#1F1F1F"`. `monochrome`, una forma di cui conta soltanto il canale alfa, è ciò che Android 13 e versioni successive colorano quando l'utente attiva le icone a tema; senza di essa l'icona mantiene i propri colori. Il pacchetto conserva `icon.android.png`, oppure `icon.png`, come semplice icona bitmap, che nessun launcher di Android 9 o successivo (la versione più vecchia su cui gira NSGo) sceglie al posto di quella adattiva; senza `png`, lo strumento la disegna sovrapponendo il primo piano allo sfondo. Senza livelli adattivi, il launcher mostra la bitmap Android o il `png` condiviso.

`android.uses_cleartext_traffic: true` abilita per un APK o App Bundle l'HTTP in chiaro verso qualsiasi host, impostando sia il flag del manifest sia la relativa politica di sicurezza di rete. Senza questa abilitazione esplicita, `android.loopback_cleartext: true` consente l'HTTP in chiaro solo da `localhost`, `127.0.0.1` e `::1`, ad esempio per video serviti dall'app stessa. La politica solo loopback nega il traffico in chiaro verso altri host; l'abilitazione per tutti gli host prevale quando entrambe le impostazioni sono vere. Per un AAR incorporato, configura la politica di sicurezza di rete Android dell'app host.

Ogni build accetta anche i flag propri di NSGo all'avvio, come `--nsgo-fps` ([Esecuzione e debug](debugging.md#flag-di-ogni-build)). In `nsgo dev`, Comando/Control+R ricarica da ogni finestra aperta e resta disponibile dopo la chiusura della vista principale quando un'altra finestra mantiene attiva una funzione dell'app.

### Schermata di avvio

Imposta `launch_screen` per mostrare uno sfondo opaco, un PNG centrato o entrambi su iOS e Android 12 o versioni successive. I percorsi delle immagini sono relativi a `nsgo.yaml`. L'immagine si adatta proporzionalmente all'interno di 128 × 128 punti o dp. Un PNG trasparente con il lato più lungo di almeno 512 pixel la mantiene nitida sugli schermi ad alta densità.

```yaml
launch_screen:
  background: "#F5F1EA"
  image: assets/Launch.png
  dark:
    background: "#201C26"
    image: assets/LaunchDark.png
```

Il sistema seleziona l'aspetto a meno che `appearance` non lo fissi. Ometti uno dei campi `dark` per riutilizzare il valore corrispondente. Uno sfondo o un'immagine omessi mantengono il valore predefinito della piattaforma. Le app incorporate ottengono la propria schermata di avvio dall'host.

## Classi native Android

Elenca le classi Android precompilate nel file `nsgo.yaml` dell'app per usare una `View` personalizzata da `core.Native`.

```yaml
android:
  classes: [java/BadgeView.jar, java/Helpers.dex]
```

I percorsi sono relativi a `nsgo.yaml` a meno che non siano assoluti. Ciascun percorso indica un file `.dex` o un JAR contenente `classes.dex` alla radice, `classes2.dex` e gli shard successivi di D8. Prepara questi file con D8 usando `--min-api 28`. Su arm64, NSGo compila l'APK o l'App Bundle da questi file precompilati usando il solo Go. Le sessioni di sviluppo Android ricompilano quando un file elencato cambia. Per un AAR incorporato, la build Android dell'host fornisce le proprie dipendenze Java. [Documentazione di D8](https://developer.android.com/tools/d8)

L'[esempio nativo Android](https://github.com/nsgo-dev/nsgo/tree/main/examples/android-native) registra una classe con un costruttore pubblico `Context` e la configura tramite una prop Go.

## Giochi

Imposta `game: true` in `nsgo.yaml` per un gioco. Android classifica l'app come gioco e usa le ottimizzazioni della modalità Game Mode di sistema. macOS e iOS attivano la Game Mode, macOS imposta come predefinita la categoria Giochi e Linux aggiunge `Game` alla voce desktop. Un valore esplicito di `macos.category` o la chiave Game Mode in `apple.info` ha la precedenza.

Imposta `gamepad: true` quando la tua app gestisce un gamepad hardware. Android dichiara sia il gamepad sia il touchscreen come opzionali. macOS, iOS e tvOS dichiarano il profilo Extended Gamepad, e tvOS anche il Micro Gamepad del Siri Remote, con precedenza alle chiavi dei controller in `apple.info`.

## Note sulle licenze

`nsgo build` inserisce `THIRD_PARTY_NOTICES.txt` nelle risorse del pacchetto autonomo. Include i testi delle licenze dei moduli Go usati da quel target, del runtime Go e del runtime Java o JavaScript incluso in NSGo. Le build web denominano il file in base al suo contenuto, come per le altre risorse. Usa `core.ThirdPartyNotices()` per mostrare lo stesso testo in una schermata dei crediti in-app. La funzione restituisce una stringa vuota in sviluppo, nei test e nelle build di librerie incorporate. Le [dichiarazioni delle librerie native](native-libraries.md) aggiungono i rispettivi testi di licenza originali alle stesse note.

Per impostazione predefinita, una licenza di terze parti sconosciuta o mancante fa fallire la build e una licenza copyleft rilevata emette un avviso. I moduli elencati nel workspace Go dell'app (`go.work`) sono trattati come codice proprietario di prima parte insieme al modulo dell'app stessa: eventuali file di licenza che contengono vengono inclusi e non attivano i controlli sulle terze parti. Imposta ciascuna regola su `error` o `warn`. L'elenco `allow` esenta percorsi esatti di moduli da entrambe le regole e conserva eventuali testi di licenza trovati dalla build.

```yaml
licenses:
  unknown: error
  copyleft: warn
  allow: [example.org/reviewed-module]
```

## Librerie dai pacchetti Go

Le app che importano `base/terminal` distribuiscono anche `libghostty-vt`: i pacchetti Windows e Linux collocano la libreria accanto all'eseguibile; i bundle macOS e iOS la firmano all'interno dei propri framework. [Incorporamento](embedding.md) illustra l'XCFramework di accompagnamento per gli host iOS.

Una dipendenza può dichiarare librerie condivise desktop precompilate in `nsgo-natives.yaml`. NSGo ne verifica lo SHA-256, le copia nell'app prima della firma e le include negli installer; l'autore dell'app non esegue alcun passaggio aggiuntivo. [Librerie native](native-libraries.md) descrive il formato per gli autori di pacchetti e i percorsi di caricamento.

## Comandi

Un'app può distribuire programmi a riga di comando accanto al proprio eseguibile desktop, come una CLI che opera sui dati dell'app. Elencali sotto `commands`, ciascuno compilato da un pacchetto Go `main`:

```yaml
commands:
  - name: hello-cli             # the file's name, hello-cli.exe on Windows
    package: ./cmd/hello-cli    # a main package, built for each desktop target with CGO_ENABLED=0 and build.tags
    ldflags: [-X main.channel=beta]  # its own linker flags, after build.ldflags
windows:
  path: true                    # the installer puts the app's folder on the user's Path
```

Un nome è composto da lettere, cifre, `-` e `_`, e non può coincidere, indipendentemente da maiuscole o minuscole, con l'eseguibile dell'app, il suo comando Linux, `Uninstall`, `AppRun` o un altro comando. `macos_universal` unisce le due build macOS di ciascun comando, esattamente come unisce quelle dell'app.

Un pacchetto effettua il link con `build.ldflags` e `-ldflags`, poi con i propri `ldflags`, i cui valori `-X` prevalgono, infine con l'identità dell'app, così `core.App()` fornisce al comando nome, ID, versione e numero di build di `nsgo.yaml`, proprio come li fornisce all'app. Un comando che non importa `core` riceve la versione tramite `-X`, come `-ldflags "-X main.version=1.2.0"` su `build`, accettato sia dall'app sia dai suoi comandi.

| Pacchetto | Comandi |
| --- | --- |
| macOS | In `Contents/MacOS`, firmati prima dell'app così com'è, come `<id>.<command>`: firma ad hoc, oppure con `sign.identity`, hardened runtime e marca temporale, così l'app supera la [notarizzazione](#notarizzazione), ma senza gli entitlement dell'app. L'immagine disco non mette nulla su `PATH`; il [cask Homebrew](#canali) collega ciascun comando e un utente può creare autonomamente un collegamento a `/Applications/<name>.app/Contents/MacOS/<command>` |
| Windows | `<command>.exe` accanto a `<name>.exe`, [firmato](#firmare-i-pacchetti-windows) così com'è, installato dall'installer e rimosso dal relativo programma di disinstallazione; con `windows.path` l'installer aggiunge inoltre la cartella al `Path` dell'utente, come fa `windows.command_line`, così le console aperte dopo l'installazione trovano i comandi. Un comando rinominato o rimosso da un aggiornamento resta nella cartella, così come qualsiasi file non più distribuito dall'app, poiché il programma di disinstallazione rimuove solo ciò che la sua versione ha installato |
| Linux | In `/opt/<slug>` con un collegamento `/usr/bin/<command>` di cui il pacchetto `.deb` e quello Arch sono proprietari, così la rimozione del pacchetto lo rimuove e dpkg e pacman rifiutano l'installazione sopra il file di un altro pacchetto; l'AppImage li conserva accanto all'app e non mette nulla su `PATH` |

Gli installer, i rispettivi checksum e gli appcast di [aggiornamento](#aggiornamenti) trasportano i comandi come trasportano gli altri file dell'app, così un aggiornamento li sostituisce insieme all'app. I [pacchetti per gli store](#pacchetti-per-gli-store) li omettono, poiché la sandbox del Mac App Store, l'MSIX e il Flatpak eseguono solo l'eseguibile proprio dell'app.

## Installer

Per i target desktop, `build` scrive i file da cui gli utenti installano l'app in `dist/releases`, ciascuno con un checksum `.sha256`:

| Target | Installer |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.dmg`, un'immagine disco che mostra l'app, una freccia e un collegamento ad Applications su cui trascinarla, firmata con `sign.identity` e [notarizzata](#notarizzazione) |
| Windows | `<name>-<version>-windows-<arch>-setup.exe`, un installer nativo di Windows che installa per l'utente, di nuovo nella cartella scelta dall'utente in precedenza, chiudendo l'app se è in esecuzione, con un collegamento nel menu Start, uno facoltativo sul desktop e un programma di disinstallazione che rimuove solo ciò che ha installato, [firmato con `sign.certificate`](#firmare-i-pacchetti-windows) laddove l'app firma i pacchetti Windows; `windows.command_line` installa anche il file compagno `.com` e aggiunge la cartella dell'app al `Path` dell'utente, così le console aperte dopo l'installazione eseguono il comando dell'app, mentre Win+R, che trova anch'esso il file compagno, avvia l'app e chiude la console aperta; il programma di disinstallazione rimuove la cartella da `Path` solo se è stato un installer ad aggiungerla, mai una voce preesistente |
| Linux | `<name>-<version>-linux-<arch>.deb` per Debian e Ubuntu, e `<name>-<version>-linux-<arch>.pkg.tar.gz` per Arch Linux, che installano l'app in `/opt/<slug>` con un comando, i collegamenti ai suoi [comandi](#comandi), la voce desktop e l'icona (il pacchetto Arch include `.MTREE` per `pacman -Qkk`), e `<name>-<version>-linux-<arch>.AppImage`, che gira su qualsiasi distribuzione senza installazione |

Il programma di installazione e il programma di disinstallazione per Windows si compilano con il solo Go, su qualsiasi sistema, e usano le finestre di dialogo native di Windows. Parlano le lingue dei cataloghi dell'app per cui dispongono dei testi, incluso il cinese semplificato e tradizionale, scegliendo la lingua dell'interfaccia di sistema; `/LANG=<LCID>` ne sceglie una per una singola esecuzione, ad esempio `/LANG=2052` per il cinese semplificato. `/S` installa o disinstalla in modalità silenziosa, un `/D=<folder>` finale sceglie la cartella, spazi inclusi, e `/RELAUNCH` apre l'app dopo un aggiornamento. Senza `/D`, un aggiornamento installa nella cartella dell'installazione che sostituisce. `-installers=false` o `build.installers: false` compila soltanto i pacchetti.

Gli installer accettano immagini e icone proprie dell'app; altrimenti ciascuna usa l'icona dell'app o l'immagine integrata:

```yaml
macos:
  dmg_background: assets/DMGBackground.png  # the disk image's window, which takes its size in points
  dmg_icon: assets/DMGIcon.png              # the disk image's icon: an .icns or a square PNG
windows:
  installer_icon: assets/Setup.png          # the setup program's icon: an .ico or a square PNG
  uninstaller_icon: assets/Uninstall.png    # the uninstaller's
```

Un PNG di sfondo accompagnato da un gemello di dimensioni doppie, come `DMGBackground@2x.png`, rimane nitido sugli schermi Retina. L'icona dell'app si trova a un quarto dal bordo sinistro, Applications a un quarto dal bordo destro, ciascuna con il proprio nome centrato verticalmente, dove l'immagine integrata disegna la freccia; lascia spazio per essi se usi uno sfondo personalizzato. I pacchetti Linux non hanno immagini proprie; installano l'icona dell'app.

I pacchetti Linux installano anche i metadati AppStream dell'app, `/usr/share/metainfo/<id>.metainfo.xml`, tramite cui i software center la elencano, scritti a partire da `linux.summary`, `linux.license`, se impostato, e i nomi dell'app, con `Utility` come categoria quando `linux.categories` non ne nomina alcuna; l'AppImage li conserva nella propria directory AppDir `usr/share`, accanto a un collegamento alla sua voce desktop in `usr/share/applications`, richiamata dai metadati. I file letti dal sistema da `/usr/share`, come metadati più ricchi, ulteriori voci desktop, completamenti della shell o pagine di manuale, vanno nella directory indicata da `linux.share`, relativa a `nsgo.yaml`: i pacchetti `.deb` e Arch installano il relativo albero sotto `/usr/share`, e l'AppImage sotto la propria `usr/share` di AppDir. Il suo `metainfo/<id>.metainfo.xml` sostituisce quello scritto da NSGo, e un file in un percorso che il pacchetto scrive già da sé, come la voce desktop o l'icona dell'app, fa fallire la build. I file in `resources/`, qualunque sia il nome, rimangono accanto all'eseguibile in `/opt/<slug>`, e i Flatpak delle build per gli store utilizzano esclusivamente i metadati di NSGo.

L'AppImage richiede FUSE, presente nella maggior parte delle distribuzioni, e utilizza GTK 4 dal sistema, come fanno i pacchetti; il suo runtime viene scaricato una sola volta nella cache utente.

Le AppImage incorporano un localizzatore `gh-releases-zsync` utilizzando `release.github` o il remote GitHub `origin`, selezionando l'ultima release stabile e l'architettura di questa app. Carica ogni `.AppImage` insieme al file `.AppImage.zsync` generato. Per un altro host, imposta `linux.appimage_update` su `zsync|https://example.com/latest.AppImage.zsync`, un URL stabile servito con supporto per i range HTTP; accetta anche `gh-releases-zsync|owner|repo|latest-pre|name-*-linux-x64.AppImage.zsync` per un canale prerelease. Una stringa vuota esplicita disabilita il rilevamento incorporato; senza un repository GitHub o un localizzatore configurato resta anch'esso vuoto. Il file `.zsync` viene comunque generato, separatamente dall'appcast firmato.

## Pacchetti per gli store

`-channel store`, oppure `updates.channel: store`, compila i target desktop e Android per i rispettivi store, che provvedono ad aggiornarli, al posto degli installer e degli script di installazione:

| Target | Pacchetto per lo store |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.pkg` per il Mac App Store: l'app nell'App Sandbox, firmata con l'identità del profilo di provisioning e impacchettata con l'identità installer del tuo team |
| Windows | `<name>-<version>-windows-<arch>.msix` per il Microsoft Store, che lo firma al momento della pubblicazione |
| Linux | `<name>-<version>-linux-<arch>.flatpak` installabile, più un `.tar.gz` con AppStream e `flatpak/<id>.yml` per Flathub o flatpak-builder sul runtime di GNOME |
| Android | `<name>-<version>-android-<arch>.aab`, un App Bundle per Google Play firmato con `sign.keystore`, la chiave di upload |

L'MSIX legge il nome visualizzato di ciascuna lingua dalla voce `appName` dei cataloghi, ripiegando sulla lingua sorgente. I suoi loghi per Start, barra delle applicazioni e Store includono varianti al 100%, 125%, 150%, 200% e 400%. Lo strumento scrive il relativo indice delle risorse su macOS; non è necessario alcun Windows SDK.

`-target macos` seleziona entrambe le architetture macOS sul canale store e scrive un unico `<name>-<version>-macos-universal.pkg`, proprio come `-target macos_universal`. Seleziona `-target android_arm64,android_x64` per ottenere un unico `<name>-<version>-android-universal.aab` contenente `arm64-v8a` e `x86_64`. La slice `x86_64` richiede un NDK Android.

```yaml
macos:
  category: public.app-category.productivity  # LSApplicationCategoryType, which the Mac App Store requires
  # entitlements: { com.apple.security.files.downloads.read-write: true }  # beyond the network, the files the user picks and what apple.info explains
windows:
  store:                               # as Partner Center's Product identity lists them
    identity: 12345Example.Hello
    publisher: CN=00000000-0000-0000-0000-000000000000
    publisher_name: Example
  # device_capabilities: [microphone, webcam]  # the devices the app uses, which Windows asks the user for
linux:
  summary: Says hello                  # one line for software centers
  license: MIT                         # an SPDX expression; LicenseRef-proprietary by default
  # flatpak_runtime: "51"              # GNOME's runtime version
android:
  target_sdk: 36                       # the targetSdkVersion Google Play requires
sign:
  macos_profile: assets/MacAppStore.provisionprofile  # the Mac App Store's profile
  # installer: "3rd Party Mac Developer Installer: Example (ABCDE12345)"  # by default the keychain's of the profile's team
```

Quando `android.target_sdk` non è impostato, il runtime ha come target l'API 36. Una build Android App Bundle fallisce al di sotto dell'API 36, richiesta da [Google Play](https://support.google.com/googleplay/android-developer/answer/11926878) per le nuove app mobili e per gli aggiornamenti a partire dal 31 agosto 2026. Una build APK autonoma emette un avviso e prosegue.

Imposta `sign.macos_profile` per firmare il pacchetto Mac per il caricamento. Senza widget, il pacchetto non firmato gira comunque nella sandbox, consentendoti di testare l'accesso al container dell'app, alla rete e ai file scelti dall'utente. Una build per lo store con widget richiede sia `sign.macos_profile` per `<id>` sia un `sign.widgets_profile` macOS per `<id>.widgets`, appartenenti allo stesso team e che concedano `apple.app_group` (predefinito `group.<id>`). Entrambi i bundle usano quel container condiviso; non viene aggiunta alcuna eccezione per la directory home. `-installers=false` mantiene la sandbox e le firme del profilo omettendo il `.pkg` e la relativa identità installer. Carica l'`.msix` nel Partner Center e l'`.aab` in Play Console. Invia `flatpak/<id>.yml` a Flathub una volta che la release contiene gli archivi nominati al suo interno. Anche `flatpak-builder` lo compila in locale. Installa un bundle con `flatpak install --user ./<name>-<version>-linux-<arch>.flatpak`; il runtime corrispondente di GNOME deve essere installato o disponibile da un remote configurato. Le build scrivono il bundle in Go senza bisogno di una toolchain Linux. I pacchetti per gli store usano un'icona trasparente quando manca la grafica e `Utility` quando `linux.categories` non è impostato. Per l'installazione al di fuori dello Store, imposta [`sign.certificate`](#firmare-i-pacchetti-windows) per firmare l'MSIX; il soggetto del certificato deve corrispondere a `windows.store.publisher` e la macchina destinataria deve considerare attendibile il certificato. Senza impostazioni di firma, è lo Store a firmare il pacchetto caricato.

Su macOS e iOS, gli effetti ai bordi di una vista scorrevole (`Scroll.EdgeEffects`), le sfocature dello sfondo su macOS senza `IOS.BackdropMaterial` e i filtri colore sullo sfondo (da soli o sopra una sfocatura senza `IOS.BackdropMaterial`) disegnano tramite `CABackdropLayer` e `CAFilter` privati di Core Animation, come fanno i materiali e gli effetti ai bordi propri di AppKit e UIKit, poiché nessuna API pubblica sfoca secondo un raggio, varia una sfocatura su una vista o applica una matrice colore allo sfondo. La revisione dell'App Store potrebbe rifiutare le app che usano API private: NSGo le richiama solo per le viste scorrevoli con barre ai bordi e per quei filtri sullo sfondo, ma ogni build macOS e iOS contiene i loro nomi.

Ogni bundle Apple scritto da `build` — l'app, la sua estensione widget, l'App Clip e l'app Apple Watch, nonché un framework `-dynamic` `-embed` — contiene un manifest per la privacy, `PrivacyInfo.xcprivacy`, che dichiara le API con motivazione obbligatoria utilizzate da NSGo e dal runtime Go: preferenze utente (`CA92.1`), timestamp dei file (`C617.1`) e tempo di avvio del sistema (`35F9.1`). `apple.privacy` aggiunge quelle dell'app: `accessed_apis`, i motivi di ciascuna categoria, `tracking`, `tracking_domains` e `collected_data`, voci di `NSPrivacyCollectedDataTypes` secondo la denominazione Apple; un eventuale file `PrivacyInfo.xcprivacy` in `resources/` viene unito a esse anziché sostituito.

Il file `Info.plist` di ogni bundle iOS, tvOS e watchOS scritto da `build` per i dispositivi — quello dell'app, della sua estensione widget e dell'App Clip — include le chiavi `DT` dell'SDK e di Xcode verificate da App Store Connect, lette dallo Xcode che esegue la compilazione, e `UIRequiredDeviceCapabilities: [arm64]`; i bundle per simulatore le omettono e una chiave impostata da `apple.info` mantiene il proprio valore. `apple.multiple_scenes: true` imposta `UIApplicationSupportsMultipleScenes` nel manifest delle scene di un'app iOS, che per impostazione predefinita è falso: su iPad l'utente può quindi aprire più finestre dell'app, e ciascuna mostra di nuovo la vista principale dell'app, con un proprio stato e con i segnali e lo spazio di archiviazione condivisi dell'app, finché l'utente non la chiude.

### Pubblicare su Google Play

Abilita la Google Play Developer API e concedi a un account di servizio l'autorizzazione per rilasciare l'app sul canale (track) scelto. Imposta `NSGO_PLAY_SERVICE_ACCOUNT` sulla chiave JSON dell'account o sul percorso del relativo file chiave. [Configurazione dell'API Google Play](https://developers.google.com/android-publisher/getting_started)

```sh
go tool nsgo build -target android -channel store
go tool nsgo publish -target android -track internal
```

Il comando trova l'App Bundle per la versione del progetto in `build.output/releases`. Passa un percorso `.aab` dopo i flag per selezionare un file quando sono presenti più build. Usa `internal`, `production` oppure l'ID del track di un test chiuso. Il comando legge il pacchetto e il codice di versione dal bundle, richiede un codice superiore a ogni release su quel track, quindi carica il bundle ed effettua il commit della release.

## Schede sugli store

I metadati per gli store (titoli, sottotitoli, descrizioni, parole chiave, note di rilascio, URL, screenshot e anteprime) risiedono nel repository come testo semplice YAML e Markdown sotto `store/` (o `store.dir` in `nsgo.yaml`):

```text
store/
  listing.yaml              # version: 1, default_locale, app_store and google_play settings
  en-US/
    listing.yaml            # version: 1, title, subtitle, short_description, keywords, URLs, media
    description.md          # full description (optional file in place of YAML description)
    release_notes.md        # what's new (optional file in place of YAML release_notes)
    promotional_text.md     # App Store promotional text (optional file)
    screenshots/
  zh-Hans/
    listing.yaml
    description.md
    release_notes.md
    screenshots/
```

Entrambi i file `listing.yaml` richiedono `version: 1` e rifiutano chiavi sconosciute. Le directory di localizzazione canoniche come `zh-Hans` vengono mappate automaticamente al codice di ciascuno store (`zh-Hans` su App Store Connect, `zh-CN` su Google Play) a meno che non vengano sovrascritte per singolo store con `app_store.locale` o `google_play.locale`.

```sh
go tool nsgo publish -store all -dry-run                  # validate lengths, locales and media without uploading
go tool nsgo publish -store appstore,play                 # validate and upload to App Store Connect and Google Play
go tool nsgo publish -store play -track production        # also update release notes on the named Play track
```

La validazione viene eseguita prima di leggere le credenziali o di effettuare qualsiasi richiesta HTTP, verificando campi obbligatori, codici di localizzazione, schemi URL, file multimediali e conteggi (fino a 10 screenshot e 3 anteprime per tipo di display App Store; fino a 8 screenshot per tipo di immagine Google Play) e limiti di caratteri (App Store: titolo 30, sottotitolo 30, parole chiave 100, testo promozionale 170, descrizione 4000, note di rilascio 4000; Google Play: titolo 30, descrizione breve 80, descrizione 4000, note di rilascio 500).

Le credenziali di caricamento provengono esclusivamente dall'ambiente: `NSGO_APP_STORE_KEY`, `NSGO_APP_STORE_KEY_ID` e `NSGO_APP_STORE_ISSUER` (ripiegando sulla chiave di notarizzazione `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID` e `NSGO_NOTARY_ISSUER`) per App Store Connect, e `NSGO_PLAY_SERVICE_ACCOUNT` (percorso del file JSON dell'account di servizio o testo JSON) per Google Play.

### Conformità agli store

Usa `nsgo.yaml` per tenere traccia dello stato di ciascun questionario di rilascio.

```yaml
compliance:
  app_store: { age_rating: false }
  google_play: { data_safety: false }
  steam: { content_survey: false }
```

Imposta una voce su `true` dopo aver completato il relativo questionario. Una build corrispondente `-channel store` o `-channel steam` ti ricorda le voci impostate su `false`. Ometti qualsiasi voce che non tracci nel file YAML.

- Completa il [questionario sulla classificazione per età](https://developer.apple.com/help/app-store-connect/manage-app-information/set-an-app-age-rating) e i [dettagli sulla privacy dell'app](https://developer.apple.com/help/app-store-connect/manage-app-information/manage-app-privacy) in App Store Connect.
- Per il [questionario sulla crittografia](https://developer.apple.com/help/app-store-connect/manage-app-information/determine-and-upload-app-encryption-documentation) di Apple, imposta `apple.info.ITSAppUsesNonExemptEncryption` sulla risposta della tua app come booleano YAML. Se Apple invia un codice di approvazione, inseriscilo in `apple.info.ITSEncryptionExportComplianceCode` come stringa. Puoi omettere le chiavi e rispondere al momento del caricamento. Le build per gli store Apple verificano i tipi di queste chiavi e la struttura del manifest della privacy del bundle.
- Completa la sezione [Sicurezza dei dati](https://support.google.com/googleplay/android-developer/answer/10787469) in Play Console. Le app distribuite esclusivamente sul track di test interno sono esenti.
- Completa il [Sondaggio sui contenuti](https://partner.steamgames.com/doc/gettingstarted/contentsurvey) in Steamworks prima di inviare il gioco per la revisione.
## App Store Connect

Crea un'app in App Store Connect e firma un IPA per dispositivo iOS con un profilo di distribuzione App Store oppure un PKG store macOS con l'identità installer del tuo team. Su un Mac con Xcode, imposta `NSGO_APP_STORE_KEY` sul file `.p8` della chiave del tuo team App Store Connect o sul suo testo, `NSGO_APP_STORE_KEY_ID` sul relativo ID chiave e `NSGO_APP_STORE_ISSUER` sul relativo ID emittente; in loro assenza viene usata la chiave di notarizzazione (`NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID`, `NSGO_NOTARY_ISSUER`), come per le schede sugli store. Concedi a quella chiave l'accesso all'app e l'autorizzazione a caricare le build.

```sh
go tool nsgo publish ios path/to/Hello.ipa
go tool nsgo publish macos path/to/Hello.pkg
```

Lo strumento verifica il numero di build del bundle prima del caricamento, quindi segnala l'esito dell'elaborazione di Apple per TestFlight. Incrementa `build_number` per il caricamento successivo. I numeri di build macOS crescono attraverso tutte le versioni dell'app, mentre su iOS la sequenza può ripartire per una nuova versione. Invia separatamente una build elaborata per l'App Review in App Store Connect. [Guida alla distribuzione di Apple](https://developer.apple.com/documentation/xcode/preparing-your-app-for-distribution)

## Pacchetti di asset per gli store

Dichiara le directory di asset di grandi dimensioni in `mobile.asset_packs`. I file all'interno di tali directory vengono distribuiti separatamente dall'Android App Bundle di base e dall'app iOS. I nomi dei pacchetti iniziano con una lettera minuscola e possono contenere lettere minuscole, cifre e trattini bassi.

```yaml
mobile:
  asset_packs:
    - {name: levels, dir: resources/levels, delivery: install-time}
    - {name: voices, dir: large-assets/voices, delivery: fast-follow}
```

Compila i pacchetti Android con `go tool nsgo build -channel store -target android_arm64`. Usa `bundletool build-apks --local-testing` e `bundletool install-apks` per testare il bundle su un dispositivo. NSGo apre i pacchetti `install-time` installati. I moduli `fast-follow` sono inclusi per la distribuzione tramite Play, mentre la lettura di tali pacchetti dall'SDK Play Asset Delivery è ancora in sospeso.

Su iOS, `go tool nsgo build -target ios` mappa `install-time` sulla politica essential di Apple e `fast-follow` su prefetch, sia alla prima installazione sia agli aggiornamenti. Xcode 26 o successivo compila il downloader Background Assets. Usa un `apple.app_group` condiviso o il valore predefinito `group.<id>`. Sia `sign.profile` dell'app sia `sign.asset_packs_profile` dell'estensione `<id>.assetdownloader` necessitano di quel gruppo. Carica i file `.aar` da `dist/releases` insieme all'IPA usando il flusso di caricamento degli asset pack di App Store Connect. [Creazione di asset pack gestiti](https://developer.apple.com/documentation/backgroundassets/creating-managed-asset-packs)

Da una goroutine, chiama `mobile.AssetPackReady(ctx, "levels")` per verificare la disponibilità e `mobile.OpenAssetPackFile("levels", "intro.map")` per aprire un file relativo alla directory dichiarata. Chiudi il reader restituito dopo l'uso. [Test locale degli asset pack Apple](https://developer.apple.com/documentation/backgroundassets/testing-asset-packs-locally)

## Depot Steam

```sh
go tool nsgo build -channel steam -target macos_universal,windows_x64,linux_x64
```

Imposta `updates.channel: steam` per usare il canale come predefinito. Seleziona target macOS, Windows e Linux. Carica ogni directory `dist/<name>-<version>-<target>` nel rispettivo depot Steam, usando il bundle dell'app su macOS, l'eseguibile su Windows e Linux arm64 e `steam-launch.sh` su Linux x64 come opzione di avvio. Steam gestisce gli aggiornamenti. Le directory non contengono installer, programma di auto-aggiornamento né `steam_appid.txt`.

I depot Linux x64 includono GTK 4 e le relative dipendenze per Steam Linux Runtime 3.0 e 4.0. Il launcher li usa solo quando il sistema non dispone di GTK 4.14 o successivo. Mantieni `nsgo-gtk/` accanto al launcher: la sua directory `licenses/` contiene le licenze delle librerie, i sorgenti da cui sono state compilate e lo script di build, come richiesto dalla LGPL per GLib, Pango e GTK. Linux arm64 richiede la disponibilità di GTK 4 nel proprio runtime.

Per i test locali al di fuori del client Steam, imposta `steam: {app_id: 480}` in `nsgo.yaml`, sostituendo `480` con il tuo App ID Steamworks. `go tool nsgo dev` scrive l'ID in `steam_appid.txt` nella propria directory di build nativa e avvia l'app con quella directory come directory di lavoro. [API Steamworks](https://partner.steamgames.com/doc/api/steam_api), [SteamPipe](https://partner.steamgames.com/doc/sdk/uploading).

## Pubblicare su GitHub

Quando la build include un target macOS, Windows o Linux, `dist/releases` contiene anche `install.sh` e `install.ps1`, che installano l'app dalle release del repository GitHub indicato da `release.github`, altrimenti da quello su cui punta il remote Git `origin` dell'app; senza nessuno dei due `build` non li scrive e lo segnala. Carica i relativi file — quelli al livello principale escludendo `miniprogram/` — nella release con tag `v<version>` (`release.tag_prefix` sostituisce `v`; gli script codificano i caratteri URL riservati presenti in quel prefisso):

```sh
gh release create v1.2.0 dist/releases/*.*
```

Gli utenti installano o aggiornano quindi l'app con una sola riga, stampata dalla build:

```sh
curl -fsSL https://github.com/me/hello/releases/latest/download/install.sh | sh    # macOS and Linux
```

```powershell
irm https://github.com/me/hello/releases/latest/download/install.ps1 | iex         # Windows
```

Su macOS lo script copia l'app dall'immagine disco in `/Applications`, oppure `~/Applications` quando non può scrivere lì; su Linux installa il pacchetto Arch con `pacman`, altrimenti quello Debian con `apt-get` o `dpkg`, tramite `sudo`; su Windows esegue l'installer in modalità silenziosa, che installa dove l'app è già presente. Ciascuno controlla prima il file rispetto al proprio checksum. `<NAME>_VERSION`, ovvero il nome dell'app in maiuscolo come `HELLO_VERSION=1.1.0`, installa un'altra release, e `<NAME>_DOWNLOAD_URL` scarica da un mirror.

## Pubblicare su itch.io

Imposta `itch.project` sullo slug `owner/game` del tuo gioco su itch.io in `nsgo.yaml`. Per sovrascrivere i nomi dei canali, imposta `itch.channels` per target di build. I valori predefiniti sono `html5` per il web, `osx-<arch>` per macOS, `windows-<arch>` per Windows e `linux-<arch>` per Linux.

```yaml
itch:
  project: example/my-game
  channels:
    macos_arm64: osx-arm64-beta
```

Con `BUTLER_API_KEY` impostata come secret in CI, compila e carica i target.

```sh
go tool nsgo build -channel none -installers=false -target web,macos_arm64
go tool nsgo publish itch -target web,macos_arm64
```

Usa gli stessi `-output` e `-version` su entrambi i comandi se li sovrascrivi. Il caricamento prende la directory del pacchetto di ciascun target e imposta la versione su itch.io sulla versione della build. NSGo utilizza `butler` presente su `PATH` oppure scarica una copia fissata da itch.io. Dopo il primo caricamento web, imposta il tipo di gioco su itch.io su HTML e contrassegna il caricamento come giocabile nel browser. [Documentazione di Butler](https://itch.io/docs/butler/pushing.html)

## Aggiornamenti

`updates` fa sì che l'app si mantenga aggiornata nel modo previsto dalla sua piattaforma e dal canale da cui gli utenti la ottengono:

```yaml
updates:
  key: zE0hx2FZwwHy6X7LbEP3GjTTE3KWfCAp5gsaBysC+vg=  # the public key nsgo keygen prints
  # channel: store                    # direct, store, steam or none; direct by default with a key, none without
  # feed: https://example.com/hello/  # where the appcasts and installers are; by default the latest GitHub release
  # automatic: false                  # check only when asked; true by default
  # automatic_downloads: true          # Sparkle downloads automatically; false by default, macOS only
```

`nsgo keygen` stampa una coppia di chiavi e non scrive alcun file: inserisci la chiave pubblica in `updates.key` ed effettua il commit, e conserva quella segreta fuori dal repository, in un gestore di password e in `NSGO_UPDATES_KEY` dove vengono compilate le release, ad esempio un secret di GitHub Actions. Ogni build diretta firma gli installer con la chiave segreta e scrive `appcast-<target>.xml` accanto a essi, un feed nel formato di [Sparkle](https://sparkle-project.org) che l'app legge sotto `feed`: carica gli appcast insieme agli installer. Senza la chiave segreta, `build` non scrive alcun appcast per macOS o Windows, poiché questi installano solo installer firmati, mentre le app Linux e Android che leggono un appcast non firmato aprono la pagina della release anziché installarne il pacchetto.

`-release-notes notes.md` inserisce un file Markdown delle note della release, ad esempio la sua sezione del changelog, in ciascun appcast, renderizzato come HTML escludendo eventuale HTML grezzo presente nel file. Sparkle e WinSparkle lo mostrano nelle proprie finestre di aggiornamento, mentre il prompt su Linux e Android lo mostra come testo sotto la versione, fino a 1.500 caratteri; le app lo leggono come `core.UpdateCheck.Notes`.

| Target | Come si aggiorna una build diretta |
| --- | --- |
| macOS | Sparkle, che `build` inserisce nell'app, controlla quotidianamente, propone la nuova versione, scarica l'immagine disco, ne verifica la firma, la installa e riavvia l'app; il menu predefinito dell'app include Verifica aggiornamenti…, che un'app con un proprio menu aggiunge tramite `updater.Check` |
| Windows | WinSparkle, accanto all'eseguibile, fa lo stesso nella lingua mostrata dall'app all'avvio, quando WinSparkle ne dispone, ed esegue l'installer in modalità silenziosa, che avvia la nuova versione |
| Linux, Android | L'app legge il proprio appcast e, una volta che l'utente sceglie Aggiorna, scarica in streaming e verifica direttamente su disco il pacchetto firmato segnalando l'avanzamento (`updater.Status` e l'indicatore di avanzamento dell'attività della piattaforma), quindi lo installa tramite `pkexec dpkg`, `pkexec pacman` (`.pkg.tar.gz`/`.pkg.tar.zst`), sostituzione atomica dell'AppImage (con riavvio su Linux) o il programma di installazione pacchetti di Android; installazioni al di fuori di questi formati o un appcast non firmato offrono invece la pagina della release |
| iOS | L'app interroga l'App Store, che la aggiorna, e offre la propria pagina sullo store |

Per un'app web autonoma pubblicata, `updater.Check(cx)` confronta la release in esecuzione con un `nsgo.json` aggiornato e propone il ricaricamento previa conferma. Rileva inoltre i file di build modificati sotto la stessa etichetta di versione. Non richiede alcuna chiave appcast; i canali espliciti `none`/`steam` disabilitano comunque i controlli e gli host incorporati gestiscono autonomamente i propri ricaricamenti. I mini program usano il gestore degli aggiornamenti della propria piattaforma e si riavviano solo quando l'aggiornamento scaricato è pronto. Questi target controllano quando l'app richiama l'updater; la tempistica di controllo e scaricamento dei mini program spetta all'host. Non espongono setter per la preferenza di controllo automatico. Un gestore personalizzato `cx.CheckForUpdates` usa `UpdateCheck.Restart` per un aggiornamento preparato la cui etichetta di versione può mancare.

### Canali

Il canale decide chi aggiorna l'app. `-channel` sostituisce `updates.channel` per una singola build, così un solo progetto compila per ciascuno:

| Canale | Per | I pacchetti |
| --- | --- | --- |
| `direct` | GitHub, un sito web, cask Homebrew e winget, che installano i file stessi della release | Si aggiornano da soli come sopra |
| `store` | App Store, Mac App Store, Microsoft Store, Google Play, Flathub e altri store | Sono i [pacchetti per gli store](#pacchetti-per-gli-store) e non contengono alcun programma di auto-aggiornamento; iOS e macOS interrogano il proprio App Store senza una chiave e un'installazione Google Play può usare il flusso di aggiornamento nativo di Play |
| `steam` | Steam | Producono [directory depot](#depot-steam) aggiornate da Steam |
| `none` | Distribuzioni Linux e chiunque reimpacchetti l'app | Non effettuano alcun controllo |

Gli store rifiutano le app che si aggiornano da sole, quindi carica una build `store`, mai una diretta. Ogni release diretta con installer macOS o Windows scrive anche un cask Homebrew (`dist/releases/homebrew/<name>.rb`, che dichiara `auto_updates true`, i checksum e gli URL di scaricamento per le immagini disco compilate e un collegamento `binary` a ciascuno dei [comandi](#comandi) dell'app) e un manifest multi-file per winget (`dist/releases/winget/<id>.yaml`, `<id>.installer.yaml` e `<id>.locale.en-US.yaml` per gli installer NSIS compilati). Ovunque il sistema indichi chi ha installato l'app, l'app controlla anche questo: un'app Android proveniente da un altro store, un Flatpak o Snap e una build TestFlight lasciano gli aggiornamenti a quello store. Un `updater.Check` manuale in un'installazione Google Play chiede a Play In-App Updates di offrire il proprio flusso immediato quando consentito; Play gestisce consenso, scaricamento, installazione e riavvio. Le build di sviluppo non entrano mai in quel flusso. I pacchetti Android autonomi includono l'SDK; gli [host di incorporamento](embedding.md#android) lo forniscono tramite Gradle.

### Verifica dall'app

`updater.Check(cx)` verifica come fa una voce di menu *Verifica aggiornamenti…* e mostra ciò che trova; `updater.CheckInBackground(cx)` mostra solo una versione più nuova e `updater.CheckDaily(cx)` lo fa al massimo una volta al giorno. `updater.Status(cx)` legge l'avanzamento in tempo reale di scaricamento e installazione su Linux e Android. Quando `automatic` è attivo, l'app controlla in questo modo su Linux, sulle build Android dirette e sulle build Apple App Store all'avvio e ogni volta che l'utente vi ritorna; un controllo che fallisce, ad esempio offline, viene ritentato un'ora dopo. Le build di sviluppo non si aggiornano mai. Nelle build Steam queste tre funzioni non fanno nulla.

`updater.CapabilityOf(cx)` segnala come si aggiorna questa copia senza effettuare una richiesta di rete: `ManagedSelf`, `ManagedPage`, `ManagedDevelopment`, `ManagedChannel`, `ManagedStore` (con il relativo nome `Store`) oppure `ManagedUnavailable`. Una build di sviluppo, un canale disabilitato o un'installazione da store produce una spiegazione specifica del motivo quando viene verificata; `core.UpdateCheck.Err` è riservato a errori come un feed non valido o un updater incluso che non riesce a caricarsi.

Leggi `updater.Status(cx)` all'interno di un binding per seguire `Capability`, `AutomaticChecks`, `AutomaticDownloads`, `Available`, `Notes` e `LastCheck`, e su Linux e Android anche `Phase`, `Loaded` e `Total` di un aggiornamento (`Fraction()` per una barra di avanzamento). `updater.SetAutomaticChecks(cx, on)` rende persistente la scelta dell'utente e modifica lo scheduler giornaliero a runtime. Sparkle e WinSparkle gestiscono le proprie preferenze; Linux, Android e iOS conservano la scelta in una partizione di archiviazione privata. `updater.SetAutomaticDownloads(cx, on)` è supportato solo su macOS, con valore predefinito `updates.automatic_downloads`; le altre piattaforme restituiscono falso e ignorano il setter. Le build non supportate e quelle gestite esternamente segnalano entrambe le impostazioni come false. WinSparkle segnala i metadati dell'aggiornamento trovato leggendo l'appcast dopo il callback di rilevamento; continua comunque a controllare l'installazione. Linux risolve all'avvio l'appartenenza a un pacchetto Debian o Arch e l'identità AppImage; tali copie segnalano `ManagedSelf`, mentre altre copie non impacchettate segnalano `ManagedPage` prima di qualsiasi verifica.

## Avvio all'accesso

`desktop.SetLaunchAtLogin(cx, on)` registra l'app presso il meccanismo di accesso della piattaforma e `desktop.LaunchAtLogin(cx, done)` lo legge. Un MSIX compilato dallo strumento corrente include automaticamente un'attività di avvio disabilitata; abilitarla rispetta l'interruttore App all'avvio dell'utente e le politiche dell'amministratore. Le normali installazioni Windows e Linux mantengono la propria voce Run e la voce XDG autostart. macOS usa il servizio di accesso nativo dell'app nel bundle.

Flatpak interroga il portale Background, che può richiedere il consenso senza concedere accesso ai file di configurazione dell'host. Una lettura successiva attende la risposta nativa. Il portale non dispone di un'interrogazione di avvio di sola lettura: le letture Flatpak restituiscono solo l'ultima risposta confermata in questa esecuzione, falso prima che ve ne sia una, e non possono rilevare una modifica esterna all'app né recuperare l'impostazione all'esecuzione successiva.

Uno Snap impacchettato esternamente dichiara il nome del file di avvio automatico sulla propria app:

```yaml
apps:
  app:
    command: bin/app
    autostart: app.desktop
```

NSGo scrive quel nome file sotto `$SNAP_USER_DATA/.config/autostart`, che snapd avvia tramite il wrapper dell'app. `desktop.LaunchedAtLogin(cx)` distingue un avvio all'accesso prima che l'app venga costruita, inclusa l'attivazione di una StartupTask MSIX.

## Tipi di file

`file_types` registra i documenti aperti da un'app: dal Finder, dal Dock e da Apri con su macOS, da Apri con di Esplora file tramite l'installer Windows o l'MSIX su Windows, tramite la voce desktop dei pacchetti Debian, Arch, AppImage e Flatpak su Linux e tramite Apri in su iOS:

```yaml
file_types:
  - name: Flow File
    extensions: [flow]
    # id: dev.example.hello.flow   # <id>.<first extension> by default; another's ID imports its type
    # icon: assets/Flow.icns       # macOS document icon; or a square PNG
    # role: viewer                 # editor by default
    # conforms: [public.json]      # public.data by default
    # mime: [application/json]     # existing MIME types; otherwise inferred on Linux
```

L'installer Windows aggiunge l'app ad Apri con senza sostituire l'app predefinita dell'utente e rimuove le proprie registrazioni in caso di aggiornamento e disinstallazione. Linux assegna i tipi MIME delle estensioni comuni da una tabella fissa e attribuisce a un'estensione sconosciuta del tipo proprio dell'app `application/x-<type id>` con una definizione shared-mime-info. `mime` indica invece tipi esistenti; ne ha bisogno un tipo importato la cui estensione manca nella tabella. L'esecuzione di un'AppImage non registra nulla: è uno strumento di integrazione desktop a installarne voce e definizioni.

L'app riceve i file con `desktop.OnOpenFiles`, incluso un avvio successivo su Windows o Linux mentre è in esecuzione e le aperture di documenti nelle scene iOS. I file che hanno avviato l'app attendono il primo gestore:

```go
desktop.OnOpenFiles(cx, func(paths []string) { open(paths[0]) })
```

Su iOS, i documenti ordinari vengono importati in file temporanei di proprietà dell'app con i loro nomi originali. Le letture dei file provider sono coordinate fuori dal thread della UI e i loro ambiti di sicurezza terminano prima che il gestore venga eseguito. Sposta un file importato in `cx.DataDir()` per conservarlo; le modifiche riguardano la copia. iOS usa l'icona documento dell'app; `icon` si applica su macOS. Le App Clip e i pacchetti tvOS non registrano alcun tipo di documento. Le aperture di file restano separate da `OnOpenLinks`.

`desktop.AddRecentFile(cx, path)` registra un documento aperto dall'utente: nel menu del Dock e in File › Apri recenti su macOS, nella categoria Recenti della Jump List su Windows e nei file usati di recente del desktop su Linux. Una scelta raggiunge lo stesso gestore. `desktop.ClearRecentFiles` dimentica le registrazioni dell'app, mantenendo la cronologia di altre app per i file condivisi su Linux; Linux rispetta anche l'impostazione della cronologia di GTK.

`desktop.SetDockMenu(cx, items)` imposta azioni sull'icona dell'app: il menu del Dock di macOS, le attività della Jump List di Windows e la quicklist sul dock di Ubuntu, su Plasma e su Dash to Dock su Linux laddove le quicklist sono supportate. Linux richiede una voce desktop installata corrispondente all'`id` dell'app e mantiene sottomenu, separatori, voci selezionate e disabilitate. Il menu appartiene allo scope che lo ha impostato e l'ultimo chiamante tra le finestre dell'app lo sostituisce.

## Link

`url_schemes` assegna all'app propri link, come `myapp://settings`, che la aprono da un browser, da un'altra app, da `open`, `start` o `xdg-open`. Le app macOS, iOS e Android li registrano, così come l'installer Windows, il pacchetto MSIX e la voce desktop Linux inclusa nei pacchetti Debian, Arch, AppImage e Flatpak. La disinstallazione li rimuove.

```yaml
url_schemes: [myapp]
# app_links: [links.example.com]  # HTTPS hosts for the full app
```

L'app li riceve con `desktop.OnOpenLinks`, anche mentre è in esecuzione: su Windows e Linux un secondo avvio passa il proprio link all'app in esecuzione ed esce. Un link che ha avviato l'app attende il primo gestore. Chiunque può scrivere un link simile, e su Windows e Linux il gestore riceve qualsiasi URL con cui l'app viene avviata, quindi verifica lo schema e chiedi conferma all'utente prima che un link modifichi qualcosa:

```go
desktop.OnOpenLinks(cx, func(links []string) { show(links[0]) })
```

Su iOS e Android, `mobile.OnOpenLinks` riceve i link all'avvio e mentre l'app è in esecuzione. Aprine uno con `xcrun simctl openurl booted myapp://settings` sul simulatore iOS oppure `adb shell am start -a android.intent.action.VIEW -d myapp://settings` su Android. `app.OpenLinks("myapp://settings")` ne consegna uno in un test.

Per gli universal link iOS, dichiara host HTTPS espliciti con `app_links: [links.example.com]`. Ciascun host è un nome DNS privo di schema, percorso, porta, indirizzo IP o carattere jolly; i nomi vengono convertiti in minuscolo e i duplicati vengono rifiutati. Lo strumento scrive `applinks:<host>` nell'entitlement Associated Domains dell'app completa, inclusa la firma gestita da Xcode; un profilo IPA deve autorizzarlo. Le App Clip mantengono le dichiarazioni separate `appclips:` provenienti da `app_clip.domains`.

Ogni host deve servire un file `apple-app-site-association` che nomini l'app completa all'indirizzo `https://<host>/.well-known/apple-app-site-association`; i soli metadati del pacchetto non possono autorizzare un universal link. I [requisiti di associazione di Apple](https://developer.apple.com/documentation/xcode/supporting-associated-domains) descrivono la configurazione del server. Le firme ad hoc del simulatore omettono gli entitlement riservati agli sviluppatori, quindi un test sul delegato non dimostra l'associazione del server.

Su iOS, `mobile.OpenedURL(cx)` tiene traccia dell'invocazione di App Clip, dell'universal link o dell'URL con schema personalizzato più recente, incluso il valore iniziale prima che venga montata la radice. La consegna con schema personalizzato raggiunge comunque `mobile.OnOpenLinks`; il valore dell'invocazione è aggiornato quando quel gestore viene eseguito. Le aperture di file restano in `OnOpenFiles` e non sostituiscono l'URL dell'invocazione. Il supporto per l'invocazione su Android/Web è tracciato separatamente nella issue #1475.

## Condivisioni in ingresso su Android

Scegli i tipi MIME che l'app gestisce dal foglio di condivisione Android:

```yaml
android:
  share_types: [text/plain, 'image/*', application/pdf]
```

Le build APK e App Bundle registrano quei tipi sia per elementi singoli sia per elementi multipli. Registra [`mobile.OnShared`](services.md#aprire-link-e-file) per ricevere ogni condivisione.

Per un'app web con indirizzi basati su percorso, imposta `web.base` sulla relativa directory di distribuzione (`/` o `/my-app/`) e configura il server di produzione affinché serva `index.html` per le rotte dell'applicazione. Il server di sviluppo fornisce già questo fallback per le richieste di navigazione HTML. Gli asset rimangono relativi a quel base HTML fisso quando un deep link viene ricaricato. Vedi [Indirizzi dell'applicazione](addresses.md).

## Condivisioni in ingresso su iOS

Dichiara gli Uniform Type Identifier accettati dall'estensione di condivisione, posizionando i tipi URL e testo prima dei tipi di file più ampi:

```yaml
apple:
  share_types: [public.url, public.plain-text, public.image, com.adobe.pdf]
  # app_group: group.com.example.app  # defaults to group.<id>
sign:
  # share_profile: assets/Share.mobileprovision
```

`build` e `dev` impacchettano un'estensione di condivisione Go, `<id>.share`, insieme all'app. Xcode la firma e ne registra l'App Group per le esecuzioni su dispositivo. Per un IPA, abilita lo stesso App Group per `<id>` e `<id>.share`; `sign.profile` e `sign.share_profile` devono concederlo. App Clip, tvOS e watchOS non dispongono di un'estensione di condivisione.

L'estensione copia fino a 128 elementi e 512 MiB di file nell'App Group, quindi completa il foglio di condivisione. `mobile.OnShared` riceve tali batch mentre l'app è attiva o alla successiva apertura da parte dell'utente; essi sopravvivono alla chiusura dell'estensione e dell'app prima della consegna. iOS non fornisce alcuna API supportata che consenta a un'estensione di condivisione di portare in primo piano o avviare l'app contenitore. Questo passaggio non apre quindi automaticamente l'app.

## Mini program

```sh
go tool nsgo build -target miniprogram_wechat -run
go tool nsgo dev -target miniprogram_alipay
```

Ogni target scrive un progetto per lo strumento di sviluppo della propria piattaforma, anche in `dist/releases/miniprogram/<platform>`, che `-run` e `dev` aprono; `dev` ricompila al salvataggio e lo strumento ricarica:

| Target | Piattaforma | Strumento |
| --- | --- | --- |
| `miniprogram_wechat` | WeChat | [WeChat DevTools](https://developers.weixin.qq.com/miniprogram/dev/devtools/download.html), con la porta di servizio attiva (Settings, Security) |
| `miniprogram_qq` | QQ | [Strumento di sviluppo QQ](https://q.qq.com/wiki/tools/devtool/), su macOS |
| `miniprogram_alipay` | Alipay | [Mini Program Studio di Alipay](https://opendocs.alipay.com/mini/ide/download), su macOS |
| `miniprogram_douyin` | Douyin e le altre app di ByteDance | [Strumento di sviluppo Douyin](https://developer.open-douyin.com/docs/resource/zh-CN/mini-app/develop/developer-instrument/download/developer-instrument-update-and-download), su macOS |

`miniprogram.appid` fornisce l'AppID del mini program su ciascuna piattaforma (`wechat`, `qq`, `alipay`, `douyin`); senza di esso, WeChat DevTools lo visualizza in anteprima con il proprio AppID turistico e gli altri strumenti ne richiedono uno. Su WeChat, `-run` e ogni ricompilazione `dev` chiudono e riaprono automaticamente il progetto tramite la CLI, così DevTools compila i file aggiornati. Questo comportamento è stato verificato con DevTools 2.01.2510290. La barra di navigazione della piattaforma mostra il titolo dell'app; con `custom_navigation`, l'app riempie la pagina e la sua area sicura la mantiene libera dal pulsante di menu. L'app legge il sistema operativo del telefono con `Env.Simulated` impostato su true, come sul web con `--nsgo-os`: le pagine di uno stack mostrano il proprio `Header`, poiché nessun header di sistema lo sostituisce. QQ, Alipay e Douyin non sono ancora stati provati nei rispettivi strumenti, e Alipay non gestisce ancora il ritorno indietro tramite l'app, i keyframe o l'altezza della tastiera.

I mini program sono sperimentali. Un mini program esegue soltanto app Go, non TSX, e le piattaforme limitano a 2 MB il modulo compresso dell'app: un'app di poche schermate su `base` rientra nel limite, come fa `examples/gallery-miniprogram`, una basata sui controlli di `ui` non ancora, e `build` segnala quando non ci sta. Lì gli shader non disegnano nulla, e viste web, video, componenti nativi e file di font dell'app non sono ancora mostrati.

## HarmonyOS

```sh
go tool nsgo build -target harmonyos -run
```

`harmonyos` scrive `<name>-<version>-harmonyos.hap` in `dist/releases`. Il motore gira come WebAssembly in ArkWeb e un host ArkTS disegna ciò che monta con viste ArkUI, quindi testo, campi, scorrimento e finestre di dialogo sono quelli di sistema. Con `hdc` su `PATH`, `-run` installa l'HAP sul dispositivo o sull'emulatore connesso e lo avvia, mentre `nsgo test -target harmonyos` esegue i test dell'app da un HAP non firmato su un emulatore con un'identità bundle `.nsgotest` separata, rimuovendo quell'HAP di test al termine dell'esecuzione in modo che l'app installata rimanga disponibile.

Per compilare l'host ArkTS dai template e usare l'`id` dell'app come nome del bundle, inserisci `hvigorw` in `PATH` e imposta `OHOS_BASE_SDK_HOME` su un OpenHarmony SDK con API 26 (`26.0.0`), la versione SDK di compilazione e target dei template. `build.harmonyos_bundles` sceglie `auto` (predefinito), `exact` (che richiede l'SDK) oppure `padded` (che riempie l'id fino alla lunghezza fissa dell'host committato e non richiede l'SDK). I test sui target usano la stessa impostazione e stampano quale host hanno impacchettato. `harmonyos.permissions` elenca i nomi `ohos.permission` richiesti dall'app, come `MICROPHONE` e `CAMERA`, e `widget.title` è ciò che mostra la sua scheda nella schermata iniziale.

HarmonyOS è un'anteprima. Gli emulatori possono installare HAP non firmati. [Le note sull'architettura](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/harmonyos.md) elencano le funzionalità ArkUI disegnate dall'anteprima.

### HAP firmati

Imposta `harmonyos.signing` per firmare un HAP con i tuoi materiali di AppGallery Connect. Un'installazione diretta su un telefono Huawei richiede un certificato di debug e un profilo per quel dispositivo. Usa i materiali di rilascio per la distribuzione. [Guida alla firma Huawei](https://developer.huawei.com/consumer/en/doc/harmonyos-guides-V2/command-line-building-app-hap-hvigor-0000001553061909-V2/)

```yaml
harmonyos:
  signing:
    keystore: signing/app.p12
    key_alias: app
    certificate: signing/app.cer
    profile: signing/app.p7b
```

I percorsi sono relativi a `nsgo.yaml`. Imposta `NSGO_HARMONY_KEYSTORE_PASSWORD` e `NSGO_HARMONY_KEY_PASSWORD` nell'ambiente. La firma utilizza `hap-sign-tool.jar` dell'SDK e Java da `JAVA_HOME` o `PATH`. La build mantiene l'`id` esatto dell'app affinché corrisponda al profilo, quindi la firma richiede l'SDK e `hvigorw` anche in modalità `auto`. `go tool nsgo doctor`, eseguito nella directory dell'app, controlla la configurazione della firma e il nome del bundle nel profilo senza password. `build -run` passa a `hdc` l'HAP firmato.

## Firma

```yaml
sign:
  identity: "Apple Development: you@example.com (ABCDE12345)"  # signs the macOS app, else ad hoc
  profile: assets/AppStore.mobileprovision  # signs the IPA
  keystore: assets/release.p12              # signs APKs, with key_alias
  certificate: assets/codesign.pfx          # signs the Windows app and its installer
  # widgets_profile: assets/Widgets.mobileprovision  # the iOS extension's profile; use a macOS profile for a Mac App Store build
  # share_profile: assets/Share.mobileprovision      # signs the IPA's share extension
  # clip_profile: assets/Clip.mobileprovision        # signs the IPA's App Clip
```

Le password del keystore provengono da `NSGO_KEYSTORE_PASSWORD` e `NSGO_KEY_PASSWORD`. `-team` firma il pacchetto dell'iPhone. `-profile`, `-widgets-profile`, `-share-profile`, `-clip-profile` e `-macos-profile` sostituiscono da riga di comando i profili di provisioning dell'app, dell'estensione widget, dell'estensione di condivisione, dell'App Clip e del Mac App Store. `-installer` sostituisce l'identità Mac Installer Distribution.

### Firmare i pacchetti Windows

Authenticode identifica l'editore di un download Windows; [SmartScreen ne verifica la reputazione separatamente](https://learn.microsoft.com/windows/apps/package-and-deploy/smartscreen-reputation). `sign.certificate`, un file `.pfx`, firma l'eseguibile, il compagno da riga di comando, i programmi e le librerie portati dalle risorse dell'app, l'installer con il proprio programma di disinstallazione e l'MSIX compilato per il canale Store; la relativa password proviene da `NSGO_CERTIFICATE_PASSWORD`. Le firme vengono generate in Go, come le genera `signtool`, così un Mac le appone senza bisogno del Windows SDK:

```yaml
sign:
  certificate: assets/codesign.pfx
  timestamp_url: http://timestamp.digicert.com  # the RFC 3161 server that timestamps them
```

`sign.timestamp_url` ha come valore predefinito quello di DigiCert, mentre `none` lascia le firme prive di marca temporale, facendole scadere al termine del certificato. Un certificato su token hardware o su un servizio cloud firma invece tramite un comando proprio, in cui `%1` rappresenta il file da firmare e che la build esegue per ciascuno dei file sopra indicati:

```yaml
sign:
  sign_command: "signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 /a %1"
```

`build` si arresta se il comando riesce ma lascia l'eseguibile dell'app, il compagno da riga di comando, una risorsa PE o l'eseguibile di installazione privi di una firma Authenticode incorporata.

Il programma di disinstallazione all'interno dell'installer viene firmato prima di essere incorporato, con lo stesso `sign.certificate` o `sign.sign_command` dell'app e del programma di installazione. `-certificate`, `-sign-command` e `-timestamp-url` impostano questi valori da riga di comando.

### Notarizzazione

Le app scaricate su un Mac si aprono senza l'avviso di Gatekeeper una volta che Apple le ha notarizzate. Firma con un Developer ID e indica le credenziali conservate da `notarytool` nel tuo portachiavi:

```sh
xcrun notarytool store-credentials hello --apple-id you@example.com --team-id ABCDE12345
```

```yaml
sign:
  identity: "Developer ID Application: Example Inc (ABCDE12345)"
  notarize: hello   # the keychain profile above; or -notarize hello
```

`build` firma quindi con l'hardened runtime, invia l'immagine disco, attende il verdetto di Apple — qualche minuto — e applica il ticket sia all'immagine sia all'app; un eventuale rifiuto stampa il log di Apple. In CI, `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID` e `NSGO_NOTARY_ISSUER`, ovvero file `.p8`, ID ed emittente di una chiave API App Store Connect, sostituiscono il profilo. L'hardened runtime nega microfono, fotocamera, calendari e posizione a meno che l'app non li dichiari, cosa che `build` fa quando `apple.info` contiene `NSMicrophoneUsageDescription`, `NSCameraUsageDescription`, una descrizione d'uso per calendari o promemoria oppure una descrizione d'uso per la posizione.

## Widget e Attività in tempo reale

`build` e `dev` compilano l'estensione che disegna i widget e le Attività in tempo reale dell'app nei pacchetti macOS e iOS utilizzando il compilatore Swift di Xcode, senza passaggi separati. Su iOS l'app e i suoi widget condividono un app group: `-run` su un iPhone fa sì che Xcode firmi entrambi e registri il gruppo; per un IPA, abilitalo sia per `<id>` sia per `<id>.widgets` e firma l'estensione con `sign.widgets_profile`. Gli APK e gli AAB autonomi per Android includono i provider App Widget e le risorse da `widgets`; il runtime Java precompilato disegna le loro timeline senza che le build dell'app necessitino di un SDK o di un compilatore Java. Android 16 mostra le Attività in tempo reale come Live Updates, che non richiedono alcuna estensione. [Widget](widgets.md) e [Attività in tempo reale](live-activities.md) illustrano configurazione e API.

## Android App Links

`app_links` indica host HTTPS espliciti i cui link aprono l'app Android completa:

```yaml
app_links: [example.com, shop.example.com]
```

L'APK e l'App Bundle dichiarano un filtro `ACTION_VIEW` navigabile con `autoVerify` per ogni host. Usa host DNS privi di schema, percorso, porta, carattere jolly o indirizzo IP. Servi `https://<host>/.well-known/assetlinks.json` con l'ID pacchetto dell'app e l'impronta digitale del certificato di firma; Android verifica tale associazione prima di instradare i link all'app. Lo strumento non pubblica i file di associazione; la [guida alla verifica di Android](https://developer.android.com/training/app-links/verify-applinks) ne descrive la configurazione. Un host Android incorporato dichiara i propri intent filter e inoltra i link accettati alla propria `NsgoView`.

`mobile.OpenedURL(cx)` legge l'invocazione accettata prima del primo rendering della radice e cambia quando un altro link apre l'app in esecuzione. `mobile.OnOpenLinks` continua a ricevere ogni link accettato. Un'app web a pagina intera legge l'indirizzo corrente della propria pagina tramite `mobile.OpenedURL`, incluse la cronologia del browser e le modifiche all'hash; un'app montata all'interno di un'altra pagina non legge alcun indirizzo della pagina contenitore. Usa [`cx.AppURL`](addresses.md) quando l'app scrive anche il proprio indirizzo.

## App Clip

Una App Clip è una piccola parte di un'app iOS che si avvia da un link, un codice o un tag NFC senza installare l'app. `app_clip` ne compila una nel pacchetto iOS, a partire da un proprio pacchetto `main` così da collegare solo ciò che mostra:

```yaml
app_clip:
  main: ./clip              # the clip's main package, the app's own by default
  domains: [example.com]    # whose links open it
  # name: Hello Clip        # "<name> Clip" by default
  # id: dev.example.hello.Clip  # <id>.Clip by default
  # minimum: "17.0"         # 16.0 by default; 17.0 allows 100 MB when only links open it
  # notifications: true     # ask for notifications for a day
  # location: true          # let the clip confirm where it was opened
```

La clip legge il link che l'ha aperta tramite `mobile.OpenedURL`, che cambia quando un altro link la riapre, e `mobile.IsAppClip` indica se il codice è in esecuzione come tale. Il sistema scarica una clip fino a 15 MB, oppure 100 MB da iOS 17 quando viene aperta soltanto dai link, e `build` rifiuta una clip più grande; una clip che collega `ui` occupa circa 11 MB. `dev -target ios_simulator -clip https://example.com/order?table=12` esegue la sola clip nel simulatore, aperta da quel link; `-target ios` la esegue su un iPhone e stampa il codice QR del link: aggiungi il link in Impostazioni > Sviluppatore > Esperienze locali, poi scansionalo con la fotocamera. Usa `-release` se i simboli di sviluppo spingono la clip oltre il limite di dimensione del QR. Apple firma le App Clip solo per i team dell'Apple Developer Program, non per i team personali.

`dev -target ios` e `build -run -target ios` mantengono anche la clip all'interno dell'app completa: Xcode la firma come target separato e la incorpora prima di firmare l'app. La firma automatica richiede un account di team a pagamento autenticato in Xcode, con provisioning sia per l'app sia per la clip. Per l'App Store, abilita App Clips per `<id>`, firma la clip con `sign.clip_profile` e servi il file `apple-app-site-association` dei domini. `examples/appclip` nel repository è una caffetteria la cui clip consente di ordinare al tavolo.

Per passare un ordine o altri dati locali all'app completa, imposta un gruppo esplicito accanto ad `app_clip`:

```yaml
apple:
  app_group: group.dev.example.hello
```

La clip e l'app completa condividono quindi `cx.DataDir()` e `cx.Storage()` nel container del gruppo. I file sotto la directory dei dati e le voci chiave-valore scritte dalla clip sono disponibili all'avvio dell'app completa; `cx.LogsDir()` segue quella directory, mentre ciascun bundle mantiene la propria cache. La firma Xcode richiede il gruppo per entrambi i target; quando usi i profili, abilita lo stesso gruppo per entrambi gli App ID e concedilo in `sign.profile` e `sign.clip_profile`. Una concessione mancante rifiuta la firma e un gruppo inaccessibile arresta l'avvio. Una directory dei dati esplicita di un host ha comunque la precedenza.

Senza `apple.app_group`, ogni bundle mantiene il proprio spazio di archiviazione in sandbox. Abilitare l'impostazione non migra i dati precedentemente salvati in tali sandbox e le app con soli widget mantengono la posizione di archiviazione esistente. Conserva i segreti condivisi nel portachiavi (keychain) anziché nell'archiviazione semplice. L'esempio della caffetteria salva l'ordine della clip e lo mostra nell'app completa.

Da un'azione esplicita dell'utente all'interno della clip, chiama `mobile.ShowAppClipOverlay(cx, mobile.AppClipOverlayBottom)` per chiedere a StoreKit di consigliare la corrispondente app completa. `AppClipOverlayBottomRaised` lascia spazio per una barra inferiore. La funzione restituita chiude quella richiesta; anche la pulizia dello scope la chiude e una richiesta precedente non può chiuderne una più recente. Le chiamate al di fuori di un'App Clip iOS nativa non fanno nulla. L'App Store decide se può caricare e mostrare il suggerimento, quindi la chiamata non segnala né un'offerta mostrata né un'installazione. Mantieni utilizzabile l'attività della clip senza dover installare l'app completa. [Linee guida di Apple sull'overlay](https://developer.apple.com/documentation/appclip/recommending-your-app-to-app-clip-users)

## Incorporare in un'altra app

`-embed` impacchetta l'app per una tua app iOS o Android: un XCFramework, composto da framework dinamici con `-dynamic`, oppure un AAR la cui `NSGoView` o `NsgoView` la mostra. I programmi desktop Go mostrano le app con `nsgo.NewView` e le pagine web con il file `nsgo.js` del sito, il cui host è quello della pagina o quello indicato da `web.host` in `nsgo.yaml`. Gli esempi `examples/embed-*` del repository mostrano ciascun caso; [Incorporamento](embedding.md) tratta il lato host.
