---
title: Componentes y estado
description: Elementos, componentes, señales y estilos.
group: Conceptos básicos
order: 50
---

## Elementos

La pantalla es un árbol de elementos, que son valores de estructuras de Go. `core` contiene las primitivas: `View`, `Text`, `Image`, `Scroll`, `Input` y más. `ui` contiene los controles que usa una aplicación, como `Button`, `Select`, `DatePicker` y `EmojiPicker`, con un aspecto que sigue al tema. (`base` contiene los mismos controles sin estilo, para [autores de componentes](authoring/index.md).)

```go
core.View{
	Style: core.Style{Padding: core.All(24), Gap: 12},
	Children: []core.Node{
		core.Text{Content: "Hello"},
	},
}
```

`core.WebView` muestra contenido web; un `WebSource` vacío carga `about:blank` e informa los eventos habituales de carga y navegación. Establezca `SourceFunc` con una fuente vacía o llame a `Ref.Load(core.WebSource{})` para limpiar la página anterior. Establezca `InjectedJavaScriptBeforeContentFunc` cuando el script anterior a los propios scripts de la página siga una señal: cuando cambia, la vista vuelve a cargar su fuente con el nuevo script, en lugar de la página que muestra, incluso si el usuario o `Ref.Load` navegaron hasta ella. Establezca `UserAgentSuffix` (TSX `userAgentSuffix`) para añadir un token, como `Version/26.0 Safari/605.1.15`, después del agente de usuario propio de la plataforma, o después de `UserAgent` cuando esté definido; en macOS e iOS un sufijo añadido al propio de la plataforma solo surte efecto al crear la vista por primera vez, y HarmonyOS y la web conservan su propio agente de usuario. Adjunte un `core.Ref` para llamar a `Reload`, `GoBack` y `GoForward`. En destinos nativos, estos comandos mantienen las fuentes HTML y de datos propias de la aplicación exentas de `core.AccessWebSchemes`; las páginas que crean nuevas navegaciones siguen sujetas a la política de acceso.

En tvOS, `cx.Env().HasWebView()` es falso. Montar un `WebView` informa un `*core.WebError` a través de `OnError` sin cargar WebKit; proporcione contenido de la aplicación para ese destino.

En macOS, iOS, Windows y Linux, `OnNavigationStateChange` también informa cambios de URL dentro del mismo documento, historial y título.

Establezca `WebView.Media` en `core.WebMicrophone`, `core.WebCamera` o ambos para permitir las solicitudes `getUserMedia` de una página. Declare las descripciones de uso en `apple.info` y `RECORD_AUDIO` o `CAMERA` en `android.permissions`; el sistema seguirá preguntando al usuario. En Android, una solicitud pendiente se rechaza si la aplicación elimina uno de los dispositivos solicitados, la página cambia o la vista se cierra mientras el aviso del sistema está abierto. La página debe volver a solicitar acceso después de un cambio así.

`WebView.Muted` y `MutedFunc` (TSX `muted`) silencian todo el audio de la página en Windows y Linux, incluidos Web Audio y los marcos incrustados. macOS 12 e iOS 15 o posteriores suspenden la reproducción multimedia mientras están silenciados. Otros hosts mantienen su reproducción habitual. Las actualizaciones se aplican a la página actual y se conservan entre navegaciones.

En TSX, un cambio de `injectedJavaScriptBeforeContentLoaded` vuelve a cargar la fuente con el nuevo script en la misma vista web, tal como hace `InjectedJavaScriptBeforeContentFunc`; un cambio de `injectedJavaScript` crea una nueva vista web, que carga la fuente de nuevo, tal como hace un cambio de `userAgent`, `userAgentSuffix` o `mobile`. En HarmonyOS, un cambio de cualquiera de los scripts inyectados crea una nueva vista web e inicia un nuevo historial nativo. En Windows y HarmonyOS cada script inyectado se ejecuta en el marco principal dentro de un bloque, por lo que sus declaraciones de nivel superior `let`, `const` y `class` permanecen locales a él; asigne a `window` para un valor global que la página deba ver.

