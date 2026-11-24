---
title: Intégration
description: Intégrer une application NSGo dans votre propre application iOS, Android, desktop ou web.
group: Chaîne d’outils
order: 23
---

## Compiler une application intégrable

Une application intégrable est un paquet applicatif normal doté d'un `Root` et d'un `main`, compilé avec `-embed` pour la cible de l'hôte. Chaque vue exécute l'application une fois et constitue son propre moteur, de sorte qu'un hôte peut afficher autant de vues qu'il le souhaite, chacune avec son propre état.

```sh
go tool nsgo build -embed -target ios examples/embedded
go tool nsgo build -embed -target android_arm64 examples/embedded
go tool nsgo build -target web examples/embedded
```

La compilation iOS produit `<Name>.xcframework` et `<Name>Resources` ; la compilation Android produit `<Name>.aar` ; la compilation web produit un répertoire contenant `nsgo.json` et le chargeur.

## iOS

Lorsque l'application importe `base/terminal`, la compilation produit également `GhosttyVT.xcframework` à côté du framework de l'application. Ajoutez ce compagnon à la cible hôte avec **Embed & Sign**, tant pour les frameworks applicatifs statiques que `-dynamic`. Il contient les bibliothèques pour appareil, simulateur Intel et Apple Silicon, ainsi que Mac Catalyst ; conservez son nom `GhosttyVT.framework`.

