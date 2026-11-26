---
title: Compilación y distribución
description: Empaqueta una aplicación para cada plataforma, crea instaladores, fírmala y notarízala, publícala en GitHub e incrústala en otras aplicaciones.
group: Herramientas
order: 22
---

## Compilar

```sh
go tool nsgo build
```

`build` escribe un paquete de lanzamiento optimizado para cada destino que enumera `nsgo.yaml`, en `dist/<name>-<version>-<target>`: un `.app`, un `.ipa`, un `.apk`, los directorios de Windows y Linux, o un sitio web cuya página obtiene cada módulo TSX que nombra un `import()` cuando dicha importación se ejecuta. `-target` reemplaza la lista, y `all` significa todos los destinos que solo requieren Go y Xcode; añade `android_x64` cuando esté instalado un Android NDK, y `miniprogram`, o únicamente `miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay` o `miniprogram_douyin`, para [miniprogramas](#miniprogramas), y `harmonyos` para una vista previa de [HarmonyOS](#harmonyos). `-run` inicia el paquete compilado para esta máquina, o lo instala en el iPhone, simulador o dispositivo Android que elija `-device`.

Todo lo que se distribuye termina en `dist/releases`, el único directorio en el que hay que mirar:

| En `dist/releases` | Qué es |
| --- | --- |
| `*.dmg`, `*-setup.exe`, `*.deb`, `*.pkg.tar.gz`, `*.AppImage` | [Instaladores](#instaladores) para macOS, Windows y Linux |
| `*.AppImage.zsync` | Controles de actualización delta para AppImageUpdate y lanzadores compatibles |
| `*-ios.ipa`, `*-tvos.ipa`, `*.apk` | Los paquetes de iOS, tvOS y Android, para tiendas o dispositivos |
| `*.pkg`, `*.msix`, `*.aab`, `*.flatpak`, `*.tar.gz` y `flatpak/` | Los [paquetes de tienda](#paquetes-de-tienda) de una compilación `store` |
| `install.sh`, `install.ps1` | Scripts que instalan la aplicación desde los [lanzamientos de GitHub](#publicar-en-github) |
| `appcast-<target>.xml` | Los feeds que leen las aplicaciones que se [actualizan](#actualizaciones) |
| `homebrew/<name>.rb`, `winget/` | [Manifiestos de Homebrew cask y winget](#canales) para distribución directa |
| `*-web.zip` | El sitio web, listo para descomprimir y servir tal cual |
| `miniprogram/<platform>/` | El proyecto de cada miniprograma, para subirlo desde su herramienta de desarrollo |

Cada archivo se denomina `<name>-<version>-<system>-<arch>`, como `hello-1.2.0-windows-x64-setup.exe`, para que los usuarios encuentren el de su sistema en la página de lanzamientos, y tiene junto a él su `.sha256`. Una compilación de otra versión vacía primero el directorio; las compilaciones de la misma versión, incluso en otras máquinas, se añaden a él. Los paquetes de simuladores y de watchOS permanecen en sus directorios `dist/<name>-<version>-<target>`.

El sitio web incluye `.wasm.br` y `.wasm.gz` cuando son más pequeños que el módulo original. Habilita archivos precomprimidos en los servidores que los admitan. Sírvelos en la URL `.wasm` original con `Content-Type: application/wasm`, el `Content-Encoding` correspondiente y `Vary: Accept-Encoding`. El módulo original funciona en cualquier servidor estático. [Publicación en la web](web.md) cubre encabezados semánticos, enlaces de navegador, metadatos por página y carga desde CDN.

### Paquetes web sin conexión

Establece `web.offline: true` en `nsgo.yaml` para almacenar en caché una versión web en su primer inicio con conexión.

```yaml
web:
  offline: true
```

Sirve el directorio publicado a través de HTTPS o usa `go tool nsgo build -target web -run` en localhost. El service worker almacena en caché la página, WebAssembly, los scripts y los recursos empaquetados. Las visitas con conexión cargan la versión publicada. Una vez completada la instalación y cerradas las pestañas de la versión anterior, la última versión puede iniciarse desde su caché. Las peticiones a los servicios en línea de una aplicación siguen necesitando conexión.

## Terminal

Ejecuta la aplicación en una terminal interactiva con `go tool nsgo dev -target tui`. `go tool nsgo test -target tui` ejecuta sus pruebas allí a través de `nsgotest.Main`, una aplicación por proceso de prueba. La terminal controla el tamaño: una celda representa 8 por 16 puntos de disposición. Elige explícitamente los estilos de tu aplicación; los componentes conservan sus formas y tamaños habituales. `examples/tui` muestra campos, botones, atajos y cambio de tamaño.

Compila un ejecutable de consola con `go tool nsgo build -target tui_macos_arm64`, o el correspondiente `tui_macos_x64`, `tui_windows_arm64`, `tui_windows_x64`, `tui_linux_arm64` o `tui_linux_x64`. El grupo de compilación/comprobación `tui` incluye los seis, y `all` también los incluye. Cada directorio de paquete contiene el ejecutable y los recursos; no tiene instalador gráfico. Los seis se compilan solo con Go, salvo que una compilación de terminal para macOS que enlaza un paquete con bibliotecas de macOS declaradas las firma de forma ad hoc en un Mac; en otros sistemas las incluye tal como se publicaron y exige que una de arm64 ya esté firmada.

El backend admite texto en celdas, fondos, bordes sólidos, recorte, desplazamiento, foco y edición de texto. Usa la fuente y los colores de la terminal; Control es `mod`, incluso en Mac. Ctrl+C copia dentro de un editor y sale cuando ningún enlace o editor lo consume. El pegado entre corchetes inserta texto sin ejecutar atajos, y el portapapeles es local al proceso. El lienzo, las imágenes, los medios, los componentes nativos alojados, los menús/alertas del sistema, las animaciones, el zoom, el moldeado bidireccional, la elipsis inicial/media y un puente con el portapapeles del sistema aún no están implementados. `base/terminal` sigue siendo el control de terminal incrustado en una aplicación gráfica.

Mientras `nsgo dev` controla la terminal, los diagnósticos de la herramienta y de la aplicación se escriben en `.nsgo/tui.log`. Al salir se restauran la pantalla, el cursor y los modos de la terminal antes de volver al shell. Las capturas de pantalla de pruebas en terminal rasterizan las celdas con una fuente portátil fija en lugar de capturar el emulador; usa aserciones de texto para caracteres Unicode que falten en esa tipografía.

## nsgo.yaml

```yaml
name: Hello
id: dev.example.hello   # the bundle ID; without it, dev.nsgo. and the name's letters and digits
version: 0.1.0
build_number: 1         # CFBundleVersion and Android's versionCode; else from version
# copyright: © 2026 Example  # the Windows executable's details and installer, and the macOS About panel
# appearance: dark      # light or dark whatever the system's
# theme: theme.json     # a TSX app's theme: NSGo's or a shadcn/ui one
# i18n: { dir: locales, source: en }  # the app's catalogs and the language they are written in
icon:
  apple: assets/AppIcon.icon  # macOS, iOS and watchOS, or an .icns for macOS
  windows: assets/IconWindows.png  # optional Windows artwork
  linux: assets/IconLinux.png      # optional Linux artwork
  png: assets/Icon.png        # every other target
  tvos: assets/AppIconTV.brandassets  # the Apple TV's layered icon and top shelf art, or a wide PNG
  watchos: assets/AppIconWatch.icon   # the Apple Watch's own, drawn in circles, or a square PNG
  android:
    png: assets/IconAndroid.png  # optional bitmap; valid without adaptive layers
    foreground: assets/IconForeground.png  # Android's adaptive icon: 432 px, the logo within the center 264 px
    background: "#1F1F1F"                 # a color or a square PNG
    monochrome: assets/IconMonochrome.png  # optional: the shape Android 13's themed icons tint
dev:
  target: native        # native, web, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android, harmonyos or miniprogram_<platform>
  hot_reload: true      # false builds once, watching no files
  # icon: { png: assets/Icon-dev.png }  # replaces icon's files in dev's builds; defaults to assets/*-dev.*
build:
  output: dist
  tags: []              # Go build tags every build, test and check of the app takes, such as timetzdata
  ldflags: []           # the app's own linker flags, such as -X main.releaseDate=2026-11-17
  open: false           # true opens the output directory after building
  installers: true      # false leaves out the disk image, the Windows installer and the Linux packages
  jobs: 0               # how many targets build at once; 0 is half the CPUs, 1 one after another
  targets: [macos_arm64, macos_x64, windows_arm64, windows_x64, linux_arm64, linux_x64, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android_arm64, web]
# apple: { sign_in_with_apple: true }  # the Sign in with Apple entitlement, for iOS, tvOS and Mac App Store builds
# apple: { purchases: true }  # StoreKit's library, which in-app purchases go through, in macOS, iOS and tvOS packages
# apple: { privacy: { accessed_apis: { NSPrivacyAccessedAPICategoryDiskSpace: [E174.1] }, tracking: false } }  # add required-reason APIs and tracking declarations to PrivacyInfo.xcprivacy
# apple: { requires_full_screen: false }  # let an iPad run the app in Split View, Slide Over and Stage Manager, in all four orientations
# apple: { orientations: [portrait, landscape_left] }  # the orientations a full-screen app takes on an iPad
# apple: { multiple_scenes: true }  # let an iPad open more than one window of the app, each showing the main view with its own state
# apple: { info: { UIBackgroundModes: [audio] } }  # background playback for iOS Picture in Picture
# android: { permissions: [REQUEST_INSTALL_PACKAGES] }  # manifest permissions beyond the runtime's
# android: { recording: true }  # the microphone service microphone.RecordWAV records in, with RECORD_AUDIO, the foreground-service permissions and POST_NOTIFICATIONS, which RecordWAV asks for so its notification shows from Android 13; a refusal still records
# android: { uses_cleartext_traffic: true }  # allow plain HTTP to any host in the APK and App Bundle
# android: { background_downloads: true }  # dataSync foreground service for cx.Download
# android: { background_motion: true }  # explicit finite motion shortService; streams also need a notification title
# android: { loopback_cleartext: true }  # allow plain HTTP to loopback in the APK and App Bundle
# commands: [{ name: hello-cli, package: ./cmd/hello-cli }]  # command-line programs the desktop packages ship beside the app: see Commands
# windows: { command_line: true }  # add <name>.com, a console companion that forwards arguments and streams to <name>.exe; not in store builds
# windows: { path: true }  # the installer puts the app's folder on the user's Path, so consoles find its commands
# windows: { publisher: Example Ltd }  # the company the Windows executable's details, its installer and the uninstall entry name; else windows.store.publisher_name
# linux: { maintainer: "Jane Doe <jane@example.com>" }  # the Linux packages' maintainer, else the app's name
# linux: { categories: [Office, Dictionary] }  # the freedesktop menu categories of the app's desktop entry, at least one a main category such as Office, Utility or Game, else menus file it under Other; your own start with X-
# linux: { share: packaging/share }  # files the .deb and Arch package install under /usr/share and the AppImage in its usr/share, such as metainfo/<id>.metainfo.xml or bash-completion/completions/<command>
# linux: { depends: { deb: [gstreamer1.0-plugins-good], arch: [gst-plugins-good] }, recommends: { deb: [gstreamer1.0-libav], arch: ["gst-libav: H.264 and AAC video"] } }  # packages the .deb and Arch package also depend on and recommend, after GTK 4 and WebKitGTK 6, each as dpkg or pacman writes it
# release: { github: me/hello }  # the GitHub repository the install scripts and updates install from, else the Git remote origin's; tag_prefix: v by default
# updates: { key: <public key> }  # how the app updates, by platform and channel: see Updates
# store: { dir: store }           # store listings directory (YAML and Markdown per language)
# miniprogram: { appid: { wechat: wx0123456789abcdef, alipay: "2021000000000000" }, custom_navigation: false }  # the mini programs'
```

Un nombre de sistema operativo sin arquitectura en `targets` —o en `nsgo build -target` y `nsgo check -target`— equivale a sus arquitecturas (`windows` es `windows_arm64` y `windows_x64`; `android` es solo `android_arm64`, ya que la compilación x86_64 necesita un NDK). `macos_universal`, que `macos` y `all` omiten, compila ambas arquitecturas de macOS y las une en una sola aplicación, incluidas sus bibliotecas y widgets. El canal de Steam usa el paquete de aplicación universal. Las compilaciones de instalador lo empaquetan en una sola imagen de disco, con un único feed de Sparkle en el canal directo. `nsgo test -target` también admite el nombre de la arquitectura para la que se ejecutan sus pruebas: la de un sistema de escritorio en la arquitectura de esta máquina y la de Android en `arm64`, de modo que `windows_x64` ejecuta el destino `windows` en una máquina Windows x64, mientras que una arquitectura para la que el destino no se ejecuta se rechaza indicando el nombre correcto.

Las marcas reemplazan los valores que nombran, como `-target`, `-version` o `-output`; `-h` las enumera todas.

`build.ldflags` contiene las marcas de enlazador propias de la aplicación, escritas tal como las acepta `-ldflags` de go build, como `-X main.releaseDate=2026-11-17` o `-X 'main.channel=beta builds'`. Toda compilación que enlaza la aplicación las aplica: `dev` y sus recargas, `build` también con `-embed` y `test` también con `-target`, en todos los destinos. `-ldflags` en `build` y `test` se añade a ellas en lugar de reemplazarlas, y puede repetirse para añadir más. Van antes de las marcas `-X` propias de la herramienta, y el enlazador conserva la última de cada variable, de modo que el nombre, el ID y la versión en `core.App` sigan siendo los de `nsgo.yaml`. La herramienta pasa a Go su propio `-ldflags`, que reemplaza al de `GOFLAGS`, por lo que las marcas de enlazador pertenecen aquí; el `-ldflags` de un `test` sin interfaz gráfica después del directorio pertenece a go test y las reemplaza todas.

Cada comando de Go que ejecuta la herramienta para la aplicación usa `build.tags`, en `dev` y sus recargas, en `build` también con `-embed`, en `test` también con `-target` y en `check`, en todos los destinos. Se añaden las etiquetas propias de NSGo, como `nsgodev` en `dev` o `nsgotest` en pruebas de destino, así como las etiquetas de `-tags` en `GOFLAGS`, que de otro modo serían reemplazadas; el resto de `GOFLAGS` se aplica tal cual.

`apple.info.LSMinimumSystemVersion` eleva la versión mínima del paquete macOS, y `apple.info.MinimumOSVersion` eleva la del paquete iOS, tvOS o watchOS seleccionado. Escribe cadenas de versión entre comillas, como `"14.2"` o `"18.0"`, con una a tres partes numéricas. Los mínimos del entorno de ejecución son macOS 13.0 (Go 1.27), iOS 15.0, tvOS 17.0 y watchOS 26.0; valores inferiores o mal formados producen un error. La clave móvil debe cumplir el mínimo de cada destino móvil seleccionado. Los App Clips conservan su propio `app_clip.minimum`. Los valores llegan a los metadatos del paquete, la compilación de activos, los proyectos de firma y el enlazador de la aplicación; los frameworks incrustados de iOS también los utilizan, con Mac Catalyst al menos en 16.0 para el entorno de ejecución de macOS 13 de Go.

`icon.windows` e `icon.linux` seleccionan archivos PNG cuadrados para sus paquetes de aplicación, material gráfico de instaladores y logotipos de tienda. Sus valores predeterminados son `assets/IconWindows.png` y `assets/IconLinux.png`, y si no existen recurren a `icon.png`; `dev.icon` acepta la misma configuración y archivos predeterminados `-dev`. `icon.android.png` selecciona un mapa de bits de Android de forma independiente, incluso cuando no hay capas adaptativas configuradas. Los demás destinos conservan el recurso alternativo compartido `icon.png`.

El icono de Apple TV es una tarjeta por capas, no un cuadrado: `tvos` acepta el `.brandassets` de Xcode, sus pilas `App Icon` e imágenes de estante superior, o una imagen ancha como 1280×768, que la herramienta dibuja ajustada a la tarjeta sobre el color de sus bordes y centrada en las imágenes del estante superior. Sin ninguno de los dos, el PNG se sitúa a tres cuartos de la altura de la tarjeta. Apple Watch recorta su icono en círculo, por lo que `watchos` acepta un `.icon` diseñado para los círculos de watchOS o un PNG cuadrado; sin él, la aplicación usa `apple` y luego `png`. Ambos recurren por defecto a `assets/AppIconTV.*` y `assets/AppIconWatch.*`, igual que hacen `apple` y `png`.

En Android, `icon.android.foreground` y `background` crean un icono de lanzador adaptativo, de modo que los lanzadores lo enmascaran con su forma y lo mueven como prefieran en lugar de reducir `png` dentro de un marco propio. Cada capa es un cuadrado de 108 dp, un PNG escalado a 432 px; el lanzador muestra los 72 dp centrales (288 px) y los recorta con una forma que puede invadir todo lo que quede fuera de los 66 dp centrales (264 px), así que mantén el logotipo dentro de ese círculo y deja que el fondo rellene todo el cuadrado. `background` es un PNG o un color como `"#1F1F1F"`. `monochrome`, una silueta en la que solo cuenta el canal alfa, es lo que Android 13 y posteriores tiñen cuando el usuario activa los iconos temáticos; sin ella, el icono conserva allí sus colores. El paquete conserva `icon.android.png`, o en su defecto `icon.png`, como icono de mapa de bits plano, que ningún lanzador de Android 9 o posterior —la versión más antigua en la que funciona NSGo— elige por encima del adaptativo; sin `png`, la herramienta lo dibuja superponiendo el primer plano sobre el fondo. Sin capas adaptativas, el lanzador muestra el mapa de bits de Android o el `png` compartido.

`android.uses_cleartext_traffic: true` habilita HTTP plano hacia cualquier host en un APK o App Bundle, estableciendo tanto la marca del manifiesto como su política de seguridad de red. Sin esa habilitación, `android.loopback_cleartext: true` permite HTTP plano únicamente desde `localhost`, `127.0.0.1` y `::1`, como un vídeo servido por la propia aplicación. La política exclusiva de loopback deniega el tráfico en claro a otros hosts; la habilitación para todos los hosts tiene prioridad cuando ambos ajustes son verdaderos. Para un AAR incrustado, configura la política de seguridad de red de Android de la aplicación anfitriona.

Toda compilación acepta también las marcas propias de NSGo al iniciarse, como `--nsgo-fps` ([Ejecución y depuración](debugging.md#banderas-de-todas-las-compilaciones)). En `nsgo dev`, Comando/Control+R recarga desde cualquier ventana abierta y sigue disponible después de cerrar la vista principal cuando otra ventana mantiene en ejecución una función de la aplicación.

### Pantalla de inicio

Configura `launch_screen` para mostrar un fondo opaco, un PNG centrado o ambos en iOS y Android 12 o posterior. Las rutas de imagen son relativas a `nsgo.yaml`. La imagen se ajusta proporcionalmente dentro de 128 × 128 puntos o dp. Un PNG transparente con un lado mayor de al menos 512 píxeles se mantiene nítido en pantallas de alta densidad.

```yaml
launch_screen:
  background: "#F5F1EA"
  image: assets/Launch.png
  dark:
    background: "#201C26"
    image: assets/LaunchDark.png
```

El sistema elige la apariencia salvo que `appearance` la fije. Omite cualquiera de los campos de `dark` para reutilizar el valor correspondiente. Un fondo o una imagen omitidos conservan el valor predeterminado de la plataforma. Las aplicaciones incrustadas obtienen su pantalla de inicio del anfitrión.

## Clases nativas de Android

Enumera clases de Android precompiladas en el `nsgo.yaml` de la aplicación para usar una `View` personalizada desde `core.Native`.

```yaml
android:
  classes: [java/BadgeView.jar, java/Helpers.dex]
```

Las rutas son relativas a `nsgo.yaml` salvo que sean absolutas. Cada ruta designa un archivo `.dex` o un JAR que contenga el `classes.dex` raíz, `classes2.dex` y fragmentos posteriores de D8. Prepara esos archivos con D8 usando `--min-api 28`. En arm64, NSGo compila el APK o App Bundle a partir de estos archivos precompilados solo con Go. Las sesiones de desarrollo en Android recompilan cuando cambia alguno de los archivos listados. Para un AAR incrustado, la compilación Android del anfitrión aporta sus dependencias Java. [Documentación de D8](https://developer.android.com/tools/d8)

El [ejemplo nativo de Android](https://github.com/nsgo-dev/nsgo/tree/main/examples/android-native) registra una clase con un constructor público `Context` y la configura mediante una propiedad de Go.

## Juegos

Establece `game: true` en `nsgo.yaml` para un juego. Android clasifica la aplicación como juego y utiliza las optimizaciones del modo Juego del sistema. macOS e iOS activan el modo Juego, macOS usa de forma predeterminada la categoría Juegos y Linux añade `Game` a la entrada de escritorio. Un valor explícito de `macos.category` o una clave de modo Juego en `apple.info` tiene prioridad.

Establece `gamepad: true` cuando tu aplicación gestione un mando de juego físico. Android declara opcionales tanto el mando como la pantalla táctil. macOS, iOS y tvOS declaran el perfil Extended Gamepad, y tvOS también el Micro Gamepad del Siri Remote, teniendo prioridad las claves de controlador en `apple.info`.

## Avisos de licencia

`nsgo build` incluye `THIRD_PARTY_NOTICES.txt` en los recursos del paquete independiente. Contiene los textos de licencia de los módulos de Go usados por ese destino, el entorno de ejecución de Go y el entorno de ejecución de Java o JavaScript empaquetado por NSGo. Las compilaciones web nombran el archivo según su contenido, igual que otros recursos. Usa `core.ThirdPartyNotices()` para mostrar el mismo texto en una pantalla de créditos dentro de la aplicación. La función devuelve una cadena vacía en desarrollo, pruebas y compilaciones de bibliotecas incrustadas. Las [declaraciones de bibliotecas nativas](native-libraries.md) añaden sus textos de licencia originales a los mismos avisos.

De forma predeterminada, una licencia de terceros desconocida o ausente hace fallar la compilación, y una licencia copyleft detectada emite una advertencia. Los módulos listados en el espacio de trabajo de Go de la aplicación (`go.work`) se tratan como código propio junto al módulo de la aplicación: se incluyen todos los archivos de licencia que contengan y no activan las comprobaciones de terceros. Configura cualquiera de las reglas como `error` o `warn`. La lista `allow` exime rutas de módulo exactas de ambas reglas y conserva cualquier texto de licencia que la compilación haya encontrado.

```yaml
licenses:
  unknown: error
  copyleft: warn
  allow: [example.org/reviewed-module]
```

## Añadir bibliotecas

Añade una biblioteca, es decir, un módulo de Go, con `nsgo add`, que es un `go get` que muestra antes su plan:

```sh
nsgo add github.com/acme/chart           # su versión publicada más reciente, salvo que go.mod ya lo tenga
nsgo add github.com/acme/chart@v1.3.0    # esta versión, aunque sea inferior a la de go.mod
nsgo add -dry-run github.com/acme/chart  # muestra el plan y no escribe nada
```

```text
github.com/acme/chart
  module     github.com/acme/chart v1.3.0, published 2026-11-20
  owner      github.com/acme, the first module from it in go.mod
  next       import "github.com/acme/chart"

example.com/hello
  go.mod     + require github.com/acme/chart v1.3.0 // indirect
             + require example.com/units v1.0.0 // indirect
             ~ require github.com/nsgo-dev/nsgo v0.4.0 => v0.4.2, a release
  go.sum     + 6 lines
  tools      go tool nsgo then runs v0.4.2, and so does an installed nsgo here
Apply these changes? [y/N]
```

Para cada módulo, el plan indica su versión y su fecha de publicación, y su propietario: el host y, en un servicio de alojamiento de código como github.com, también la cuenta, señalado cuando `go.mod` todavía no tiene ningún módulo suyo. Cuando corresponde, siguen los avisos de retirada (retract) y de obsolescencia, una versión sin etiqueta de release, un `replace` del módulo y la línea de import de su paquete raíz. Después vienen las líneas de `go.mod` que cambian, incluidas `go` y `toolchain`, los comandos `go tool` que pasarían a ejecutar otra versión, los módulos que se obtienen de sus propios hosts y aquellos cuyas sumas acepta `go.sum` sin que las compruebe la base de datos de sumas de verificación. `go get` marca con `// indirect` un módulo que todavía no importa ningún paquete, y `go mod tidy` quita la marca en cuanto alguno lo importa.

En una terminal, `add` pregunta antes de escribir, y `-y` aplica el plan sin preguntar; sin terminal y sin `-y`, no escribe nada y termina con estado 1. Una interrupción, como Ctrl+C, deja `go.mod` y `go.sum` como estaban y termina con estado 130. Un módulo que `go.mod` ya requiere se deja como está y se muestra su versión, salvo que se escriba una versión.

La planificación no ejecuta nada más que el comando go, sobre copias de `go.mod` y `go.sum`:

- con la toolchain que el módulo ya usa y `GOTOOLCHAIN=local`, de modo que no se descarga ni se ejecuta ninguna otra toolchain;
- a través de los proxies de módulos que `GOPROXY` enumera antes de su primer `direct`, que pasa a ser `off`, de modo que solo los módulos que marcan `GOPRIVATE` o `GONOPROXY` se obtienen de sus propios hosts, con `GONOSUMDB` y `GOAUTH` tal como están;
- con `GOWORK=off`, de modo que en un espacio de trabajo planifica `go.mod` solo para el módulo, como lo escribe `GOWORK=off go get`, y lo indica.

Con `-y` o sin él, `add` rechaza:

- mover el requisito de NSGo a una pseudoversión o a una versión preliminar, como haría una biblioteca que requiere un commit de NSGo; `go get github.com/nsgo-dev/nsgo@<versión>` lo mueve allí a propósito antes;
- un módulo que necesita un Go más reciente que la toolchain del módulo; `go get go@<versión>` sube antes la del módulo;
- bajar un módulo mediante una consulta como `@latest`; escribir la versión lo baja, y el plan lo llama una degradación (downgrade).

Escribe `go.mod` y `go.sum` solo si ninguno ha cambiado desde la planificación, cada uno mediante un archivo temporal que se renombra sobre él, y restaura ambos ante un error o una interrupción. En un módulo con un directorio `vendor`, el plan indica `go mod vendor` como siguiente paso.

`add` es nuevo después de NSGo 0.3.4. En un módulo que fija la 0.3.4 o una versión anterior, un `nsgo` instalado no ejecuta para ello el `nsgo` del módulo, sino que muestra el `go get github.com/nsgo-dev/nsgo@<versión>` que trae `add`; allí, `go tool nsgo add` ejecuta el `nsgo` fijado, que no conoce `add`.

## Bibliotecas de paquetes Go

Las aplicaciones que importan `base/terminal` también distribuyen `libghostty-vt`: los paquetes de Windows y Linux colocan la biblioteca junto al ejecutable; los paquetes de macOS e iOS la firman dentro de sus frameworks. [Incrustación](embedding.md) explica el XCFramework complementario para anfitriones iOS.

Una dependencia puede declarar bibliotecas compartidas de escritorio precompiladas en `nsgo-natives.yaml` y, desde su versión 2, frameworks para iOS y tvOS. NSGo verifica su SHA-256, las copia en la aplicación antes de firmar y las incluye en los instaladores; el autor de la aplicación no realiza ningún paso adicional. Desde la versión 2 también puede nombrar paquetes de Linux, de los que el `.deb` y el paquete de Arch dependen o que recomiendan después de `linux.depends` y `linux.recommends` de la propia aplicación. [Bibliotecas nativas](native-libraries.md) describe el formato para el autor del paquete y las rutas de carga.

## Comandos

Una aplicación puede incluir programas de línea de comandos junto a su ejecutable de escritorio, como una CLI que opere sobre los datos de la aplicación. Enuméralos bajo `commands`, cada uno compilado a partir de un paquete `main` de Go:

```yaml
commands:
  - name: hello-cli             # the file's name, hello-cli.exe on Windows
    package: ./cmd/hello-cli    # a main package, built for each desktop target with CGO_ENABLED=0 and build.tags
    ldflags: [-X main.channel=beta]  # its own linker flags, after build.ldflags
windows:
  path: true                    # the installer puts the app's folder on the user's Path
```

Un nombre consta de letras, dígitos, `-` y `_`, y no puede coincidir en ninguna combinación de mayúsculas y minúsculas con el ejecutable de la aplicación, su comando de Linux, `Uninstall`, `AppRun` ni otro comando. `macos_universal` une las dos compilaciones de macOS de cada comando, igual que une las de la aplicación.

Un paquete se enlaza con `build.ldflags` y `-ldflags`, luego sus propios `ldflags`, cuyos valores `-X` prevalecen, y después la identidad de la aplicación, de modo que `core.App()` devuelve al comando el nombre, el ID, la versión y el número de compilación de `nsgo.yaml`, igual que a la aplicación. Un comando que no importe `core` recibe la versión mediante `-X`, como `-ldflags "-X main.version=1.2.0"` en `build`, que reciben tanto la aplicación como sus comandos.

| Paquete | Comandos |
| --- | --- |
| macOS | En `Contents/MacOS`, firmados antes que la aplicación tal cual, como `<id>.<command>`: ad hoc, o con `sign.identity`, el entorno de ejecución reforzado y una marca de tiempo para que la aplicación se [notarice](#notarizar), pero sin los entitlements de la aplicación. La imagen de disco no añade nada a `PATH`; el [cask de Homebrew](#canales) enlaza cada comando, y el usuario puede enlazar `/Applications/<name>.app/Contents/MacOS/<command>` por su cuenta |
| Windows | `<command>.exe` junto a `<name>.exe`, [firmado](#firmar-paquetes-de-windows) tal cual, que el instalador instala y su desinstalador elimina; con `windows.path`, el instalador también añade la carpeta al `Path` del usuario, tal como hace `windows.command_line`, para que las consolas abiertas tras instalar encuentren los comandos. Un comando que una actualización renombre o retire permanece en la carpeta, igual que cualquier archivo que la aplicación ya no distribuya, ya que el desinstalador solo elimina lo que instaló su versión |
| Linux | En `/opt/<slug>` con un enlace `/usr/bin/<command>` del que son propietarios el `.deb` y el paquete de Arch, de modo que eliminar el paquete lo borra, y dpkg y pacman rechazan instalar sobre el de otro paquete; la AppImage los guarda junto a la aplicación y no añade nada a `PATH` |

Los instaladores, sus sumas de comprobación y los appcasts de [actualización](#actualizaciones) incluyen los comandos igual que los demás archivos de la aplicación, por lo que una actualización los reemplaza junto con ella. Los [paquetes de tienda](#paquetes-de-tienda) los omiten, ya que la zona aislada del Mac App Store, MSIX y Flatpak solo ejecutan el ejecutable propio de la aplicación.

## Instaladores

Para los destinos de escritorio, `build` escribe en `dist/releases` aquello desde lo que los usuarios instalan la aplicación, cada uno con una suma de comprobación `.sha256`:

| Destino | Instalador |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.dmg`, una imagen de disco que muestra la aplicación, una flecha y un enlace a Aplicaciones al que arrastrarla, firmada con `sign.identity` y [notarizada](#notarizar) |
| Windows | `<name>-<version>-windows-<arch>-setup.exe`, un instalador nativo de Windows que instala para el usuario, de nuevo en la carpeta que eligió anteriormente, cerrando la aplicación si está en ejecución, con un acceso directo en el menú Inicio, uno opcional en el escritorio y un desinstalador que elimina solo lo que instaló, [firmado con `sign.certificate`](#firmar-paquetes-de-windows) donde la aplicación firma paquetes de Windows; `windows.command_line` también instala el complemento `.com` y añade la carpeta de la aplicación al `Path` del usuario, de modo que las consolas abiertas tras instalar ejecutan el comando de la aplicación, mientras que Win+R, que también encuentra el complemento, inicia la aplicación y cierra la consola que abrió; el desinstalador retira la carpeta de `Path` solo si la añadió un instalador, nunca una entrada previa |
| Linux | `<name>-<version>-linux-<arch>.deb` para Debian y Ubuntu, y `<name>-<version>-linux-<arch>.pkg.tar.gz` para Arch Linux, que instalan la aplicación en `/opt/<slug>` con un comando, enlaces a sus [comandos](#comandos), su entrada de escritorio e icono (el paquete de Arch incluye `.MTREE` para `pacman -Qkk`), y `<name>-<version>-linux-<arch>.AppImage`, que se ejecuta en cualquier distribución sin instalar |

El instalador y el desinstalador de Windows se compilan solo con Go, en cualquier sistema, y usan diálogos nativos de Windows. Hablan los idiomas de los catálogos de la aplicación para los que tienen textos, incluidos chino simplificado y tradicional, eligiendo el idioma de la interfaz del sistema; `/LANG=<LCID>` elige uno para una sola ejecución, como `/LANG=2052` para chino simplificado. `/S` instala o desinstala en silencio, un `/D=<folder>` final elige la carpeta, con espacios incluidos, y `/RELAUNCH` abre la aplicación tras una actualización. Sin `/D`, una actualización instala en la carpeta de la instalación que reemplaza. `-installers=false` o `build.installers: false` compila únicamente los paquetes.

Los instaladores admiten imágenes e iconos propios de la aplicación; de lo contrario, cada uno usa el icono de la aplicación o la imagen integrada:

```yaml
macos:
  dmg_background: assets/DMGBackground.png  # the disk image's window, which takes its size in points
  dmg_icon: assets/DMGIcon.png              # the disk image's icon: an .icns or a square PNG
windows:
  installer_icon: assets/Setup.png          # the setup program's icon: an .ico or a square PNG
  uninstaller_icon: assets/Uninstall.png    # the uninstaller's
```

Un PNG de fondo con un gemelo del doble de tamaño, como `DMGBackground@2x.png`, se mantiene nítido en pantallas Retina. El icono de la aplicación se sitúa a un cuarto desde la izquierda y Aplicaciones a un cuarto desde la derecha, cada uno con su nombre centrado verticalmente, donde la imagen integrada dibuja su flecha; deja espacio para ellos en un fondo propio. Los paquetes de Linux no tienen imágenes propias; instalan el icono de la aplicación.

Los paquetes de Linux también instalan los metadatos AppStream de la aplicación, `/usr/share/metainfo/<id>.metainfo.xml`, con los que la listan los centros de software, redactados a partir de `linux.summary`, `linux.license` (si está definido) y los nombres de la aplicación, con `Utility` como categoría cuando `linux.categories` no indica ninguna; la AppImage los guarda en el `usr/share` de su AppDir, junto a un enlace a su entrada de escritorio en `usr/share/applications`, que los metadatos mencionan. Los archivos que el sistema lee desde `/usr/share`, como metadatos más completos, entradas de escritorio adicionales, completados de shell o páginas de manual, van en el directorio indicado por `linux.share`, relativo a `nsgo.yaml`: el `.deb` y el paquete de Arch instalan su árbol bajo `/usr/share`, y la AppImage bajo el `usr/share` de su AppDir. Su `metainfo/<id>.metainfo.xml` reemplaza al que escribe NSGo, y cualquier archivo en una ruta que el paquete escriba por sí mismo, como la entrada de escritorio o el icono de la aplicación, hace fallar la compilación. Los archivos de `resources/`, se llamen como se llamen, permanecen junto al ejecutable en `/opt/<slug>`, y los Flatpak de compilaciones para tienda usan únicamente los metadatos de NSGo.

La AppImage necesita FUSE, presente en la mayoría de las distribuciones, y toma GTK 4 del sistema, igual que los paquetes; su entorno de ejecución se descarga una vez en la caché del usuario.

Las AppImages incrustan un localizador `gh-releases-zsync` usando `release.github` o el remoto `origin` de GitHub, seleccionando el último lanzamiento estable y la arquitectura de esta aplicación. Sube cada `.AppImage` junto con su archivo `.AppImage.zsync` generado. Para otro alojamiento, establece `linux.appimage_update` en `zsync|https://example.com/latest.AppImage.zsync`, una URL estable servida con soporte de rangos HTTP; también acepta `gh-releases-zsync|owner|repo|latest-pre|name-*-linux-x64.AppImage.zsync` para un canal preliminar. Una cadena vacía explícita desactiva el descubrimiento incrustado; sin repositorio de GitHub ni localizador configurado también queda vacío. El archivo `.zsync` sigue generándose, separado del appcast firmado.

## Paquetes de tienda

`-channel store`, o `updates.channel: store`, compila los destinos de escritorio y Android para sus tiendas, que son quienes los actualizan, en lugar de los instaladores y scripts de instalación:

| Destino | Paquete de tienda |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.pkg` para el Mac App Store: la aplicación en la App Sandbox, firmada con la identidad del perfil de aprovisionamiento y empaquetada con la identidad de instalador de tu equipo |
| Windows | `<name>-<version>-windows-<arch>.msix` para Microsoft Store, que lo firma al publicarlo |
| Linux | `<name>-<version>-linux-<arch>.flatpak` instalable, además de un `.tar.gz` con AppStream y `flatpak/<id>.yml` para Flathub o flatpak-builder sobre el entorno de ejecución de GNOME |
| Android | `<name>-<version>-android-<arch>.aab`, un App Bundle para Google Play firmado con `sign.keystore`, la clave de subida |

El MSIX lee el nombre visible de cada idioma del campo `appName` de los catálogos, recurriendo al idioma de origen. Sus logotipos de Inicio, barra de tareas y Tienda incluyen variantes al 100 %, 125 %, 150 %, 200 % y 400 %. La herramienta escribe su índice de recursos en macOS; no se necesita el SDK de Windows.

`-target macos` selecciona ambas arquitecturas de macOS en el canal de tienda y genera un único `<name>-<version>-macos-universal.pkg`, al igual que `-target macos_universal`. Selecciona `-target android_arm64,android_x64` para obtener un único `<name>-<version>-android-universal.aab` con `arm64-v8a` y `x86_64`. La porción `x86_64` requiere un Android NDK.

```yaml
macos:
  category: public.app-category.productivity  # LSApplicationCategoryType, which the Mac App Store requires
  # entitlements: { com.apple.security.files.downloads.read-write: true }  # beyond the network, the files the user picks and what apple.info explains
windows:
  store:                               # as Partner Center's Product identity lists them
    identity: 12345Example.Hello
    publisher: CN=00000000-0000-0000-0000-000000000000
    publisher_name: Example
  # device_capabilities: [microphone, webcam]  # the devices the app uses, which Windows asks the user for
linux:
  summary: Says hello                  # one line for software centers
  license: MIT                         # an SPDX expression; LicenseRef-proprietary by default
  # flatpak_runtime: "51"              # GNOME's runtime version
android:
  target_sdk: 36                       # the targetSdkVersion Google Play requires
sign:
  macos_profile: assets/MacAppStore.provisionprofile  # the Mac App Store's profile
  # installer: "3rd Party Mac Developer Installer: Example (ABCDE12345)"  # by default the keychain's of the profile's team
```

El entorno de ejecución apunta al nivel de API 36 cuando `android.target_sdk` no está configurado. La compilación de un Android App Bundle falla por debajo de la API 36, requisito que [Google Play exige](https://support.google.com/googleplay/android-developer/answer/11926878) para nuevas aplicaciones móviles y actualizaciones a partir del 31 de agosto de 2026. Una compilación de APK independiente muestra una advertencia y continúa.

Configura `sign.macos_profile` para firmar el paquete de Mac para subirlo. Sin widgets, el paquete sin firmar sigue ejecutándose en la zona aislada, lo que te permite probar el acceso al contenedor de la aplicación, a la red y a los archivos que elige el usuario. Una compilación de tienda con widgets requiere tanto `sign.macos_profile` para `<id>` como un `sign.widgets_profile` de macOS para `<id>.widgets`, del mismo equipo y que conceda `apple.app_group` (de forma predeterminada `group.<id>`). Ambos paquetes utilizan ese contenedor compartido; no se añade ninguna excepción al directorio personal. `-installers=false` conserva las firmas de la zona aislada y del perfil mientras omite el `.pkg` y su identidad de instalador. Sube el `.msix` en el Centro de partners y el `.aab` en Play Console. Envía `flatpak/<id>.yml` a Flathub una vez que el lanzamiento contenga los archivos que nombra. `flatpak-builder` también lo compila localmente. Instala un paquete con `flatpak install --user ./<name>-<version>-linux-<arch>.flatpak`; el entorno de ejecución correspondiente de GNOME debe estar instalado o disponible en un remoto configurado. Las compilaciones generan el paquete en Go sin cadena de herramientas de Linux. Los paquetes de tienda usan un icono transparente cuando falta el material gráfico y `Utility` cuando `linux.categories` no está configurado. Para instalarlo fuera de la Tienda, configura [`sign.certificate`](#firmar-paquetes-de-windows) para firmar el MSIX; el sujeto de su certificado debe coincidir con `windows.store.publisher`, y la máquina receptora debe confiar en el certificado. Sin configuración de firma, la Tienda firma el paquete subido.

En macOS e iOS, los efectos de borde de una vista desplazable (`Scroll.EdgeEffects`), los desenfoques de fondo de macOS sin `IOS.BackdropMaterial` y los filtros de color de fondo (solos o sobre un desenfoque sin `IOS.BackdropMaterial`) se dibujan mediante las clases privadas `CABackdropLayer` y `CAFilter` de Core Animation, igual que los propios materiales y efectos de borde de AppKit y UIKit, ya que ninguna API pública desenfoca por un radio, varía un desenfoque a través de una vista ni aplica una matriz de color de fondo. La revisión del App Store puede rechazar aplicaciones que usen API privadas: NSGo solo las llama para vistas desplazables con barras de borde y esos filtros de fondo, pero todas las compilaciones de macOS e iOS contienen sus nombres.

Todo paquete de Apple que escribe `build` —la aplicación, su extensión de widgets, App Clip y aplicación de Apple Watch, así como un framework `-dynamic` `-embed`— lleva un manifiesto de privacidad, `PrivacyInfo.xcprivacy`, que declara las API de motivo requerido que usan NSGo y el entorno de ejecución de Go: valores predeterminados de usuario (`CA92.1`), marcas de tiempo de archivos (`C617.1`) y hora de arranque del sistema (`35F9.1`). `apple.privacy` añade los propios de la aplicación: `accessed_apis`, los motivos de cada categoría, `tracking`, `tracking_domains` y `collected_data`, entradas de `NSPrivacyCollectedDataTypes` tal como Apple las nombra; un `PrivacyInfo.xcprivacy` en `resources/` se fusiona con ellos en lugar de reemplazarse.

El `Info.plist` de cada paquete de iOS, tvOS y watchOS que `build` genera para dispositivos —el de la aplicación, el de su extensión de widgets y el de su App Clip— incluye las claves `DT` del SDK y de Xcode que comprueba App Store Connect, leídas del Xcode que realiza la compilación, y `UIRequiredDeviceCapabilities: [arm64]`; los paquetes de simulador las omiten, y cualquier clave que defina `apple.info` conserva su valor. `apple.multiple_scenes: true` establece `UIApplicationSupportsMultipleScenes` en el manifiesto de escenas de una aplicación iOS, que es falso de forma predeterminada: en un iPad, el usuario puede abrir entonces más de una ventana de la aplicación, y cada una muestra de nuevo la vista principal con su propio estado y con las señales y almacenamiento compartidos de la aplicación, hasta que el usuario la cierre.

### Publicar en Google Play

Habilita la API de Google Play Developer y da permiso a una cuenta de servicio para publicar la aplicación en la pista elegida. Establece `NSGO_PLAY_SERVICE_ACCOUNT` con la clave JSON de la cuenta o con la ruta a su archivo de clave. [Configuración de la API de Google Play](https://developers.google.com/android-publisher/getting_started)

```sh
go tool nsgo build -target android -channel store
go tool nsgo publish -target android -track internal
```

El comando busca el App Bundle de la versión del proyecto en `build.output/releases`. Pasa una ruta `.aab` después de las marcas para elegir un archivo cuando haya varias compilaciones. Usa `internal`, `production` o el ID de pista de una prueba cerrada. El comando lee el paquete y el código de versión del bundle, exige un código superior al de cualquier lanzamiento en esa pista, y luego sube el bundle y confirma el lanzamiento.

## Fichas de tienda

Los metadatos de la tienda (títulos, subtítulos, descripciones, palabras clave, notas de la versión, URL, capturas de pantalla y vistas previas) residen en el repositorio como archivos YAML y Markdown en texto plano bajo `store/` (o `store.dir` en `nsgo.yaml`):

```text
store/
  listing.yaml              # version: 1, default_locale, app_store and google_play settings
  en-US/
    listing.yaml            # version: 1, title, subtitle, short_description, keywords, URLs, media
    description.md          # full description (optional file in place of YAML description)
    release_notes.md        # what's new (optional file in place of YAML release_notes)
    promotional_text.md     # App Store promotional text (optional file)
    screenshots/
  zh-Hans/
    listing.yaml
    description.md
    release_notes.md
    screenshots/
```

Ambos archivos `listing.yaml` exigen `version: 1` y rechazan claves desconocidas. Los directorios de configuración regional canónicos como `zh-Hans` se asignan automáticamente al código de cada tienda (`zh-Hans` en App Store Connect, `zh-CN` en Google Play) salvo que se sobrescriban por tienda con `app_store.locale` o `google_play.locale`.

```sh
go tool nsgo publish -store all -dry-run                  # validate lengths, locales and media without uploading
go tool nsgo publish -store appstore,play                 # validate and upload to App Store Connect and Google Play
go tool nsgo publish -store play -track production        # also update release notes on the named Play track
```

La validación se ejecuta antes de leer credenciales o realizar cualquier petición HTTP, comprobando campos obligatorios, códigos de idioma, esquemas de URL, archivos multimedia y cantidades (hasta 10 capturas de pantalla y 3 vistas previas por tipo de pantalla en App Store; hasta 8 capturas por tipo de imagen en Google Play), además de los límites de caracteres (App Store: título 30, subtítulo 30, palabras clave 100, texto promocional 170, descripción 4000, notas de la versión 4000; Google Play: título 30, descripción breve 80, descripción 4000, notas de la versión 500).

Las credenciales de subida proceden únicamente del entorno: `NSGO_APP_STORE_KEY`, `NSGO_APP_STORE_KEY_ID` y `NSGO_APP_STORE_ISSUER` (recurriendo a la clave de notarización `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID` y `NSGO_NOTARY_ISSUER`) para App Store Connect, y `NSGO_PLAY_SERVICE_ACCOUNT` (ruta al archivo JSON de la cuenta de servicio o el texto JSON) para Google Play.

### Cumplimiento de tiendas

Usa `nsgo.yaml` para hacer seguimiento del estado de cada cuestionario de lanzamiento.

```yaml
compliance:
  app_store: { age_rating: false }
  google_play: { data_safety: false }
  steam: { content_survey: false }
```

Cambia una entrada a `true` tras completar su cuestionario. Una compilación correspondiente con `-channel store` o `-channel steam` te recordará las entradas configuradas en `false`. Omite cualquier entrada de la que no hagas seguimiento en YAML.

- Completa el [cuestionario de clasificación por edades](https://developer.apple.com/help/app-store-connect/manage-app-information/set-an-app-age-rating) y los [detalles de privacidad de la app](https://developer.apple.com/help/app-store-connect/manage-app-information/manage-app-privacy) en App Store Connect.
- Para el [cuestionario de cifrado de Apple](https://developer.apple.com/help/app-store-connect/manage-app-information/determine-and-upload-app-encryption-documentation), establece `apple.info.ITSAppUsesNonExemptEncryption` con la respuesta de tu aplicación como booleano YAML. Si Apple envía un código de aprobación, ponlo en `apple.info.ITSEncryptionExportComplianceCode` como cadena. Puedes omitir las claves y responder al subir la compilación. Las compilaciones de tienda de Apple comprueban los tipos de estas claves y la estructura del manifiesto de privacidad del paquete.
- Completa la sección [Seguridad de los datos](https://support.google.com/googleplay/android-developer/answer/10787469) en Play Console. Las aplicaciones distribuidas exclusivamente en la pista de pruebas internas están exentas.
- Completa la [Encuesta de contenido](https://partner.steamgames.com/doc/gettingstarted/contentsurvey) en Steamworks antes de enviar el juego a revisión.

## App Store Connect

Crea una aplicación en App Store Connect y firma un IPA de dispositivo iOS con un perfil de distribución del App Store o un PKG de tienda de macOS con la identidad de instalador de tu equipo. En un Mac con Xcode, establece `NSGO_APP_STORE_KEY` al archivo `.p8` de la clave de tu equipo en App Store Connect o a su contenido, `NSGO_APP_STORE_KEY_ID` al ID de la clave y `NSGO_APP_STORE_ISSUER` a su ID de emisor; sin ellos se usa la clave de notarización (`NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID`, `NSGO_NOTARY_ISSUER`), igual que para las fichas de tienda. Da a esa clave acceso a la aplicación y permiso para subir compilaciones.

```sh
go tool nsgo publish ios path/to/Hello.ipa
go tool nsgo publish macos path/to/Hello.pkg
```

La herramienta comprueba el número de compilación del paquete antes de subirlo y luego informa del resultado de procesamiento de Apple para TestFlight. Incrementa `build_number` para la siguiente subida. Los números de compilación de macOS aumentan a lo largo de todas las versiones de la aplicación, mientras que iOS permite reiniciar la secuencia en una nueva versión. Envía por separado una compilación procesada a revisión en App Store Connect. [Guía de distribución de Apple](https://developer.apple.com/documentation/xcode/preparing-your-app-for-distribution)

## Paquetes de activos de tienda

Declara directorios grandes de activos en `mobile.asset_packs`. Los archivos contenidos en esos directorios se distribuyen por separado del Android App Bundle base y de la aplicación iOS. Los nombres de paquete empiezan por una letra minúscula y pueden contener letras minúsculas, dígitos y guiones bajos.

```yaml
mobile:
  asset_packs:
    - {name: levels, dir: resources/levels, delivery: install-time}
    - {name: voices, dir: large-assets/voices, delivery: fast-follow}
```

Compila paquetes de Android con `go tool nsgo build -channel store -target android_arm64`. Usa `bundletool build-apks --local-testing` y `bundletool install-apks` para probar el paquete en un dispositivo. NSGo abre los paquetes `install-time` instalados. Los módulos `fast-follow` se incluyen para distribución en Play, aunque la lectura de esos paquetes desde el SDK de Play Asset Delivery aún está pendiente.

En iOS, `go tool nsgo build -target ios` asigna `install-time` a la política essential de Apple y `fast-follow` a prefetch, tanto en la primera instalación como en las actualizaciones. Xcode 26 o posterior compila el descargador de Background Assets. Usa un `apple.app_group` compartido o el predeterminado `group.<id>`. Tanto el `sign.profile` de la aplicación como el `sign.asset_packs_profile` de la extensión `<id>.assetdownloader` necesitan ese grupo. Sube los archivos `.aar` de `dist/releases` junto con el IPA mediante el flujo de subida de paquetes de activos de App Store Connect. [Creación de paquetes de activos gestionados](https://developer.apple.com/documentation/backgroundassets/creating-managed-asset-packs)

Desde una goroutine, llama a `mobile.AssetPackReady(ctx, "levels")` para comprobar si está listo y a `mobile.OpenAssetPackFile("levels", "intro.map")` para abrir un archivo relativo al directorio declarado. Cierra el lector devuelto después de usarlo. [Prueba local de paquetes de activos de Apple](https://developer.apple.com/documentation/backgroundassets/testing-asset-packs-locally)

## Depósitos de Steam

```sh
go tool nsgo build -channel steam -target macos_universal,windows_x64,linux_x64
```

Establece `updates.channel: steam` para usar este canal de forma predeterminada. Selecciona destinos de macOS, Windows y Linux. Sube cada directorio `dist/<name>-<version>-<target>` a su depósito de Steam, usando el paquete de aplicación en macOS, el ejecutable en Windows y Linux arm64, y `steam-launch.sh` en Linux x64 como opción de inicio. Steam gestiona las actualizaciones. Los directorios no contienen instaladores, actualizador propio ni `steam_appid.txt`.

Los depósitos de Linux x64 incluyen GTK 4 y sus dependencias para Steam Linux Runtime 3.0 y 4.0. El lanzador los usa únicamente cuando el sistema no dispone de GTK 4.14 o posterior. Mantén `nsgo-gtk/` junto al lanzador: su carpeta `licenses/` contiene las licencias de las bibliotecas, las fuentes desde las que se compilaron y el script de compilación, tal como exige la LGPL para GLib, Pango y GTK. Linux arm64 necesita GTK 4 disponible en su entorno de ejecución.

Para pruebas locales fuera del cliente de Steam, configura `steam: {app_id: 480}` en `nsgo.yaml`, sustituyendo 480 por tu App ID de Steamworks. `go tool nsgo dev` escribe el ID en `steam_appid.txt` dentro de su directorio de compilación nativa e inicia la aplicación con ese directorio como directorio de trabajo. [API de Steamworks](https://partner.steamgames.com/doc/api/steam_api), [SteamPipe](https://partner.steamgames.com/doc/sdk/uploading).

## Publicar en GitHub

Cuando la compilación incluye un destino de macOS, Windows o Linux, `dist/releases` también contiene `install.sh` e `install.ps1`, que instalan la aplicación desde los lanzamientos del repositorio de GitHub que nombra `release.github`, o en su defecto el del remoto Git `origin` de la aplicación; sin ninguno de los dos, `build` no escribe ninguno y lo indica:

```sh
gh release create v1.2.0 dist/releases/*.*
```

Sube sus archivos —los del nivel superior sin `miniprogram/`— al lanzamiento etiquetado `v<version>` (`release.tag_prefix` sustituye a `v`; los scripts codifican los caracteres reservados de URL en ese prefijo).

Los usuarios instalan o actualizan entonces la aplicación con una sola línea, que la compilación imprime:

```sh
curl -fsSL https://github.com/me/hello/releases/latest/download/install.sh | sh    # macOS and Linux
```

```powershell
irm https://github.com/me/hello/releases/latest/download/install.ps1 | iex         # Windows
```

En macOS el script copia la aplicación desde la imagen de disco en `/Applications`, o en `~/Applications` cuando no puede escribir allí; en Linux instala el paquete de Arch con `pacman`, o en su defecto el de Debian con `apt-get` o `dpkg`, mediante `sudo`; en Windows ejecuta el instalador en modo silencioso, que instala donde ya esté la aplicación. Cada uno verifica primero el archivo frente a su suma de comprobación. `<NAME>_VERSION`, el nombre de la aplicación en mayúsculas, como `HELLO_VERSION=1.1.0`, instala otro lanzamiento, y `<NAME>_DOWNLOAD_URL` descarga desde un espejo.

## Publicar en itch.io

Establece `itch.project` con el identificador `owner/game` de tu juego en itch.io dentro de `nsgo.yaml`. Para sobrescribir los nombres de canal, configura `itch.channels` por destino de compilación. Los valores predeterminados son `html5` para web, `osx-<arch>` para macOS, `windows-<arch>` para Windows y `linux-<arch>` para Linux.

```yaml
itch:
  project: example/my-game
  channels:
    macos_arm64: osx-arm64-beta
```

Con `BUTLER_API_KEY` configurado como secreto en CI, compila y sube los destinos.

```sh
go tool nsgo build -channel none -installers=false -target web,macos_arm64
go tool nsgo publish itch -target web,macos_arm64
```

Usa los mismos `-output` y `-version` en ambos comandos si los sobrescribes. La subida toma el directorio de paquete de cada destino y establece su versión en itch.io con la versión de la compilación. NSGo usa `butler` en `PATH` o descarga una copia fijada desde itch.io. Tras la primera subida web, configura el tipo de juego en itch.io como HTML y marca la subida como ejecutable en el navegador. [Documentación de Butler](https://itch.io/docs/butler/pushing.html)

## Actualizaciones

`updates` hace que la aplicación se mantenga actualizada tal como lo hacen su plataforma y el canal desde el que la obtienen los usuarios:

```yaml
updates:
  key: zE0hx2FZwwHy6X7LbEP3GjTTE3KWfCAp5gsaBysC+vg=  # the public key nsgo keygen prints
  # channel: store                    # direct, store, steam or none; direct by default with a key, none without
  # feed: https://example.com/hello/  # where the appcasts and installers are; by default the latest GitHub release
  # automatic: false                  # check only when asked; true by default
  # automatic_downloads: true          # Sparkle downloads automatically; false by default, macOS only
```

`nsgo keygen` imprime un par de claves sin escribir ningún archivo: pon la clave pública en `updates.key` y haz commit de ella, y guarda la clave secreta fuera del repositorio, en un gestor de contraseñas y en `NSGO_UPDATES_KEY` donde se compilen los lanzamientos, como un secreto de GitHub Actions. Cada compilación directa firma los instaladores con la clave secreta y escribe `appcast-<target>.xml` junto a ellos, un feed en el formato de [Sparkle](https://sparkle-project.org) que la aplicación lee bajo `feed`: sube los appcasts junto con los instaladores. Sin la clave secreta, `build` no escribe appcast de macOS ni de Windows, ya que estos solo instalan instaladores firmados, y las aplicaciones de Linux y Android que lean un appcast sin firmar abren la página del lanzamiento en lugar de instalar su paquete.

`-release-notes notes.md` inserta un archivo Markdown con las notas del lanzamiento, como su sección del registro de cambios, en cada appcast, renderizado como HTML sin el HTML en bruto del propio archivo. Sparkle y WinSparkle lo muestran en sus cuadros de diálogo de actualización, y el aviso en Linux y Android lo muestra como texto debajo de la versión, hasta 1.500 caracteres; las aplicaciones lo leen como `core.UpdateCheck.Notes`.

| Destino | Cómo se actualiza una compilación directa |
| --- | --- |
| macOS | Sparkle, que `build` incluye en la aplicación, comprueba a diario, ofrece la nueva versión, descarga la imagen de disco, verifica su firma, la instala y reinicia la aplicación; el menú predeterminado de la aplicación incluye Buscar actualizaciones…, que una aplicación con menú propio añade con `updater.Check` |
| Windows | WinSparkle, junto al ejecutable, hace lo mismo en el idioma que muestra la aplicación al iniciarse, cuando WinSparkle dispone de él, y ejecuta el instalador en silencio, que inicia la nueva versión |
| Linux, Android | La aplicación lee su appcast y, en cuanto el usuario elige Actualizar, transmite y verifica el paquete firmado directamente en disco informando del progreso (`updater.Status` y el indicador de progreso de tarea de la plataforma), luego lo instala mediante `pkexec dpkg`, `pkexec pacman` (`.pkg.tar.gz`/`.pkg.tar.zst`), sustitución atómica de AppImage (reiniciando en Linux) o el instalador de paquetes de Android; las instalaciones fuera de esos formatos o un appcast sin firmar ofrecen en su lugar la página del lanzamiento |
| iOS | La aplicación consulta al App Store, que la actualiza, y ofrece su página allí |

Para una aplicación web independiente publicada, `updater.Check(cx)` compara la versión en ejecución con un `nsgo.json` actualizado y ofrece recargar tras confirmar. También detecta archivos de compilación modificados bajo la misma etiqueta de versión. No necesita clave de appcast; los canales explícitos `none`/`steam` siguen desactivando las comprobaciones, y los anfitriones incrustados gestionan sus propias recargas. Los miniprogramas usan el gestor de actualizaciones de su plataforma y solo reinician una vez que la actualización descargada está lista. Estos destinos comprueban actualizaciones cuando la aplicación llama al actualizador; el momento de comprobación y descarga en miniprogramas corresponde al anfitrión. No exponen configuradores de preferencias de comprobación automática. Un manejador personalizado `cx.CheckForUpdates` usa `UpdateCheck.Restart` para una actualización preparada cuya etiqueta de versión puede faltar.

### Canales

El canal decide quién actualiza la aplicación. `-channel` reemplaza a `updates.channel` para una compilación, de modo que un mismo proyecto compila para cada uno:

| Canal | Para | Los paquetes |
| --- | --- | --- |
| `direct` | GitHub, un sitio web, casks de Homebrew y winget, que instalan los propios archivos del lanzamiento | Se actualizan a sí mismos como se indicó arriba |
| `store` | El App Store, el Mac App Store, Microsoft Store, Google Play, Flathub y otras tiendas | Son los [paquetes de tienda](#paquetes-de-tienda) y no llevan autoactualizador; iOS y macOS consultan a su App Store sin clave, y una instalación de Google Play puede usar el flujo de actualización nativo de Play |
| `steam` | Steam | Producen [directorios de depósito](#depósitos-de-steam) que actualiza Steam |
| `none` | Distribuciones de Linux y cualquiera que reempaquete la aplicación | No comprueban nada |

Las tiendas rechazan aplicaciones que se actualizan a sí mismas, así que sube siempre una compilación `store`, nunca una directa. Cada lanzamiento directo con instaladores de macOS o Windows también genera un cask de Homebrew (`dist/releases/homebrew/<name>.rb`, que declara `auto_updates true`, sumas de comprobación y URL de descarga para las imágenes de disco compiladas, y un enlace `binary` para cada uno de los [comandos](#comandos) de la aplicación) y un manifiesto multiarchivo de winget (`dist/releases/winget/<id>.yaml`, `<id>.installer.yaml` y `<id>.locale.en-US.yaml` para los instaladores NSIS compilados). Dondequiera que el sistema indique quién instaló la aplicación, la aplicación también lo comprueba: una aplicación Android de otra tienda, un Flatpak o Snap y una compilación de TestFlight dejan las actualizaciones en manos de esa tienda. Una llamada manual a `updater.Check` en una instalación de Google Play solicita a Play In-App Updates que ofrezca su flujo inmediato cuando esté permitido; Play gestiona el consentimiento, la descarga, la instalación y el reinicio. Las compilaciones de desarrollo nunca entran en ese flujo. Los paquetes independientes de Android incluyen el SDK; los [anfitriones incrustados](embedding.md#android) lo proporcionan a través de Gradle.

### Comprobación desde la aplicación

`updater.Check(cx)` comprueba igual que un elemento de menú *Buscar actualizaciones…* y muestra lo que encuentra; `updater.CheckInBackground(cx)` muestra solo si hay una versión más nueva, y `updater.CheckDaily(cx)` hace eso como máximo una vez al día. `updater.Status(cx)` lee el progreso en vivo de descarga e instalación en Linux y Android. Mientras `automatic` esté activado, la aplicación comprueba de ese modo en Linux, compilaciones directas de Android y compilaciones del App Store de Apple al iniciarse y cada vez que el usuario vuelve a ella; si una comprobación falla, como cuando no hay conexión, se vuelve a intentar una hora después. Las compilaciones de desarrollo nunca se actualizan. En compilaciones de Steam, estas tres funciones no hacen nada.

`updater.CapabilityOf(cx)` informa de cómo se actualiza esta copia sin hacer una petición de red: `ManagedSelf`, `ManagedPage`, `ManagedDevelopment`, `ManagedChannel`, `ManagedStore` (con su nombre `Store`) o `ManagedUnavailable`. Una compilación de desarrollo, un canal desactivado o una instalación de tienda producen una explicación específica del motivo al comprobar; `core.UpdateCheck.Err` se reserva para fallos como un feed erróneo o un actualizador empaquetado que no puede cargarse.

Lee `updater.Status(cx)` dentro de un enlace reactivo para seguir `Capability`, `AutomaticChecks`, `AutomaticDownloads`, `Available`, `Notes` y `LastCheck`, y en Linux y Android las propiedades `Phase`, `Loaded` y `Total` de una actualización (`Fraction()` para una barra de progreso). `updater.SetAutomaticChecks(cx, on)` guarda la elección del usuario y cambia el planificador diario en tiempo de ejecución. Sparkle y WinSparkle gestionan sus propias preferencias; Linux, Android e iOS guardan la elección en una partición de almacenamiento privada. `updater.SetAutomaticDownloads(cx, on)` solo está admitido en macOS, con `updates.automatic_downloads` como valor predeterminado; otras plataformas informan `false` e ignoran el configurador. Las compilaciones no admitidas o gestionadas externamente informan ambos ajustes en falso. WinSparkle informa de los metadatos de actualización encontrada leyendo el appcast después de su llamada de retorno de hallazgo; sigue controlando la instalación. Linux resuelve al inicio la pertenencia a paquetes de Debian o Arch y la identidad de AppImage; esas copias informan `ManagedSelf`, mientras que otras copias sin empaquetar informan `ManagedPage` antes de cualquier comprobación.

## Iniciar al iniciar sesión

`desktop.SetLaunchAtLogin(cx, on)` registra la aplicación en el mecanismo de inicio de sesión de la plataforma, y `desktop.LaunchAtLogin(cx, done)` lo lee. Un MSIX compilado con la herramienta actual incluye automáticamente una tarea de inicio deshabilitada; habilitarla respeta el interruptor de aplicaciones de inicio del usuario y la política del administrador. Las instalaciones ordinarias de Windows y Linux conservan su valor Run y su entrada XDG autostart. macOS usa el servicio de inicio de sesión nativo del paquete de aplicación.

Flatpak solicita el portal Background, que puede pedir consentimiento, sin conceder acceso a los archivos de configuración del anfitrión. Una lectura posterior espera la respuesta nativa. El portal no dispone de una consulta de inicio de solo lectura: las lecturas en Flatpak devuelven únicamente la última respuesta confirmada en esta ejecución, falso antes de haber una, y no pueden detectar un cambio fuera de la aplicación ni recuperar la configuración en la siguiente ejecución.

Un Snap empaquetado externamente declara el nombre del archivo autostart en su aplicación:

```yaml
apps:
  app:
    command: bin/app
    autostart: app.desktop
```

NSGo escribe ese nombre de archivo en `$SNAP_USER_DATA/.config/autostart`, que snapd lanza mediante el envoltorio de la aplicación. `desktop.LaunchedAtLogin(cx)` distingue un inicio al iniciar sesión antes de que la aplicación se construya, incluida una activación `StartupTask` de MSIX.

## Tipos de archivo

`file_types` registra los documentos que abre una aplicación: desde el Finder, el Dock y Abrir con en macOS, desde Abrir con del Explorador mediante el instalador de Windows o el MSIX en Windows, mediante la entrada de escritorio de los paquetes Debian, Arch, AppImage y Flatpak en Linux, y mediante Abrir en en iOS:

```yaml
file_types:
  - name: Flow File
    extensions: [flow]
    # id: dev.example.hello.flow   # <id>.<first extension> by default; another's ID imports its type
    # icon: assets/Flow.icns       # macOS document icon; or a square PNG
    # role: viewer                 # editor by default
    # conforms: [public.json]      # public.data by default
    # mime: [application/json]     # existing MIME types; otherwise inferred on Linux
```

El instalador de Windows añade la aplicación a Abrir con sin reemplazar la predeterminada del usuario, y elimina sus registros al actualizar y desinstalar. Linux asigna los tipos MIME de extensiones comunes desde una tabla fija, y da a una extensión desconocida del tipo propio de la aplicación `application/x-<type id>` con una definición de MIME compartido. `mime` designa tipos existentes en su lugar; un tipo importado cuya extensión falte en la tabla lo necesita. Ejecutar una AppImage no registra nada: una herramienta de integración de escritorio instala su entrada y definiciones.

La aplicación recibe los archivos mediante `desktop.OnOpenFiles`, incluido un lanzamiento posterior en Windows o Linux mientras está en ejecución y aperturas de documentos en escenas de iOS. Los archivos que lanzaron la aplicación esperan al primer manejador:

```go
desktop.OnOpenFiles(cx, func(paths []string) { open(paths[0]) })
```

En iOS, los documentos normales se importan en archivos temporales propiedad de la aplicación con sus nombres originales. Las lecturas de proveedores de archivos se coordinan fuera del hilo de interfaz de usuario y sus ámbitos de seguridad finalizan antes de que se ejecute el manejador. Mueve una importación a `cx.DataDir()` para conservarla; las ediciones afectan a la copia. iOS usa el icono de documento de la aplicación; `icon` se aplica en macOS. Los App Clips y los paquetes de tvOS no registran tipos de documento. Las aperturas de archivos permanecen separadas de `OnOpenLinks`.

`desktop.AddRecentFile(cx, path)` registra un documento abierto por el usuario: en el menú del Dock y Archivo › Abrir recientes en macOS, en la categoría Recientes de la Jump List en Windows, y en los archivos usados recientemente del escritorio en Linux. Una selección llega al mismo manejador. `desktop.ClearRecentFiles` olvida los registros de la aplicación, conservando en Linux el historial de otras aplicaciones sobre archivos compartidos; Linux también respeta la configuración de historial de GTK.

`desktop.SetDockMenu(cx, items)` establece acciones en el icono de la aplicación: el menú del Dock en macOS, las tareas de la Jump List en Windows y la lista rápida en el dock de Ubuntu, Plasma y Dash to Dock en Linux cuando se admiten listas rápidas. Linux necesita una entrada de escritorio instalada que coincida con el `id` de la aplicación, y conserva submenús, separadores, elementos marcados y deshabilitados. El menú pertenece al ámbito que lo estableció, y el último configurador entre las ventanas de la aplicación lo reemplaza.

## Enlaces

`url_schemes` dota a la aplicación de enlaces propios, como `myapp://settings`, que la abren desde un navegador, otra aplicación, `open`, `start` o `xdg-open`. Las aplicaciones de macOS, iOS y Android los registran, así como el instalador de Windows, el paquete MSIX y la entrada de escritorio de Linux incluida en los paquetes Debian, Arch, AppImage y Flatpak. Desinstalar los elimina.

```yaml
url_schemes: [myapp]
# app_links: [links.example.com]  # HTTPS hosts for the full app
```

La aplicación los recibe mediante `desktop.OnOpenLinks`, también mientras está en ejecución: en Windows y Linux un segundo lanzamiento entrega su enlace a la aplicación en ejecución y sale. Un enlace que lanzó la aplicación espera al primer manejador. Cualquiera puede escribir un enlace así, y en Windows y Linux el manejador recibe cualquier URL con la que se inicie la aplicación, por lo que conviene comprobar el esquema y preguntar al usuario antes de que un enlace modifique algo:

```go
desktop.OnOpenLinks(cx, func(links []string) { show(links[0]) })
```

En iOS y Android, `mobile.OnOpenLinks` recibe enlaces al iniciar y mientras la aplicación está en ejecución. Abre uno con `xcrun simctl openurl booted myapp://settings` en el simulador de iOS o `adb shell am start -a android.intent.action.VIEW -d myapp://settings` en Android. `app.OpenLinks("myapp://settings")` entrega uno en una prueba.

Para enlaces universales de iOS, declara hosts HTTPS explícitos con `app_links: [links.example.com]`. Cada host es un nombre DNS sin esquema, ruta, puerto, dirección IP ni comodín; los nombres se pasan a minúsculas y se rechazan los duplicados. La herramienta escribe `applinks:<host>` en el entitlement Associated Domains de la aplicación completa, incluida la firma gestionada por Xcode; un perfil IPA debe autorizarlo. Los App Clips conservan las declaraciones `appclips:` independientes de `app_clip.domains`.

Cada host debe servir un archivo `apple-app-site-association` que nombre la aplicación completa en `https://<host>/.well-known/apple-app-site-association`; los metadatos del paquete por sí solos no pueden autorizar un enlace universal. Los [requisitos de asociación de Apple](https://developer.apple.com/documentation/xcode/supporting-associated-domains) describen la configuración del servidor. Las firmas ad hoc del simulador omiten los entitlements restringidos de desarrollador, por lo que una prueba de delegado no establece la asociación con el servidor.

En iOS, `mobile.OpenedURL(cx)` rastrea la invocación más reciente de App Clip, enlace universal o URL de esquema personalizado, incluido el valor inicial antes de que se monte la raíz. La entrega de esquemas personalizados sigue llegando a `mobile.OnOpenLinks`; el valor de invocación está actualizado cuando se ejecuta ese manejador. Las aperturas de archivos permanecen en `OnOpenFiles` y no reemplazan la URL de invocación. El soporte de invocación en Android/Web se sigue por separado en #1475.

## Elementos compartidos entrantes en Android

Elige los tipos MIME que la aplicación gestiona desde la hoja de compartir de Android:

```yaml
android:
  share_types: [text/plain, 'image/*', application/pdf]
```

Las compilaciones de APK y App Bundle registran esos tipos tanto para un único elemento como para múltiples elementos. Registra [`mobile.OnShared`](services.md#abrir-enlaces-y-archivos) para recibir cada elemento compartido.

Para una aplicación web con direcciones basadas en rutas, establece `web.base` a su directorio de despliegue (`/` o `/my-app/`) y configura tu servidor de producción para servir `index.html` en las rutas de la aplicación. El servidor de desarrollo ya proporciona ese respaldo para peticiones de navegación HTML. Los recursos permanecen relativos a esa base HTML fija cuando se recarga un enlace profundo. Consulta [Direcciones de la aplicación](addresses.md).

## Elementos compartidos entrantes en iOS

Declara los Uniform Type Identifiers que acepta la extensión de compartir, con tipos de URL y texto antes de tipos de archivo más amplios:

```yaml
apple:
  share_types: [public.url, public.plain-text, public.image, com.adobe.pdf]
  # app_group: group.com.example.app  # defaults to group.<id>
sign:
  # share_profile: assets/Share.mobileprovision
```

`build` y `dev` empaquetan una extensión de compartir en Go, `<id>.share`, junto con la aplicación. Xcode la firma y registra su App Group para ejecuciones en dispositivo. Para un IPA, habilita el mismo App Group para `<id>` y `<id>.share`; `sign.profile` y `sign.share_profile` deben concederlo. Los App Clips, tvOS y watchOS no tienen extensión de compartir.

La extensión copia hasta 128 elementos y 512 MiB de archivos en el App Group y luego completa la hoja de compartir. `mobile.OnShared` recibe esos lotes mientras la aplicación está activa o la siguiente vez que el usuario la abre; sobreviven a que la extensión y la aplicación se cierren antes de la entrega. iOS no proporciona ninguna API compatible para que una extensión Share pase a primer plano o lance su aplicación contenedora. Por tanto, esta transferencia no abre automáticamente la aplicación.

## Miniprogramas

```sh
go tool nsgo build -target miniprogram_wechat -run
go tool nsgo dev -target miniprogram_alipay
```

Cada destino escribe un proyecto para la herramienta de desarrollo de su plataforma, también en `dist/releases/miniprogram/<platform>`, que `-run` y `dev` abren; `dev` vuelve a compilar al guardar y la herramienta recarga:

| Destino | Plataforma | Herramienta |
| --- | --- | --- |
| `miniprogram_wechat` | WeChat | [WeChat DevTools](https://developers.weixin.qq.com/miniprogram/dev/devtools/download.html), con su puerto de servicio activado (Ajustes, Seguridad) |
| `miniprogram_qq` | QQ | [Herramienta de desarrollo de QQ](https://q.qq.com/wiki/tools/devtool/), en macOS |
| `miniprogram_alipay` | Alipay | [Mini Program Studio de Alipay](https://opendocs.alipay.com/mini/ide/download), en macOS |
| `miniprogram_douyin` | Douyin y otras aplicaciones de ByteDance | [Herramienta de desarrollo de Douyin](https://developer.open-douyin.com/docs/resource/zh-CN/mini-app/develop/developer-instrument/download/developer-instrument-update-and-download), en macOS |

`miniprogram.appid` proporciona el AppID del miniprograma en cada plataforma (`wechat`, `qq`, `alipay`, `douyin`); sin él, WeChat DevTools lo previsualiza con su AppID de visitante y las demás herramientas solicitan uno. En WeChat, `-run` y cada recompilación de `dev` cierran y vuelven a abrir automáticamente el proyecto mediante la CLI, de modo que DevTools compile los archivos nuevos. Esto se comprobó con DevTools 2.01.2510290. La barra de navegación de la plataforma muestra el título de la aplicación; con `custom_navigation`, la aplicación ocupa toda la página y su área segura la mantiene apartada del botón de menú. La aplicación lee el sistema operativo del teléfono con `Env.Simulated` verdadero, igual que en la web con `--nsgo-os`: las páginas de una pila muestran su propio `Header`, ya que ningún encabezado del sistema lo sustituye. QQ, Alipay y Douyin aún no se han probado en sus herramientas, y Alipay todavía no pasa la acción atrás a la aplicación, los fotogramas clave ni la altura del teclado.

Los miniprogramas son experimentales. Un miniprograma ejecuta únicamente aplicaciones en Go, no TSX, y las plataformas limitan el módulo comprimido de la aplicación a 2 MB: una aplicación de unas pocas pantallas sobre `base` cabe, como hace `examples/gallery-miniprogram`, mientras que una con los controles de `ui` aún no, y `build` avisa cuando no cabe. Los shaders no dibujan nada allí, y las vistas web, los vídeos, los componentes nativos y los archivos de fuentes de la aplicación todavía no se muestran.

## HarmonyOS

```sh
go tool nsgo build -target harmonyos -run
```

`harmonyos` escribe `<name>-<version>-harmonyos.hap` en `dist/releases`. El motor se ejecuta como WebAssembly en ArkWeb, y un anfitrión ArkTS dibuja lo que monta con vistas ArkUI, de modo que el texto, los campos, el desplazamiento y los cuadros de diálogo son los del propio sistema. Con `hdc` en `PATH`, `-run` instala el HAP en el dispositivo o emulador conectado y lo inicia, y `nsgo test -target harmonyos` ejecuta las pruebas de una aplicación desde un HAP sin firmar en un emulador bajo una identidad de paquete `.nsgotest` separada, eliminando ese HAP de prueba tras la ejecución para que la aplicación instalada siga disponible.

Para compilar el anfitrión ArkTS a partir de las plantillas y usar el `id` de la aplicación como nombre del paquete, pon `hvigorw` en `PATH` y configura `OHOS_BASE_SDK_HOME` apuntando a un SDK de OpenHarmony con API 26 (`26.0.0`), la versión de SDK de compilación y destino de las plantillas. `build.harmonyos_bundles` elige `auto` (el predeterminado), `exact` (que requiere el SDK) o `padded` (que rellena el ID hasta la longitud fija del anfitrión enviado en el repositorio y no necesita SDK). Las pruebas de destino usan la misma configuración e imprimen qué anfitrión empaquetaron. `harmonyos.permissions` enumera los nombres `ohos.permission` que solicita la aplicación, como `MICROPHONE` y `CAMERA`, y `widget.title` es lo que muestra su tarjeta en la pantalla de inicio.

HarmonyOS es una vista previa. Los emuladores pueden instalar HAP sin firmar. [Las notas de arquitectura](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/harmonyos.md) enumeran las funciones de ArkUI que dibuja la vista previa.

### HAPs firmados

Establece `harmonyos.signing` para firmar un HAP con tus materiales de AppGallery Connect. Una instalación directa en un teléfono Huawei necesita un certificado y un perfil de depuración para ese dispositivo. Usa los materiales de lanzamiento para la distribución. [Guía de firma de Huawei](https://developer.huawei.com/consumer/en/doc/harmonyos-guides-V2/command-line-building-app-hap-hvigor-0000001553061909-V2/)

```yaml
harmonyos:
  signing:
    keystore: signing/app.p12
    key_alias: app
    certificate: signing/app.cer
    profile: signing/app.p7b
```

Las rutas son relativas a `nsgo.yaml`. Establece `NSGO_HARMONY_KEYSTORE_PASSWORD` y `NSGO_HARMONY_KEY_PASSWORD` en el entorno. La firma utiliza `hap-sign-tool.jar` del SDK y Java de `JAVA_HOME` o `PATH`. La compilación mantiene el `id` exacto de la aplicación para coincidir con el perfil, por lo que firmar necesita el SDK y `hvigorw` incluso en modo `auto`. `go tool nsgo doctor`, ejecutado en el directorio de la aplicación, comprueba la configuración de firma y el nombre de paquete del perfil sin contraseñas. `build -run` entrega a `hdc` el HAP firmado.

## Firma

```yaml
sign:
  identity: "Apple Development: you@example.com (ABCDE12345)"  # signs the macOS app, else ad hoc
  profile: assets/AppStore.mobileprovision  # signs the IPA
  keystore: assets/release.p12              # signs APKs, with key_alias
  certificate: assets/codesign.pfx          # signs the Windows app and its installer
  # widgets_profile: assets/Widgets.mobileprovision  # the iOS extension's profile; use a macOS profile for a Mac App Store build
  # share_profile: assets/Share.mobileprovision      # signs the IPA's share extension
  # clip_profile: assets/Clip.mobileprovision        # signs the IPA's App Clip
```

Las contraseñas del almacén de claves provienen de `NSGO_KEYSTORE_PASSWORD` y `NSGO_KEY_PASSWORD`. `-team` firma el paquete de iPhone. `-profile`, `-widgets-profile`, `-share-profile`, `-clip-profile` y `-macos-profile` reemplazan desde la línea de comandos los perfiles de aprovisionamiento de la aplicación, la extensión de widgets, la extensión de compartir, el App Clip y el Mac App Store. `-installer` reemplaza la identidad de Mac Installer Distribution.

### Firmar paquetes de Windows

Authenticode identifica al editor de una descarga de Windows; [SmartScreen comprueba su reputación por separado](https://learn.microsoft.com/windows/apps/package-and-deploy/smartscreen-reputation). `sign.certificate`, un `.pfx`, firma el ejecutable, el complemento de comandos, los programas y bibliotecas que aportan los recursos de la aplicación, el instalador con su desinstalador y el MSIX compilado para el canal Store; su contraseña proviene de `NSGO_CERTIFICATE_PASSWORD`. Las firmas se generan en Go, igual que las genera `signtool`, por lo que un Mac las firma sin el SDK de Windows:

```yaml
sign:
  certificate: assets/codesign.pfx
  timestamp_url: http://timestamp.digicert.com  # the RFC 3161 server that timestamps them
```

`sign.timestamp_url` usa por defecto el de DigiCert, y `none` deja las firmas sin marca de tiempo, con lo que caducan al expirar el certificado. Un certificado en un token de hardware o en un servicio en la nube firma en su lugar mediante un comando propio, en el que `%1` es el archivo a firmar y que la compilación ejecuta para cada archivo anterior:

```yaml
sign:
  sign_command: "signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 /a %1"
```

`build` se detiene si el comando tiene éxito pero deja el ejecutable de la aplicación, el complemento de comandos, un recurso PE o el ejecutable de instalación sin una firma Authenticode incrustada.

El desinstalador incluido en el instalador se firma antes de incrustarse, con el mismo `sign.certificate` o `sign.sign_command` que la aplicación y el instalador. `-certificate`, `-sign-command` y `-timestamp-url` configuran estos valores desde la línea de comandos.

### Notarizar

Las aplicaciones descargadas en un Mac se abren sin la advertencia de Gatekeeper una vez que Apple las ha notarizado. Firma con un Developer ID y especifica las credenciales que `notarytool` guarda en tu llavero:

```sh
xcrun notarytool store-credentials hello --apple-id you@example.com --team-id ABCDE12345
```

```yaml
sign:
  identity: "Developer ID Application: Example Inc (ABCDE12345)"
  notarize: hello   # the keychain profile above; or -notarize hello
```

`build` firma entonces con el entorno de ejecución reforzado, envía la imagen de disco, espera el veredicto de Apple (unos minutos) y grapa el tique a la imagen y a la aplicación; un rechazo imprime el registro de Apple. En CI, `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID` y `NSGO_NOTARY_ISSUER`, el archivo `.p8`, el ID y el emisor de una clave de API de App Store Connect, ocupan el lugar del perfil. El entorno de ejecución reforzado deniega el micrófono, la cámara, los calendarios y la ubicación salvo que la aplicación los solicite, lo cual hace `build` cuando `apple.info` contiene `NSMicrophoneUsageDescription`, `NSCameraUsageDescription`, una descripción de uso de calendarios o recordatorios, o una descripción de uso de ubicación.

## Widgets y Live Activities

`build` y `dev` compilan con el compilador Swift de Xcode la extensión que dibuja los widgets y Live Activities de la aplicación dentro de los paquetes de macOS e iOS, sin ningún paso separado. En iOS, la aplicación y sus widgets comparten un grupo de aplicaciones: `-run` en un iPhone hace que Xcode firme ambos y registre el grupo; para un IPA, habilítalo tanto para `<id>` como para `<id>.widgets`, y firma la extensión con `sign.widgets_profile`. Los APK y AAB independientes de Android incluyen los proveedores de App Widget y los recursos de `widgets`; el entorno de ejecución Java precompilado dibuja sus líneas temporales sin necesidad de SDK ni compilador Java en las compilaciones de aplicaciones. Android 16 muestra las Live Activities como Live Updates, que no necesitan extensión. [Widgets](widgets.md) y [Live Activities](live-activities.md) cubren su configuración y API.

## Android App Links

`app_links` nombra hosts HTTPS explícitos cuyos enlaces abren la aplicación completa de Android:

```yaml
app_links: [example.com, shop.example.com]
```

El APK y el App Bundle declaran un filtro `ACTION_VIEW` navegable con `autoVerify` para cada host. Usa hosts DNS sin esquema, ruta, puerto, comodín ni dirección IP. Sirve `https://<host>/.well-known/assetlinks.json` con el ID de paquete de la aplicación y la huella del certificado de firma; Android verifica esa asociación antes de dirigir enlaces a la aplicación. La herramienta no publica archivos de asociación; la [guía de verificación de Android](https://developer.android.com/training/app-links/verify-applinks) describe su configuración. Un anfitrión Android incrustado declara sus propios filtros de intención y reenvía los enlaces aceptados a su `NsgoView`.

`mobile.OpenedURL(cx)` lee la invocación aceptada antes del primer renderizado de la raíz y cambia cuando otro enlace abre la aplicación en ejecución. `mobile.OnOpenLinks` sigue recibiendo cada enlace aceptado. Una aplicación web de página completa lee la dirección actual de su página mediante `mobile.OpenedURL`, incluidos el historial del navegador y los cambios de hash; una aplicación montada dentro de otra página no lee la dirección de la página contenedora. Usa [`cx.AppURL`](addresses.md) cuando la aplicación también escriba su dirección.

## App Clips

Un App Clip es una pequeña parte de una aplicación iOS que se ejecuta desde un enlace, un código o una etiqueta NFC sin instalar la aplicación. `app_clip` compila uno dentro del paquete de iOS, a partir de un paquete `main` propio para enlazar únicamente lo que muestra:

```yaml
app_clip:
  main: ./clip              # the clip's main package, the app's own by default
  domains: [example.com]    # whose links open it
  # name: Hello Clip        # "<name> Clip" by default
  # id: dev.example.hello.Clip  # <id>.Clip by default
  # minimum: "17.0"         # 16.0 by default; 17.0 allows 100 MB when only links open it
  # notifications: true     # ask for notifications for a day
  # location: true          # let the clip confirm where it was opened
```

El clip lee el enlace que lo abrió con `mobile.OpenedURL`, que cambia cuando otro enlace lo abre de nuevo, y `mobile.IsAppClip` indica si el código se ejecuta como tal. El sistema descarga un clip de hasta 15 MB, o 100 MB a partir de iOS 17 cuando solo lo abren enlaces, y `build` rechaza uno mayor; un clip que enlaza `ui` ocupa unos 11 MB. `dev -target ios_simulator -clip https://example.com/order?table=12` ejecuta únicamente el clip en el simulador, abierto desde ese enlace; `-target ios` lo ejecuta en un iPhone e imprime el código QR del enlace: añade el enlace en Ajustes > Desarrollador > Experiencias locales y luego escanéalo con la cámara. Usa `-release` si los símbolos de desarrollo hacen que el clip supere el límite de tamaño del QR. Apple firma App Clips únicamente para equipos del Apple Developer Program, no para equipos personales.

`dev -target ios` y `build -run -target ios` también conservan el clip dentro de la aplicación completa: Xcode lo firma como un destino separado y lo incrusta antes de firmar la aplicación. La firma automática necesita una cuenta de equipo de pago autenticada en Xcode, con aprovisionamiento tanto para la aplicación como para el clip. Para el App Store, habilita App Clips para `<id>`, firma el clip con `sign.clip_profile` y sirve el archivo `apple-app-site-association` de los dominios. El ejemplo `examples/appclip` del repositorio es una cafetería cuyo clip realiza pedidos en una mesa.

Para pasar un pedido u otros datos locales a la aplicación completa, configura un grupo explícito junto a `app_clip`:

```yaml
apple:
  app_group: group.dev.example.hello
```

El clip y la aplicación completa comparten entonces `cx.DataDir()` y `cx.Storage()` en el contenedor del grupo. Los archivos bajo el directorio de datos y las entradas clave-valor que escribe el clip están disponibles cuando se inicia la aplicación completa; `cx.LogsDir()` sigue ese directorio, mientras que cada paquete conserva su propia caché. La firma de Xcode reclama el grupo para ambos destinos; al usar perfiles, habilita el mismo grupo para ambos App ID y concédelo en `sign.profile` y `sign.clip_profile`. Una concesión ausente rechaza la firma, y un grupo inaccesible detiene el arranque. Un directorio de datos explícito del anfitrión sigue teniendo prioridad.

Sin `apple.app_group`, cada paquete conserva su propio almacenamiento en zona aislada. Habilitar el ajuste no migra los datos almacenados previamente en esas zonas aisladas, y las aplicaciones con solo widgets mantienen su ubicación de almacenamiento existente. Guarda los secretos compartidos en el llavero en lugar de almacenamiento plano. El ejemplo de la cafetería persiste el pedido de su clip y lo muestra en la aplicación completa.

Desde una acción explícita del usuario dentro del clip, llama a `mobile.ShowAppClipOverlay(cx, mobile.AppClipOverlayBottom)` para pedir a StoreKit que recomiende su aplicación completa correspondiente. `AppClipOverlayBottomRaised` deja espacio para una barra inferior. La función devuelta descarta esa solicitud; la limpieza del ámbito también la descarta, y una solicitud más antigua no puede descartar una más reciente. Las llamadas fuera de un App Clip nativo de iOS no hacen nada. El App Store decide si puede cargar y mostrar la recomendación, por lo que la llamada no informa ni de una oferta mostrada ni de una instalación. Mantén utilizable la tarea del clip sin instalar la aplicación completa. [Guía de superposición de Apple](https://developer.apple.com/documentation/appclip/recommending-your-app-to-app-clip-users)

## Incrustar en otra aplicación

`-embed` empaqueta la aplicación para una aplicación iOS o Android propia: un XCFramework, de frameworks dinámicos con `-dynamic`, o un AAR cuyo `NSGoView` o `NsgoView` la muestra. Los programas de escritorio en Go muestran aplicaciones con `nsgo.NewView`, y las páginas web con el `nsgo.js` del sitio, cuyo anfitrión es el de la página o el que nombra `web.host` en `nsgo.yaml`. Los ejemplos `examples/embed-*` del repositorio muestran cada caso; [Incrustación](embedding.md) cubre el lado del anfitrión.
