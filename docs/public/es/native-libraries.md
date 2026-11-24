---
title: Bibliotecas nativas
description: Distribuye bibliotecas compartidas precompiladas con un paquete de Go mediante los pasos habituales de empaquetado, licencias y firma de NSGo.
group: Herramientas
order: 23
---

Un paquete de Go puede distribuir las bibliotecas compartidas que carga mediante purego colocando `nsgo-natives.yaml` en el directorio de su paquete. Cuando una aplicación importa el paquete, NSGo selecciona sus declaraciones para el destino, verifica los archivos y los incluye antes de firmar y crear los instaladores. La aplicación no necesita configuración adicional ni ningún comando de empaquetado extra.

## Declarar bibliotecas

```yaml
version: 1
libraries:
  macos_arm64:
    - name: libexample.dylib
      path: lib/macos-arm64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  macos_x64:
    - name: libexample.dylib
      url: https://downloads.example.org/v1.0/macos-x64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  windows_arm64: []
  windows_x64: []
  linux_arm64: []
  linux_x64: []
```

Las claves admitidas son `macos_arm64`, `macos_x64`, `windows_arm64`, `windows_x64`, `linux_arm64` y `linux_x64`. Declara todos los destinos en los que se seleccione el paquete de Go; `[]` indica explícitamente que ese destino no necesita ninguna biblioteca empaquetada. En su lugar, las etiquetas de compilación (`build tags`) de Go pueden excluir el paquete de los destinos que no admite. `macos_universal` acepta ambas entradas de macOS.

Cada biblioteca tiene:

- `name`: su nombre de archivo instalado, terminado en `.dylib` en macOS, `.dll` en Windows, o `.so` o `.so.*` con versión en Linux.
- Exactamente uno de `path`, un archivo relativo a este directorio del paquete, o `url`, una URL HTTP(S) para la propia biblioteca. Publica bibliotecas extraídas en lugar de URL de archivos comprimidos. Las URL omiten nombres de usuario y contraseñas incrustados.
- `sha256`: el SHA-256 del archivo original, obligatorio para cualquiera de las dos fuentes. Por ejemplo, `shasum -a 256 lib/macos-arm64/libexample.dylib` lo imprime en macOS.
- `licenses`: rutas no vacías a los archivos originales de licencia y avisos, relativas a este directorio del paquete. Estos textos se incluyen en `THIRD_PARTY_NOTICES.txt` y llegan a `core.ThirdPartyNotices()` en compilaciones de lanzamiento (`release`). La [política de licencias](building.md#avisos-de-licencia) existente de la aplicación, incluidas las excepciones exactas de módulos, se aplica de forma independiente a las licencias nativas.

Las rutas locales y los enlaces simbólicos permanecen dentro del paquete que los declara. NSGo rechaza campos desconocidos, discrepancias de suma de verificación, archivos para otro sistema operativo o arquitectura y nombres instalados en conflicto. Dos paquetes pueden compartir un archivo con el mismo nombre si tienen la misma suma de verificación. Cada archivo está limitado a 1 GiB.

## Empaquetar y cargar

| Destino | Directorio de bibliotecas |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows y Linux | Junto al ejecutable |

El paquete carga su biblioteca desde ese directorio, usando la ubicación del ejecutable en lugar del directorio de trabajo. Precompila las dependencias para que sus bibliotecas compañeras se carguen desde el directorio en el que se distribuyen. Declara cada archivo compañero necesario y cada nombre de archivo que necesite el cargador.

NSGo vuelve a comprobar las descargas en caché antes de reutilizarlas y coordina compilaciones concurrentes mediante un bloqueo de archivo. La caché de contenido reside en el directorio de caché de usuario del sistema operativo, en `nsgo/natives`. Los archivos locales se verifican a medida que se copian.

Un paquete universal de macOS requiere los mismos nombres de biblioteca para ambas arquitecturas. NSGo une las porciones con `lipo`; una biblioteca ya universal del publicador puede servir para cualquiera de las entradas. Las bibliotecas se firman antes que la aplicación y luego pasan por los pasos normales de imagen de disco, firma y notarización de la aplicación. La firma en Windows incluye las DLL declaradas incluso con `build.installers: false`. Los instaladores de Linux incluyen los mismos archivos y avisos que el directorio del ejecutable.

El desarrollo y las pruebas en destinos de escritorio nativos usan los mismos pasos de selección y copia de paquetes. Las declaraciones se seleccionan con el destino de la compilación, las etiquetas y el overlay de Go generado, incluidos los reemplazos locales y los paquetes en `vendor`. Otros destinos actualmente no empaquetan bibliotecas desde este archivo.

## Conservar archivos al usar vendor

La operación `vendor` de Go necesita datos incrustados para conservar archivos debajo de un directorio de paquete. Un paquete puede marcar su manifiesto y licencias para incluirlos sin usar sus bytes en tiempo de ejecución:

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

Incluye también las rutas de bibliotecas locales en esa directiva `embed` cuando uses `path`, por ejemplo `lib/*`. Las descargas por URL solo necesitan el manifiesto y los archivos de licencia en el paquete de Go.
