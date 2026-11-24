---
title: Einbetten
description: Eine NSGo-App in eine eigene iOS-, Android-, Desktop- oder Web-App einbinden.
group: Toolchain
order: 23
---

## Eine einbettbare App bauen

Eine einbettbare App ist ein ganz normales App-Paket mit einem `Root` und einem `main`, das mit `-embed` für die Zielplattform des Hosts gebaut wird. Jeder View führt die App einmal aus und ist eine eigene Engine, sodass ein Host beliebig viele Views anzeigen kann – jeder mit eigenem Zustand.

```sh
go tool nsgo build -embed -target ios examples/embedded
go tool nsgo build -embed -target android_arm64 examples/embedded
go tool nsgo build -target web examples/embedded
```

Der iOS-Build schreibt `<Name>.xcframework` und `<Name>Resources`; der Android-Build schreibt `<Name>.aar`; der Web-Build schreibt ein Verzeichnis mit `nsgo.json` und dem Loader.

## iOS

Wenn die App `base/terminal` importiert, schreibt der Build außerdem `GhosttyVT.xcframework` neben das App-Framework. Fügen Sie diesen Begleiter dem Host-Target mit **Embed & Sign** hinzu – sowohl für statische als auch für `-dynamic`-App-Frameworks. Es enthält Bibliotheken für Geräte, Intel- und Apple-Silicon-Simulatoren sowie Mac Catalyst; behalten Sie seinen Namen `GhosttyVT.framework` bei.

Fügen Sie das Framework dem App-Target als statische Bibliothek hinzu (nicht einbetten) und den Ressourcenordner als Ordnerreferenz (Folder Reference), und importieren Sie dann `<Name>`. Rufen Sie `NSGoStart()` einmal beim Start der App auf, noch vor dem ersten View. Der Code der statischen Bibliothek gehört zur Host-App, daher deklariert das Privacy-Manifest des Hosts die von NSGo verwendeten Required-Reason-APIs: User Defaults (`CA92.1`), Dateizeitstempel (`C617.1`) und System-Bootzeit (`35F9.1`); ein `-dynamic`-Framework trägt sie in seiner eigenen `PrivacyInfo.xcprivacy`.

```swift
import Embedded
import UIKit

@main
final class AppDelegate: UIResponder, UIApplicationDelegate {
	func application(_ app: UIApplication, didFinishLaunchingWithOptions: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
		NSGoStart()
		return true
	}
}

let view = NSGoView(frame: .zero)
view.params = ["user": "Ada"]              // cx.HostParams()
view.dataDirectory = documents.path        // where storage lives
view.localeIdentifier = "de-CH"            // in place of the system's
view.setFontScale(1.3)
view.overrideUserInterfaceStyle = .dark
view.tokens = ["accent": "#cf222e"]        // over the app's theme
view.handle("time") { _, reply in reply(Date().description, nil) }
view.send("message", ["text": "Hello"])    // cx.OnHostEvent("message", …)
view.onOpenURL = { url in router.open(url) }
view.onError = { message in log(message) }
view.register(component: "host.badge") { context in Badge(context) }
```

`NSGoView` startet die App, sobald sich der View in einem Fenster befindet, und beendet sie, wenn er freigegeben wird oder `stop()` erhält. Setzen Sie die Startwerte, bevor er sich in einem Fenster befindet; der Rest kann sich jederzeit ändern. Handler, Komponenten und Callbacks laufen auf dem Main-Thread, und eine Antwort darf später von jedem beliebigen Thread kommen. Ein View-Controller kann `preferredStatusBarStyle`, `prefersStatusBarHidden` und `prefersHomeIndicatorAutoHidden` des Views weiterreichen, um dessen Systemleisten-Einstellungen zu folgen.
Der Host kann zudem `preferredScreenEdgesDeferringSystemGestures` des Views von seinem Controller zurückgeben, um `mobile.DeferSystemGestures` weiterzuleiten.

