---
title: TSX-Apps
description: Eine App in TSX mit Tailwind-Klassen schreiben, vorab kompiliert und ohne Node.
group: Kernkonzepte
order: 63
---

## Eine TSX-App

Eine TSX-App ist ein Go-Paket, das seine Quelltexte einbettet und seinen Einstiegspunkt ausführt:

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

`go tool nsgo init -template tsx` erzeugt eine solche App, und `examples/tsx` ist ein weiteres Beispiel. `nsgo dev` und `nsgo build` kompilieren das TSX vorab, sodass Pakete keinen Compiler mitliefern; `go run` und `go test` kompilieren es beim Start der App. In keinem der beiden Fälle wird Node benötigt.

## Komponenten und Module

TSX importiert die Grundbausteine aus `nsgo`, die Komponenten von `base` aus `nsgo/base`, Gamecontroller aus `nsgo/gamepad` (in signierten Paketen unter der Capability `gamepad`), Sounds aus `nsgo/audio` (unter `audio`) und die von `ui` aus `nsgo/ui`, sobald die App `tsx/ui` importiert; `examples/gallery-tsx` zeigt sie.

Für Mermaid-Diagramme importieren Sie `github.com/nsgo-dev/nsgo/tsx/diagram` wegen seines Effekts im Go-Host, zusammen mit `tsx/textview` oder `tsx/ui`. Dadurch wird `Diagram` zu `nsgo/base` hinzugefügt, ohne den Diagramm-Parser in Apps einzubinden, die ihn nicht verwenden. Es nimmt `source`, `fontSize`, ein partielles `theme`, `accessibilityLabel` und die üblichen Styling-Props entgegen. Verwenden Sie es direkt oder stellen Sie einen TextView-Renderer bereit:

```tsx
import { Diagram, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderDiagram={(code) => <Diagram source={code.code} errorClass="font-mono" />} />
```

Dieselbe Prop `renderDiagram` funktioniert auch auf der TextView von `nsgo/ui`. Ein Codeblock, der kein Mermaid ist, behält seinen üblichen Renderer; ein ungültiges Diagramm zeigt seinen Quelltext.

Für gesetzte Mathematik importieren Sie `github.com/nsgo-dev/nsgo/tsx/formula` wegen seines Effekts im Go-Host, zusammen mit `tsx/textview` oder `tsx/ui`. Dadurch wird `Formula` zu `nsgo/base` hinzugefügt; Apps, die das Paket weglassen, halten die Mathe-Engine und die Schrift aus ihrem Binary heraus. Es nimmt `source`, `dialect` (standardmäßig `tex`, alternativ `typst`), `display`, `accessibilityLabel` und Styling-Props entgegen. Schriftgröße und Farbe folgen dem umgebenden Text. Verwenden Sie es direkt oder stellen Sie einen TextView-Renderer bereit:

```tsx
import { Formula, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderMath={(math) => <Formula source={math.tex} display={math.display} />} />
```

Dasselbe `renderMath` funktioniert auch auf der TextView von `nsgo/ui`. Formeln behalten beim Kopieren ihr TeX. Der Go-`Renderer` bleibt eine Host-Konfiguration.

`UrlScheme.url()` liest die verfolgte Anwendungsadresse; `UrlScheme.setUrl(url, { history: "push" })` fügt einen Adresseintrag hinzu oder ersetzt ihn, wenn die Optionen weggelassen werden. `UrlScheme.on("change", fn)` beobachtet Zurück, Vorwärts und Hash-Änderungen im Browser und gibt eine Funktion zum Abmelden zurück. Siehe [Anwendungsadressen](addresses.md) für Routing, Same-Origin-Validierung und das Hosting von Deep Links.

## Go aus TSX

Registrieren Sie Go-Code, damit TSX ihn importieren kann:

```go
tsx.Register("app/native", map[string]core.Node{"Badge": Badge{}})
tsx.RegisterFunctions("app/native", map[string]any{
	"greet": func(name string) string { return "Hello, " + name },
})
```

`import { Badge, greet } from "app/native"` übernimmt dann die Felder von `Badge` als Props und ruft `greet` auf. Funktionen, die einen `context.Context` entgegennehmen, werden zu Promises, und `tsx.RegisterClass` macht Go-Typen zu Klassen, die TSX mit `new` erzeugt.

