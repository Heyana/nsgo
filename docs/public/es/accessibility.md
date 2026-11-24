---
title: Accesibilidad
description: Etiquetas, roles, valores, acciones y foco para lectores de pantalla y teclados.
group: Conceptos básicos
order: 61
---

## Qué lee un lector de pantalla

La propiedad `Label` de una vista es lo que pronuncia la tecnología de asistencia; `AccessibilityDescription` añade más información y se lee después de la etiqueta; `Hint` indica qué ocurre al activarla; y `Role` define qué es: `core.RoleButton`, `RoleLink`, `RoleHeader`, `RoleImage`, `RoleCheckbox`, `RoleSwitch`, `RoleRadio`, `RoleTab`, `RoleTextField`, `RoleSearchField`, `RoleStatus`, `RoleMenu`, `RoleMenuItem`, `RoleDialog`, `RoleAlertDialog`, `RoleAlert`, `RoleAdjustable`, `RoleProgressBar`, `RoleTable`, entre otros. De forma predeterminada, un elemento de estado es una región dinámica discreta; un campo de búsqueda es un campo de texto que se anuncia como cuadro de búsqueda. Un cuadro de diálogo se lee con su rol y su etiqueta al abrirse, y después su descripción, tal como ocurre con el título y la descripción de `ui.Dialog`. Un texto sin etiqueta se lee según su contenido. `LabelFunc`, `HintFunc` y `AccessibilityDescriptionFunc` enlazan texto de accesibilidad variable en la misma vista, de modo que los controles conservan su foco y su estado al cambiar de idioma. Todos los elementos principales con etiqueta aceptan `LabelFunc`, y tanto `View` como `Text` aceptan `HintFunc`.

Usa `TextFunc` para texto variable en `ui.Button`, `ui.Checkbox`, `ui.MenuItem` y `ui.AlertAction`. Sus nombres accesibles siguen ese texto. Los botones y las casillas de verificación también aceptan una función `LabelFunc` independiente. Los controles deslizantes, pestañas, interruptores, barras de progreso y activadores emergentes aceptan `LabelFunc` para un nombre accesible dinámico. `ui.Option.LabelFunc` proporciona el texto de la opción en pestañas, selectores y cuadros combinados. Los campos numéricos y de código, los anillos de progreso y ambas variantes de indicador de giro también aceptan `LabelFunc`.

```go
core.View{
	Role:  core.RoleButton,
	Label: "Add to cart",
	Hint:  "Adds the item and opens the cart",
	OnPress: add,
	Children: []core.Node{core.Icon{SVG: lucide.ShoppingCart}},
}
```

`AccessibilityValue` proporciona un rango o un valor textual: el mínimo, el máximo y el número actual de un control deslizante, o un texto como «3 elementos». `Accessible` convierte una vista en un único elemento, de modo que el nombre y el precio de una tarjeta se lean juntos en lugar de como dos paradas separadas, y si no tiene etiqueta, la vista lee el texto de sus descendientes. `AccessibilityElementsHidden` oculta elementos decorativos, como un chevrón o un separador, mientras que `ImportantForAccessibility` oculta todo un subárbol.

```go
core.View{Accessible: true, Label: "Apples, $1.20 a kilo", Children: []core.Node{
	core.Text{Content: "Apples"},
	core.Text{Content: "$1.20 a kilo"},
}}
```

## Etiquetas visibles

Asigna la referencia `Ref` del control al campo `LabelFor` de un `core.Text` o `core.View`. Al pulsar la etiqueta se enfoca el control y se ejecuta su acción de pulsación, como alternar una casilla de verificación. Su texto visible actual da nombre al control sin tener que repetir una cadena en `Label`, y sigue los estados `Disabled` e `Invalid` del control tanto para la accesibilidad como para los estilos. Una etiqueta no adquiere rol de botón ni parada de tabulación.

```go
field := new(core.Ref)
return core.View{Children: []core.Node{
	core.Text{Content: "Email", LabelFor: field},
	base.Input{Ref: field},
}}
```

