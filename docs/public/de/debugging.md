---
title: Ausführen und Debuggen
description: Eine App auf jeder Zielplattform mit Hot-Reload ausführen und Fehler, Updates, Layout, Geschwindigkeit sowie andere Sprachen prüfen.
group: Toolchain
order: 20
---

## Ausführen

```sh
go tool nsgo dev
```

`dev` baut die App für diesen Rechner und öffnet sie – oder für die Zielplattform, die `dev.target` in `nsgo.yaml` angibt. Bei jedem Speichern einer Go-Datei wird die App neu gebaut und neu gestartet, wobei zuerst `go mod tidy` läuft, wenn die App ein Paket importiert, dessen Modul in `go.mod` oder `go.sum` fehlt; eine reine TSX-Änderung aktualisiert die laufende App direkt an Ort und Stelle und behält ihren Zustand. Der Lauf endet zusammen mit der App.

Unter macOS und Windows öffnet ein nativer Dev-Rebuild das Fenster des Haupt-Views wieder an seiner letzten regulären Position und Größe innerhalb der laufenden Sitzung. Windows stellt auch den Vollbildmodus wieder her. Unter Linux stellt GTK die reguläre Größe wieder her und fordert den letzten Vollbildmodus an; die Position bestimmt der Fenstermanager.

`-target` startet die App auf einer anderen Plattform, und `-device` wählt das iPhone, Apple TV, die Apple Watch, den Simulator oder das Android-Gerät anhand von Name, UDID oder Seriennummer aus:

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

Der Dev-Server lauscht standardmäßig auf allen Schnittstellen. Verwenden Sie `-host 127.0.0.1`, um ihn auf Loopback zu beschränken – auch wenn Android über `adb reverse` verbindet –, oder nennen Sie die IP-Adresse einer lokalen Schnittstelle, um nur dort zu lauschen. Für Dev-Funktionen auf physischen Apple-Geräten wählen Sie eine im LAN erreichbare Adresse.

`-h` listet alle Flags auf.

## Go-Debugger

