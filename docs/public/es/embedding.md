---
title: Incrustación
description: Coloque una aplicación de NSGo dentro de una aplicación propia de iOS, Android, escritorio o web.
group: Herramientas
order: 23
---

## Compilar una aplicación incrustable

Una aplicación incrustable es un paquete de aplicación normal con un `Root` y un `main`, compilado con `-embed` para el destino del host. Cada vista ejecuta la aplicación una vez y es un motor propio, de modo que un host muestra tantas vistas como desee, cada una con su propio estado.

```sh
go tool nsgo build -embed -target ios examples/embedded
go tool nsgo build -embed -target android_arm64 examples/embedded
go tool nsgo build -target web examples/embedded
```

La compilación de iOS escribe `<Name>.xcframework` y `<Name>Resources`; la de Android escribe `<Name>.aar`; la de web escribe un directorio con `nsgo.json` y el cargador.

## iOS

Cuando la aplicación importa `base/terminal`, la compilación también escribe `GhosttyVT.xcframework` junto al framework de la aplicación. Añada este compañero al destino del host con **Embed & Sign**, tanto para frameworks estáticos como para `-dynamic`. Contiene bibliotecas para dispositivo, simuladores Intel y Apple silicon, y Mac Catalyst; conserve su nombre `GhosttyVT.framework`.

Añada el framework al destino de la aplicación como biblioteca estática (no lo incruste) y la carpeta de recursos como referencia de carpeta, y luego haga `import <Name>`. Llame a `NSGoStart()` una vez al iniciarse la aplicación, antes de la primera vista. El código de la biblioteca estática pertenece a la aplicación host, por lo que el manifiesto de privacidad del host declara las APIs de motivo obligatorio que usa NSGo: valores predeterminados de usuario (`CA92.1`), marcas de tiempo de archivos (`C617.1`) y tiempo de arranque del sistema (`35F9.1`); un framework `-dynamic` las lleva en su propio `PrivacyInfo.xcprivacy`.

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

`NSGoView` inicia la aplicación una vez que está en una ventana y la detiene cuando se desasigna o recibe `stop()`. Establezca los valores de lanzamiento antes de que esté en una ventana; el resto puede cambiar en cualquier momento. Los controladores, componentes y devoluciones de llamada se ejecutan en el hilo principal, y una respuesta puede llegar más tarde desde cualquier hilo. Un controlador de vista puede reenviar `preferredStatusBarStyle`, `prefersStatusBarHidden` y `prefersHomeIndicatorAutoHidden` de la vista para seguir sus ajustes de barras del sistema.
El host también puede devolver `preferredScreenEdgesDeferringSystemGestures` de la vista desde su controlador para reenviar `mobile.DeferSystemGestures`.

## Android

