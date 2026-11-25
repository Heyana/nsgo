---
title: Ejecución y depuración
description: Ejecute una aplicación en cualquier destino con recarga en caliente y observe sus errores, actualizaciones, diseño, velocidad y otros idiomas.
group: Herramientas
order: 20
---

## Ejecutar

```sh
go tool nsgo dev
```

`dev` compila la aplicación para esta máquina y la abre, o para el destino que indica `dev.target` en `nsgo.yaml`. Cada vez que guarda un archivo Go recompila y reinicia la aplicación, ejecutando primero `go mod tidy` cuando la aplicación importa un paquete cuyo módulo falta en `go.mod` o `go.sum`; un cambio solo en TSX actualiza la aplicación en ejecución en el acto, conservando su estado. La ejecución termina con la aplicación.

En macOS y Windows, una recompilación nativa de desarrollo vuelve a abrir la ventana de la vista principal en su última posición y tamaño habituales dentro de la sesión actual. Windows también restaura la pantalla completa. En Linux, GTK restaura su tamaño habitual y solicita su último modo de pantalla completa. El gestor de ventanas elige la posición.

`-target` la ejecuta en otro lugar, y `-device` elige el iPhone, Apple TV, Apple Watch, simulador o dispositivo Android por nombre, UDID o número de serie:

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

El servidor de desarrollo escucha en todas las interfaces de forma predeterminada. Use `-host 127.0.0.1` para mantenerlo en loopback, incluso cuando Android se conecta mediante `adb reverse`, o indique la dirección IP de una interfaz local para escuchar solo allí. Use una dirección accesible en la LAN para funciones de desarrollo en dispositivos Apple físicos.

`-h` enumera todas las banderas.

## Depurador de Go