Establezca `Mobile` (TSX `mobile`) para mostrar una página como la ve un teléfono en Windows, macOS y Linux, para sitios como YouTube Shorts que cambian de vídeo con deslizamientos verticales solo en teléfonos: la página recibe el agente de usuario de un teléfono salvo que `UserAgent` esté definido y una pantalla táctil sin puntero flotante, y un arrastre con el ratón, o un desplazamiento con rueda o panel táctil sobre lo que no se desplaza, le llega como el deslizamiento de un dedo. Windows emula un teléfono como lo hace el modo de dispositivo de Chrome. macOS y Linux lo hacen con un script, por lo que las hojas de estilo de sus páginas siguen viendo un ratón y los eventos táctiles no son de confianza, lo cual una página puede ignorar. iOS, Android, HarmonyOS, la web y los miniprogramas muestran las páginas tal como son.

En macOS, iOS, Windows y Linux, `OnOpenWindow` recibe las solicitudes de nueva ventana desde una página web. La vista no crea ninguna ventana emergente. Use `URL` y `UserInitiated` de la solicitud para decidir si navegar en la misma vista con `Ref.Load`, abrir la URL en otro lugar o ignorarla. Una vista restringida envía las solicitudes rechazadas a `OnError`.

Establezca `OnDownload` para guardar archivos que descargan las páginas de una vista web en macOS, iOS, Linux, Windows y Android. Las descargas se guardan en `downloads/<id>/<file name>` dentro del directorio de datos de la aplicación e informan su inicio, finalización con conteo de bytes o fallo; las vistas sin `OnDownload`, o las vistas restringidas, cancelan las descargas.

`core.Video` reproduce un archivo o URL en el reproductor de la plataforma. En macOS, iOS y tvOS, establezca `Video.AudioSource` en un archivo o URL independiente que contenga sonido para un clip de vídeo finito. El audio del reproductor proviene de esa fuente. La imagen y el sonido comienzan juntos, y `Ref.Seek` mueve ambos en la misma línea de tiempo. El vídeo define la duración de la reproducción, y un audio más corto queda en silencio cuando termina.

Un clip transparente muestra lo que hay detrás como HEVC con alfa en un `.mov` en macOS (iOS aún no está comprobado), y como VP9 con alfa en WebM en Linux con el complemento `codecalpha` de GStreamer (`gstreamer1.0-plugins-bad`, que un paquete de Linux puede recomendar en `nsgo.yaml`). En macOS, iOS, Windows y Linux, un clip local cuyo canal alfa no pueda conservarse (WebM en plataformas de Apple, HEVC en Linux y cualquiera en Windows hasta que la salida alfa de Media Foundation se haya comprobado en una máquina Windows) hace que `OnError` lo indique en lugar de mostrar el vídeo en un recuadro opaco: el error es un `*core.MediaError` cuyo `Reason` es `core.MediaErrorAlphaUnsupported`, para que la aplicación pueda decodificar ese clip por sí misma en su lugar. Android y la web, y las URLs remotas en todas las plataformas, reproducen esos clips tal como los genera su decodificador, que puede ser opaco.

`OnPause` y `OnPlay` siguen los controles integrados en Apple, Android, Windows, Linux y la web. `OnPause` entrega `VideoPauseControls` para una pausa desde un control. `Paused` se aplica cuando se abre una fuente o cambia su valor. Los cambios de volumen, velocidad y diseño conservan una pausa o reproducción nativa, incluida una reproducción desde los controles mientras `Paused` sigue siendo verdadero. Enlace `PausedFunc` a su estado y actualícelo en esas devoluciones de llamada cuando su aplicación muestre su propio botón de reproducción.

`Rate` establece la velocidad de reproducción y `RateFunc` puede cambiarla durante la reproducción. Use `VideoInfo.RateChangeable` de `OnLoad` para mostrar un control de velocidad. Linux puede cambiar la velocidad de los vídeos con búsqueda cuando GstPlay y el filtro `scaletempo` de GStreamer están instalados. El reproductor de respaldo usa 1×.

Establezca `Video.Headers` para enviar encabezados de solicitud HTTP como `Referer`, `User-Agent` o `Cookie` en Android y para archivos HTTP directos como MP4 o MOV en Linux, macOS, iOS y tvOS.