Eine optionale Prop, die als `undefined` weitergereicht wird, verwendet ihren Standardwert. Wenn Sie eine Go-Komponente mit einem Getter registrieren, dessen Standardwert vom Nullwert seines Typs abweicht, fügen Sie eine Option wie `tsx.Default("Max", float64(100))` hinzu; derselbe Standardwert gilt, solange eine reaktive Prop undefined zurückgibt, ohne die Komponente zu ersetzen.

Datenfelder mit dokumentierten Go-Standardwerten können ebenfalls weggelassen werden: `width` und `height` eines `Flow`-Knotens messen bei Fehlen seinen JSX-Inhalt, ein weggelassenes `width` einer Kante verwendet die Standard-Strichstärke, und ein weggelassenes `pace` einer Kontingentzeile lässt deren Tempomarkierung ausgeblendet.

Beim Registrieren von Go-Komponenten und -Klassen werden ihre exportierten Felder und Methoden zur Laufzeit der App per Reflection ermittelt. Diese Abfrage hindert den Go-Linker daran, exportierte Methoden zu verwerfen, die niemand aufruft – in der gesamten App und nicht nur in dem, was TSX registriert –, sodass das Einbinden von `tsx` einem gestrippten Desktop-Binary etwa 20 MB hinzufügt, umso mehr, je mehr Pakete die App einbindet.

## Signierte Pakete

Eine App kann heruntergeladenes TSX ausführen, etwa neu ausgelieferte eigene Bildschirme oder Plugins, sobald sie das Paket verifiziert hat. Deklarieren Sie in seiner `nsgo.yaml`, worauf der Code des Pakets zugreifen darf; alles andere wird ihm verweigert:

```yaml
capabilities:
  network:
    hosts: [api.example.com, "*.cdn.example.com"]
  open_url: true # or { schemes: [https] }
  host: [search, "player.*"] # the app's handlers it may call
  # webview: { hosts: [...], input: true, media: [microphone, camera] }, clipboard, picker, speech, text_to_speech, status_bar, notifications, biometrics, sign_in_with_apple, purchases, secrets, audio, gamepad, auth_session
```

Bewahren Sie einen Root-Schlüssel offline auf und zertifizieren Sie damit Release-Schlüssel; signieren Sie dann die App in diesem Verzeichnis. `go tool nsgo keygen` gibt ein Schlüsselpaar aus und schreibt keine Datei; führen Sie es einmal für den Root und einmal für jeden Release-Schlüssel aus. Öffentliche Schlüssel gehören in Flags und den Code der App; geheime Schlüssel bleiben aus dem Repository heraus und erreichen das Tool nur über `NSGO_ROOT_KEY` und `NSGO_RELEASE_KEY`:

```sh
go tool nsgo keygen # once for the root, once for the release key
NSGO_ROOT_KEY=<root secret> go tool nsgo certify -key <release public> -apps 'com.example.*'
NSGO_RELEASE_KEY=<release secret> go tool nsgo publish -cert release.cert plugin
go tool nsgo verify -root <root public> plugin/dist/Plugin-1.0.0.nsgopkg
```

Die App vertraut dem öffentlichen Schlüssel des Roots, der in ihrem Code steht, verifiziert das Paket außerhalb der Goroutine der Engine und zeigt es in `tsx.View` an, in einer Sandbox neben ihren eigenen Views. Das Paket erreicht die App nur über den Host-Kanal, den die View ihm gibt, und behält seinen eigenen Teil des Speichers und der Secrets der App:

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

