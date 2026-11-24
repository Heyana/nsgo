---
title: Terminal
description: Ejecuta un programa o muestra una sesión remota en una cuadrícula de terminal.
group: Conceptos básicos
order: 63
---

`base/terminal` proporciona una sesión (`Terminal`) y su `View`. Los equipos de escritorio ejecutan programas locales; iOS y los navegadores usan `Options.Conn`, un `io.ReadWriteCloser` conectado a un programa remoto. La aplicación es propietaria de la sesión y la cierra al finalizar.

```go
t, err := terminal.New(terminal.Options{CursorBlink: true})
if err != nil {
	return core.Text{Content: err.Error()}
}
cx.Cleanup(func() { t.Close() })
return terminal.View{
	Terminal: t, AutoFocus: true,
	MinimumContrast: 4.5, PaddingColor: terminal.PaddingExtend,
	Style: core.Style{FlexGrow: 1},
}
```

`Options.Cursor` elige la forma predeterminada de bloque, barra o subrayado. `CursorBlink` activa su parpadeo predeterminado; el valor cero permanece fijo. Los programas anulan la forma y el parpadeo mediante DECSCUSR o el modo DEC 12. Un cursor sin foco se muestra hueco y fijo; la composición de texto y el movimiento reducido también lo mantienen fijo. El reloj de animación de la plataforma impulsa el parpadeo sin que el motor vuelva a dibujar el texto del terminal.

`View.Font`, `Theme` y `DarkTheme` establecen la fuente y la paleta. La cuadrícula dibuja por sí misma los caracteres Unicode de dibujo de cajas y de bloque y los separadores Powerline U+E0B0–U+E0B7, alineando las celdas adyacentes a los píxeles del dispositivo. El resto del texto usa las fuentes de la plataforma y sus fuentes de respaldo.

`MinimumContrast` aumenta el contraste del texto normal frente al fondo real de su celda, incluidos los colores inversos, el texto tenue y la selección. Los valores de 1 a 21 usan la relación de contraste de las WCAG; el cero conserva los colores. Los gráficos y los emoji mantienen sus colores. Con `Transparent`, el contraste sigue utilizando el fondo configurado del terminal.

`Padding` deja espacio fuera de la cuadrícula, en puntos. `PaddingColor` elige su fondo:

| Valor | Comportamiento |
| --- | --- |
| `PaddingBackground` | Fondo del terminal, incluidos los cambios de OSC 11 |
| `PaddingExtend` | Fondo de la celda más cercana; en la pantalla principal, suprime la extensión superior/inferior junto a filas de prompt, filas Powerline o filas que contengan una celda con fondo predeterminado |
| `PaddingExtendAlways` | Extiende cada borde sin esas excepciones |

Las aplicaciones TSX importan `tsx/terminal` para que su efecto registre `Terminal` en `nsgo/base`. Las props son `cursorBlink`, `minimumContrast` y `paddingColor`; esta última acepta `background`, `extend` y `extend-always`. Las props de apariencia pueden cambiar mientras la sesión continúa.

## En un navegador

La aplicación incrusta el módulo WebAssembly fijado de libghostty-vt y ejecuta la misma vista de terminal y ruta de entrada. Compilar la aplicación no requiere Zig, ni instalación de bibliotecas nativas ni descargas en tiempo de ejecución.