```go
core.Video{
    Source:  "https://video.example/clip.mp4",
    Headers: map[string]string{"Referer": "https://video.example/player"},
}
```

El vídeo copia el mapa al montarse y usa los mismos encabezados si `SourceFunc` suministra una nueva URL. En plataformas de Apple, use una fuente con longitud en bytes conocida y soporte de rangos para buscar. El reproductor envía los encabezados configurados a través de redirecciones del mismo origen. Los mismos encabezados también se aplican a un archivo `AudioSource` HTTP directo en plataformas de Apple. Las URLs de listas de reproducción terminadas en `.m3u` o `.m3u8` usan el cargador nativo.

En Linux, instale GstPlay y los complementos `scaletempo` y `souphttpsrc` de GStreamer. Use la URL multimedia final si establece `User-Agent` o si su `souphttpsrc` carece de la propiedad `location-trusted`. Con esa propiedad y el agente de usuario predeterminado, GStreamer conserva los encabezados de la aplicación en redirecciones del mismo origen. Los encabezados de la aplicación se aplican al archivo fuente, y los segmentos multimedia adaptativos usan sus propias solicitudes.

## Componentes

Un componente es una función que recibe un ámbito y devuelve un elemento. Se ejecuta una vez; lo que cambia después lo impulsan las señales.

```go
func Greeting(cx *core.Scope) core.Node {
	return core.Text{Content: "Hello from NSGo"}
}
```

Un componente con propiedades es un tipo con un método `Render`:

```go
type Badge struct{ Text string }

func (b Badge) Render(*core.Scope) core.Node {
	return core.Text{Content: b.Text}
}
```

`ui.ToastContent` puede omitir su botón de cierre con `NoClose`, añadir un nodo `Trailing` y descartar la notificación mediante `OnPress`. `Toasts.NoClose` oculta los botones de cierre en toda la pila.

`ui.Toasts.Style` y `StyleFunc` personalizan cada superficie de notificación, incluidos su fondo, borde, esquinas y relleno. El estilo de la pila se fusiona sobre el tema, seguido por cada `ToastContent.Style` y `StyleFunc`. Cada `StyleFunc` se ejecuta después de su `Style` correspondiente y se actualiza cuando cambian sus señales. Un `Color` en estos estilos colorea el título, y la descripción con opacidad reducida, para que el texto siga siendo legible sobre una superficie oscura o coloreada. Las fases de entrada, salida y deslizamiento siguen aplicando sus estilos de movimiento. En TSX, establezca la propiedad `style` de `Toasts` y pase `style` o `styleFunc` a `show` para una notificación individual.

## Señales

Una señal guarda un valor. Leerla dentro de una función a la que llama un elemento, como `ContentFunc`, suscribe esa parte de la pantalla, y solo esa parte se actualiza cuando cambia el valor.

```go
count := core.NewSignal(cx, 0)

core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }}

ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }}
```

Una señal de un slice completo actualiza cada fila que lo lee. `core.NewCollection` guarda elementos por clave, cada uno en su propia señal, de modo que una fila sigue su propio elemento y cambiar un elemento ejecuta una sola fila:

```go
todos := core.NewCollection(cx, func(t Todo) int { return t.ID }, nil)

core.List{Count: todos.Len, Key: func(i int) any { return todos.KeyAt(i) },
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		return core.Text{ContentFunc: func() string { t, _ := todos.At(row.Index()); return t.Title }}
	}}

todos.Update(id, func(t Todo) Todo { t.Done = true; return t })
```

Otros tipos de estado se construyen sobre señales de la misma manera:

- `core.NewSelector` indica si una clave es la seleccionada y ejecuta solo las filas que una selección deja y alcanza.
- `core.NewHistory` mantiene un valor con deshacer y rehacer, con sus señales `CanUndo` y `CanRedo` para un menú Editar.
- `core.NewPersisted` guarda un valor tipado en el almacenamiento de la aplicación como JSON versionado, migrando lo que escribieron versiones anteriores.
- `core.From` sigue una fuente externa al grafo, como una suscripción que entrega valores en otra goroutine.

## Estilos

