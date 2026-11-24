---
title: Aplicaciones TSX
description: Escribe una aplicación en TSX con clases de Tailwind, compilada por adelantado sin Node.
group: Conceptos básicos
order: 63
---

## Una aplicación TSX

Una aplicación TSX es un paquete de Go que incrusta su código fuente y ejecuta su punto de entrada:

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

`go tool nsgo init -template tsx` genera una, y `examples/tsx` es otro ejemplo. `nsgo dev` y `nsgo build` compilan el TSX por adelantado, de modo que los paquetes no incluyen ningún compilador; `go run` y `go test` lo compilan al iniciar la aplicación. De cualquier manera, no se necesita Node.

## Componentes y módulos

TSX importa las primitivas desde `nsgo`, los componentes de `base` desde `nsgo/base`, los controles de videojuegos desde `nsgo/gamepad` (bajo la capacidad `gamepad` en paquetes firmados), los sonidos desde `nsgo/audio` (bajo `audio`) y los componentes de `ui` desde `nsgo/ui`, una vez que la aplicación importa `tsx/ui`; `examples/gallery-tsx` los muestra.

Para diagramas de Mermaid, importa `github.com/nsgo-dev/nsgo/tsx/diagram` por su efecto en el anfitrión de Go, junto con `tsx/textview` o `tsx/ui`. Esto agrega `Diagram` a `nsgo/base` sin enlazar el analizador de diagramas en aplicaciones que no lo usan. Recibe `source`, `fontSize`, un `theme` parcial, `accessibilityLabel` y las propiedades de estilo habituales. Úsalo directamente o proporciona un renderizador para TextView:

```tsx
import { Diagram, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderDiagram={(code) => <Diagram source={code.code} errorClass="font-mono" />} />
```

La misma propiedad `renderDiagram` funciona en el TextView de `nsgo/ui`. Un bloque de código que no es de Mermaid conserva su renderizador habitual; un diagrama no válido muestra su código fuente.

Para fórmulas matemáticas compuestas tipográficamente, importa `github.com/nsgo-dev/nsgo/tsx/formula` por su efecto en el anfitrión de Go, junto con `tsx/textview` o `tsx/ui`. Esto agrega `Formula` a `nsgo/base`; las aplicaciones que omiten el paquete mantienen el motor matemático y la fuente fuera de su binario. Recibe `source`, `dialect` (`tex` de forma predeterminada, o `typst`), `display`, `accessibilityLabel` y propiedades de estilo. El tamaño y el color de la fuente siguen al texto circundante. Úsalo directamente o proporciona un renderizador para TextView:

```tsx
import { Formula, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderMath={(math) => <Formula source={math.tex} display={math.display} />} />
```

El mismo `renderMath` funciona en el TextView de `nsgo/ui`. Las fórmulas conservan su TeX al copiarse. El `Renderer` de Go sigue siendo una configuración del anfitrión.

`UrlScheme.url()` lee la dirección rastreada de la aplicación; `UrlScheme.setUrl(url, { history: "push" })` agrega una entrada de dirección o la reemplaza cuando se omiten las opciones. `UrlScheme.on("change", fn)` observa los cambios de retroceso, avance y hash del navegador y devuelve una función para cancelar la suscripción. Consulta [Direcciones de la aplicación](addresses.md) para el enrutamiento, la validación del mismo origen y el alojamiento de enlaces profundos.

## Go desde TSX

Registra código Go para importarlo desde TSX:

```go
tsx.Register("app/native", map[string]core.Node{"Badge": Badge{}})
tsx.RegisterFunctions("app/native", map[string]any{
	"greet": func(name string) string { return "Hello, " + name },
})
```

`import { Badge, greet } from "app/native"` recibe entonces los campos de `Badge` como propiedades y llama a `greet`. Las funciones que reciben un `context.Context` se convierten en promesas, y `tsx.RegisterClass` convierte tipos de Go en clases que TSX instancia con `new`.

Una propiedad opcional reenviada como `undefined` utiliza su valor predeterminado. Al registrar un componente de Go con un getter cuyo valor predeterminado difiere del valor cero de su tipo, agrega una opción como `tsx.Default("Max", float64(100))`; el mismo valor predeterminado se aplica mientras una propiedad reactiva devuelva undefined, sin reemplazar el componente.

Los campos de datos con valores predeterminados documentados en Go también pueden omitirse: el `width` y el `height` de un nodo `Flow` miden su contenido JSX cuando faltan, el `width` omitido de una arista usa el trazo predeterminado y el `pace` omitido de una fila de cuota deja oculta su marca de ritmo.