Legen Sie eine zu Ihrer Go-Version kompatible [Delve](https://github.com/go-delve/delve/blob/master/Documentation/installation/README.md)-Version als `dlv` in Ihren `PATH`. Starten Sie eine native Desktop-App mit ihren Debug-Informationen und deaktivierter Go-Optimierung:

```sh
go tool nsgo dev -target native -debug
```

Kopieren Sie den Port aus der Zeile `API server listening at` von Delve. Fügen Sie in VS Code mit der Go-Erweiterung eine Attach-Konfiguration hinzu und ersetzen Sie `40000` durch diesen Port:

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

Legen Sie in GoLand eine Run-Konfiguration vom Typ **Go Remote** mit Host `127.0.0.1` und demselben Port an. Setzen Sie einen Breakpoint im Go-Code Ihrer App, verbinden Sie den Debugger und führen Sie die Aktion aus, die ihn erreicht. Nach einem erfolgreichen Go-Rebuild verbinden Sie den Debugger erneut unter derselben Adresse, damit er die Breakpoints an den neuen Prozess sendet. TSX-Änderungen nutzen weiterhin Hot-Reload.

`-debug` läuft unter macOS, Windows und Linux mit `-target native`. Nutzen Sie zum Debuggen einen Dev-Build und heben Sie `-release` für Messungen an einem Release-Build auf.

## Komponentenvorschauen

Legen Sie eine oder mehrere `PreviewXxx`-Funktionen neben die Komponenten in ein importierbares Go-Paket. Speichern Sie diese in einer Go-Datei, die durch das Preview-Build-Tag geschützt ist:

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

Haken Sie die Fixtures an, die gemeinsam angezeigt werden sollen, und wählen Sie Locale, Theme, helles oder dunkles Erscheinungsbild, Viewport-Größe sowie Layoutrichtung. Die Vorschau übernimmt die `nsgo.yaml`, die Übersetzungskataloge, das konfigurierte `theme` und die Ressourcen der nächstgelegenen App. Übergeben Sie hinter den Flags ein App-Verzeichnis, um eine andere Konfiguration im selben Modul auszuwählen; in diesem Fall ist der Preview-Pfad relativ zum App-Verzeichnis.

Das Speichern einer Datei baut die Vorschau neu und behält Ihre Auswahl und Anzeigeeinstellungen bei. Das Tag `nsgopreview` schließt die Fixtures von normalen Release-Builds aus. Verwenden Sie dasselbe Fixture in einer mit dem Tag versehenen `_test.go`-Datei mit `nsgotest.Start(t, PreviewEmpty)` und führen Sie sie mit `GOFLAGS=-tags=nsgopreview go tool nsgo test -target macos ./components` aus, um einen nativen Screenshot aufzunehmen.

Nach der ersten Installation der Haupt-App mit `-target ios_simulator` ersetzt ein reiner Go-Rebuild die installierte ausführbare Datei und startet die App neu. Das Bearbeiten von `nsgo.yaml`, gepackten Ressourcen oder des App-Icons löst eine Neuinstallation aus.

Nutzen Sie `go tool nsgo dev -target android -android-host` für Android-UI-Arbeit mit einem kürzeren Go-Rebuild-Zyklus. Das Werkzeug installiert die native Laufzeit einmalig und führt Go auf dem Entwicklungsrechner aus. Jeder Go-Rebuild verbindet sich über `adb` erneut mit derselben Activity. Änderungen an gepackten Ressourcen, vorkompilierten Klassen, Launcher-Icons oder `nsgo.yaml` installieren die Laufzeit-APK erneut. Der Go-Prozess nutzt die Build-Constraints, Dateien und das Netzwerk des Hosts. `Scope.DiskSpace` meldet das Volume, auf dem `DataDir` des Hosts liegt. Verwenden Sie den regulären Android-Dev-Befehl, um `_android.go`-Dateien und das Go-Verhalten direkt auf dem Gerät zu testen.

## Fehler

Ein fehlgeschlagener Build wird über der laufenden App angezeigt, mit Schaltflächen zum Kopieren des Fehlers und zum erneuten Bauen; beheben Sie die Datei, und das nächste Speichern ersetzt die Meldung. Eine Komponente, die beim Rendern in eine Panic läuft, wird genauso angezeigt. Befehl+R in der App – bzw. Strg+R außerhalb von Apple-Plattformen – baut sie jederzeit neu und startet sie neu, auch über der Fehlermeldung, sofern die App die Taste nicht selbst über `cx.OnKey` oder als aktivierten Eintrag in ihrem Anwendungsmenü belegt; im Browser lädt die Taste die Seite wie gewohnt neu. Die eigenen Ausgaben der App, etwa `log.Print`, erscheinen im Terminal unter den Zeilen des Werkzeugs.

Vor dem Ausführen prüft `go tool nsgo check` die App mit `vet` und prüft die Typen ihres TSX; `-target all` prüft sie für jede Zielplattform, sodass auch Code hinter Build-Tags anderer Plattformen vom Mac aus geprüft wird.

### Protokolle gepackter Apple-Apps

Unter macOS, iOS, tvOS und watchOS schreibt der Standard-Go-Logger in Apples Unified Log, einschließlich `log.Print`, NSGo-Diagnosemeldungen und TSX-Konsolenausgaben. Filtern Sie in der Konsole nach der Bundle-ID der laufenden App als Subsystem und `nsgo` als Kategorie. Unter macOS können Sie den Log-Store auch direkt abfragen:

```sh
log show --last 10m --predicate 'subsystem == "dev.example.hello"'
```

Einträge verwenden den Standard-Logtyp und behalten ihre Warn- oder Fehlerpräfixe bei. Ihr Text ist öffentlich einsehbar – halten Sie Zugangsdaten und andere Geheimnisse also aus den Logs fern. Lange Nachrichten werden in mehrere Einträge aufgeteilt. Die Terminalausgabe bleibt während der Entwicklung verfügbar; ein eigener Log-Writer oder `slog`-Handler steuert seine Ausgabe selbst.

### WebViews unter Linux

Ubuntu 24.04 und neuer können unprivilegierte User-Namespaces über AppArmor einschränken. Wenn `bubblewrap` die Sandbox von WebKitGTK nicht starten kann, protokolliert NSGo den Fehler und zeigt einen Platzhalter. Einige WebKitGTK-Versionen beenden sich sonst beim Öffnen des ersten WebViews mit `SIGTRAP`. [Ubuntu-AppArmor-Dokumentation](https://documentation.ubuntu.com/security/security-features/privilege-restriction/apparmor/)

Verwenden Sie das `bubblewrap`-AppArmor-Profil Ihrer Distribution, sofern vorhanden, etwa `bwrap-userns-restrict`. Andernfalls kann eine Administratorin oder ein Administrator ein Profil laden, das `/usr/bin/bwrap` das Recht `userns` gewährt, mit [dem Profil aus der Linux-CI von NSGo](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/test.yml) als Ausgangspunkt. Die Administration verwaltet das Profil, da es für jede App gilt, die das `bubblewrap` des Systems nutzt.

`WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1` schaltet die Subprozess-Sandbox von WebKit ab. Verwenden Sie dies ausschließlich für die lokale Entwicklung mit vertrauenswürdigen Inhalten.

## DevTools

```sh
go tool nsgo dev -devtools
```

`-devtools` öffnet die DevTools im Browser, ausgeliefert von `nsgo dev` für jede Zielplattform, auch für eine Smartphone-App. Sie zeigen die laufende App im Betrieb:

- **Elements**: die Views in einem Baum. Jeder Eintrag zeigt die Komponente, die ihn erzeugt hat, einen Link zu seiner Zeile in Ihrem Editor, den Selektor, über den ein Test ihn findet, seine Klassen, seine Begrenzungen, Außenabstände, Rahmen und Innenabstände nach dem Layout sowie den gesetzten Style. Das Überfahren einer Zeile zeichnet Außenabstände, Innenabstände und Inhalt in der App ein, genau wie in Browser-DevTools, und mit **Pick** können Sie einen View in jedem Fenster, Panel, Sheet oder eingebetteten View antippen, um ihn im Baum zu finden.
- **State**: die von Ihrem Code angelegten Signals unter den Komponenten und Computations, denen sie gehören, mitsamt ihren Werten, den Werten von Memos sowie der Anzahl der Computations, die jedes Signal lesen. Ein Wert, den keine Computation liest, ändert nichts auf dem Bildschirm. Strings, Booleans und Zahlen lassen sich bearbeiten, um einen Zustand ohne Codeänderung auszuprobieren. Werte, die NSGo selbst angelegt hat, stehen unter **NSGo's own**.
- **Profiler**: die Dauer jedes Commits, aufgeteilt in Ihren Code davor, den Commit samt Layout sowie das Anwenden auf dem Main-Thread der Plattform. Er zeigt außerdem die Zeilen, deren Schreibzugriffe den Commit ausgelöst haben, und öffnet bei den ausgeführten Computations – die langsamsten zuerst. Eine zweite Liste nennt die in der letzten Sekunde am häufigsten ausgeführten Computations nach der Zeile, die sie erstellt hat. Die Android-Laufzeit wendet Batches in eigenem Java-Code an, daher weisen Android-Commits keine Plattformzeit aus.
- **Recorder**: was Sie in der App drücken, tippen, löschen und scrollen, ausgegeben als `nsgotest`-Testdatei zum Kopieren in eine `_test.go`-Datei. In Elements fügen **Assert shown** und **Assert text** eine Prüfung des ausgewählten Views hinzu. Eine Textprüfung hält den Text in der Sprache fest, in der die App ihn angezeigt hat; zeichnen Sie also in der Sprache auf, in der Ihre Tests laufen.
- **Highlight updates** umrandet jeden View, den ein Commit ändert, genau wie `-highlight`.
- **Layout** blendet das [Layout-Audit](#layout) auf dem Gerät ein und aus, genau wie `-layout` es startet.

Zustand, Computations und Schreibzugriffe einer TSX-App werden über die Source-Maps der Bundles mit ihren TSX-Zeilen benannt und verlinken direkt auf die Dateien in Ihrer App.

Die DevTools lesen die App über den Server von `nsgo dev`, der im Netzwerk lauscht, damit Smartphones ihn erreichen. Jeder Lauf erzeugt ein Token, das die von `nsgo dev` ausgegebenen und für DevTools sowie das Web-Target geöffneten Adressen hinter `#` mitführen, und der Server beantwortet nur Anfragen mit diesem Token – öffnen Sie DevTools und das Web-Target also über genau diese Adressen. Jede Person, der Sie eine solche Adresse geben, kann den Zustand der App zur Laufzeit ändern und Schritte zu dem vom Recorder geschriebenen Test hinzufügen; lesen Sie einen aufgezeichneten Test daher, bevor Sie ihn ausführen. Andere in Ihrem Netzwerk sowie Webseiten anderer besuchter Sites erreichen nichts davon.

## Updates

```sh
go tool nsgo dev -highlight
```

`-highlight` umrandet eine halbe Sekunde lang jeden View, den ein Commit geändert hat, sodass ein Update, das mehr als nötig neu zeichnet, bei der Bedienung auf dem Bildschirm sichtbar wird. Die Umrandungen erscheinen über jedem Fenster, Panel, Modal, iOS-Sheet und eingebetteten View (`nsgo.NewView`). Die DevTools schalten sie während der Laufzeit ein und aus.

```sh
go tool nsgo dev -trace
```

`-trace` gibt jeden Commit der laufenden App aus: seine Nummer, seine Dauer und die gesendeten Mount-Einträge, sodass Sie sehen, welche Views ein Update erstellt, geändert oder entfernt hat. Über den Einträgen nennt die Ausgabe die Ursache des Commits: die Zeilen Ihres Codes, die seit dem letzten Commit Signals geschrieben haben – die mit den meisten Schreibzugriffen zuerst –, sowie wie viele Bindings, Effects und Memos gelaufen sind.

```text
── commit 12: 40 items, commit 1.65 ms
   after app/market.go:473 ×4, app/market.go:487 ×4, app/market.go:384
   ran 118 bindings, 18 effects, 5 memos
```

Eine Zeile, die sehr oft schreibt, oder viele Bindings, die für einen einzelnen Schreibzugriff laufen, zeigt an, wo Schreibvorgänge gebündelt (`cx.Batch`) oder das von einem Binding Gelesene eingegrenzt werden sollte. `go tool nsgo test -trace` gibt dasselbe unter jedem Schritt eines Tests aus.

## Laufzeitprofile

Erfassen Sie Go-CPU-Aktivität, Heap oder Execution-Trace einer laufenden App mit `nsgo dev`. Die App sendet die Aufzeichnung an den Dev-Server, der sie auf dem Rechner speichert, auf dem der Befehl läuft.

```sh
go tool nsgo dev -target android -profile cpu -profile-duration 10s
go tool pprof -top .nsgo/profiles/cpu.pprof
```

Das CPU-Beispiel funktioniert auch auf einem Desktop, einem iPhone oder einem iOS-Simulator. Erlauben Sie auf einem iPhone den Zugriff auf das lokale Netzwerk, falls das System danach fragt. Der Heap ist ein Schnappschuss und funktioniert auch im Web. Für Scheduler- und Goroutine-Ereignisse führen Sie `go tool nsgo dev -profile trace` aus und öffnen `.nsgo/profiles/execution.trace` mit `go tool trace`. CPU- und Execution-Traces dauern standardmäßig 10 Sekunden. `-profile-output` legt den Pfad der lokalen Datei fest. Bei Komponentenvorschauen verwenden Standard- und relative Pfade das App-Verzeichnis. Ein Dev-Lauf speichert seine erste vollständige Aufzeichnung und führt die App danach weiter aus.

```sh
go tool nsgo dev -target web -profile heap
go tool pprof -top .nsgo/profiles/heap.pprof
```

Gos Browser-Laufzeit unterstützt Heap-Profile und Execution-Traces. Für CPU-Stichproben im Browser nutzen Sie die Performance-Werkzeuge des Browsers. Wählen Sie entweder `-profile` oder `-release`. Das Ausliefern mit `nsgo build` lässt den Dev-Capture-Client außen vor. [Go-Laufzeitdiagnose](https://go.dev/doc/diagnostics)

## Layout

```sh
go tool nsgo dev -layout
go tool nsgo dev -layout=bounds,grid=4
```

`-layout` startet die App mit dem Layout-Audit der DevTools über jedem Fenster, Panel, iOS-Sheet und eingebetteten View und umrandet jeden View: seine Border-Box in Blau, die Innenkante seines Paddings in Grün und die Außenkante seines Margins in Orange – jeweils abgeschnitten, wo ein Scroll-View oder ein beschneidender View ihn verbirgt. `grid` zeichnet ein Raster von 8 Punkten und `grid=N` eines von N Punkten (mindestens 4). Geben Sie den Wert nach `=` an: Das Flag allein bedeutet `bounds`.

Eine Anzeige in der unteren Ecke zeigt die Größe des Fensters in Punkten sowie Schaltflächen zum Ein- und Ausschalten der Boxen, des Rasters und des Lineals. Auf Apple TV und anderen rein fernbedienten Geräten erreicht der Fokus die Schaltflächen der Anzeige, sodass Sie sie mit der Fernbedienung umschalten können, ohne die App zu verlassen. Solange das Lineal aktiv ist, wählen Klicks/Tipps zwei Punkte aus, statt die App zu erreichen, und die Anzeige gibt an, wie weit sie horizontal und vertikal auseinanderliegen; ein dritter Klick/Tipp beginnt von vorn. Auf Apple TV oder mit einer Tastatur bewegen Richtungspfeile (oder Wischen über das Siri-Remote-Touchpad) ein Fadenkreuz über den Bildschirm, und Select, Return oder Leertaste wählt Punkte aus, wobei die Distanz live aktualisiert wird, bis der zweite Punkt sie fixiert.

| Taste | Wirkung |
| --- | --- |
| `Cmd+Option+Shift+L`, `Ctrl+Alt+Shift+L` | Blendet das Audit in einem Build mit `-layout` ein oder aus |
| Pfeiltasten, Fernbedienungs-D-Pad / Wischen | Bewegt das Fadenkreuz des Lineals um den Rasterschritt (standardmäßig 8 pt) |
| `Shift` + Pfeiltasten | Bewegt das Fadenkreuz des Lineals um 1 pt |
| Select, `Return`, `Space` | Wählt während des Messens den Punkt des Fadenkreuzes |
| `Esc`, Zurück, Menu | Beendet das Messen |

Belegt Ihre App dieselbe Taste, gewinnt die Belegung der App. Ohne Taste – etwa auf einem Smartphone – starten Sie zusätzlich mit `-devtools` und schalten das Audit über **Layout** in den DevTools ein und aus; blenden Sie es aus, um ein Steuerelement unter der Anzeige zu erreichen. Native Backends und das Web lesen Darstellungsmatrizen aus, sodass Begrenzungen, Pick und das Audit angezeigten Transformationen, Animationen und dem Scrollen folgen; watchOS folgt seinem affinen Zeichnen. HarmonyOS und Mini-Programme behalten den Go-seitigen Tracker bei, bis ihre Render-Layer Geometrie melden. Das Neuzeichnen kostet bei jedem Update Zeit; messen Sie die Geschwindigkeit daher ohne sie.

## Geschwindigkeit

```sh
go tool nsgo dev -fps
go tool nsgo dev -release -fps
```

`-fps` zeigt den Performance-Monitor unter macOS, iOS, Windows, Linux, Android und im Web über der App an. Er misst Frames und Commits, und Android zeigt zusätzlich die Round-Trip-Zeit für nativen Text an. Dev-Builds enthalten den Dev-Client; messen Sie mit `-release`, das genau so baut wie `nsgo build`.

## Sprachen und Erscheinungsbild

```sh
go tool nsgo dev -lang zh-Hans
go tool nsgo dev -lang en-XA   # accented and longer, to find untranslated and cut text
go tool nsgo dev -lang ar-XB   # right to left
```

`-lang` startet die App in einer Locale, auch in einer Pseudo-Locale ([Lokalisierung](localization.md)).

## Agents

```sh
go tool nsgo dev -automation
go tool nsgo dev -target android -automation
go tool nsgo dev -target web -automation
```

`-automation` lässt einen KI-Agent oder einen End-to-End-Runner die laufende App wie ein Mensch bedienen – bislang unter macOS, Windows, Linux (`-target native`), Android (`-target android`), im Web (`-target web`), bei Mini-Programmen in den Simulatoren ihrer Entwicklerwerkzeuge (`-target miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay` oder `miniprogram_douyin`) sowie auf Apple-Geräten und Simulatoren (`-target ios`, `tvos`, `watchos`, `ios_simulator`, `tvos_simulator` oder `watchos_simulator`): Fenster auflisten, anzeigen, was eines darstellt, drücken, tippen, Tasten senden, scrollen, ziehen, darüberfahren, Menüpunkte wählen, Alerts beantworten, auf Erscheinendes warten und Screenshots aufnehmen. Agents greifen darauf über MCP mittels `go tool nsgo mcp` zu, das den für das Verzeichnis laufenden `dev -automation` nutzt oder einen mit denselben Flags (wie `-target android`) startet und beendet, sobald der Client geht:

```sh
claude mcp add nsgo -- go tool nsgo mcp
```

Andere Clients erhalten denselben Befehl – `go` mit den Argumenten `tool nsgo mcp`, ausgeführt im App-Verzeichnis. Views werden mit den Selektoren aus [Tests](testing.md#ein-test) angesprochen oder über die `view:`-Referenzen, die ein Snapshot ausgibt und die ein Speichern durch den Neustart der App ungültig macht.

Der Agent handelt auf den echten Daten der App, so wie `dev` sie ausführt. Nur `dev -automation`-Builds enthalten diese Schnittstelle, und nur Werkzeuge auf diesem Rechner mit dem Token in `.nsgo/automation.yaml`, das ausschließlich Sie lesen können, erreichen sie.

### App-definierte MCP-Tools

Rufen Sie `automation.Register` aus einer Go-App oder Bibliothek auf der Engine-Goroutine auf. Diese Funktion gibt Agents beispielsweise die ID der aktuell angezeigten Szene:

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

Der Name muss im Graphen der App eindeutig sein und sich von den eingebauten Namen unterscheiden. `nsgo mcp` listet das Tool auf, bis der Scope bereinigt wird. Der Handler läuft auf der Engine-Goroutine und kann daher Signals lesen oder setzen. Er erhält ein JSON-Objekt, prüft die benötigten Argumente und gibt einen JSON-kodierbaren Wert oder einen Fehler zurück.

Erfassen Sie in einem Test den Scope beim Start der App, rufen Sie `automation.Call(cx, ctx, name, args)` innerhalb von `app.Post` auf und verwenden Sie anschließend `app.Settle()`, bevor Sie das Ergebnis oder den View prüfen. Die Registrierung ist in `dev -automation` und in Tests aktiv und in Release-Builds ein No-Op.

## Flags jedes Builds

Jeder Build einer App – Pakete eingeschlossen – akzeptiert beim Start eigene Flags von NSGo, die `nsgo.yaml` und den Code der App überschreiben und niemals die eigenen Flags der App erreichen:

| Flag | Wirkung |
| --- | --- |
| `--nsgo-fps` | Zeigt den Performance-Monitor unter macOS, iOS, Windows, Linux, Android und im Web |
| `--nsgo-lang=zh-Hans` | Startet die App in einer Locale, auch in einer Pseudo-Locale |
| `--nsgo-appearance=dark` | Startet die App im Modus `light`, `dark` oder gemäß `system` |
| `--nsgo-os=ios` | Führt einen Web-Build als `macos`, `windows`, `linux`, `ios` oder `android` aus; `web` steht für sich selbst |

```sh
./Hello --nsgo-fps                                   # Windows and Linux
open Hello.app --args --nsgo-fps                     # macOS
xcrun simctl launch booted dev.example.hello --nsgo-fps
adb shell am start -n dev.example.hello/dev.oneo.nsgo.NsgoActivity --ez nsgo-fps true --es nsgo-lang de
```

Im Web stehen sie im Query-String der Seite: `?nsgo-fps&nsgo-lang=de`.

`--nsgo-os` zeigt eine Vorschau des Layouts einer anderen Plattform im Browser: Die App liest dieses Betriebssystem und dessen Idiom – ein Smartphone für iOS und Android und sonst einen Desktop –, und `Env.Simulated` meldet true. Der Browser besitzt keines der nativen Teile dieser Plattform: `NavHeader` und die gezeichnete Tab-Leiste treten an die Stelle der Systemelemente, es gibt kein Anwendungsmenü, kein natives Popup-Menü (`desktop.PopupMenu`) und keine Fensterfunktionen, und `mobile.IsAvailable` sowie `desktop.IsAvailable` sind false. Gezeichnete Menüs wie `ui.ContextMenu` und `ui.Menu` funktionieren weiterhin. Tastenkürzel folgen der Tastatur der Besucherin oder des Besuchers. Andere Builds ignorieren das Flag und protokollieren eine Warnung.