Para una aplicación arm64 que importa `base/terminal`, el AAR incluye `jni/arm64-v8a/libghostty-vt.so` junto a `libnsgo.so`; el host mantiene habilitada la extracción de bibliotecas nativas. La precompilación de Android y el cargador de terminal requieren [#1452](https://github.com/nsgo-dev/nsgo/issues/1452); las bibliotecas de terminal x86_64 aún no están disponibles.

Añada el AAR al módulo de la aplicación y empaquete las bibliotecas nativas extraídas para que el entorno de ejecución las encuentre:

```kotlin
implementation(files("libs/Embedded.aar"))

android { packaging { jniLibs { useLegacyPackaging = true } } }
```

Cree instancias de `dev.oneo.nsgo.NsgoView` en código o en un diseño. Cada una inicia la aplicación en su propio proceso en cuanto recibe un tamaño por primera vez, y `stop()` la finaliza. Reenvíe los resultados de la actividad a cada vista y gestione los cambios de configuración en la actividad para que las aplicaciones no se reinicien.

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

Para desacoplar el árbol nativo manteniendo viva la aplicación de Go, conserve un `NsgoEngine` en el host y acople su vista explícitamente:

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

Desacoplar libera los hijos nativos; volver a acoplar restaura las señales actuales, la entrada, el foco, la posición de desplazamiento y los dibujos sin volver a ejecutar componentes. Las páginas web y los reproductores se recrean desde la configuración de su aplicación. `stop()` solo desacopla un motor suministrado explícitamente. Cada motor sigue conservando su primera vista y su Activity y rechaza otra vista, así que gestione los cambios de configuración en esa Activity; reemplazarla o mostrar vistas independientes en un mismo proceso aún no está soportado.

Para comprobaciones de actualización de Google Play, añada `com.google.android.play:app-update:2.1.0` a las dependencias de Gradle del host Android. Conserve las APIs AppUpdateManager, AppUpdateInfo, AppUpdateOptions y Tasks que NSGo carga por nombre usando las [reglas de reducción del entorno de ejecución](https://github.com/nsgo-dev/nsgo/blob/main/internal/platform/android/runtime/play-review.pro). El reenvío de resultados de actividad anterior también entrega la cancelación de actualizaciones.

Para sesiones de autenticación en Android dentro de un host incrustado, declare el esquema de retorno privado en la Activity anfitriona y reenvíe los enlaces que esa Activity acepta a `view.openLink(uri.toString())` en `onNewIntent` (y en un `onCreate` nuevo). Una sesión activa coincidente consume su retorno; otros enlaces llegan a `OnOpenLinks`. La `NsgoActivity` independiente los reenvía por sí misma.

## Escritorio

Un host en Go ejecuta el bucle de eventos de la plataforma y coloca el `Handle` de cada vista en su ventana. `nsgo.NewView` inicia la aplicación una vez que el manejador está en una ventana, de modo que los ajustes realizados antes están presentes en su primer fotograma.

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

`view.Host()` envía eventos a la aplicación y recibe sus llamadas, y `nsgo.Component` registra un componente nativo para la plataforma del host.

## Web

La compilación web es una página por sí misma, pero una página puede montarla en un elemento y ser su host. `mount(element, url, options)` devuelve un reproductor, y `nsgo.json` nombra el cargador, el wasm y los recursos relativos a sí mismo.

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

Sin una opción `host`, el cargador importa el módulo que nombra `web.host` en `nsgo.yaml` y espera su `createHost(player)` si el módulo la exporta. Una aplicación en un elemento o un marco deja el historial y el título del navegador a la página.

`appearance` (`"light"`, `"dark"` o `"system"`), `locale` y `fontScale` sustituyen a los del navegador, como hacen los ajustes de un host nativo, y `setAppearance`, `setLocale` y `setFontScale` del reproductor los cambian después. `module`, un `WebAssembly.Module` compilado o una promesa de uno, permite que una página que monta la aplicación muchas veces la descargue y compile una sola vez.

El cargador muestra un elemento `progress.nsgo-load-progress` antes de que arranque la aplicación. Estílelo con CSS, pase `onProgress` en las opciones de montaje o exporte `onProgress` desde `web.host` para la página propia de la aplicación. La devolución de llamada reemplaza la barra y recibe `{ loaded, total }` en bytes decodificados, con `total` establecido en `null` cuando el tamaño es desconocido. El cargador la llama con `null` cuando termina el arranque para que pueda limpiar su interfaz. Un `module` compilado omite el progreso de descarga.

```yaml
web:
  host: host.js
```

## El canal del host

`core.Host` es el único contrato que implementa todo host: parámetros de lanzamiento, eventos que envía el host y funciones del host a las que llama la aplicación. Los valores cruzan como en JSON, de modo que los mapas se convierten en `map[string]any` y los números en `float64`, y el mismo código funciona en Swift, Java, Go y JavaScript.

- `HostParams` lee los parámetros de lanzamiento una vez, al inicio.
- `cx.OnHostEvent` escucha `Host.Send` y el `hostEvent` de la página.
- `cx.CallHost` llama a un controlador con nombre y se resuelve con su respuesta; `cx.PostHost` llama a uno sin respuesta.
- `Host.HandleOpenURL` ve `cx.OpenURL` antes que la plataforma, y su respuesta decide si la plataforma también lo abre.
- `Host.SetParams`, `Send`, `Handle` y `HandleDefault` son seguros desde cualquier goroutine.

En iOS el host establece `params`, instala controladores con `handle`, envía eventos con `send` y responde a `onOpenURL`. En Android hace lo mismo con `setParams`, `handle`, `send` y `setOpenUrlHandler`. En escritorio usa los métodos de `core.Host`, y en la web responde el objeto `host` de la página.

## Componentes nativos

`core.Native` muestra una vista que crea el host: sus `Props` llegan al `SetProp` del componente del host, lo que emite el componente llega a `OnEvent`, y una propiedad cuyo valor es un nodo animado se mueve con el reloj de fotogramas sin trabajo para el motor.

```go
core.Native{
	Component: "host.badge",
	PropsFunc: func() map[string]any { return map[string]any{"count": count.Get()} },
	OnEvent:   func(name string, data any) { if name == "tap" { count.Set(0) } },
	Style:     core.Style{MinWidth: core.Pt(120), MinHeight: core.Pt(28)},
}
```

Registre el componente en el host: `app.register(component:)` y un `NSGoComponent` en iOS, `registerComponent` y un `NsgoComponent` en Android, `nsgo.Component` en escritorio. La web no tiene componentes nativos, así que un nombre desconocido muestra allí una caja vacía.

## Ajustes

`tokens` sobrescribe el tema de la aplicación por nombre de rol (`accent`, `inset`, `raised`), en ambos aspectos o en uno; `cx.Palette()` los ve. El host también establece la configuración regional, el tamaño de texto, el aspecto y el directorio de datos, y escucha los fallos de la aplicación. `nsgo build -embed` no genera ventanas, menús ni iconos de bandeja: esos pertenecen al host.

Un pánico interno en la goroutine del motor de una vista detiene ese motor, destruye sus superficies acopladas e informa el pánico y la pila al receptor de errores del host (`nsgo.OnError` en Go). Otros motores siguen ejecutándose; cree una vista nueva para reiniciar la aplicación fallida. Los pánicos ordinarios en devoluciones de llamada de la aplicación siguen yendo primero al `core.Catch` más cercano.

`examples/embed-ios`, `examples/embed-android`, `examples/embed-desktop` y `examples/embed-web` son la misma aplicación con cada tipo de host.

## Siguiente

- [Servicios de plataforma](services.md): qué solicita la aplicación a la plataforma.
- [Compilación y distribución](building.md): los paquetes que genera `-embed` y cómo firmarlos.
- [Ejemplo: Una aplicación de tareas](tutorials/todo.md): la aplicación que incrusta un host.
