---
title: Referencia de componentes
description: Los componentes más usados, sus variantes, estados y accesibilidad.
group: Componentes
order: 30
---

Estos son los componentes de `ui`, con el aspecto propio de NSGo y siguiendo el tema. Esta referencia comienza con los componentes a los que una aplicación recurre primero; cada página empieza con el ejemplo más breve y luego cubre sus partes, estados y accesibilidad. La galería cubre todos los componentes, incluidos aquellos que aún no tienen una página aquí.

La [galería](https://nsgo.dev/gallery/) ejecuta una demostración de cada componente en el navegador.

## Acciones

- [Botón](ui/button.md): un botón pulsador, simple o en el color de acento, y el botón de ícono.

## Texto y entrada

- [Texto](ui/text.md): párrafos, fragmentos, enlaces, fuentes y texto de documento.
- [Entrada](ui/input.md): el campo de texto de la plataforma, controlado o no.
- [Casilla de verificación](ui/checkbox.md): una casilla de verificación, marcada, desmarcada o indeterminada.
- [Interruptor](ui/switch.md): un ajuste que está activado o desactivado.
- [Selector](ui/select.md): un valor de una lista, como ventana emergente o como hoja.
- [Control deslizante](ui/slider.md): un valor o un rango a lo largo de una pista.

## Colecciones

- [Lista](ui/list.md): filas de cualquier número, construidas solo cuando están a la vista, con selección y activación.
- [Desplazamiento](ui/scroll.md): la vista de desplazamiento de la plataforma, con paginación, ajuste, zoom y actualización.

## Navegación

- [Pestañas](ui/tabs.md): una lista de pestañas, un control segmentado y la barra de pestañas del sistema.

## Superposiciones

- [Diálogo](ui/dialog.md): una superficie modal, una hoja y la alerta del sistema.
- [Menú](ui/menu.md): un menú desde un disparador, un menú contextual y sus elementos.

## Más

La galería cubre el resto: tablas, gráficos, el editor de código, texto enriquecido, la terminal, el dock, la paleta de comandos, selectores de color, fecha y emoji (`ui.EmojiPicker`), notificaciones emergentes y más. Cada uno tiene una demostración allí.

`base/terminal.View` abre rutas locales existentes con Command+clic (Control+clic en Windows y Linux), al igual que abre URLs. Las rutas relativas usan el directorio de trabajo OSC 7 del shell; un shell local que no haya informado ninguno usa `terminal.Options.Dir`, o el directorio personal cuando no está definido. Pon entre comillas los nombres de archivo que contengan espacios. Las autoridades de directorios remotos y las conexiones sin un informe de directorio local no se resuelven contra archivos locales. Los archivos y carpetas usan `Scope.OpenFile`; los enlaces URL siguen pasando por `Scope.OpenURL` y el manejador de URLs del host incrustador.

`base/terminal.View` deniega por defecto las lecturas y escrituras del portapapeles por parte del programa. Establece `AllowClipboardRead` y `AllowClipboardWrite` solo cuando la aplicación las permita; el `AccessClipboard` de la vista solicitante sigue aplicándose. Las mismas propiedades están disponibles en `Terminal` de TSX y pueden cambiar sin reiniciar su sesión. Las aplicaciones proporcionan la interfaz de consentimiento. El pegado explícito del usuario ofrece de forma independiente una instantánea de texto/PNG a los programas que usan el protocolo de portapapeles de Kitty (modo 5522); su contraseña de un solo uso solo puede leer esa instantánea. Los programas sin el modo reciben texto entre corchetes ordinario. La disponibilidad de imágenes en el portapapeles sigue los servicios del portapapeles de la plataforma; la selección primaria de X11 no se expone.

## Siguiente

- [Componentes y estado](components.md): cómo un componente devuelve elementos y cómo las señales los actualizan.
- [Estilos y temas](styles.md): el tema del que los componentes toman sus colores.
- [Componentes sin estilos](authoring/base.md): los mismos controles sin apariencia visual, para un aspecto propio.
- [Accesibilidad](accessibility.md): etiquetas, roles y valores para cada control.