`Sandbox.Grant` gewährt ihm weniger, als es deklariert, `Sandbox.Limits` andere Zeitbudgets und Kontingente, und `Sandbox.Modules` Go-Funktionen und -Komponenten über die von NSGo hinaus. Seine Tastenkürzel, Zurück-Handler und Navigationsstapel wirken nur, solange der Tastaturfokus in seiner View liegt, und es verschiebt den Fokus selbst (`focus()`, `autoFocus`, `Keyboard.dismiss`, `setAccessibilityFocus`) nur, solange der Fokus bereits dort liegt oder als Antwort auf einen Druck des Nutzers darin. Es wird in einer eigenen View angezeigt, die den Platz ausfüllt, den ihr Elternelement ihr gibt, nicht mehr, und die es beschneidet, Menüs, Popover und Toasts eingeschlossen; setzen Sie es in einer Scroll-View in eine View, damit es die Höhe seines Inhalts annehmen kann. Ein Dialog, ein Alert, ein Kontextmenü oder eine Ansage des Pakets wird nur auf dieselbe Weise angezeigt, und Zurück schließt einen seiner Dialoge, selbst wenn das Paket ihn nicht entfernt. `View.Export` benennt eine Komponente, die das Einstiegsmodul des Pakets exportiert und die anstelle der vom Einstiegspunkt gemounteten Root angezeigt wird – dessen `mount` rendert dann weder noch führt es seine Komponente aus –, und erhält `View.Props`, eine JSON-kodierbare Map, die für jede View kopiert wird, sodass ein Paket mehrere Views bereitstellt, jede mit eigenem Zustand, unter denselben Capabilities und Limits; ein fehlender Export löst eine Panic mit `tsx.ErrNoViewExport` aus, und einer, der keine Funktion ist, eine mit `tsx.ErrViewExportNotComponent`. Ein Paket, dessen Einstiegspunkt Funktionen exportiert, ist ein Plugin aus Funktionen: `tsx.Load(cx, p, tsx.Sandbox{})` führt es unsichtbar und synchron auf der Goroutine des Aufrufers aus, unter einem `cx`, der zu einem View-Baum gehört (ohne einen solchen gibt es `tsx.ErrNoTree` zurück), oder gibt zurück, was sein Einstiegsmodul beim Laden geworfen hat, und `Plugin.Call` ruft sie auf. `go tool nsgo dev -limits` führt eine App so aus, wie es eine solche View tun würde, und `go tool nsgo revoke` signiert eine Liste widerrufener Release-Schlüssel und Pakete für `Trust.UpdateRevocations`. `examples/plugins` bittet den Nutzer, die Capabilities eines Pakets zu gewähren, installiert es und führt es aus. `Limits.Memory`, standardmäßig 64 MiB, budgetiert erfassten nativen Speicher für Arrays, Strings, Buffer und Typed Arrays. Dazu gehören Kopien durch `concat`, `flat`, `slice`, Array-Spread, String-Verkettung, Templates, `replace`, `replaceAll`, `TextEncoder` und `Blob`. `memory=<bytes>` in `-limits` ändert das Budget, und `memory=0` schaltet es ab. Die Prüfungen erfolgen vor der Allokation. Ein Paket, das das Budget überschreitet, stoppt mit `ERR_NSH_QUOTA_EXCEEDED` und der Ressource `memory`, nachdem Go auf einsammelbaren Speicher geprüft hat. Gewöhnliche Objekte, Closures, Stacks, kleine Allokationen sowie nicht erfasste Built-ins und Host-APIs können ebenfalls Speicher allozieren. Der Browser verwaltet seinen eigenen Heap. Eine App, die Pakete selbst mit `pkg.Build` signiert, oder mit `go tool nsgo publish -extra <file>` mit den Bytes dieser Datei, kann eigene Metadaten in `pkg.Contents.Extra` ablegen, etwa was ein Plugin ihren Menüs hinzufügt: Sie werden mit dem Paket signiert, und `Verify` gibt sie unverändert als `p.Extra` zurück, damit die App sie parsen kann, sodass die App sie liest, ohne den Code des Pakets auszuführen.

Eine App kann ihre eigenen JSON-Dokumente, etwa einen Katalog ihrer Plugins, mit `pkg.SignDocument` signieren und mit `Signed.Open` verifizieren, mit den Schlüsseln, die `nsgo keygen` ausgibt. Geben Sie jeder Art von Dokument einen eigenen `app:`-Kontext mit einer Version, etwa `app:catalog-v1`: Ein Dokument geht nie als eine andere Art durch, noch als Paket, Zertifikat oder Widerrufsliste. `Open` vertraut genau den öffentlichen Schlüsseln, die es erhält, ohne Zertifikat oder Widerrufsliste, und lehnt Felder im Body ab, die dem Typ fehlen, in den es dekodiert. Der Umschlag enthält weder Zeit noch Sequenznummer, sodass ein älteres Dokument ebenso verifiziert wird wie ein neueres: Legen Sie eine Version oder ein Ablaufdatum in den Body und prüfen Sie es.
In einer nativen Sandbox werfen `JSON.stringify`, `JSON.parse` mit einem Reviver, `Array.prototype.join`, `toString`, `toLocaleString` und `flat` und damit auch `String()` eines Arrays einen abfangbaren `RangeError`, sobald sie mehr als 1.000 Ebenen tief gehen, wobei ineinander verschachtelte Aufrufe zählen, so wie das `toString` eines Arrays und das von ihm aufgerufene `join` jeweils zählen; tiefere Daten würden den Aufruf hängen lassen oder die App beenden. Das Web verwendet die Built-ins des Browsers.