El registro de componentes y clases de Go busca sus campos y métodos exportados mediante reflexión mientras la aplicación se ejecuta. Esa búsqueda impide que el enlazador de Go descarte métodos exportados a los que nadie llama, en toda la aplicación y no solo en lo que TSX registra, por lo que enlazar `tsx` agrega unos 20 MB a un binario de escritorio sin símbolos, y más cuantos más paquetes enlace la aplicación.

## Paquetes firmados

Una aplicación puede ejecutar el TSX que descarga, como sus propias pantallas entregadas de nuevo o complementos, una vez que verifica el paquete. Declara aquello a lo que el código del paquete puede acceder en su `nsgo.yaml`; se le deniega todo lo demás:

```yaml
capabilities:
  network:
    hosts: [api.example.com, "*.cdn.example.com"]
  open_url: true # or { schemes: [https] }
  host: [search, "player.*"] # the app's handlers it may call
  # webview: { hosts: [...], input: true, media: [microphone, camera] }, clipboard, picker, speech, text_to_speech, status_bar, notifications, biometrics, sign_in_with_apple, purchases, secrets, audio, gamepad, auth_session
```

Mantén una clave raíz sin conexión y certifica las claves de lanzamiento con ella; luego firma la aplicación en ese directorio. `go tool nsgo keygen` imprime un par de claves y no escribe ningún archivo; ejecútalo una vez para la raíz y una vez para cada clave de lanzamiento. Las claves públicas van en las banderas y en el código de la aplicación; las claves secretas permanecen fuera del repositorio y llegan a la herramienta únicamente mediante `NSGO_ROOT_KEY` y `NSGO_RELEASE_KEY`:

```sh
go tool nsgo keygen # once for the root, once for the release key
NSGO_ROOT_KEY=<root secret> go tool nsgo certify -key <release public> -apps 'com.example.*'
NSGO_RELEASE_KEY=<release secret> go tool nsgo publish -cert release.cert plugin
go tool nsgo verify -root <root public> plugin/dist/Plugin-1.0.0.nsgopkg
```

La aplicación confía en la clave pública de la raíz, escrita en su código, verifica el paquete fuera de la goroutine del motor y lo muestra en `tsx.View`, aislado en un entorno de pruebas junto a sus propias vistas. El paquete solo llega a la aplicación a través del canal del anfitrión que le da la vista, y mantiene su propia parte del almacenamiento y los secretos de la aplicación:

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

`Sandbox.Grant` le concede menos de lo que declara, `Sandbox.Limits` otros presupuestos de tiempo y cuotas, y `Sandbox.Modules` funciones y componentes de Go más allá de los propios de NSGo. Sus atajos de teclado, manejadores de retroceso y pilas de navegación solo surten efecto mientras el foco del teclado está en su vista, y solo mueve el foco por sí mismo (`focus()`, `autoFocus`, `Keyboard.dismiss`, `setAccessibilityFocus`) mientras el foco ya está allí o en respuesta a una pulsación del usuario dentro de ella. Se muestra en una vista propia que ocupa el espacio que le da su padre, ni más, y lo recorta, incluidos menús, ventanas emergentes y notificaciones emergentes; dentro de una vista desplazable, colócalo en una vista para dejar que adopte la altura de su contenido. Un diálogo, una alerta, un menú contextual o un anuncio suyo solo se muestra de la misma manera, y retroceder cierra uno de sus diálogos aunque no lo elimine. `View.Export` nombra un componente que exporta el módulo de entrada del paquete, el cual se muestra en lugar de la raíz que monta la entrada, cuyo `mount` entonces ni renderiza ni ejecuta su componente, y recibe `View.Props`, un mapa codificable en JSON que se copia para cada vista, de modo que un mismo paquete proporciona varias vistas, cada una con estado propio, bajo las mismas capacidades y límites; una exportación ausente produce un pánico con `tsx.ErrNoViewExport` y una que no sea una función con `tsx.ErrViewExportNotComponent`. Un paquete cuya entrada exporta funciones es un complemento de funciones: `tsx.Load(cx, p, tsx.Sandbox{})` lo ejecuta de forma oculta y síncrona en la goroutine del llamador, bajo un `cx` que pertenece a un árbol de vistas (sin uno devuelve `tsx.ErrNoTree`), o devuelve lo que su módulo de entrada lanzó al cargarse, y `Plugin.Call` las llama. `go tool nsgo dev -limits` ejecuta una aplicación tal como lo haría una vista de este tipo, y `go tool nsgo revoke` firma una lista de claves de lanzamiento y paquetes revocados para `Trust.UpdateRevocations`. `examples/plugins` solicita al usuario que conceda las capacidades de un paquete, lo instala y lo ejecuta. `Limits.Memory`, de 64 MiB de forma predeterminada, asigna un presupuesto de almacenamiento nativo medido para arreglos, cadenas, búferes y arreglos tipados. Esto incluye `concat`, `flat`, `slice`, propagación de arreglos, concatenación de cadenas, plantillas, `replace`, `replaceAll`, `TextEncoder` y copias de `Blob`. `memory=<bytes>` en `-limits` cambia el presupuesto, y `memory=0` lo desactiva. Las comprobaciones se ejecutan antes de la asignación. Un paquete que supera el presupuesto se detiene con `ERR_NSH_QUOTA_EXCEEDED` y el recurso `memory` después de que Go haya comprobado si existe almacenamiento recolectable. Los objetos ordinarios, cierres, pilas, asignaciones pequeñas y las funciones integradas y API del anfitrión no medidas también pueden asignar memoria. El navegador mantiene su propio montón. Una aplicación que firma paquetes por sí misma con `pkg.Build`, o `go tool nsgo publish -extra <file>` con los bytes de ese archivo, puede colocar metadatos propios en `pkg.Contents.Extra`, como lo que un complemento agrega a sus menús: se firma junto con el paquete, y `Verify` lo devuelve sin cambios como `p.Extra` para que la aplicación lo analice, de modo que la aplicación lo lee sin ejecutar el código del paquete.

