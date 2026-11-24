---
title: Integrazione
description: Inserire un'app NSGo all'interno di una propria app iOS, Android, desktop o web.
group: Strumenti
order: 23
---

## Compilare un'app integrabile

Un'app integrabile è un normale pacchetto applicativo con un `Root` e un `main`, compilato con `-embed` per il target dell'host. Ogni vista esegue l'app una volta ed è un motore a sé stante, quindi un host può mostrare quante viste desidera, ciascuna con il proprio stato.

```sh
go tool nsgo build -embed -target ios examples/embedded
go tool nsgo build -embed -target android_arm64 examples/embedded
go tool nsgo build -target web examples/embedded
```

La build iOS produce `<Name>.xcframework` e `<Name>Resources`; la build Android produce `<Name>.aar`; la build web produce una directory con `nsgo.json` e il loader.

## iOS

Quando l'app importa `base/terminal`, la build genera anche `GhosttyVT.xcframework` accanto al framework dell'app. Aggiungi questo componente accessorio al target host con **Embed & Sign**, sia per i framework statici sia per quelli `-dynamic`. Contiene le librerie per dispositivo fisico, simulatori Intel e Apple Silicon e Mac Catalyst; mantieni il nome `GhosttyVT.framework`.

Aggiungi il framework al target dell'app come libreria statica (non incorporarla) e la cartella delle risorse come riferimento a cartella (folder reference), quindi `import <Name>`. Chiama `NSGoStart()` una volta all'avvio dell'app, prima della prima vista. Il codice della libreria statica appartiene all'app host, quindi il privacy manifest dell'host deve dichiarare le API con motivazione obbligatoria usate da NSGo: user defaults (`CA92.1`), timestamp dei file (`C617.1`) e tempo di avvio del sistema (`35F9.1`); un framework `-dynamic` le include nel proprio `PrivacyInfo.xcprivacy`.

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

`NSGoView` avvia l'app appena entra in una finestra e la arresta quando viene deallocata o riceve `stop()`. Imposta i valori di avvio prima che entri in una finestra; tutto il resto può cambiare in qualsiasi momento. Gestori, componenti e callback vengono eseguiti sul thread principale, e una risposta può arrivare in seguito da qualsiasi thread. Un view controller può inoltrare `preferredStatusBarStyle`, `prefersStatusBarHidden` e `prefersHomeIndicatorAutoHidden` della vista per seguirne le impostazioni delle barre di sistema.
L'host può inoltre restituire `preferredScreenEdgesDeferringSystemGestures` della vista dal proprio controller per inoltrare `mobile.DeferSystemGestures`.

## Android