Auf nativen Zielplattformen beträgt `DefaultLimits.Stack` 1.000 Frames. Setzen Sie `Limits.Stack` oder `-limits stack=<frames>`, um eine andere Tiefe bis zu 10.000 Frames zu wählen. Go-Werte kleiner oder gleich null sowie Eingaben über der Obergrenze verwenden 10.000. Das Web verwendet das Stack-Limit des Browsers.

Native Funktionen, die einander aufrufen, zählen wie JS-Aufrufe gegen `Limits.Stack`: Ein Getter, `toString` oder `valueOf`, das ein Built-in ist, ein regulärer Ausdruck, dessen `exec` eine andere seiner Methoden ist, `Reflect.apply` und `Reflect.construct` ineinander sowie Ketten gebundener Funktionen oder Proxys werfen jenseits davon einen abfangbaren `RangeError`, anstatt die App zu beenden, und ein Wert eines Plugins, der tiefer verschachtelt ist, lässt den Aufruf, der ihn nach Go konvertiert, mit einem `RangeError` fehlschlagen. Dies stammt aus dem Fork von goja, den NSGo voraussetzt, `github.com/nsgo-dev/goja`, den Ihre App mit NSGo erhält, ohne dass ihr `go.mod` geändert werden muss.

Auf nativen Zielplattformen begrenzt `Limits.Call` jeden JavaScript-Abschnitt, einschließlich Timer-Callbacks und Promise-Fortsetzungen nach `await`. Wenn ein Abschnitt abgebrochen wird, meldet ein ausstehender `Plugin.Call` einmal `tsx.ErrBudget`, stoppt seine verbleibenden Timer und ignoriert verspätete Host-Antworten. Unabhängige Aufrufe laufen weiter, einschließlich Promise-Reaktionen, die im unterbrochenen Abschnitt eingereiht wurden. Ein Aufruf, der das Promise eines anderen erfüllt, meldet weiterhin sein eigenes Ergebnis oder seinen eigenen Fehler. Reguläre Ausdrücke mit Backtracking verwenden das kürzere Lade- oder Aufrufbudget als Timeout für ihren Abgleich; ein nicht behandelter Timeout wird ebenfalls zu `tsx.ErrBudget`.

`Limits.CallTotal` fügt für jeden Plugin-Aufruf und jedes Ereignis einer Sandbox-View eine Wall-Clock-Frist hinzu. Sie umfasst Wartezeit und gilt für seine Timer, Intervalle, Frames, Promise-Fortsetzungen und Host-Antworten, auch nachdem der Aufruf abgeschlossen ist. Bei Erreichen der Frist wird die ausstehende Arbeit verworfen, und ein noch wartender Aufruf schlägt mit `tsx.ErrBudget` fehl. Die Arbeit des Einstiegsmoduls unterliegt `Limits.Load`, wobei `CallTotal` verwendet wird, wenn `Load` null ist. Eigenständige Host-Ereignisse verwenden `CallTotal` ab der Zustellung. Verwenden Sie `Plugin.CallWithBudget`, um ein Handle auf die Frist eines wartenden Aufrufs zu erhalten. Rufen Sie im `Scope.Post` eines Hosts vor dieser Frist `budget.Extend(duration)` auf, um nur diesem Aufruf mehr Zeit zu geben.