Una aplicación puede firmar sus propios documentos JSON, como un catálogo de sus complementos, con `pkg.SignDocument` y verificarlos con `Signed.Open`, usando las claves que imprime `nsgo keygen`. Asigna a cada tipo de documento un contexto `app:` propio con una versión, como `app:catalog-v1`: un documento nunca pasa por otro tipo, ni por un paquete, certificado o lista de revocación. `Open` confía exactamente en las claves públicas que recibe, sin certificado ni lista de revocación, y rechaza campos del cuerpo que faltan en el tipo en el que decodifica. El sobre no contiene marca de tiempo ni secuencia, por lo que un documento antiguo se verifica igual que uno más reciente: incluye una versión o fecha de expiración en el cuerpo y compruébala.
En un entorno aislado nativo, `JSON.stringify`, `JSON.parse` con un reviver, `Array.prototype.join`, `toString`, `toLocaleString` y `flat`, y por tanto `String()` sobre un arreglo, lanzan un `RangeError` capturable una vez que superan los 1,000 niveles de profundidad, contando uno dentro de otro, ya que el `toString` de un arreglo y el `join` que invoca cuentan por separado; datos más profundos colgarían la llamada o terminarían la aplicación. La web utiliza las funciones integradas del navegador.

En destinos nativos, `DefaultLimits.Stack` es de 1,000 marcos. Establece `Limits.Stack` o `-limits stack=<frames>` para elegir una profundidad diferente de hasta 10,000 marcos. Los valores de Go iguales o inferiores a cero y las entradas por encima del límite usan 10,000. La web utiliza el límite de pila del navegador.

Las funciones nativas que se llaman entre sí cuentan para `Limits.Stack` igual que las llamadas de JS: un getter, `toString` o `valueOf` integrado, una expresión regular cuyo `exec` es otro de sus métodos, `Reflect.apply` y `Reflect.construct` anidados entre sí y cadenas de funciones enlazadas o proxies lanzan un `RangeError` capturable al superarlo en lugar de terminar la aplicación, y un valor de un complemento anidado más allá de ese límite hace que la llamada que lo convierte a Go falle con un `RangeError`. Esto proviene de la bifurcación de goja que requiere NSGo, `github.com/nsgo-dev/goja`, que tu aplicación obtiene con NSGo sin ningún cambio en su `go.mod`.

En destinos nativos, `Limits.Call` acota cada porción de JavaScript, incluidas las devoluciones de llamada de temporizadores y las continuaciones de promesas después de `await`. Cuando se corta una porción, un `Plugin.Call` pendiente informa `tsx.ErrBudget` una sola vez, detiene sus temporizadores restantes e ignora las respuestas tardías del anfitrión. Las llamadas independientes siguen ejecutándose, incluidas las reacciones de promesas encoladas en la porción interrumpida. Una llamada que resuelve la promesa de otra sigue informando su propio resultado o error. Las expresiones regulares con retroceso usan el presupuesto más corto entre el de carga y el de llamada como tiempo máximo de coincidencia; un tiempo de espera agotado no manejado también se convierte en `tsx.ErrBudget`.

