---
title: Comparación con otros frameworks
description: Dónde encaja NSGo junto a NSH, MyGo y otros frameworks de interfaz de usuario, y qué sacrifica.
order: 2
---

El lugar de NSGo entre los frameworks de interfaz de usuario proviene de tres decisiones: las aplicaciones se escriben en Go (o TSX sobre él), cada plataforma dibuja con sus propias vistas y una única base de código cubre teléfonos, escritorios y la web. Otros frameworks toman otras decisiones, y cada decisión tiene un costo.

## De un vistazo

| Framework | Lenguaje | Dibuja con | Destinos |
| --- | --- | --- | --- |
| NSGo | Go, TSX | Las vistas de cada plataforma | iOS, App Clips, tvOS, watchOS, Android, macOS, Windows, Linux, web; miniprogramas, de forma experimental |
| [NSH](https://nsh.dev) | Rust, TSX | Las vistas de cada plataforma | iOS, Android, macOS, Windows, Linux, web, aplicaciones GPUI, terminales; HarmonyOS y miniprogramas de WeChat a través de plataformas de la comunidad |
| [MyGo](https://github.com/egoist/mygo) | Go, páginas web | Su propio renderizador GPU, o la vista web del sistema | macOS, Windows, Linux, web |
| [Fyne](https://fyne.io), [Gio](https://gioui.org) | Go | Su propio renderizador | Escritorios, teléfonos, web |
| [Wails](https://wails.io) | Go, páginas web | La vista web del sistema | Escritorios |
| [Flutter](https://flutter.dev) | Dart | Su propio renderizador | Teléfonos, escritorios, web |
| [Compose Multiplatform](https://www.jetbrains.com/compose-multiplatform/) | Kotlin | Su propio renderizador; Compose de Android | Teléfonos, escritorios, web |
| [React Native](https://reactnative.dev) | JavaScript, TSX | Las vistas de cada plataforma | iOS, Android; otros a través de plataformas de la comunidad |
| [.NET MAUI](https://dotnet.microsoft.com/apps/maui) | C# | Los controles de cada plataforma | iOS, Android, macOS, Windows |
| [Avalonia](https://avaloniaui.net) | C# | Su propio renderizador | Escritorios, teléfonos, web |
| [Qt](https://www.qt.io) | C++, QML | Su propio renderizador | Escritorios, teléfonos, dispositivos integrados |
| [Electron](https://www.electronjs.org), [Tauri](https://tauri.app) | Páginas web; Rust para Tauri | Un Chromium empaquetado; la vista web del sistema para Tauri | Escritorios; teléfonos para Tauri |

## Puntos fuertes

- **Vistas nativas.** El texto, los métodos de entrada, la selección, el desplazamiento, la accesibilidad y los menús son los propios de la plataforma, por lo que una aplicación se comporta como los usuarios esperan allí, mientras que un framework que dibuja por sí mismo reconstruye cada uno de ellos. Lo que solo tiene una plataforma, como Liquid Glass de iOS, las demás lo imitan de la misma forma en lugar de descartarlo.
- **Actualizaciones granulares.** Los componentes se ejecutan una vez y las señales actualizan solo lo que las lee, sin DOM virtual ni función de vista por fotograma que comparar; las animaciones se ejecutan con el reloj de fotogramas de la plataforma.
- **Los paquetes más pequeños y el inicio más rápido.** En las [pruebas de rendimiento](https://nsgo.dev/benchmarks/), la misma aplicación de contador ocupa unos 8 MB en macOS, Windows y Linux, menos que las de MyGo, GPUI y NSH (de 33 a 45 MB para NSH); también arranca más rápido y consume menos memoria que los frameworks que dibujan en la GPU, como MyGo y GPUI.
- **Solo Go.** El motor, el diseño y los backends están en Go, sin C, sin cgo y sin motor de JavaScript para las aplicaciones en Go.
- **Todas las plataformas desde un Mac.** Un Mac con Go y Xcode compila de forma cruzada todos los destinos: sin NDK, Android SDK, JDK, GTK ni cadena de herramientas de Windows.
- **Todos los archivos de lanzamiento con un solo comando.** `nsgo build` genera una imagen de disco firmada y notarizada para macOS, un instalador firmado para Windows, paquetes deb y Arch y una AppImage, paquetes para Mac App Store, Microsoft Store, Google Play y Flathub, y scripts que instalan desde lanzamientos de GitHub ([compilación](building.md)).
- **Las propias actualizaciones de cada plataforma.** En macOS Sparkle, y en Windows WinSparkle, descargan, verifican e instalan la siguiente versión; en Linux y Android la aplicación la encuentra y abre su página de lanzamiento; en iOS lo hace la App Store. Una aplicación instalada desde una tienda se deja a esa tienda ([actualizaciones](building.md#actualizaciones)).
- **Todas las plataformas.** Teléfonos, tabletas, televisores, relojes, escritorios y la web comparten un mismo motor y una misma API, junto con iOS App Clips, widgets de iPhone y Mac, y, de forma experimental, miniprogramas de WeChat, QQ, Alipay y Douyin.
- **Incrustación.** Una aplicación de NSGo puede integrarse como una vista en una aplicación existente de iOS, Android, escritorio en Go o web, para migrar pantalla a pantalla ([incrustación](embedding.md)).
- **Un ciclo de desarrollo rápido.** Al guardar un archivo Go se recompila y reinicia la aplicación; un cambio en TSX actualiza la aplicación en ejecución en el acto y conserva su estado; los errores de compilación y los pánicos durante el renderizado se muestran sobre la aplicación ([ejecución y depuración](debugging.md)).
- **Un solo conjunto de pruebas en todas partes.** Las pruebas buscan y usan controles por texto o rol como lo hacen los usuarios, rápidamente sin ventana, o sin cambios en ventanas reales de cada destino con la entrada propia de la plataforma, guardando capturas de pantalla ([pruebas](testing.md)).
- **Localización integrada.** Los textos de una aplicación y de sus componentes viven en catálogos, y las pseudoconfiguraciones regionales detectan lo que está sin traducir, recortado o no preparado para escritura de derecha a izquierda ([localización](localization.md)).
- **Shaders en Go.** `core.Shader` ejecuta shaders de GPU escritos en sintaxis de Go o WGSL, en todas las plataformas ([lienzo](canvas.md#shaders)).
- **TSX cuando se desea.** Las pantallas se pueden escribir en TSX con clases de Tailwind, sobre el mismo motor que los componentes en Go.

## Puntos débiles

- **Temprano.** 0.1.0 es el primer lanzamiento y la API aún no es estable; la [hoja de ruta](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md) enumera lo que falta, como un límite de heap para TSX entregado en tiempo de ejecución.
- **Un ecosistema pequeño.** Pocos componentes de terceros, ejemplos o respuestas en línea, frente a Flutter, React Native o Qt.
- **Las plataformas difieren.** Las vistas nativas implican pequeñas diferencias entre plataformas, por lo que una aplicación debe comprobarse en cada una. Un framework que dibuja por sí mismo se ve igual en todas partes.
- **Sus propios puentes.** En Android el programa Go se ejecuta junto al entorno de ejecución de Java y se comunica con él mediante tuberías, y en Linux los usuarios necesitan GTK 4.14 o posterior.
- **TSX se interpreta.** TSX se ejecuta en goja, un intérprete de JavaScript escrito en Go, más lento que los motores que usan React Native o Electron.

## NSH

NSGo porta el diseño de NSH: la arquitectura, los backends, los nombres de la API y los componentes sin estilo de `base` son los de NSH ([estado](index.md#estado)).

- **Elija NSGo** para escribir aplicaciones en Go en lugar de Rust o TSX, para compilar todos los destinos desde un Mac con Go y Xcode, o por lo que añade NSGo: shaders de GPU escritos en Go, iconos vectoriales, una terminal, aplicaciones que abren sus propias ventanas, paneles e iconos de bandeja, y componentes con estilo en `ui`.
- **Elija NSH** para un proyecto más maduro, TSX ejecutado por QuickJS con bytecode y límites de heap, el backend GPUI o Android de 32 bits.

## MyGo

Ambos son interfaces de usuario escritas en Go sin cgo, pero MyGo dibuja su interfaz nativa por sí mismo en la GPU y la reconstruye desde una función de vista en cada fotograma, mientras que NSGo entrega a las vistas nativas lo que cambió.

- **Elija MyGo** para aplicaciones de escritorio que se ven igual en todos los escritorios, ventanas de páginas web con llamadas tipadas hacia Go, y sus APIs de escritorio y distribución: iconos de bandeja, asociaciones de archivos, instaladores, notarización y actualizaciones delta firmadas.
- **Elija NSGo** para llegar también a teléfonos, para usar el texto, la entrada y la accesibilidad propios de cada plataforma, o para estado mantenido en señales en lugar de redibujarse desde los datos de la aplicación.

## Otros kits de herramientas en Go

- **Fyne y Gio** lo dibujan todo por sí mismos, como hace MyGo, y ambos llegan a teléfonos y a la web. Fyne ofrece un conjunto completo de widgets con su propio aspecto; Gio es de modo inmediato, de bajo nivel y flexible. Ambos tienen versiones publicadas y usuarios; ninguno tiene aspecto ni comportamiento nativo.
- **Wails** coloca un frontend web en la vista web del sistema con Go detrás: cualquier biblioteca web funciona, pero la interfaz es una página web, solo para escritorio.

## Frameworks en otros lenguajes

- **Flutter y Compose Multiplatform** dibujan cada píxel por sí mismos: idénticos en todas las plataformas y maduros, con grandes ecosistemas, pero con texto, desplazamiento y entrada reconstruidos en lugar de nativos, y con Dart o Kotlin por aprender.
- **React Native** comparte el modelo de NSGo de un núcleo reactivo que controla vistas nativas, y tiene el mayor ecosistema de su tipo. Se centra en iOS y Android, y ejecuta la lógica de la aplicación en JavaScript; NSGo la ejecuta en Go y trata los escritorios y la web como plataformas de primer nivel.
- **.NET MAUI** envuelve los controles de cada plataforma en C#, sin Linux ni la web. **Avalonia** dibuja por sí mismo, cercano a WPF, en todos los escritorios, teléfonos y la web.
- **Qt** es maduro, rápido y llega a dispositivos integrados, pero dibuja sus propios widgets, requiere C++ o QML y se licencia bajo LGPL o de forma comercial.
- **Electron y Tauri** muestran páginas web: las herramientas y bibliotecas de la web al completo, a costa de la memoria de un navegador, especialmente en Electron, y de una interfaz que se siente como una página web.

## Cómo elegir

Elija NSGo para una aplicación que deba sentirse nativa en teléfonos, escritorios y la web desde una sola base de código en Go, cuando sea aceptable un proyecto en etapa temprana. Para una aplicación que deba lanzarse pronto sobre una API estable, elija un framework maduro de la tabla anterior: Flutter o React Native para teléfonos; MyGo, Wails o Tauri solo para escritorios.