`Plugin.Interrupt` stoppt die Laufzeitumgebung von jeder Goroutine aus, verwirft ihre ausstehenden Timer und lässt aktuelle und spätere Aufrufe fehlschlagen. `Plugin.Stopped()` und `Plugin.Err()` können ebenfalls von jeder Goroutine aus gelesen werden. `Err()` ist nil, solange die Laufzeitumgebung läuft, und gibt nach dem Stoppen ihren ersten Stoppgrund zurück. Das Schließen des Plugins oder das Freigeben seines Besitzers führt normalerweise seine Aufräumarbeiten aus und stoppt es mit `tsx.ErrPluginClosed`. Setzen Sie `Sandbox.OnStop` beim Aufruf von `tsx.Load`, um diesen Grund einmal auf der Goroutine der Engine zu empfangen, nachdem wartende Aufrufe fehlgeschlagen sind.

Setzen Sie `Sandbox.OnError` beim Laden eines Funktions-Plugins, um nicht abgefangene Fehler aus Timern, Animation-Frames, `queueMicrotask` und `reportError` außerhalb eines wartenden `Plugin.Call` zu empfangen. Der Handler jedes Plugins läuft auf der Goroutine der Engine, sodass ein Host Fehler zuordnen und `Plugin.Interrupt` aufrufen kann, wenn sein Circuit Breaker auslöst. Native Abbrüche wegen des Abschnittsbudgets melden ebenfalls `tsx.ErrBudget`, auch für einen Promise-Job, dessen Aufruf bereits abgeschlossen ist. Gewöhnliche Callback-Fehler lassen das Plugin weiterlaufen. Fehler beim Laden und bei Aufrufergebnissen werden weiterhin an ihre Aufrufer zurückgegeben, und Views verwenden weiterhin `core.Catch`; ohne Handler bleibt das bestehende Verhalten von Boundaries und App-Fehlerberichten erhalten.

Setzen Sie `Sandbox.OnViewRuntime`, um die `*tsx.ViewRuntime` jeder View auf der Goroutine der Engine zu empfangen, bevor ihr Einstiegsmodul ausgeführt wird. Rufen Sie `Interrupt` von einer beliebigen Goroutine aus auf, um ihre Arbeit zu stoppen und die zuletzt gerenderte View bestehen zu lassen. `Stopped` und `Err` melden ihren Endzustand, bei normaler Freigabe mit `tsx.ErrViewClosed`. `Sandbox.OnStop` meldet den Endgrund einer View ebenfalls einmal.

`Plugin.JSTime` und `ViewRuntime.JSTime` melden die verstrichene Zeit, die in den JavaScript-Einstiegen der nativen Laufzeitumgebung verbracht wurde, einschließlich des aktuellen Einstiegs. Wartezeit zwischen Einstiegen und Zeit in registrierten synchronen Go-Funktionen sind ausgenommen. `ViewRuntime.EngineTime` umfasst zusätzlich synchrone Go-Aufrufe sowie eigene Arbeit für Reaktivität, Mounten, Commit und Layout, einschließlich noch laufender Arbeit. Ausgenommen sind Wartezeit zwischen Engine-Einstiegen, Arbeit, die zum Host oder zu einer anderen View gehört, gemeinsame Commit-Verwaltung und natives Zeichnen. Beide Zähler können sicher von jeder Goroutine aus gelesen werden. JavaScript-Durchläufe des Browsers außerhalb von NSGo können weder gemessen noch unterbrochen werden. Dort melden beide Zähler „nicht verfügbar“, und `Interrupt` stoppt künftige NSGo-Callbacks, sobald der Host wieder die Kontrolle erhält.

Im Web kann ein Paket noch nicht von der Seite isoliert werden: Sein Code würde über seine Capabilities hinaus auf das eigene Netzwerk, die Zwischenablage und den Speicher der Seite zugreifen, einschließlich des Trust-Stores. Dort lehnen `tsx.View` und `tsx.Load` ein Paket mit `tsx.ErrUnisolated` ab, es sei denn, Sie setzen `Sandbox.WebUnisolated`, um es mit den Befugnissen der Seite auszuführen – was nur für Pakete geeignet ist, denen Sie die Befugnisse der App selbst anvertrauen würden.

## Editoren und Tests

Eine `tsconfig.json`, die `.nsgo/tsconfig.json` erweitert, stellt Editoren die Typen des TSX bereit, die `go tool nsgo dev` und `go tool nsgo check` schreiben. `*.test.tsx`-Dateien neben der App, geschrieben mit `nsgo/test`, laufen mit ihren Go-Tests ([Testen](testing.md)).