`core.Style` sigue a CSS: flexbox y grid, rellenos y márgenes, esquinas, bordes, sombras, degradados, transformaciones y estados como `Active`. Los elementos también aceptan clases de Tailwind mediante `Class`:

```go
core.Text{Content: "Done", Class: "self-start rounded-full bg-zinc-100 px-3 py-1 text-sm"}
```

Los temas dan a los componentes de `ui` su aspecto en claro y oscuro.

## Aplicaciones de escritorio

`nsgo.Run(Root)` muestra una única vista raíz. En su lugar, una aplicación de escritorio puede abrir lo que muestra por sí misma: `nsgo.Run(App)` ejecuta `func App(app *core.Scope)` una sola vez y abre ventanas, paneles, iconos de bandeja y atajos globales con `desktop`, que comparten sus señales. Esto funciona en macOS, Windows y Linux; en macOS `examples/tray` permanece fuera del Dock y vive en la barra de menús. `nsgo.Run(App, nsgo.StartHidden())` abre la ventana principal sin mostrarla, de modo que una aplicación de bandeja, o una iniciada por el sistema al iniciar sesión (`desktop.LaunchedAtLogin`), no la hace parpadear; la aplicación la muestra con `app.MainWindow().Show()`. Los teléfonos y la web muestran la aplicación al iniciarla.

`desktop.SetWindowLevel` mantiene una ventana por encima de las demás, como hace un temporizador o una mascota de escritorio, o por debajo de ellas, y `desktop.SetWindowFrame` la mueve y redimensiona. `desktop.CanSetWindowLevel` y `desktop.CanPlaceWindows` indican si surten efecto en el ámbito de una ventana: ambos lo hacen en macOS, Windows y Linux bajo X11, y ninguno para una ventana principal ordinaria de Wayland, cuyo compositor la apila y ubica.

Una ventana abierta con `desktop.WindowOptions{Transparent: true}`, o la ventana principal con `nsgo.WindowTransparent()`, no tiene fondo ni marco, de modo que el escritorio se ve donde la aplicación no dibuja nada, como en una mascota de escritorio o un widget; una vista con `Desktop.WindowDrag` la mueve. `NoShadow` elimina la sombra del sistema y `SkipTaskbar` mantiene la ventana fuera de la barra de tareas de Windows, de Alt+Tab y de la lista de tareas de X11, y `desktop.SetWindowTransparent`, `SetWindowShadow` y `SetSkipTaskbar` los cambian después. Windows solo cambia la transparencia en una ventana abierta como transparente, que no tiene sombra, y Wayland no tiene ninguna indicación para mantener una ventana fuera de su lista de tareas. `desktop.CanMakeWindowsTransparent` y `desktop.CanSkipTaskbar` indican en el ámbito de una ventana si estos cambios surten efecto: la transparencia requiere DirectComposition en Windows y un compositor en Linux, ya que X11 sin compositor no tiene transparencia, y en macOS `desktop.SetDockVisible` quita la aplicación del Dock.

## Terminal

Importe `base/terminal` y cree una sesión con `terminal.New(terminal.Options{Conn: conn})` para un flujo remoto, o establezca `Command` para un programa local en macOS, Linux o Windows. Renderice `terminal.View{Terminal: session}` y llame a `Close` cuando finalice la sesión. `Options.ImageLimit` limita el almacenamiento gráfico de Kitty en bytes: cero usa 64 MiB y un valor negativo desactiva las imágenes. Los programas pueden enviar imágenes RGB, RGBA o PNG y ubicaciones mediante marcadores Unicode.

Llame a `Search(text)` para resaltar coincidencias en la pantalla y el historial de desplazamiento, `SearchNext` y `SearchPrevious` para seleccionarlas, y `EndSearch` para limpiarlas. `Options.OnSearch` informa `SearchStatus`; `Theme.Search`, `SearchText`, `SearchSelected` y `SearchSelectedText` definen los colores de coincidencia. Los intérpretes de comandos que emiten marcadores OSC 133 admiten Alt/Opción-clic dentro de su entrada de comando activa para mover el cursor. NSGo no instala scripts de integración con el intérprete de comandos.