Per un'app arm64 che importa `base/terminal`, l'AAR include `jni/arm64-v8a/libghostty-vt.so` accanto a `libnsgo.so`; l'host mantiene abilitata l'estrazione delle librerie native. Il prebuild Android e il loader del terminale richiedono [#1452](https://github.com/nsgo-dev/nsgo/issues/1452); le librerie del terminale per x86_64 non sono ancora disponibili.

Aggiungi l'AAR al modulo dell'app e impacchetta le librerie native estratte, così che il runtime le trovi:

```kotlin
implementation(files("libs/Embedded.aar"))

android { packaging { jniLibs { useLegacyPackaging = true } } }
```

Crea viste `dev.oneo.nsgo.NsgoView` da codice o in un layout. Ciascuna avvia l'app in un processo proprio appena riceve una dimensione, e `stop()` la termina. Inoltra i risultati dell'Activity a ogni vista e gestisci i cambi di configurazione nell'Activity in modo che le app non vengano riavviate.

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

Per scollegare l'albero nativo mantenendo viva l'app Go, conserva un `NsgoEngine` nell'host e collega esplicitamente la relativa vista:

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

Lo scollegamento rilascia i figli nativi; il ricollegamento ripristina i signal correnti, l'input, il focus, la posizione di scorrimento e i disegni senza rieseguire i componenti. Pagine web e player vengono ricreati a partire dalla configurazione dell'app. `stop()` si limita a scollegare un engine fornito esplicitamente. Ogni engine conserva comunque la propria prima vista e la propria Activity e rifiuta un'altra vista, quindi gestisci i cambi di configurazione in quella Activity; sostituirla o mostrare viste indipendenti in uno stesso processo non è ancora supportato.

Per le verifiche di aggiornamento di Google Play, aggiungi `com.google.android.play:app-update:2.1.0` alle dipendenze Gradle dell'host Android. Preserva le API AppUpdateManager, AppUpdateInfo, AppUpdateOptions e Tasks che NSGo carica per nome utilizzando le [regole di shrink del runtime](https://github.com/nsgo-dev/nsgo/blob/main/internal/platform/android/runtime/play-review.pro). L'inoltro dei risultati dell'Activity visto sopra consegna anche l'annullamento dell'aggiornamento.

Per le sessioni di autenticazione Android in un host integrato, dichiara lo schema di callback privato sull'Activity ospitante e inoltra i link accettati da tale Activity a `view.openLink(uri.toString())` in `onNewIntent` (e in una nuova `onCreate`). Una sessione attiva corrispondente consuma il proprio callback; gli altri link raggiungono `OnOpenLinks`. La `NsgoActivity` standalone li inoltra autonomamente.

## Desktop

Un host Go esegue il ciclo degli eventi della piattaforma e inserisce l'`Handle` di ciascuna vista nella propria finestra. `nsgo.NewView` avvia l'app appena l'handle si trova in una finestra, quindi le impostazioni applicate prima sono già presenti nel primo frame.

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

`view.Host()` invia eventi all'app e ne riceve le chiamate, mentre `nsgo.Component` registra un componente nativo per la piattaforma dell'host.

## Web

La build web è una pagina autonoma, ma una pagina può montarla dentro un elemento e fungerne da host. `mount(element, url, options)` restituisce un player, e `nsgo.json` indica il loader, il file wasm e le risorse in modo relativo a sé stesso.

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

Senza un'opzione `host`, il loader importa il modulo indicato da `web.host` in `nsgo.yaml` e attende il suo `createHost(player)` se il modulo lo esporta. Un'app dentro un elemento o un frame lascia alla pagina la gestione della cronologia e del titolo del browser.

`appearance` (`"light"`, `"dark"` o `"system"`), `locale` e `fontScale` sostituiscono le impostazioni del browser, proprio come fanno le impostazioni di un host nativo, e i metodi `setAppearance`, `setLocale` e `setFontScale` del player le aggiornano in seguito. `module`, un `WebAssembly.Module` compilato o una sua promise, permette a una pagina che monta l'app molte volte di scaricarla e compilarla una sola volta.

Il loader mostra un elemento `progress.nsgo-load-progress` prima che l'app si avvii. Personalizzalo con i CSS, passa `onProgress` nelle opzioni di montaggio oppure esporta `onProgress` da `web.host` per la pagina stessa dell'app. La callback sostituisce la barra e riceve `{ loaded, total }` in byte decodificati, con `total` impostato a `null` quando la dimensione è sconosciuta. Il loader la chiama con `null` al termine dell'avvio così che possa ripulire la propria UI. Un `module` già compilato salta l'avanzamento del download.

```yaml
web:
  host: host.js
```

## Il canale host

`core.Host` è l'unico contratto implementato da ogni host: parametri di avvio, eventi inviati dall'host e funzioni dell'host chiamate dall'app. I valori attraversano il canale come in JSON, quindi le mappe diventano `map[string]any`, i numeri `float64`, e lo stesso codice funziona in Swift, Java, Go e JavaScript.

- `HostParams` legge i parametri di avvio una sola volta, all'inizio.
- `cx.OnHostEvent` ascolta `Host.Send` e `hostEvent` della pagina.
- `cx.CallHost` chiama un gestore per nome e si risolve con la sua risposta; `cx.PostHost` ne chiama uno senza risposta.
- `Host.HandleOpenURL` riceve `cx.OpenURL` prima della piattaforma, e la sua risposta decide se anche la piattaforma lo apre.
- `Host.SetParams`, `Send`, `Handle` e `HandleDefault` sono sicuri da qualsiasi goroutine.

Su iOS l'host imposta `params`, installa gestori con `handle`, invia eventi con `send` e risponde a `onOpenURL`. Su Android fa lo stesso con `setParams`, `handle`, `send` e `setOpenUrlHandler`. Su desktop usa i metodi di `core.Host`, e sul web risponde l'oggetto `host` della pagina.

## Componenti nativi

`core.Native` mostra una vista creata dall'host: le sue `Props` arrivano a `SetProp` del componente host, ciò che il componente emette raggiunge `OnEvent`, e una proprietà il cui valore è un nodo animato si muove sul clock dei frame senza lavoro per il motore.

```go
core.Native{
	Component: "host.badge",
	PropsFunc: func() map[string]any { return map[string]any{"count": count.Get()} },
	OnEvent:   func(name string, data any) { if name == "tap" { count.Set(0) } },
	Style:     core.Style{MinWidth: core.Pt(120), MinHeight: core.Pt(28)},
}
```

Registra il componente nell'host: `app.register(component:)` e un `NSGoComponent` su iOS, `registerComponent` e un `NsgoComponent` su Android, `nsgo.Component` su desktop. Il web non ha componenti nativi, quindi lì un nome sconosciuto mostra un riquadro vuoto.

## Impostazioni

`tokens` sovrascrive il tema dell'app per nome di ruolo (`accent`, `inset`, `raised`), in entrambi gli aspetti o in uno solo; `cx.Palette()` li legge. L'host imposta inoltre la lingua, la dimensione del testo, l'aspetto e la directory dei dati, e riceve gli errori dell'app. `nsgo build -embed` non genera finestre, menu o icone nella tray: quelli spettano all'host.

Un panic interno sulla goroutine del motore di una vista arresta quel motore, distrugge le superfici collegate e segnala il panic e lo stack al listener degli errori dell'host (`nsgo.OnError` in Go). Gli altri motori continuano a funzionare; crea una nuova vista per riavviare l'app fallita. I normali panic nelle callback dell'app continuano a raggiungere prima il `core.Catch` più vicino.

`examples/embed-ios`, `examples/embed-android`, `examples/embed-desktop` ed `examples/embed-web` mostrano la stessa app con ciascun tipo di host.

## Prossimi passi

- [Servizi della piattaforma](services.md): cosa chiede l'app alla piattaforma.
- [Compilazione e distribuzione](building.md): i pacchetti prodotti da `-embed` e la loro firma.
- [Esempio: un'app todo](tutorials/todo.md): l'app che viene integrata da un host.