Ponga en `PATH` como `dlv` una versión de [Delve](https://github.com/go-delve/delve/blob/master/Documentation/installation/README.md) compatible con su versión de Go. Inicie una aplicación de escritorio nativa con su información de depuración y con la optimización de Go desactivada.

```sh
go tool nsgo dev -target native -debug
```

Copie el puerto de la línea `API server listening at` de Delve. En VS Code con la extensión de Go, añada una configuración de acoplamiento y reemplace `40000` por ese puerto.

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

En GoLand, añada una configuración de ejecución **Go Remote** con el host `127.0.0.1` y el mismo puerto. Ponga un punto de interrupción en el código Go de su aplicación, acóplese y ejecute la acción que llega hasta él. Tras una recompilación de Go exitosa, vuelva a conectar el depurador en la misma dirección para que envíe los puntos de interrupción al nuevo proceso. Los cambios en TSX siguen usando recarga en caliente.

`-debug` se ejecuta en macOS, Windows y Linux con `-target native`. Use una compilación de desarrollo para depurar y reserve `-release` para medir una compilación de lanzamiento.

## Vistas previas de componentes

Coloque una o más funciones `PreviewXxx` junto a los componentes en un paquete Go importable. Guárdelas en un archivo Go protegido por la etiqueta de compilación de vista previa.

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

Marque los casos de ejemplo que desee mostrar juntos y elija una configuración regional, tema, aspecto claro u oscuro, tamaño del área de visualización y dirección de diseño. La vista previa toma el `nsgo.yaml` de la aplicación más cercana, sus catálogos de traducción, el `theme` configurado y los recursos. Pase un directorio de aplicación después de las banderas para seleccionar una configuración distinta en el mismo módulo. En ese caso, la ruta de vista previa es relativa al directorio de la aplicación.

Al guardar un archivo se recompila la vista previa y se conservan su selección y ajustes de visualización. La etiqueta `nsgopreview` excluye estos ejemplos de las compilaciones de lanzamiento normales. Use el mismo ejemplo en un archivo `_test.go` etiquetado con `nsgotest.Start(t, PreviewEmpty)` y ejecútelo con `GOFLAGS=-tags=nsgopreview go tool nsgo test -target macos ./components` para tomar una captura de pantalla nativa.

Después de la primera instalación de la aplicación principal con `-target ios_simulator`, una recompilación solo de Go reemplaza el ejecutable instalado y relanza la aplicación. Editar `nsgo.yaml`, los recursos empaquetados o el icono de la aplicación activa una reinstalación.

Use `go tool nsgo dev -target android -android-host` para trabajar en la interfaz de Android con un ciclo de recompilación de Go más corto. La herramienta instala el entorno de ejecución nativo una vez y ejecuta Go en la máquina de desarrollo. Cada recompilación de Go vuelve a conectarse a la misma Activity por adb. Los cambios en recursos empaquetados, clases precompiladas, iconos de lanzamiento o `nsgo.yaml` vuelven a instalar el APK del entorno de ejecución. El proceso de Go usa las restricciones de compilación, archivos y red del host. `Scope.DiskSpace` informa el volumen que contiene el `DataDir` del host. Use el comando habitual de desarrollo de Android para ejercitar archivos `_android.go` y el comportamiento de Go en el dispositivo.

## Errores

Una compilación fallida se muestra sobre la aplicación en ejecución, con botones para copiar el error y volver a compilar; corrija el archivo y el siguiente guardado lo reemplaza. Un componente que entra en pánico mientras se renderiza se muestra de la misma manera. Comando+R en la aplicación, Control+R fuera de las plataformas de Apple, la compila e inicia de nuevo en cualquier momento, también sobre el error, salvo que la aplicación asigne la tecla por sí misma, con `cx.OnKey` o como un elemento habilitado de su menú de aplicación; en el navegador la tecla recarga la página como de costumbre. La salida propia de la aplicación, como `log.Print`, aparece en la terminal debajo de las líneas de la herramienta.

Antes de ejecutar, `go tool nsgo check` analiza la aplicación y verifica los tipos de su TSX; `-target all` la analiza para todos los destinos, de modo que el código detrás de las etiquetas de compilación de otra plataforma también se comprueba desde un Mac.

### Registros de aplicaciones empaquetadas de Apple

En macOS, iOS, tvOS y watchOS, el registrador predeterminado de Go escribe en el registro unificado de Apple, incluidos `log.Print`, los diagnósticos de NSGo y la salida de consola de TSX. En Consola, filtre por el identificador del paquete de la aplicación en ejecución como subsistema y `nsgo` como categoría. En macOS también puede consultar el almacén:

```sh
log show --last 10m --predicate 'subsystem == "dev.example.hello"'
```

Los registros usan el tipo de registro predeterminado y conservan sus prefijos de advertencia o error. Su texto es público, así que mantenga credenciales y otros secretos fuera de los registros. Los mensajes largos se convierten en varias entradas. La salida de terminal sigue disponible durante el desarrollo; un escritor de registro o controlador `slog` personalizado controla su propia salida.

### Vistas web en Linux

Ubuntu 24.04 y posteriores pueden restringir los espacios de nombres de usuario sin privilegios mediante AppArmor. Cuando bubblewrap no puede iniciar el entorno aislado de WebKitGTK, NSGo registra el error y muestra un marcador de posición. De lo contrario, algunas versiones de WebKitGTK salen con `SIGTRAP` al abrir la primera vista web. [Documentación de AppArmor de Ubuntu](https://documentation.ubuntu.com/security/security-features/privilege-restriction/apparmor/)

Use el perfil de AppArmor para bubblewrap de su distribución si proporciona uno, como `bwrap-userns-restrict`. De lo contrario, un administrador puede cargar un perfil que conceda `userns` a `/usr/bin/bwrap` usando [el perfil del CI de Linux de NSGo](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/test.yml) como punto de partida. El administrador gestiona el perfil porque se aplica a todas las aplicaciones que usan el bubblewrap del sistema.

`WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1` desactiva el entorno aislado de subprocesos de WebKit. Úselo solo para desarrollo local con contenido de confianza.

## DevTools

```sh
go tool nsgo dev -devtools
```

`-devtools` abre DevTools en el navegador, servido por `nsgo dev` para cada destino, también para la aplicación de un teléfono. Muestra la aplicación en ejecución mientras se ejecuta:

- **Elements**: las vistas en un árbol. Cada una muestra el componente que la construyó, un enlace a su línea en su editor, el selector con el que una prueba la encuentra, sus clases, sus límites, su margen, borde y relleno tal como quedaron dispuestos y el estilo que establece. Al pasar el puntero sobre una fila se dibujan su margen, relleno y contenido en la aplicación, como hacen las DevTools de un navegador, y **Pick** permite pulsar una vista en cualquier ventana, panel, hoja o vista incrustada para encontrarla en el árbol.
- **State**: las señales que creó su código, bajo los componentes y cálculos que las poseen, con sus valores, valores de memos y cuántos cálculos leen cada una. Un valor que ningún cálculo lee no cambia nada en pantalla. Cadenas, booleanos y números se pueden editar, para probar un estado sin modificar código. Los valores que NSGo creó para sí mismo están bajo **NSGo's own**.
- **Profiler**: el tiempo de cada commit, dividido entre su código previo, el commit con su diseño y su aplicación en el hilo principal de la plataforma. También muestra las líneas cuyas escrituras provocaron el commit y se abre sobre los cálculos que ejecutó, los más largos primero. Una segunda lista nombra los cálculos que más se ejecutaron en el último segundo, por la línea que los creó. El entorno de ejecución de Android aplica lotes en su propio código Java, por lo que los commits de Android no tienen tiempo de plataforma.
- **Recorder**: lo que pulsa, escribe, borra y desplaza en la aplicación, escrito como un archivo de prueba `nsgotest` para copiar en un archivo `_test.go`. En Elements, **Assert shown** y **Assert text** añaden una comprobación de la vista seleccionada. Una comprobación de texto conserva el texto en el idioma en que lo mostró la aplicación, así que grabe con el idioma en el que se ejecutan sus pruebas.
- **Highlight updates** rodea cada vista que cambia un commit, como hace `-highlight`.
- **Layout** muestra y oculta la [auditoría de diseño](#diseño) en el dispositivo, como la inicia `-layout`.

El estado, los cálculos y las escrituras de una aplicación TSX se nombran por sus líneas TSX, a través de los mapas de código fuente de los paquetes, y enlazan con los archivos de su aplicación.

DevTools lee la aplicación a través del servidor de `nsgo dev`, que escucha en su red para que los teléfonos lleguen a él. Cada ejecución genera un token, que llevan después de `#` las direcciones que `nsgo dev` imprime y abre para DevTools y el destino web, y el servidor responde solo a solicitudes que lo porten, así que abra DevTools y el destino web desde esas direcciones. Cualquiera a quien le dé una puede cambiar el estado de la aplicación mientras se ejecuta y añadir pasos a la prueba que escribe el Recorder, así que revise una prueba grabada antes de ejecutarla. Otros equipos de su red y las páginas web de otros sitios que visite no tienen acceso a nada de ello.

## Actualizaciones

```sh
go tool nsgo dev -highlight
```

`-highlight` rodea durante medio segundo cada vista que cambió un commit, de modo que una actualización que redibuja más de lo debido se muestra en pantalla mientras usa la aplicación. Los contornos se dibujan sobre cada ventana, panel, modal, hoja de iOS y vista incrustada (`nsgo.NewView`). DevTools los activa y desactiva mientras la aplicación se ejecuta.

```sh
go tool nsgo dev -trace
```

`-trace` imprime cada commit de la aplicación en ejecución: su número, su tiempo y los elementos de montaje que envió, para ver qué vistas creó, cambió o eliminó una actualización. Encima de los elementos nombra de qué provino el commit: las líneas de su código que escribieron señales desde el último commit, las que más escribieron primero, y cuántos enlaces, efectos y memos se ejecutaron.

```text
── commit 12: 40 items, commit 1.65 ms
   after app/market.go:473 ×4, app/market.go:487 ×4, app/market.go:384
   ran 118 bindings, 18 effects, 5 memos
```

Una línea que escribe muchas veces, o muchos enlaces ejecutados por una escritura, indica dónde agrupar escrituras (`cx.Batch`) o acotar lo que lee un enlace. `go tool nsgo test -trace` imprime lo mismo bajo cada paso de una prueba.

## Perfiles en tiempo de ejecución

Capture la actividad de CPU de Go, el heap o la traza de ejecución de una aplicación en marcha con `nsgo dev`. La aplicación envía la captura al servidor de desarrollo, que la guarda en la máquina que ejecuta el comando.

```sh
go tool nsgo dev -target android -profile cpu -profile-duration 10s
go tool pprof -top .nsgo/profiles/cpu.pprof
```

El ejemplo de CPU también funciona en un escritorio, un iPhone o un simulador de iOS. En un iPhone, permita el acceso a Red local si el sistema lo solicita. El heap es una instantánea y también funciona en la web. Para eventos del planificador y de goroutines, ejecute `go tool nsgo dev -profile trace` y abra `.nsgo/profiles/execution.trace` con `go tool trace`. Las trazas de CPU y de ejecución duran 10 segundos de forma predeterminada. `-profile-output` establece la ruta del archivo local. Para vistas previas de componentes, la ruta predeterminada y las rutas relativas usan el directorio de la aplicación. Una ejecución de desarrollo guarda su primera captura completa y luego continúa ejecutando la aplicación.

```sh
go tool nsgo dev -target web -profile heap
go tool pprof -top .nsgo/profiles/heap.pprof
```

El entorno de ejecución en el navegador de Go admite perfiles de heap y trazas de ejecución. Para muestras de CPU en el navegador, use las herramientas de rendimiento del navegador. Elija `-profile` o `-release`. Empaquetar con `nsgo build` deja fuera el cliente de captura de desarrollo. [Diagnósticos del entorno de ejecución de Go](https://go.dev/doc/diagnostics)

## Diseño

```sh
go tool nsgo dev -layout
go tool nsgo dev -layout=bounds,grid=4
```

`-layout` inicia la aplicación con la auditoría de diseño de DevTools sobre cada ventana, panel, hoja de iOS y vista incrustada, que rodea cada vista: su caja de borde en azul, el borde interior de su relleno en verde y el borde exterior de su margen en naranja, cada uno recortado donde lo oculta una vista de desplazamiento o una vista que recorta. `grid` dibuja una cuadrícula de 8 puntos, y `grid=N` una de N puntos, al menos 4. Indique el valor después de `=`: la bandera sola significa `bounds`.

Una lectura en la esquina inferior muestra el tamaño de la ventana en puntos, con botones que activan y desactivan las cajas, la cuadrícula y la regla. En Apple TV y otros dispositivos solo con mando a distancia, el foco alcanza los botones de la lectura para poder alternarlos con el mando sin salir de la aplicación. Mientras la regla está activada, las pulsaciones eligen dos puntos en lugar de llegar a la aplicación, y la lectura muestra la distancia horizontal y vertical entre ellos; una tercera pulsación vuelve a empezar. En Apple TV o con teclado, las flechas de dirección (o deslizar la superficie táctil del Siri Remote) mueven un retículo por la pantalla y Seleccionar, Intro o Espacio elige puntos, actualizando la distancia en vivo hasta que el segundo punto la fija.

| Tecla | Efecto |
| --- | --- |
| `Cmd+Option+Shift+L`, `Ctrl+Alt+Shift+L` | Muestra u oculta la auditoría, en una compilación con `-layout` |
| Flechas, cruceta del mando / deslizar | Mueve el retículo de la regla según el paso de la cuadrícula (8 pt por defecto) |
| `Shift` + Flechas | Mueve el retículo de la regla 1 pt |
| Seleccionar, `Return`, `Space` | Elige el punto del retículo mientras mide |
| `Esc`, atrás, Menú | Detiene la medición |

Si su aplicación asigna la misma tecla, gana la asignación de la aplicación. Sin la tecla, como en un teléfono, ejecute también con `-devtools` y muestre y oculte la auditoría con **Layout** de DevTools; ocúltela para alcanzar un control debajo de la lectura. Los backends nativos y la web leen matrices de presentación para que los límites, Pick y la auditoría sigan las transformaciones mostradas, la animación y el desplazamiento; watchOS sigue su dibujo afín. HarmonyOS y los miniprogramas conservan el rastreador del lado de Go hasta que sus capas de renderizado informen la geometría. Redibujarlas cuesta tiempo en cada actualización, así que mida la velocidad sin ellas.

## Velocidad

```sh
go tool nsgo dev -fps
go tool nsgo dev -release -fps
```

`-fps` muestra el monitor de rendimiento sobre la aplicación en macOS, iOS, Windows, Linux, Android y la web. Mide fotogramas y commits, y Android también muestra el tiempo de ida y vuelta del texto nativo. Las compilaciones de desarrollo incluyen el cliente de desarrollo; mida con `-release`, que compila igual que `nsgo build`.

## Idiomas y aspecto

```sh
go tool nsgo dev -lang zh-Hans
go tool nsgo dev -lang en-XA   # accented and longer, to find untranslated and cut text
go tool nsgo dev -lang ar-XB   # right to left
```

`-lang` inicia la aplicación en una configuración regional, incluida una pseudoconfiguración regional ([Localización](localization.md)).

## Agentes

```sh
go tool nsgo dev -automation
go tool nsgo dev -target android -automation
go tool nsgo dev -target web -automation
```

`-automation` permite a un agente de IA o a un ejecutor de extremo a extremo usar la aplicación en ejecución como lo hace una persona, en macOS, Windows, Linux (`-target native`), Android (`-target android`), la web (`-target web`), miniprogramas en los simuladores de sus herramientas de desarrollo (`-target miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay` o `miniprogram_douyin`) y dispositivos y simuladores de Apple (`-target ios`, `tvos`, `watchos`, `ios_simulator`, `tvos_simulator` o `watchos_simulator`) hasta ahora: listar sus ventanas, describir lo que muestra una, pulsar, escribir, enviar teclas, desplazar, arrastrar, pasar el puntero, elegir elementos de menú, responder alertas, esperar lo que aparece y tomar capturas de pantalla. Los agentes acceden mediante MCP a través de `go tool nsgo mcp`, que usa el `dev -automation` en ejecución para el directorio, o inicia uno con las mismas banderas (como `-target android`) y lo detiene cuando el cliente sale:

```sh
claude mcp add nsgo -- go tool nsgo mcp
```

Otros clientes aceptan el mismo comando, `go` con los argumentos `tool nsgo mcp`, ejecutado en el directorio de la aplicación. Las vistas se nombran con los selectores de [pruebas](testing.md#una-prueba), o con las referencias `view:` que imprime una instantánea, que un guardado vuelve obsoletas al reiniciar la aplicación.

El agente actúa sobre los datos reales de la aplicación, tal como los ejecuta `dev`. Solo las compilaciones `dev -automation` lo incluyen, y solo llegan a él herramientas en esta máquina que posean el token en `.nsgo/automation.yaml`, que únicamente usted puede leer.

### Herramientas MCP definidas por la aplicación

Llame a `automation.Register` desde una aplicación o biblioteca en Go en la goroutine del motor. Por ejemplo, esta función entrega a los agentes el ID de la escena mostrada actualmente.

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

El nombre debe ser único en el grafo de la aplicación y distinto de los nombres integrados. `nsgo mcp` enumera la herramienta hasta que se limpia el ámbito. El controlador se ejecuta en la goroutine del motor, por lo que puede leer o establecer señales. Recibe un objeto JSON, comprueba los argumentos que necesita y devuelve un valor codificable en JSON o un error.

En una prueba, capture el ámbito al iniciar la aplicación, llame a `automation.Call(cx, ctx, name, args)` dentro de `app.Post` y luego use `app.Settle()` antes de comprobar el resultado o la vista. El registro está activo en `dev -automation` y pruebas, y no hace nada en compilaciones de lanzamiento.

## Banderas de todas las compilaciones

Toda compilación de una aplicación, incluidos los paquetes, acepta las propias banderas de NSGo al iniciarse, que prevalecen sobre `nsgo.yaml` y el código de la aplicación y nunca llegan a las banderas propias de la aplicación:

| Bandera | Efecto |
| --- | --- |
| `--nsgo-fps` | Muestra el monitor de rendimiento en macOS, iOS, Windows, Linux, Android y la web |
| `--nsgo-lang=zh-Hans` | Inicia la aplicación en una configuración regional, incluida una pseudoconfiguración regional |
| `--nsgo-appearance=dark` | Inicia la aplicación en `light`, `dark` o siguiendo el `system` |
| `--nsgo-os=ios` | Ejecuta una compilación web como `macos`, `windows`, `linux`, `ios` o `android`, siendo `web` ella misma |

```sh
./Hello --nsgo-fps                                   # Windows and Linux
open Hello.app --args --nsgo-fps                     # macOS
xcrun simctl launch booted dev.example.hello --nsgo-fps
adb shell am start -n dev.example.hello/dev.oneo.nsgo.NsgoActivity --ez nsgo-fps true --es nsgo-lang de
```

En la web van en la consulta de la página: `?nsgo-fps&nsgo-lang=de`.

`--nsgo-os` previsualiza el diseño de otra plataforma en un navegador: la aplicación lee ese SO y su estilo de dispositivo, teléfono para iOS y Android y escritorio en caso contrario, y `Env.Simulated` informa verdadero. El navegador no tiene ninguna de las partes nativas de esa plataforma: `NavHeader` y la barra de pestañas dibujada ocupan el lugar de las del sistema, no hay menú de aplicación, no hay menú emergente nativo (`desktop.PopupMenu`) ni funciones de ventana, y `mobile.IsAvailable` y `desktop.IsAvailable` son falsos. Los menús dibujados como `ui.ContextMenu` y `ui.Menu` siguen funcionando. Los atajos siguen el teclado del visitante. Las demás compilaciones ignoran la bandera y registran una advertencia.