`LabelledBy: []*core.Ref{label, requirement}` da nombre a un control a partir de varios elementos visibles en el orden indicado, con prioridad sobre `Label`; no reenvía pulsaciones. En caso contrario, `Label` prevalece sobre los nombres implícitos de `LabelFor`, cuyo orden sigue el del árbol. Las actualizaciones de texto, los desplazamientos, el ocultamiento, la eliminación y el remontaje actualizan la relación. Las referencias ausentes, repetidas, cíclicas u ocultas se ignoran; las asociaciones permanecen dentro de una misma superficie y ámbito de confinamiento. Una etiqueta no puede activar un control situado detrás de un modal.

Los botones, casillas de verificación, botones de radio, interruptores, selectores y entradas numéricas de `base` y `ui` pasan `Ref` y `LabelledBy` a su control; los controles deslizantes exponen `ThumbRef` y `StartThumbRef`. En TSX, usa `labelFor={field()}` y `labelledBy={[label()]}` con señales de nodo rellenadas mediante `ref={node => field.set(node)}`, de modo que una etiqueta colocada antes de su campo se actualice cuando dicho campo se monte.

## Acciones

`AccessibilityActions` enumera las acciones que el lector de pantalla ofrece sobre un elemento: las acciones estándar (`core.ActionActivate`, `ActionIncrement`, `ActionDecrement`, `ActionEscape`, `ActionMagicTap`) y las propias de la aplicación, cada una con `Name` y `Label`. `OnAccessibilityAction` recibe el nombre. Usa `AccessibilityActionsFunc` en `core.View` o `core.Input` para cambiar las acciones disponibles o traducir sus etiquetas mientras el elemento sigue montado.

```go
core.View{
	Role: core.RoleAdjustable,
	Label: "Quantity",
	AccessibilityValue: func() core.AccessibilityValue {
		return core.AccessibilityValue{Max: 10, Now: float32(n.Get()), Text: fmt.Sprintf("%d items", n.Get())}
	},
	AccessibilityActions: []core.AccessibilityAction{{Name: core.ActionIncrement}, {Name: core.ActionDecrement}},
	OnAccessibilityAction: func(name string) {
		if name == core.ActionIncrement {
			n.Update(func(v int) int { return v + 1 })
		} else {
			n.Update(func(v int) int { return v - 1 })
		}
	},
	Children: quantityControls,
}
```

`MultiSelectable` marca una cuadrícula, un cuadro de lista o un árbol que permite seleccionar más de un elemento. `base.DataTable` lo establece para la selección de múltiples filas; la web, Windows y Linux exponen la marca de accesibilidad correspondiente.

`Expanded` indica si un panel desplegable o un botón de menú está abierto, `Busy` indica que el contenido se está cargando, `Invalid` indica que la aplicación rechazó el contenido de un campo o el valor de un control (estilizado con `aria-invalid:`), y `Checked`, `Selected` y `Pressed` informan del estado de un control. Las partes de un control pueden tomar su estado mediante `PartStates`, de forma que un indicador siga a su casilla de verificación sin constituir una parada independiente.

`SelectionItem` marca una vista como un elemento dentro de una colección seleccionable independientemente de su valor actual de `Selected`. `base.DataTable` lo establece en filas o celdas cuando la tabla permite seleccionarlas, y `base.Tree` lo establece en cada fila; Windows mantiene el patrón SelectionItem y la web escribe `aria-selected="true"` o `"false"`. Una tabla sin selección deja las filas sin ninguno de los dos marcadores.

## Cambios en vivo

`LiveRegion` hace que los lectores de pantalla anuncien la etiqueta de una vista y el texto que contiene cuando cambian: `core.LiveRegionPolite` espera a que el usuario esté inactivo y `LiveRegionAssertive` interrumpe. `cx.Announce` pronuncia un mensaje una sola vez.

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("%d items in the basket", count.Get()) },
	LiveRegion: core.LiveRegionPolite}
