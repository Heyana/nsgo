---
title: Bibliotecas nativas
description: Distribuye bibliotecas compartidas precompiladas y frameworks de Apple con un paquete de Go mediante los pasos habituales de empaquetado, licencias y firma de NSGo.
group: Herramientas
order: 23
---

Un paquete de Go puede distribuir las bibliotecas compartidas que carga mediante purego colocando `nsgo-natives.yaml` en el directorio de su paquete y, desde la versión 2, sus frameworks para iOS y tvOS. Cuando una aplicación importa el paquete, NSGo selecciona sus declaraciones para el destino, verifica los archivos y los incluye antes de firmar y crear los instaladores. La aplicación no necesita configuración adicional ni ningún comando de empaquetado extra.

Las compilaciones de aplicaciones de escritorio y las pruebas en destinos desactivan cgo para el código de la aplicación. El código nativo debe usar purego y una biblioteca declarada en `nsgo-natives.yaml`. NSGo rechaza los paquetes de la aplicación que contienen archivos cgo antes de que empiece la compilación.

## Declarar bibliotecas

```yaml
version: 2
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
  ios:
    - name: Example.framework
      url: https://downloads.example.org/v1.0/ios/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  ios_simulator:
    - name: Example.framework
      path: lib/ios-simulator/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  tvos: []
  tvos_simulator: []
```

La versión 2 admite las claves `macos_arm64`, `macos_x64`, `windows_arm64`, `windows_x64`, `linux_arm64`, `linux_x64`, `ios`, `ios_simulator`, `tvos` y `tvos_simulator`. La versión 1 admite solo las seis claves de escritorio; un archivo se lee solo para las claves de su versión, así que las compilaciones de iOS y tvOS no empaquetan nada de un archivo de la versión 1. Declara todos los destinos en los que se seleccione el paquete de Go; `[]` indica explícitamente que ese destino no necesita ninguna biblioteca empaquetada. En su lugar, las etiquetas de compilación (`build tags`) de Go pueden excluir el paquete de los destinos que no admite. `macos_universal` acepta ambas entradas de macOS, y un destino de terminal las de su escritorio: `tui_macos_arm64` acepta las de `macos_arm64`.

Cada biblioteca tiene:

- `name`: su nombre de archivo instalado, terminado en `.dylib` en macOS, `.dll` en Windows, o `.so` o `.so.*` con versión en Linux. En iOS y tvOS es el de un framework, `<Name>.framework`, donde `Name` consta de letras ASCII, dígitos, `.` y `-`, y empieza y termina con una letra o un dígito.
- Exactamente uno de `path`, un archivo relativo a este directorio del paquete, o `url`, una URL HTTP(S) para la propia biblioteca. La versión 2 solo admite URL https, y una descarga falla cuando una redirección abandona https. Publica bibliotecas extraídas en lugar de URL de archivos comprimidos y, para un framework, solo su binario. Las URL omiten nombres de usuario y contraseñas incrustados.
- `sha256`: el SHA-256 del archivo original, obligatorio para cualquiera de las dos fuentes. Por ejemplo, `shasum -a 256 lib/macos-arm64/libexample.dylib` lo imprime en macOS.
- `licenses`: rutas no vacías a los archivos originales de licencia y avisos, relativas a este directorio del paquete. Estos textos se incluyen en `THIRD_PARTY_NOTICES.txt` y llegan a `core.ThirdPartyNotices()` en compilaciones de lanzamiento (`release`). La [política de licencias](building.md#avisos-de-licencia) existente de la aplicación, incluidas las excepciones exactas de módulos, se aplica de forma independiente a las licencias nativas.

Las rutas locales y los enlaces simbólicos permanecen dentro del paquete que los declara. NSGo rechaza campos desconocidos, discrepancias de suma de verificación, archivos para otro sistema operativo o arquitectura y nombres instalados en conflicto. Dos paquetes pueden compartir un archivo con el mismo nombre si tienen la misma suma de verificación. Cada archivo está limitado a 1 GiB.

## Empaquetar y cargar

| Destino | Directorio de bibliotecas |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows, Linux y destinos de terminal | Junto al ejecutable |
| iOS y tvOS | `<app>.app/Frameworks/<Name>.framework/<Name>` |

El paquete carga su biblioteca desde ese directorio, usando la ubicación del ejecutable en lugar del directorio de trabajo. Precompila las dependencias para que sus bibliotecas compañeras se carguen desde el directorio en el que se distribuyen. Declara cada archivo compañero necesario y cada nombre de archivo que necesite el cargador.

NSGo vuelve a comprobar las descargas en caché antes de reutilizarlas y coordina compilaciones concurrentes mediante un bloqueo de archivo. La caché de contenido reside en el directorio de caché de usuario del sistema operativo, en `nsgo/natives`. Los archivos locales se verifican a medida que se copian.

Un paquete universal de macOS requiere los mismos nombres de biblioteca para ambas arquitecturas. NSGo une las porciones con `lipo`; una biblioteca ya universal del publicador puede servir para cualquiera de las entradas. Las bibliotecas se firman antes que la aplicación y luego pasan por los pasos normales de imagen de disco, firma y notarización de la aplicación. La firma en Windows incluye las DLL declaradas incluso con `build.installers: false`. Los instaladores de Linux incluyen los mismos archivos y avisos que el directorio del ejecutable. En un Mac, los destinos de terminal firman de forma ad hoc las bibliotecas de macOS; en otros sistemas las incluyen tal como se publicaron, una universal entera, y rechazan una de arm64 que no esté ya firmada.

El desarrollo, `-run` y las pruebas en destinos usan los mismos pasos de selección y copia de paquetes, tanto en escritorio como en iOS y tvOS. Las declaraciones se seleccionan con el destino de la compilación, las etiquetas y el overlay de Go generado, incluidos los reemplazos locales y los paquetes en `vendor`. Android, HarmonyOS, watchOS, la web y los miniprogramas no empaquetan nada desde este archivo.

## Frameworks de iOS y tvOS

NSGo comprueba el binario de cada framework y detiene la compilación indicando el motivo, salvo que sea:

- una biblioteca dinámica Mach-O arm64 compilada para la plataforma del destino, dispositivos o simulador, según indica su `LC_BUILD_VERSION`; un archivo universal aporta su porción arm64; se rechaza arm64e;
- para un sistema operativo mínimo no más reciente que el de la aplicación, `apple.info.MinimumOSVersion` o, en su defecto, iOS 15.0 y tvOS 17.0 ([Compilación y distribución](building.md#nsgoyaml));
- con el nombre de instalación `@rpath/<Name>.framework/<Name>`;
- y que cargue mediante `@rpath` solo frameworks que los paquetes de la aplicación declaran para el mismo destino.

NSGo escribe el `Info.plist` del framework, cuyo `CFBundleIdentifier` es el ID de la aplicación, un punto y `Name` en minúsculas, con la plataforma y el sistema operativo mínimo de la aplicación. Las compilaciones para el simulador firman el framework de forma ad hoc. Una compilación para dispositivos lo deja a la firma de la aplicación: `sign.profile` lo firma con la identidad de la aplicación antes que la propia aplicación, como hace Xcode con `nsgo build -run` y `nsgo dev` en un dispositivo. Un framework que carga a sus compañeros necesita la ruta de ejecución `@loader_path/..` o `@executable_path/Frameworks` para encontrarlos en `Frameworks`, ya que el ejecutable de la aplicación no tiene ninguna, y no carga nada más mediante `@rpath` salvo que tenga una ruta de ejecución del sistema como `/usr/lib/swift`. Un framework cuyo identificador de bundle ya tiene otro bundle de la aplicación, sin distinguir mayúsculas, como `StoreKit.framework` junto al `NSGoStoreKit.framework` de NSGo, detiene la compilación.

`nsgo build -embed` se detiene, nombrando el paquete, cuando la aplicación enlaza un paquete que declara frameworks de iOS: el XCFramework para una aplicación anfitriona aún no puede llevarlos.

## Conservar archivos al usar vendor

La operación `vendor` de Go necesita datos incrustados para conservar archivos debajo de un directorio de paquete. Un paquete puede marcar su manifiesto y licencias para incluirlos sin usar sus bytes en tiempo de ejecución:

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

Incluye también las rutas de bibliotecas locales en esa directiva `embed` cuando uses `path`, por ejemplo `lib/*`. Las descargas por URL solo necesitan el manifiesto y los archivos de licencia en el paquete de Go.
