---
title: Entwicklungsumgebung einrichten
description: NSGo aus dem Quellcode bauen, die Gallery ausführen und eine Änderung testen.
group: Mitwirken
order: 80
---

## Voraussetzungen

macOS mit Xcode und Go 1.27.3 oder neuer baut jede Zielplattform für arm64, einschließlich der Android-APK. Für die Ausführung auf Geräten gilt das unter [Einrichtung](getting-started.md#requirements) Aufgeführte, und für das Ziel `android_x64` des x86_64-Android-Emulators wird ein Android NDK benötigt, da Go `android/amd64` nur mit cgo linkt. Setup- und Deinstallationsprogramme für Windows bauen allein mit Go.

```sh
git clone https://github.com/nsgo-dev/nsgo.git
cd nsgo
```

Lesen Sie [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) und [AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md), bevor Sie Code ändern.

## Die Gallery

Die Gallery enthält eine Demo für jedes Element sowie jede `base`- und `ui`-Komponente und jeden Dienst. Ein breites Fenster zeigt die Demos unter Reitern in einer Seitenleiste; ein schmales zeigt die Reiter und Einstellungen in einer unteren Leiste, während Demos darübergelegt werden. Starten Sie sie aus dem Stammverzeichnis des Repositories:

```sh
go tool nsgo dev examples/gallery                   # nsgo.yaml's dev target, exiting with the app
go tool nsgo dev examples/gallery-tsx               # the gallery's components and apps, written in TSX
go tool nsgo dev -target web examples/gallery       # in the browser, serving until interrupted
go tool nsgo dev -target ios_simulator examples/gallery
go tool nsgo dev -target ios examples/gallery       # on the connected iPhone, signed by Xcode
go tool nsgo dev -target android examples/gallery   # on the connected device or emulator, over adb
go tool nsgo build -target all examples/gallery     # every platform's packages in examples/gallery/dist, what to hand out in dist/releases
```

## Prüfen und testen

```sh
go tool nsgo check                 # vet every package
go tool nsgo check -target all     # vet every available Go/Xcode target instead
go tool nsgo test                  # every package's tests
go tool nsgo test -bench . ./core  # core's benchmarks
(cd benchmark && go run ./runner)  # measure against other frameworks
# capacity curve for NSGo's 600-cell grid, without rebuilding it
(cd benchmark && go run ./runner -frameworks nsgo -cases grid -grid-cols 20 -grid-rows 15 -out /tmp/grid.json)
```

`-target` prüft statt des Hosts die genannten Zielplattformen und – im NSGo-Repository selbst – auch den Dev-Client, den ein einfaches `check` prüft. Um alle Plattformen zu prüfen, die eine Änderung berührt, nennen Sie sie gemeinsam in einem einzigen Aufruf, etwa `check -target web,android -jobs 2`, statt mehrere parallel zu starten: `-jobs n` prüft bis zu n Zielplattformen gleichzeitig, jedes `go vet` mit n Compile-Workern, während standardmäßig eine Zielplattform nach der anderen mit dem Go-Standardwert läuft; ein `check`, das gestartet wird, während im selben Modul bereits ein anderes läuft, wartet auf dieses und gibt eine Meldung aus. `check.yml` in der CI prüft jede Zielplattform.

Wenn Sie den Befehl im Stammverzeichnis des Repositories oder in `internal/dev` ausführen, testet er `internal/dev` ohne GUI ein zweites Mal mit dem Build-Tag `nsgodev`, das die Dateien des Dev-Clients benötigen, sodass diese Tests lokal wie in der CI laufen. Beide Läufe akzeptieren dieselben Filter und Flags und teilen sich eine gemeinsame Gesamtzählung sowie einen JUnit-Bericht, in dem das Paket des zweiten Laufs `internal/dev [nsgodev]` heißt.

`check -target all` prüft die Zielplattformen macOS, Windows, Linux, Android arm64 und Web auf jedem Host. Es prüft außerdem iOS, tvOS und watchOS, sofern deren Xcode-SDKs verfügbar sind, und listet die übersprungenen auf. Eine ausdrücklich genannte Zielplattform wie `check -target ios` schlägt fehl, wenn ihre Werkzeuge fehlen – auch in Kombination mit `all`. Eine fehlgeschlagene Zielplattform verhindert nicht die Prüfung der übrigen, und der Befehl schlägt fehl, falls eine erforderliche Prüfung fehlschlägt. `build -target all` benötigt weiterhin macOS mit Xcode. Im eigenen Repository von NSGo prüft jede genannte Zielplattform außerdem ihre Plattform-Test-Hosts mit `nsgotest`; die CI nennt das Preview-Target `harmonyos` ausdrücklich, da es außerhalb von `all` liegt.

Der Linux-CI-Job führt die `js/wasm`-Tests des HarmonyOS-Go-Backends mit Node.js und dem Wasm-Launcher von Go aus. Führen Sie den Gallery-Test lokal mit einem über `hdc` verbundenen HarmonyOS-Emulator aus. Das Kompilieren der [HarmonyOS-Host-Vorlagen](building.md#harmonyos) erfordert das OpenHarmony SDK API 26.

```sh
go tool nsgo test -env GOOS=js -env GOARCH=wasm ./internal/platform/harmonyos \
  -- "-exec=env -i \"PATH=$PATH\" \"$(go env GOROOT)/lib/wasm/go_js_wasm_exec\"" -count=1
go tool nsgo test -target harmonyos -run TestHarmonyOSPages -timeout 10m ./examples/gallery
```

Führen Sie in einer Wayland-Sitzung auf einem Linux-Desktop die echte Akzeptanzprüfung des Global-Shortcut-Portals mit `NSGO_TEST_GLOBAL_SHORTCUT_PORTAL=<installed-desktop-id> go tool nsgo test -run TestGlobalShortcutPortalWithPermission ./internal/platform/linux` aus. Die ID muss einen `.desktop`-Eintrag besitzen. Bestätigen Sie Strg+Alt+K in der Abfrage des Compositors und drücken Sie die Tastenkombination dann, während eine andere Anwendung den Fokus hat; die Prüfung wartet sowohl auf die Registrierung als auch auf die Aktivierung. Ohne gesetzte Umgebungsvariable wird der Test übersprungen.

Ist Docker auf einem Linux-Host verfügbar, prüft `go tool nsgo test -run '^TestLinuxPackagesNative$' -env NSGO_TEST_LINUX_PACKAGES=1 ./cmd/nsgo -- -timeout=20m` echte Installationen in Wegwerf-Containern für Debian, Ubuntu und Arch. Der Test lädt deren offizielle Images herunter, installiert darin GTK-Abhängigkeiten, verifiziert jeweils die Paketdatenbank und die installierten Dateien und entfernt das Paket anschließend wieder. Die Paketdatenbank des Hosts wird niemals verwendet.

## Releases

NSGo veröffentlicht nach Bedarf, wenn ein nachgelagertes Projekt eine getaggte Version benötigt oder Maintainer fertige Änderungen ausliefern wollen. Wenn Sie ein Release für eine Bibliothek anfordern, verlinken Sie die abhängige Arbeit und nennen Sie den Commit, der die letzte benötigte NSGo-Änderung enthält. Das erste Release-Tag, das diesen Commit enthält, ist die Version, die die Bibliothek anfordern kann.

Um eine bereits gemergte Änderung vor dem Taggen zu testen, führen Sie im abhängigen Modul `go get github.com/nsgo-dev/nsgo@main` aus. Anstelle von `main` können Sie eine Commit-ID angeben, um genau die verifizierte Revision auszuwählen. Go trägt die resultierende Pseudo-Version in `go.mod` ein.

Ein Maintainer versieht die Änderungen in `CHANGELOG.md` mit Version und Datum, lässt einen frischen „Unreleased“-Abschnitt stehen und taggt den resultierenden Commit auf `main`. Der [Release-Workflow](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/release.yml) prüft das Tag und veröffentlicht es, nachdem der Test-Workflow bestanden wurde; die veröffentlichten Release-Notes wandern dann aus `CHANGELOG.md`, das nur unveröffentlichte Änderungen behält, zu den [Releases](https://github.com/nsgo-dev/nsgo/releases).

## NSH

NSGo zieht [Native Shell](https://github.com/longbridge/nsh) als Designreferenz heran; keiner seiner Tests benötigt es. Um darin zu lesen, klonen Sie es neben das Repository und checken das neueste Release-Tag aus, niemals `main`:

```sh
git clone https://github.com/longbridge/nsh.git ../nsh
git -C ../nsh checkout "$(git -C ../nsh describe --tags --abbrev=0)"
```

## Nächste Schritte

- [Die Codebasis](contributing/codebase.md): Wo jeder Teil von NSGo liegt.
- [Eine Änderung vornehmen](contributing/changes.md): Was ein Pull Request braucht.
