---
title: Configuración
description: Instala lo que NSGo necesita, crea una aplicación y ejecútala en cada destino.
group: Primeros pasos
order: 10
---

## Requisitos

- [Go](https://go.dev/dl/) 1.27.3 o posterior.
- Para compilar para iOS, tvOS y watchOS, una Mac con Xcode. Todos los demás destinos se compilan desde macOS solo con Go; el emulador de Android x86_64 también necesita un Android NDK.
- Para ejecutar en un iPhone, un Apple ID en las cuentas (Accounts) de Xcode y el iPhone marcado como confiable, con el Modo de desarrollador activado.
- Para ejecutar en Android, `adb` de las platform-tools del Android SDK.
- Para ejecutar en Windows, solo Go, que también compila los paquetes de Windows y web.
- En Linux, GTK 4.14 o más reciente, Pango y Cairo. Las imágenes de red usan libsoup 3, las vistas web usan WebKitGTK 6.0 y el audio y el video usan complementos de GStreamer. En sistemas con restricciones de espacios de nombres de usuario de AppArmor, [WebKitGTK necesita un perfil de bubblewrap](debugging.md#vistas-web-en-linux).

## Crea una aplicación

Crea un módulo, agrega NSGo y su herramienta, y luego deja que la herramienta cree una aplicación:

```sh
mkdir hello && cd hello
go mod init example.com/hello
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
```

`go get -tool` agrega `nsgo` al módulo una sola vez, de modo que `go tool nsgo` lo ejecuta sin instalar nada. `init` escribe:

| Archivo | Qué es |
| --- | --- |
| `app.go` | La aplicación, un contador: un componente `Root` y `main`, que llama a `nsgo.Run(Root)` |
| `app_test.go` | Una prueba que presiona el botón |
| `nsgo.yaml` | El nombre, id, versión y destinos de la aplicación |

`go tool nsgo init -template tsx` escribe en su lugar una aplicación en TSX, y `-template mixed` una aplicación en Go cuya entrada TSX utiliza sus propios componentes de Go.

Añade bibliotecas con `nsgo add <módulo>` en lugar de `go get`: muestra lo que recibirían `go.mod` y `go.sum` antes de escribirlos y nunca saca el NSGo del proyecto de sus versiones publicadas ([Añadir bibliotecas](building.md#añadir-bibliotecas)).

### Plantillas desde módulos de Go

Pasa la ruta del módulo de la plantilla con una versión opcional. Si se omite la versión, se resuelve a la última versión que pueda encontrar el proxy de módulos de Go.

```sh
go tool nsgo init -template example.com/my-template@v1.2.0 -module example.com/hello hello
```

Publica la plantilla inicial dentro del directorio `template/` del módulo. Incluye `nsgo.yaml` y `main.go`. La herramienta renderiza ambos, además de cualquier archivo terminado en `.tmpl`, con `text/template` de Go, y elimina `.tmpl` de los nombres de salida. Copia los demás archivos, incluidos los recursos binarios, sin cambios.

Las plantillas reciben `.Name` (el nombre visible de la aplicación), `.ID` (el ID generado de la aplicación), `.Slug` (el nombre en minúsculas con guiones) y `.Module` (el valor de `-module`, o el slug). Incluye un archivo `go.mod.tmpl` para declarar el módulo y las dependencias de la aplicación generada, ya que Go excluye los archivos `go.mod` anidados de las descargas de módulos. Sin uno, `-module` escribe el mismo `go.mod` mínimo que para una plantilla integrada.

## Ejecútala

```sh
go tool nsgo dev
```

`dev` compila la aplicación para esta máquina y la abre. En cada guardado vuelve a compilar y reinicia la aplicación, y un cambio solo en TSX actualiza la aplicación en ejecución en el acto. Si una compilación falla, se muestra sobre la aplicación con botones para copiar el error y volver a compilar.

Ejecútala en otro destino con `-target`:

```sh
go tool nsgo dev -target web                # in the browser
go tool nsgo dev -target ios_simulator      # in the iOS simulator
go tool nsgo dev -target tvos_simulator     # in the Apple TV simulator
go tool nsgo dev -target watchos_simulator  # in the Apple Watch simulator
go tool nsgo dev -target android            # on a connected device or emulator
```

`go tool nsgo doctor` lista cada destino indicando si esta máquina puede compilarlo y ejecutarlo, y qué le falta para aquellos que no puede, como Xcode, `adb`, un dispositivo conectado o GTK.

[Ejecución y depuración](debugging.md) cubre dispositivos, rastreo de actualizaciones, el monitor de rendimiento y otros lenguajes.

## Con un agente de programación

La skill de NSGo enseña a los agentes de programación, como Claude Code, Codex y Cursor, cómo se escribe, prueba y distribuye una aplicación de NSGo. Agrégala al directorio de la aplicación, o con `-g` para todos los proyectos:

```sh
bunx skills add nsgo-dev/nsgo
```

Los agentes que leen la documentación por sí mismos empiezan desde <https://nsgo.dev/llms.txt>, que lista cada página en Markdown en `https://nsgo.dev/docs/<page>.md`; <https://nsgo.dev/llms-full.txt> las contiene todas. Un agente también puede usar la aplicación en ejecución ([Ejecución y depuración](debugging.md#agentes)).

## Siguiente

- [Ejemplo: una aplicación de tareas](tutorials/todo.md): estado, una lista y una prueba, en unos diez minutos.
- [Ejemplo: una aplicación de notas](tutorials/notes.md): una pila de navegación y notas guardadas en el dispositivo.
- [Ejemplo: repositorios populares de GitHub](tutorials/repos.md): carga desde la red con errores y reintento.
