---
title: Configurazione dell'ambiente di sviluppo
description: Compilare NSGo dai sorgenti, eseguire la galleria e testare una modifica.
group: Contribuire
order: 80
---

## Requisiti

macOS con Xcode e Go 1.27.3 o successivo compila ogni target per arm64, incluso l'APK di Android. L'esecuzione sui dispositivi richiede quanto indicato in [Configurazione](getting-started.md#requirements), e il target dell'emulatore Android x86_64, `android_x64`, richiede un NDK Android, poiché Go effettua il link di android/amd64 solo con cgo. I programmi di installazione e disinstallazione per Windows si compilano con il solo Go.

```sh
git clone https://github.com/nsgo-dev/nsgo.git
cd nsgo
```

Leggi [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) e [AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md) prima di modificare il codice.

## La galleria

La galleria contiene una demo per ciascun elemento, componente `base` e `ui` e servizio. Una finestra ampia mostra le demo organizzate in schede in una barra laterale; una finestra stretta mostra le schede e le impostazioni in una barra inferiore, con le demo sovrapposte. Avviala dalla radice del repository:

```sh
go tool nsgo dev examples/gallery                   # nsgo.yaml's dev target, exiting with the app
go tool nsgo dev examples/gallery-tsx               # the gallery's components and apps, written in TSX
go tool nsgo dev -target web examples/gallery       # in the browser, serving until interrupted
go tool nsgo dev -target ios_simulator examples/gallery
go tool nsgo dev -target ios examples/gallery       # on the connected iPhone, signed by Xcode
go tool nsgo dev -target android examples/gallery   # on the connected device or emulator, over adb
go tool nsgo build -target all examples/gallery     # every platform's packages in examples/gallery/dist, what to hand out in dist/releases
```

## Verifiche e test

```sh
go tool nsgo check                 # vet every package
go tool nsgo check -target all     # vet every available Go/Xcode target instead
go tool nsgo test                  # every package's tests
go tool nsgo test -bench . ./core  # core's benchmarks
(cd benchmark && go run ./runner)  # measure against other frameworks
# capacity curve for NSGo's 600-cell grid, without rebuilding it
(cd benchmark && go run ./runner -frameworks nsgo -cases grid -grid-cols 20 -grid-rows 15 -out /tmp/grid.json)
```

`-target` verifica i target specificati anziché l'host e, nel repository di NSGo, anche il client dev, che il comando `check` semplice verifica. Per verificare tutte le piattaforme toccate da una modifica, elencale tutte in un unico check, ad esempio `check -target web,android -jobs 2`, invece di avviarne diversi: `-jobs n` verifica fino a n target contemporaneamente, eseguendo ciascun `go vet` con n worker di compilazione, mentre il valore predefinito è un target alla volta con le impostazioni predefinite di Go; un check avviato mentre un altro è in esecuzione nello stesso modulo attende il precedente e lo segnala. Il workflow `check.yml` della CI verifica ogni target.

Eseguito dalla radice del repository o in `internal/dev`, il comando headless testa `internal/dev` una seconda volta con il build tag `nsgodev`, necessario ai file del client dev, così i suoi test vengono eseguiti sia in locale sia in CI. Entrambe le esecuzioni accettano gli stessi filtri e flag e condividono un unico conteggio e report JUnit, in cui il pacchetto della seconda esecuzione si chiama `internal/dev [nsgodev]`.

`check -target all` verifica i target macOS, Windows, Linux, Android arm64 e web su qualsiasi host. Verifica anche iOS, tvOS e watchOS quando i rispettivi SDK Xcode sono disponibili, elencando quelli saltati. Un target indicato esplicitamente, come `check -target ios`, fallisce se i suoi strumenti non sono disponibili, anche quando è combinato con `all`. Il fallimento di un target non impedisce la verifica dei target rimanenti, e il comando fallisce se una qualsiasi verifica richiesta fallisce. `build -target all` richiede comunque macOS con Xcode. Nel repository di NSGo, ogni target nominato verifica anche i propri host di test di piattaforma con `nsgotest`; la CI nomina esplicitamente il target in anteprima `harmonyos`, poiché è escluso da `all`.

Il job Linux della CI esegue i test `js/wasm` del backend Go per HarmonyOS con Node.js e il launcher Wasm di Go. Esegui il test della galleria in locale con un emulatore HarmonyOS collegato tramite `hdc`. La compilazione dei [template host HarmonyOS](building.md#harmonyos) richiede OpenHarmony SDK API 26.

```sh
go tool nsgo test -env GOOS=js -env GOARCH=wasm ./internal/platform/harmonyos \
  -- "-exec=env -i \"PATH=$PATH\" \"$(go env GOROOT)/lib/wasm/go_js_wasm_exec\"" -count=1
go tool nsgo test -target harmonyos -run TestHarmonyOSPages -timeout 10m ./examples/gallery
```

In una sessione Wayland su un desktop Linux, esegui la verifica di accettazione reale del portale per le scorciatoie globali con `NSGO_TEST_GLOBAL_SHORTCUT_PORTAL=<installed-desktop-id> go tool nsgo test -run TestGlobalShortcutPortalWithPermission ./internal/platform/linux`. L'ID deve avere una voce `.desktop`. Concedi Ctrl+Alt+K nel prompt del compositore, poi premilo mentre un'altra applicazione ha il focus; il controllo attende sia la registrazione sia l'attivazione. Viene saltato se la variabile d'ambiente non è impostata.

Con Docker disponibile su un host Linux, `go tool nsgo test -run '^TestLinuxPackagesNative$' -env NSGO_TEST_LINUX_PACKAGES=1 ./cmd/nsgo -- -timeout=20m` verifica le installazioni reali in container usa e getta Debian, Ubuntu e Arch. Scarica le loro immagini ufficiali e installa al loro interno le dipendenze GTK, verifica ciascun database dei pacchetti e i file installati, quindi rimuove il pacchetto. Il database dei pacchetti dell'host non viene mai utilizzato.

## Rilasci

NSGo effettua rilasci su richiesta quando un progetto a valle necessita di una versione taggata oppure quando i manutentori hanno modifiche pronte da pubblicare. Quando richiedi un rilascio per una libreria, collega il lavoro dipendente e indica il commit che contiene l'ultima modifica di NSGo di cui ha bisogno. Il primo tag di rilascio che include quel commit è la versione che la libreria potrà richiedere.

Per testare una modifica già unita su `main` prima che sia taggata, esegui `go get github.com/nsgo-dev/nsgo@main` dal modulo dipendente. Puoi usare un ID di commit al posto di `main` per selezionare la revisione verificata. Go registra la risultante pseudo-versione in `go.mod`.

Un manutentore assegna alle modifiche la versione e la data in `CHANGELOG.md`, lascia una nuova sezione Unreleased e tagga il commit risultante su `main`. Il [workflow di rilascio](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/release.yml) controlla il tag e lo pubblica dopo che il workflow dei test è passato; le note pubblicate lasciano quindi `CHANGELOG.md`, che conserva solo le modifiche non ancora rilasciate, per i [Rilasci](https://github.com/nsgo-dev/nsgo/releases).

## NSH

NSGo utilizza [Native Shell](https://github.com/longbridge/nsh) come riferimento progettuale; nessuno dei suoi test lo richiede. Per consultarlo, clonalo accanto al repository ed effettua il checkout del suo ultimo tag di rilascio, mai di `main`:

```sh
git clone https://github.com/longbridge/nsh.git ../nsh
git -C ../nsh checkout "$(git -C ../nsh describe --tags --abbrev=0)"
```

## Prossimi passi

- [La codebase](contributing/codebase.md): dove risiede ogni parte di NSGo.
- [Apportare una modifica](contributing/changes.md): cosa richiede una pull request.