```

## Foco y teclado

Las descripciones emergentes describen su activador antes de que reciba el foco. `View.Tooltip`, `base.Tooltip` y `ui.Tooltip` usan su texto actual como descripción de accesibilidad del control, salvo que repita el nombre del control, la aplicación proporcione `AccessibilityDescription` o la descripción emergente tenga `Disabled`. Un contenedor pasa la descripción a sus controles. La burbuja y sus hijos quedan ocultos para los lectores de pantalla y nunca anuncian una alerta; por ello, la descripción emergente predeterminada de `ui.IconButton` no repite su etiqueta.

Una `base.DataTable` expone su cuadrícula con nombre como parada de foco para el teclado y la accesibilidad; la capa de detección de puntero queda oculta a la tecnología de asistencia.

En escritorio, la web y Apple TV, las vistas que admiten pulsaciones son paradas de tabulación junto a los campos de texto; `NoFocus` excluye una vista y `Focusable` convierte en parada una vista sin pulsaciones. `Style.FocusVisible` dibuja el anillo de foco solo cuando el teclado movió el foco (`focus-visible:`), `Style.Focus` ante cualquier foco y `Style.FocusWithin` cuando el foco está dentro de una vista; `OnFocus` y `OnBlur` lo escuchan (`OnBlur` también cuando la vista enfocada abandona el árbol), y `Ref.Focus()` y `Ref.SetAccessibilityFocus()` lo mueven; `core.FocusedRef(cx)` es una referencia a la vista que lo tiene, útil para que aquello que se abre sin un activador —como una barra de búsqueda mediante un atajo— pueda devolverle el foco al cerrarse. Enter y Espacio pulsan la vista enfocada, y las teclas de flecha navegan por listas de pestañas, grupos de botones de radio y menús. El `Tooltip` de una vista se muestra de inmediato cuando el teclado mueve el foco hacia ella, hasta que el foco sale o se pulsa Escape. El `OnOpen` de una ventana emergente se ejecuta una vez que su contenido está presente, de modo que un `Focus` ahí dentro mueve el foco hacia su interior, como debe hacer un cuadro de diálogo no modal; el contenido de un panel emergente sigue a su activador en el orden de tabulación, y el foco vuelve al activador al cerrarse la ventana emergente.

En Android, iPhone y iPad, `Ref.Focus()` y el campo `AutoFocus` de un botón pueden enfocar una vista pulsable habilitada o `Focusable` para un teclado físico. Tocar un control no mueve ese foco. Espacio y Enter activan el elemento pulsable enfocado después del mapa de teclas, conservando Control, Alt, Shift y la tecla de plataforma en cada fase de pulsación. `NoFocus`, los controles deshabilitados u ocultos y los controles situados detrás de un modal no pueden recibir ese foco. Enfocar una vista no abre ningún teclado en pantalla; los campos nativos mantienen su comportamiento de edición. watchOS y los miniprogramas no disponen de ruta de activación mediante teclado físico.

`cx.Env().ScreenReader()` indica si hay un lector de pantalla activo, `cx.Env().ReduceMotion()` si el usuario solicitó reducir el movimiento, `cx.Env().IncreaseContrast()` si solicitó mayor contraste (Aumentar contraste en macOS, iOS y watchOS, un tema de contraste de Windows, Contraste alto de GNOME, el ajuste de contraste de GTK 4.20 o un tema GTK de alto contraste, el ajuste de contraste superior al estándar en Android 14, `prefers-contrast: more` o `forced-colors: active` en la web), `cx.Env().FontScale()` el tamaño del texto y `cx.Env().RTL()` la dirección de la disposición. Los textos siguen el tamaño del texto salvo que se establezca `NoFontScaling`, y una disposición de derecha a izquierda invierte sus bordes físicos cuando utiliza los bordes lógicos.

## Siguiente

- [Estilos y temas](styles.md): estados de foco y deshabilitado.
- [Formularios](forms.md): etiquetas y valores para campos.
- [Lienzo y shaders](canvas.md): cómo exponer las partes de un dibujo.