## Android

Für eine arm64-App, die `base/terminal` importiert, enthält das AAR `jni/arm64-v8a/libghostty-vt.so` neben `libnsgo.so`; der Host behält die Extraktion nativer Bibliotheken aktiviert. Der Android-Vorab-Build und Terminal-Loader erfordern [#1452](https://github.com/nsgo-dev/nsgo/issues/1452); x86_64-Terminalbibliotheken sind noch nicht verfügbar.

Fügen Sie das AAR dem App-Modul hinzu und verpacken Sie native Bibliotheken extrahiert, damit die Laufzeit sie findet:

```kotlin
implementation(files("libs/Embedded.aar"))

android { packaging { jniLibs { useLegacyPackaging = true } } }
```

Erstellen Sie `dev.oneo.nsgo.NsgoView`-Instanzen im Code oder in einem Layout. Jede startet die App in einem eigenen Prozess, sobald sie erstmals eine Größe erhält, und `stop()` beendet sie. Leiten Sie die Activity-Ergebnisse an jeden View weiter und behandeln Sie Konfigurationsänderungen in der Activity, damit die Apps nicht neu gestartet werden.

```java
NsgoView app = new NsgoView(context);
app.setParams(Map.of("user", "Ada"));
app.setStorageDirectory(dir);
app.setLocale(Locale.forLanguageTag("de-CH"));
app.setFontScale(1.3f);
app.setAppearance(NsgoView.APPEARANCE_DARK);
app.setTokens(Map.of("accent", "#cf222e"), null);
app.handle("time", (data, reply) -> reply.resolve(now()));
app.handle("tapped", (data, reply) -> status.setText("tapped " + ((Map<?, ?>) data).get("count")));
app.setOpenUrlHandler(url -> router.open(url));
app.setErrorListener(message -> log(message));
app.registerComponent("host.badge", Badge::new);
```

```java
@Override
protected void onActivityResult(int request, int result, Intent data) {
	super.onActivityResult(request, result, data);
	for (NsgoView app : apps) app.activityResult(request, result, data);
}
```

Um den nativen Baum auszuhängen, während die Go-App weiterläuft, halten Sie eine `NsgoEngine` im Host fest und hängen ihren View explizit ein:

```java
NsgoEngine engine = new NsgoEngine(context.getApplicationContext());
app.attachToEngine(engine);
// Before taking the view off screen:
app.detachFromEngine();
// Reuse the same view later:
app.attachToEngine(engine);
// When the host ends the app:
engine.destroy();
```

Das Aushängen gibt native Kind-Views frei; das Wiedereinhängen stellt aktuelle Signals, Eingaben, Fokus, Scrollposition und Zeichnungen wieder her, ohne Komponenten erneut auszuführen. Webseiten und Player werden aus ihrer App-Konfiguration heraus neu erstellt. `stop()` hängt lediglich eine explizit übergebene Engine aus. Jede Engine behält weiterhin ihren ersten View und ihre Activity bei und lehnt einen weiteren View ab – behandeln Sie Konfigurationsänderungen also in dieser Activity; das Ersetzen oder Anzeigen unabhängiger Views in einem Prozess wird noch nicht unterstützt.

Für Google-Play-Update-Prüfungen fügen Sie `com.google.android.play:app-update:2.1.0` zu den Gradle-Abhängigkeiten des Android-Hosts hinzu. Bewahren Sie die APIs AppUpdateManager, AppUpdateInfo, AppUpdateOptions und Tasks, die NSGo per Namen lädt, mithilfe der [Runtime-Shrink-Regeln](https://github.com/nsgo-dev/nsgo/blob/main/internal/platform/android/runtime/play-review.pro). Die obige Weiterleitung der Activity-Ergebnisse liefert auch Update-Abbrüche aus.

Für Android-Authentifizierungssitzungen in einem eingebetteten Host deklarieren Sie das private Callback-Schema in der hostenden Activity und leiten die Links, die diese Activity annimmt, in `onNewIntent` (sowie einem frischen `onCreate`) an `view.openLink(uri.toString())` weiter. Eine passende aktive Sitzung konsumiert ihren Callback; andere Links erreichen `OnOpenLinks`. Die eigenständige `NsgoActivity` leitet sie selbst weiter.

## Desktop

Ein Go-Host betreibt die Ereignisschleife der Plattform und setzt das `Handle` jedes Views in sein Fenster. `nsgo.NewView` startet die App, sobald das Handle in einem Fenster liegt, sodass zuvor gesetzte Einstellungen bereits im ersten Frame wirksam sind.

```go
channel := core.NewHost()
channel.SetParams(map[string]any{"user": "Ada"})
channel.Handle("time", func(c *core.HostCall) { c.Resolve(time.Now().Format("15:04:05")) })
channel.HandleOpenURL(func(url string, answer func(bool)) {
	answer(strings.HasPrefix(url, "https://example.com/orders/"))
})

view := nsgo.NewView(app.Root, nsgo.HostChannel(channel), nsgo.DataDir(dataDir))
defer view.Close()

view.SetTokens(core.Tokens{Values: map[string]string{"accent": "#cf222e"}})
view.SetLocale("de-CH")
view.SetFontScale(1.3)
view.SetAppearance(core.AppearanceDark)
handle := view.Handle() // an NSView, GtkWidget or child window
```

`view.Host()` sendet Ereignisse an die App und empfängt ihre Aufrufe, und `nsgo.Component` registriert eine native Komponente für die Plattform des Hosts.

## Web

Der Web-Build ist eine eigene Seite, aber eine Seite kann ihn in ein Element einhängen und sein Host sein. `mount(element, url, options)` gibt einen Player zurück, und `nsgo.json` benennt den Loader, das Wasm und die Ressourcen relativ zu sich selbst.

```html
<div id="app"></div>
<script type="module">
const url = new URL("./Embedded-0.1.0-web/nsgo.json", location.href);
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url));

const player = await mount(document.getElementById("app"), url.href, {
	config,
	params: { user: "Ada" },
	host: {
		call(name) {
			if (name === "time") return new Date().toLocaleTimeString();
			throw new Error(`no host function ${name}`);
		},
		post(name, data) { console.log(name, data); },
		openURL(url) { return url.startsWith("https://example.com/orders/"); },
	},
});
player.hostEvent("message", { text: "Hello from the page" });
</script>
```

Ohne Option `host` importiert der Loader das in `nsgo.yaml` unter `web.host` genannte Modul und wartet auf dessen `createHost(player)`, sofern das Modul es exportiert. Eine App in einem Element oder einem Frame überlässt Verlauf und Titel des Browsers der Seite.

`appearance` (`"light"`, `"dark"` oder `"system"`), `locale` und `fontScale` treten an die Stelle der Browser-Einstellungen, genau wie die Einstellungen eines nativen Hosts, und `setAppearance`, `setLocale` sowie `setFontScale` des Players ändern sie später. `module` – ein kompiliertes `WebAssembly.Module` oder ein Promise darauf – erlaubt es einer Seite, die die App mehrfach einhängt, sie nur einmal herunterzuladen und zu kompilieren.

Der Loader zeigt vor dem Start der App ein Element `progress.nsgo-load-progress` an. Gestalten Sie es mit CSS, übergeben Sie `onProgress` in den Mount-Optionen oder exportieren Sie `onProgress` aus `web.host` für die eigene Seite der App. Der Callback ersetzt den Balken und erhält `{ loaded, total }` in dekodierten Bytes, wobei `total` auf `null` steht, wenn die Größe unbekannt ist. Der Loader ruft ihn am Ende des Starts mit `null` auf, damit er seine Oberfläche ausblenden kann. Ein kompiliertes `module` überspringt den Download-Fortschritt.

```yaml
web:
  host: host.js
```

## Der Host-Kanal

`core.Host` ist der einzige Vertrag, den jeder Host implementiert: Startparameter, Ereignisse, die der Host sendet, sowie Funktionen des Hosts, die die App aufruft. Werte passieren die Grenze wie JSON, sodass Maps zu `map[string]any` und Zahlen zu `float64` werden und derselbe Code in Swift, Java, Go und JavaScript funktioniert.

- `HostParams` liest die Startparameter einmalig beim Start.
- `cx.OnHostEvent` lauscht auf `Host.Send` und `hostEvent` der Seite.
- `cx.CallHost` ruft einen benannten Handler auf und löst mit seiner Antwort auf; `cx.PostHost` ruft einen ohne Antwort auf.
- `Host.HandleOpenURL` sieht `cx.OpenURL` noch vor der Plattform, und seine Antwort entscheidet, ob die Plattform die URL ebenfalls öffnet.
- `Host.SetParams`, `Send`, `Handle` und `HandleDefault` sind von jeder Goroutine aus sicher aufrufbar.

Unter iOS setzt der Host `params`, installiert Handler mit `handle`, sendet Ereignisse mit `send` und beantwortet `onOpenURL`. Unter Android tut er dasselbe mit `setParams`, `handle`, `send` und `setOpenUrlHandler`. Auf dem Desktop nutzt er die Methoden von `core.Host`, und im Web antwortet das `host`-Objekt der Seite.

## Native Komponenten

`core.Native` zeigt einen vom Host erstellten View an: Seine `Props` erreichen `SetProp` der Host-Komponente, was die Komponente ausgibt, erreicht `OnEvent`, und ein Prop, dessen Wert ein animierter Node ist, bewegt sich im Frame-Takt ohne Arbeit für die Engine.

```go
core.Native{
	Component: "host.badge",
	PropsFunc: func() map[string]any { return map[string]any{"count": count.Get()} },
	OnEvent:   func(name string, data any) { if name == "tap" { count.Set(0) } },
	Style:     core.Style{MinWidth: core.Pt(120), MinHeight: core.Pt(28)},
}
```

Registrieren Sie die Komponente im Host: `app.register(component:)` samt einem `NSGoComponent` unter iOS, `registerComponent` samt einem `NsgoComponent` unter Android, `nsgo.Component` auf dem Desktop. Das Web kennt keine nativen Komponenten; ein unbekannter Name zeigt dort ein leeres Kästchen.

## Einstellungen

`tokens` überschreibt das Theme der App anhand von Rollennamen (`accent`, `inset`, `raised`), in beiden Erscheinungsbildern oder in einem einzelnen; `cx.Palette()` sieht diese Werte. Der Host setzt außerdem Locale, Textgröße, Erscheinungsbild und Datenverzeichnis und erhält die Fehler der App. `nsgo build -embed` schreibt weder Fenster noch Menüs oder Tray-Icons: Diese gehören dem Host.

Eine interne Panic auf der Engine-Goroutine eines Views stoppt diese Engine, zerstört ihre angehängten Oberflächen und meldet Panic samt Stack an den Error-Listener des Hosts (`nsgo.OnError` in Go). Andere Engines laufen weiter; erstellen Sie einen neuen View, um die fehlgeschlagene App neu zu starten. Gewöhnliche Panics in App-Callbacks gehen weiterhin zuerst an das nächste `core.Catch`.

`examples/embed-ios`, `examples/embed-android`, `examples/embed-desktop` und `examples/embed-web` zeigen dieselbe App mit jeder Art von Host.

## Nächste Schritte

- [Plattformdienste](services.md): Was die App von der Plattform anfordert.
- [Bauen und Ausliefern](building.md): Welche Pakete `-embed` erzeugt und wie man sie signiert.
- [Beispiel: Eine Todo-App](tutorials/todo.md): Die App, die ein Host einbettet.