Adapta el WebSocket del navegador a `io.ReadWriteCloser`: envía bytes binarios y une o divide los mensajes entrantes como un flujo. La aplicación y su servidor gestionan la autenticación, la reconexión y cualquier mensaje de cambio de tamaño o control; NSGo consume los bytes del terminal. El [fixture de aceptación en navegador](https://github.com/nsgo-dev/nsgo/blob/main/internal/testhost/testdata/terminalweb/socket_js_test.go) muestra un adaptador completo de flujo de bytes con manejo de cierre.

## Acciones y zoom

`View.Keybinds` sobrescribe los valores predeterminados con `Keybind{Key, Action, Amount}`. Las teclas usan los mismos nombres que `Scope.OnKey`, como `mod+=` o `ctrl+shift+c`. Las entradas posteriores tienen prioridad. Los atajos se ejecutan mientras la entrada de este terminal tiene el foco y ningún método de entrada está componiendo texto.

Las acciones son `CopyToClipboard`, `PasteFromClipboard`, `SelectAll`, `ClearScreen`, `IncreaseFontSize`, `DecreaseFontSize`, `ResetFontSize`, `SetFontSize` e `Ignore`. Las acciones de ventana y pestaña pertenecen al mapa de teclas de la aplicación. `Ignore` consume una tecla sin enviarla al programa.

De forma predeterminada, `mod+=` y `mod+shift+=` aumentan la fuente en un punto, `mod+-` la reducen y `mod+0` la restablece a `View.Font.Size` (13 cuando no se ha definido). `Amount` establece el incremento o decremento —uno cuando es cero— o el tamaño para `SetFontSize`. Los tamaños se limitan a entre 1 y 255 puntos; las solicitudes no finitas se ignoran. El zoom conserva la sesión y la vista exterior, actualiza las filas y columnas de la cuadrícula y cambia el tamaño del programa.

```go
ref := &terminal.Ref{}
view := terminal.View{
	Terminal: session, Ref: ref,
	Keybinds: []terminal.Keybind{
		{Key: "ctrl+shift+k", Action: terminal.ClearScreen},
		{Key: "mod+=", Action: terminal.IncreaseFontSize, Amount: 2},
	},
}
```

`Ref.FontSize()` se rastrea para los controles que lo muestran; `Ref.SetFontSize(size)` y `Ref.Perform(action, amount)` se ejecutan en el motor de la aplicación, al igual que los manejadores de eventos. `Perform` indica si una acción está disponible. Una referencia no montada devuelve un tamaño de cero y rechaza las acciones.

## Menú contextual

Un clic secundario abre un `base.ContextMenu` sin estilos con los elementos localizados Copiar, Pegar, Seleccionar todo y Limpiar. Copiar requiere una selección. Limpiar elimina el contenido situado por encima del prompt y está deshabilitado en la pantalla alternativa. Las pantallas táctiles usan la presentación por pulsación larga del menú contextual. Da estilo a la superficie con `MenuClass`/`MenuStyle` y a los elementos con `MenuItemClass`/`MenuItemStyle`.

Establece `RightClickAction` en `RightClickPaste`, `RightClickCopy`, `RightClickCopyOrPaste` (copia una selección; de lo contrario, pega) o `RightClickIgnore`. Ignore deja pasar el evento para un menú de aplicación envolvente; úsalo cuando proporciones un `ui.ContextMenu` con estilos que llame a las acciones de la referencia.

Importa `tsx/terminal` para registrar `<Terminal>` en `nsgo/base`. Sus props reactivas `keybinds` y `rightClickAction` actualizan la vista manteniendo la sesión. Los valores de acción usan nombres de Ghostty como `increase_font_size` y `copy_to_clipboard`; los valores para clic derecho son `context-menu`, `paste`, `copy`, `copy-or-paste` e `ignore`. Las props de estilo del menú usan los nombres de Go en lower camel case.

## Informes del ratón

Los programas que habilitan el seguimiento del ratón reciben las pulsaciones, liberaciones, arrastres y movimientos sobre el área solicitados, codificados por libghostty-vt en el protocolo seleccionado. Shift omite la captura de clics y arrastres para permitir la selección. `View.MouseReporting` es una función reactiva opcional `func() bool`, verdadera cuando es nil; devuelve false para mantener la selección con el ratón y el desplazamiento nativo del historial incluso cuando un programa solicita la captura. El clic derecho de un programa tiene prioridad sobre un menú contextual envolvente mientras los informes están habilitados.

En la pantalla alternativa, con el modo de desplazamiento alternativo habilitado y sin seguimiento del ratón solicitado, la entrada vertical de la rueda se convierte en teclas de cursor, siguiendo el modo de teclas de cursor normal o de aplicación. `MouseScrollMultiplier{Precision: 1, Discrete: 3}` establece los recuentos de informes predeterminados para paneles táctiles y ruedas; el cero usa esos valores predeterminados, y los valores positivos se limitan al rango de 0.01 a 10 000. La entrada fraccionaria se acumula hasta que haya un paso completo disponible. El historial de desplazamiento normal conserva el desplazamiento nativo y la inercia de la plataforma.

Importa `tsx/terminal` para registrar `<Terminal>` en `nsgo/base`. Su prop `mouseReporting` es true de forma predeterminada; `mouseScrollPrecision` y `mouseScrollDiscrete` establecen los multiplicadores.
