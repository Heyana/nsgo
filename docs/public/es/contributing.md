---
title: Configuración de desarrollo
description: Compile NSGo desde el código fuente, ejecute la galería y pruebe un cambio.
group: Contribuir
order: 80
---

## Requisitos

macOS con Xcode y Go 1.27.3 o posterior compila todos los destinos para arm64, incluido el APK de Android. Ejecutar en dispositivos requiere lo que enumera [Configuración](getting-started.md#requirements), y el destino del emulador de Android x86_64, `android_x64`, requiere un NDK de Android, ya que Go enlaza android/amd64 solo con cgo. Los programas de instalación y desinstalación de Windows se compilan solo con Go.

```sh
git clone https://github.com/nsgo-dev/nsgo.git
cd nsgo
```

Lea [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) y [AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md) antes de modificar código.

## La galería

La galería tiene una demostración de cada elemento, componente de `base` y `ui` y servicio. Una ventana ancha muestra las demostraciones bajo pestañas en una barra lateral; una estrecha muestra las pestañas y la configuración en una barra inferior, con las demostraciones superpuestas. Ejecútela desde la raíz del repositorio:

```sh
go tool nsgo dev examples/gallery                   # nsgo.yaml's dev target, exiting with the app
go tool nsgo dev examples/gallery-tsx               # the gallery's components and apps, written in TSX
go tool nsgo dev -target web examples/gallery       # in the browser, serving until interrupted
go tool nsgo dev -target ios_simulator examples/gallery
go tool nsgo dev -target ios examples/gallery       # on the connected iPhone, signed by Xcode
go tool nsgo dev -target android examples/gallery   # on the connected device or emulator, over adb
go tool nsgo build -target all examples/gallery     # every platform's packages in examples/gallery/dist, what to hand out in dist/releases
```

## Comprobación y pruebas

```sh
go tool nsgo check                 # vet every package
go tool nsgo check -target all     # vet every available Go/Xcode target instead
go tool nsgo test                  # every package's tests
go tool nsgo test -bench . ./core  # core's benchmarks
(cd benchmark && go run ./runner)  # measure against other frameworks
# capacity curve for NSGo's 600-cell grid, without rebuilding it
(cd benchmark && go run ./runner -frameworks nsgo -cases grid -grid-cols 20 -grid-rows 15 -out /tmp/grid.json)
```

`-target` analiza los destinos que nombra en lugar del host y, en el repositorio de NSGo, el cliente de desarrollo, que el `check` simple ya analiza. Para analizar las plataformas que toca un cambio, nómbrenlas todas en una sola comprobación, como `check -target web,android -jobs 2`, en lugar de iniciar varias: `-jobs n` analiza hasta n destinos a la vez, cada `go vet` con n trabajadores de compilación, donde el valor predeterminado es un destino a la vez con el valor predeterminado de Go, y una comprobación iniciada mientras otra se ejecuta en el mismo módulo espera por ella y lo indica. El flujo `check.yml` de CI analiza todos los destinos.

Ejecutado en la raíz del repositorio, o en `internal/dev`, el comando sin interfaz gráfica prueba `internal/dev` una segunda vez con la etiqueta de compilación `nsgodev`, que los archivos del cliente de desarrollo necesitan, de modo que sus pruebas se ejecutan localmente y en CI. Ambas ejecuciones aceptan los mismos filtros y banderas y comparten un mismo recuento e informe JUnit, donde el paquete de la segunda ejecución se llama `internal/dev [nsgodev]`.

`check -target all` analiza los destinos de macOS, Windows, Linux, Android arm64 y web en cualquier host. También analiza iOS, tvOS y watchOS cuando sus SDK de Xcode están disponibles, y enumera los que omite. Un destino nombrado explícitamente, como `check -target ios`, falla si sus herramientas no están disponibles, incluso cuando se combina con `all`. Un destino fallido no impide comprobar los destinos restantes, y el comando falla si falla cualquier comprobación requerida. `build -target all` sigue requiriendo macOS con Xcode. En el propio repositorio de NSGo, cada destino nombrado también analiza sus hosts de prueba de plataforma con `nsgotest`; CI nombra explícitamente el destino preliminar `harmonyos`, ya que está fuera de `all`.

El trabajo de CI de Linux ejecuta las pruebas `js/wasm` del backend de Go para HarmonyOS con Node.js y el lanzador Wasm de Go. Ejecute localmente la prueba de la galería con un emulador de HarmonyOS conectado mediante `hdc`. Compilar las [plantillas de host de HarmonyOS](building.md#harmonyos) requiere la API 26 del SDK de OpenHarmony.

```sh
go tool nsgo test -env GOOS=js -env GOARCH=wasm ./internal/platform/harmonyos \
  -- "-exec=env -i \"PATH=$PATH\" \"$(go env GOROOT)/lib/wasm/go_js_wasm_exec\"" -count=1
go tool nsgo test -target harmonyos -run TestHarmonyOSPages -timeout 10m ./examples/gallery
```

En una sesión de Wayland en un escritorio Linux, ejecute la comprobación real de aceptación del portal de atajos globales con `NSGO_TEST_GLOBAL_SHORTCUT_PORTAL=<installed-desktop-id> go tool nsgo test -run TestGlobalShortcutPortalWithPermission ./internal/platform/linux`. El ID debe tener una entrada `.desktop`. Conceda Ctrl+Alt+K en el aviso del compositor y luego púlselo con otra aplicación enfocada; la comprobación espera tanto el registro como la activación. Se omite salvo que la variable de entorno esté definida.

Con Docker disponible en un host Linux, `go tool nsgo test -run '^TestLinuxPackagesNative$' -env NSGO_TEST_LINUX_PACKAGES=1 ./cmd/nsgo -- -timeout=20m` comprueba instalaciones reales en contenedores desechables de Debian, Ubuntu y Arch. Descarga sus imágenes oficiales e instala dependencias de GTK dentro de ellos, verifica cada base de datos de paquetes y los archivos instalados, y luego elimina el paquete. Nunca se utiliza la base de datos de paquetes del host.

## Lanzamientos

NSGo lanza versiones bajo demanda cuando un proyecto dependiente necesita una versión etiquetada o los mantenedores tienen cambios listos para publicar. Al solicitar un lanzamiento para una biblioteca, enlace el trabajo dependiente y nombre el commit que contiene el último cambio de NSGo que necesita. La primera etiqueta de lanzamiento que contenga ese commit es la versión que la biblioteca puede requerir.

Para probar un cambio fusionado antes de que sea etiquetado, ejecute `go get github.com/nsgo-dev/nsgo@main` desde el módulo dependiente. Puede usar un ID de commit en lugar de `main` para seleccionar la revisión que verificó. Go registra la pseudoversión resultante en `go.mod`.

Un mantenedor asigna a los cambios su versión y fecha en `CHANGELOG.md`, deja una nueva sección Unreleased y etiqueta el commit resultante en `main`. El [flujo de lanzamiento](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/release.yml) comprueba la etiqueta y la publica después de que pase el flujo de pruebas; las notas publicadas salen entonces de `CHANGELOG.md`, que conserva solo los cambios no publicados, hacia [Lanzamientos](https://github.com/nsgo-dev/nsgo/releases).

## NSH

NSGo consulta [Native Shell](https://github.com/longbridge/nsh) como referencia de diseño; ninguna de sus pruebas lo necesita. Para leerlo, clónelo junto al repositorio y cambie a su última etiqueta de lanzamiento, nunca a `main`:

```sh
git clone https://github.com/longbridge/nsh.git ../nsh
git -C ../nsh checkout "$(git -C ../nsh describe --tags --abbrev=0)"
```

## Siguiente

- [La base de código](contributing/codebase.md): dónde vive cada parte de NSGo.
- [Realizar un cambio](contributing/changes.md): lo que necesita una pull request.