`Limits.CallTotal` agrega una fecha límite de tiempo real para cada llamada al complemento y cada evento de vista en entorno aislado. Incluye el tiempo de espera y retiene sus temporizadores, intervalos, fotogramas, continuaciones de promesas y respuestas del anfitrión incluso después de que la llamada se resuelva. Al cumplirse el plazo, el trabajo pendiente se descarta y una llamada que aún espera falla con `tsx.ErrBudget`. El trabajo del módulo de entrada está sujeto a `Limits.Load`, usando `CallTotal` si `Load` es cero. Los eventos independientes del anfitrión usan `CallTotal` desde su entrega. Usa `Plugin.CallWithBudget` para obtener un identificador del plazo de una llamada en espera. En `Scope.Post` del anfitrión, llama a `budget.Extend(duration)` antes de ese plazo para dar más tiempo únicamente a esta llamada.

`Plugin.Interrupt` detiene el entorno de ejecución desde cualquier goroutine, descarta sus temporizadores pendientes y hace fallar las llamadas actuales y posteriores. `Plugin.Stopped()` y `Plugin.Err()` también pueden leerse desde cualquier goroutine. `Err()` es nil mientras el entorno de ejecución está activo y devuelve el primer motivo de detención una vez detenido. Cerrar el complemento o eliminar normalmente su propietario ejecuta sus limpiezas y lo detiene con `tsx.ErrPluginClosed`. Configura `Sandbox.OnStop` al llamar a `tsx.Load` para recibir ese motivo una vez en la goroutine del motor, después de que fallen las llamadas en espera.

Configura `Sandbox.OnError` al cargar un complemento de funciones para recibir fallos no capturados de temporizadores, fotogramas de animación, `queueMicrotask` y `reportError` fuera de un `Plugin.Call` en espera. El manejador de cada complemento se ejecuta en la goroutine del motor, de modo que un anfitrión puede atribuir fallos y llamar a `Plugin.Interrupt` cuando se abre su disyuntor. Los cortes nativos del presupuesto de porción también informan `tsx.ErrBudget`, incluido un trabajo de promesa cuya llamada ya se resolvió. Los errores ordinarios de devolución de llamada dejan el complemento en ejecución. Los errores de carga y de resultado de llamada siguen devolviéndose a sus llamadores, y las vistas siguen usando `core.Catch`; sin un manejador, se mantiene el comportamiento existente de límites y reportes de la aplicación.

Configura `Sandbox.OnViewRuntime` para recibir el `*tsx.ViewRuntime` de cada vista en la goroutine del motor antes de que se ejecute su módulo de entrada. Llama a `Interrupt` desde cualquier goroutine para detener su trabajo y dejar en su lugar la última vista renderizada. `Stopped` y `Err` informan su estado terminal, con `tsx.ErrViewClosed` en una eliminación normal. `Sandbox.OnStop` también informa una vez el motivo terminal de una vista.

`Plugin.JSTime` y `ViewRuntime.JSTime` informan el tiempo transcurrido en las entradas de JavaScript del entorno de ejecución nativo, incluida la entrada actual. Se excluyen las esperas entre entradas y el tiempo dentro de funciones Go síncronas registradas. `ViewRuntime.EngineTime` también incluye llamadas Go síncronas y el trabajo propio de reactividad, montaje, confirmación y diseño, incluido el trabajo aún en ejecución. Excluye las esperas entre entradas del motor, el trabajo perteneciente al anfitrión o a otra vista, el registro compartido de confirmaciones y el dibujado nativo. Ambos contadores son seguros para leer desde cualquier goroutine. Los turnos de JavaScript del navegador fuera de NSGo no pueden medirse ni interrumpirse de forma preventiva. Allí ambos contadores indican que no están disponibles e `Interrupt` detiene futuras devoluciones de llamada de NSGo cuando el anfitrión recupera el control.

La web aún no puede aislar un paquete de la página: su código alcanzaría la propia red, el portapapeles y el almacenamiento de la página, incluido el del almacén de confianza, más allá de sus capacidades. Allí `tsx.View` y `tsx.Load` rechazan un paquete con `tsx.ErrUnisolated`, salvo que configures `Sandbox.WebUnisolated` para ejecutarlo con la autoridad de la página, lo cual solo es adecuado para paquetes en los que confiarías tanto como en la propia aplicación.

## Editores y pruebas

Un `tsconfig.json` que extienda `.nsgo/tsconfig.json` proporciona a los editores los tipos de TSX, que `go tool nsgo dev` y `go tool nsgo check` generan. Los archivos `*.test.tsx` junto a la aplicación, escritos con `nsgo/test`, se ejecutan con sus pruebas de Go ([Pruebas](testing.md)).