Ajoutez le framework à la cible applicative en tant que bibliothèque statique (ne l'embarquez pas) et le dossier de ressources en tant que référence de dossier, puis écrivez `import <Name>`. Appelez `NSGoStart()` une fois au lancement de l'application, avant la première vue. Le code de la bibliothèque statique fait partie de l'application hôte : le manifeste de confidentialité de l'hôte déclare donc les API à motif requis utilisées par NSGo : préférences utilisateur (`CA92.1`), horodatages de fichiers (`C617.1`) et durée de démarrage du système (`35F9.1`) ; un framework `-dynamic` les embarque dans son propre `PrivacyInfo.xcprivacy`.

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

`NSGoView` démarre l'application dès qu'elle est placée dans une fenêtre et l'arrête lorsqu'elle est désallouée ou reçoit `stop()`. Définissez les valeurs de lancement avant qu'elle ne soit dans une fenêtre ; le reste peut changer à tout moment. Les gestionnaires, composants et callbacks s'exécutent sur le thread principal, et une réponse peut arriver ultérieurement depuis n'importe quel thread. Un contrôleur de vue peut relayer `preferredStatusBarStyle`, `prefersStatusBarHidden` et `prefersHomeIndicatorAutoHidden` de la vue pour suivre ses réglages des barres système.
L'hôte peut également retourner `preferredScreenEdgesDeferringSystemGestures` de la vue depuis son contrôleur pour relayer `mobile.DeferSystemGestures`.

## Android

Pour une application arm64 qui importe `base/terminal`, l'AAR inclut `jni/arm64-v8a/libghostty-vt.so` à côté de `libnsgo.so` ; l'hôte garde activée l'extraction des bibliothèques natives. La précompilation Android et le chargeur terminal nécessitent [#1452](https://github.com/nsgo-dev/nsgo/issues/1452) ; les bibliothèques terminal x86_64 ne sont pas encore disponibles.

Ajoutez l'AAR au module d'application et empaquetez les bibliothèques natives en mode extrait afin que le runtime les trouve :

```kotlin
implementation(files("libs/Embedded.aar"))

android { packaging { jniLibs { useLegacyPackaging = true } } }
```

Créez des `dev.oneo.nsgo.NsgoView` par code ou dans une mise en page XML. Chacune démarre l'application dans son propre processus dès qu'elle reçoit une taille pour la première fois, et `stop()` y met fin. Transférez les résultats de l'activité à chaque vue et gérez les changements de configuration dans l'activité afin que les applications ne soient pas redémarrées.

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

Pour détacher l'arbre natif tout en gardant l'application Go en vie, conservez un `NsgoEngine` dans l'hôte et attachez sa vue explicitement :

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

Le détachement libère les enfants natifs ; le rattachement restaure les signaux courants, les saisies, le focus, la position de défilement et les dessins sans réexécuter les composants. Les pages web et les lecteurs sont recréés à partir de leur configuration applicative. `stop()` se contente de détacher un moteur fourni explicitement. Chaque moteur conserve encore sa première vue et son Activity et rejette toute autre vue : gérez donc les changements de configuration dans cette Activity ; la remplacer ou afficher des vues indépendantes dans un seul processus n'est pas encore pris en charge.

Pour les vérifications de mise à jour Google Play, ajoutez `com.google.android.play:app-update:2.1.0` aux dépendances Gradle de l'hôte Android. Préservez les API AppUpdateManager, AppUpdateInfo, AppUpdateOptions et Tasks que NSGo charge par nom à l'aide des [règles de réduction du runtime](https://github.com/nsgo-dev/nsgo/blob/main/internal/platform/android/runtime/play-review.pro). Le transfert des résultats d'activité ci-dessus délivre également l'annulation de la mise à jour.

Pour les sessions d'authentification Android dans un hôte intégré, déclarez le schéma de rappel privé sur l'Activity hôte et transmettez les liens acceptés par cette Activity à `view.openLink(uri.toString())` dans `onNewIntent` (et lors d'un `onCreate` initial). Une session active correspondante consomme son rappel ; les autres liens parviennent à `OnOpenLinks`. La classe autonome `NsgoActivity` les transmet elle-même.

## Bureau

Un hôte Go exécute la boucle d'événements de la plateforme et place le `Handle` de chaque vue dans sa fenêtre. `nsgo.NewView` démarre l'application dès que le handle se trouve dans une fenêtre, de sorte que les réglages effectués auparavant figurent dans sa première image.

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

`view.Host()` envoie des événements à l'application et reçoit ses appels, et `nsgo.Component` enregistre un composant natif pour la plateforme de l'hôte.

## Web

La compilation web est une page autonome, mais une page peut la monter dans un élément et devenir son hôte. `mount(element, url, options)` retourne un lecteur (`player`), et `nsgo.json` nomme le chargeur, le fichier wasm et les ressources relativement à lui-même.

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

Sans option `host`, le chargeur importe le module nommé par `web.host` dans `nsgo.yaml` et attend son `createHost(player)` si le module l'exporte. Une application dans un élément ou un cadre laisse à la page la gestion de l'historique et du titre du navigateur.

`appearance` (`"light"`, `"dark"` ou `"system"`), `locale` et `fontScale` remplacent ceux du navigateur, comme le font les réglages d'un hôte natif, et les méthodes `setAppearance`, `setLocale` et `setFontScale` du lecteur les modifient ultérieurement. `module`, un `WebAssembly.Module` compilé ou une promesse d'un tel module, permet à une page qui monte l'application plusieurs fois de ne la télécharger et compiler qu'une seule fois.

Le chargeur affiche un élément `progress.nsgo-load-progress` avant le démarrage de l'application. Stylez-le en CSS, passez `onProgress` dans les options de montage, ou exportez `onProgress` depuis `web.host` pour la propre page de l'application. Le callback remplace la barre et reçoit `{ loaded, total }` en octets décodés, avec `total` défini à `null` lorsque la taille est inconnue. Le chargeur l'appelle avec `null` à la fin du démarrage afin qu'il puisse effacer son interface. Un `module` compilé ignore la progression du téléchargement.

```yaml
web:
  host: host.js
```

## Le canal hôte

`core.Host` est le contrat unique que chaque hôte implémente : paramètres de lancement, événements envoyés par l'hôte et fonctions de l'hôte appelées par l'application. Les valeurs transitent comme en JSON : les objets deviennent des `map[string]any`, les nombres des `float64`, et le même code fonctionne en Swift, Java, Go et JavaScript.

- `HostParams` lit les paramètres de lancement une seule fois, au démarrage.
- `cx.OnHostEvent` écoute `Host.Send` et le `hostEvent` de la page.
- `cx.CallHost` appelle un gestionnaire nommé et se résout avec sa réponse ; `cx.PostHost` en appelle un sans réponse.
- `Host.HandleOpenURL` intercepte `cx.OpenURL` avant la plateforme, et sa réponse décide si la plateforme l'ouvre également.
- `Host.SetParams`, `Send`, `Handle` et `HandleDefault` sont sûrs depuis n'importe quelle goroutine.

Sur iOS, l'hôte définit `params`, installe les gestionnaires avec `handle`, envoie des événements avec `send` et répond via `onOpenURL`. Sur Android, il fait de même avec `setParams`, `handle`, `send` et `setOpenUrlHandler`. Sur ordinateur de bureau, il utilise les méthodes de `core.Host`, et sur le web l'objet `host` de la page répond.

## Composants natifs

`core.Native` affiche une vue fabriquée par l'hôte : ses `Props` parviennent au `SetProp` du composant hôte, ce que le composant émet parvient à `OnEvent`, et une propriété dont la valeur est un nœud animé bouge sur l'horloge d'images sans travail pour le moteur.

```go
core.Native{
	Component: "host.badge",
	PropsFunc: func() map[string]any { return map[string]any{"count": count.Get()} },
	OnEvent:   func(name string, data any) { if name == "tap" { count.Set(0) } },
	Style:     core.Style{MinWidth: core.Pt(120), MinHeight: core.Pt(28)},
}
```

Enregistrez le composant dans l'hôte : `app.register(component:)` et un `NSGoComponent` sur iOS, `registerComponent` et un `NsgoComponent` sur Android, `nsgo.Component` sur ordinateur de bureau. Le web ne possède pas de composants natifs ; un nom inconnu y affiche donc une boîte vide.

## Réglages

`tokens` surcharge le thème de l'application par nom de rôle (`accent`, `inset`, `raised`), dans les deux apparences ou dans une seule ; `cx.Palette()` les voit. L'hôte définit également la locale, la taille du texte, l'apparence et le répertoire de données, et reçoit les erreurs de l'application. `nsgo build -embed` ne crée ni fenêtres, ni menus, ni icônes de barre d'état : ceux-ci appartiennent à l'hôte.

Un panic interne sur la goroutine moteur d'une vue arrête ce moteur, détruit ses surfaces attachées et signale le panic et la pile d'appels à l'écouteur d'erreurs de l'hôte (`nsgo.OnError` en Go). Les autres moteurs continuent de tourner ; créez une nouvelle vue pour redémarrer l'application défaillante. Les panics ordinaires dans les callbacks applicatifs vont toujours d'abord au `core.Catch` le plus proche.

`examples/embed-ios`, `examples/embed-android`, `examples/embed-desktop` et `examples/embed-web` présentent la même application avec chaque type d'hôte.

## Suite

- [Services plateforme](services.md) : ce que l'application demande à la plateforme.
- [Compilation et distribution](building.md) : les paquets générés par `-embed`, et comment les signer.
- [Exemple : Une application de tâches](tutorials/todo.md) : l'application qu'un hôte intègre.
