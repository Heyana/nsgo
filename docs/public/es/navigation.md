---
title: Navegación
description: Pilas de navegación nativas, encabezados del sistema, modales, menús y barras de pestañas.
group: Conceptos básicos
order: 56
---

## Una pila

`base.NewNav` mantiene una pila de rutas de un solo tipo: `Push` agrega una página, `Pop` elimina la página superior, `PopToRoot` vuelve a la primera y `Replace` sustituye la superior. Su `Stack` renderiza cada ruta como una página, manteniendo montadas las páginas inferiores con su estado mientras se muestra la superior. `ui.NavPage` da a cada página el fondo de `ui` y un `ui.NavHeader` con su título, botón de retroceso y botones.

watchOS no tiene barra de navegación del sistema, barra de pestañas ni interruptor, así que allí `ui.NavPage` dibuja su `ui.NavHeader` encima de la página, incluido el botón de retroceso, `ui.TabShell` dibuja su propia barra de pestañas y `ui.SwitchSystem` es el interruptor propio de `ui`.

```go
nav := base.NewNav(cx, "inbox")

return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, route string, depth func() int) base.NavPage {
	if route == "inbox" {
		return ui.NavPage(cx, base.NavPage{Title: "Inbox", Children: []core.Node{
			ui.Button{Text: "Open a message", OnPress: func() { nav.Push("message") }},
		}}, ui.NavHeader{})
	}
	return ui.NavPage(cx, base.NavPage{Title: "Message"}, ui.NavHeader{
		OnBack: func() { nav.Pop(1) }, BackLabel: "Inbox",
		Trailing: []core.Node{ui.IconButton{Icon: lucide.Archive, Label: "Archive", Variant: ui.IconButtonGhost, OnPress: archive}},
	})
})
```

En iOS y Android la plataforma dibuja el encabezado del sistema a partir del título de la página y los botones del encabezado, con su botón de retroceso y su transición; en macOS, Windows, Linux y la web lo dibuja `ui`.

Android usa una barra de aplicación superior de Material 3. Una barra normal mide 64 dp con un título de 22 sp. `HeaderLargeTitle` ofrece una barra grande de 152 dp con un título de 28 sp que se colapsa en la barra normal a medida que la vista desplazable en la parte superior de la página desplaza su contenido por debajo, y se expande cuando el contenido vuelve arriba.

Establece `HeaderTitleStyle` y `HeaderLargeTitleStyle` para dar estilo a las fuentes nativas del título compacto y expandido en iOS y Android: `FontSize`, `FontWeight`, `FontFamily` y `FontStyle`. Los campos sin definir conservan los valores predeterminados de cada plataforma; `HeaderTitleStyleFunc` y `HeaderLargeTitleStyleFunc` siguen los cambios. `HeaderStyle` sigue aportando el fondo, el color del título y el tinte de los botones. En TSX son los objetos o arreglos de estilo `headerTitleStyle` y `headerLargeTitleStyle`.

Un `HeaderTitle` personalizado reemplaza solo el título compacto en Android; `Title` sigue siendo el título grande expandido. Establece `Android: core.AndroidScreenProps{HeaderRightWithLargeTitle: true}` para colocar `HeaderRight` junto a ese título grande. Sus controles pasan a la barra de herramientas fijada cuando el título se colapsa y siguen siendo presionables en todo momento. TSX usa `android={{ headerRightWithLargeTitle: true }}`. La banda grande crece cuando su fuente necesita más espacio. Las compilaciones independientes de Android habilitan la disposición RTL nativa; un host integrado debe establecer `android:supportsRtl="true"` en su aplicación para que los encabezados nativos sigan la dirección de la aplicación.

Da a una página su propio `Header` para reemplazar `ui.NavHeader`. En iOS y Android se convierte en la vista de título del encabezado del sistema, en lugar de `HeaderTitle`, junto a `HeaderLeft`, `HeaderRight` y el botón de retroceso de la página; llena el espacio entre esos extremos cuando sus vistas crecen o usan un ancho porcentual, como un campo de búsqueda, y toma el tamaño de su contenido en caso contrario. En el resto de plataformas se dibuja encima del contenido. `ui.NavHeader.Scroll` sigue conectando el desplazamiento de una página de iOS o Android con el desenfoque de su encabezado.

`NoPopOnBack` mantiene una página cuando se presiona atrás, `OnBackPress` escucha la pulsación y `NoPopOnSwipe` desactiva el gesto de deslizar hacia atrás de iOS. `cx.OnBackPress(func() bool)` da a la aplicación un manejador antes de la pila, para cambios sin guardar. `OnDismiss` informa que el usuario descartó la página, y la pila la elimina.

## Pantalla completa inmersiva

Llama a `mobile.Immersive(cx, true)` para solicitar pantalla completa para la ventana y a `mobile.Immersive(cx, false)` para volver a su configuración normal de barras del sistema. Para una página o un diálogo, usa `StatusBar: core.StatusBarSetting{Immersive: core.ToggleOn}` para solicitarla mientras esa capa se muestra. En TSX, usa `statusBar={{ immersive: true }}` en una página o diálogo, o `status_bar: { immersive: true }` en `nsgo.yaml` como valor predeterminado de la ventana.

En Android, deslizar desde un borde muestra brevemente las barras de estado y navegación. En HarmonyOS, la ventana pide al sistema que oculte la barra de estado, la barra de navegación de tres botones y el indicador de navegación. En iOS, la barra de estado se oculta y UIKit decide cuándo ocultar o mostrar el indicador de inicio. `cx.Env().SafeArea()` sigue informando los márgenes que proporciona el sistema, incluidos los recortes de pantalla. En HarmonyOS con navegación de tres botones en orientación horizontal, el sistema informa el área de los botones en el borde inferior mientras los dibuja en un lateral, por lo que el área segura puede dejar espacio abajo mientras los botones cubren el lateral.

## Modales

`ui.Dialog` muestra una superficie sobre una aplicación atenuada, o una hoja desde abajo en pantallas táctiles, y `ui.Sheet` un panel desde abajo. Cada uno se cierra con Escape y al presionar su fondo salvo que se indique lo contrario.

```go
ui.Dialog{
	Open:         dialog.Get,
	OnOpenChange: func(open bool, _ base.Reason) { dialog.Set(open) },
	Title:        "Rename",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { dialog.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

`ui.AlertDialog` pide confirmación al usuario, y `cx.ShowAlert` muestra la alerta o la hoja de acciones propia del sistema ([Diálogo](ui/dialog.md)).

## Menús

`ui.Menu` abre elementos `ui.MenuItem` desde un botón: junto a él, o como una hoja desde abajo cuando su `Presentation` lo solicita, en cualquier dispositivo. `ui.ContextMenu` hace lo mismo con un clic secundario o una pulsación larga, y `ui.Popover`, `ui.HoverCard` y `ui.Tooltip` muestran otro contenido anclado.

```go
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", OnSelect: duplicate},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## Pestañas

`ui.Tabs[T]` es un control segmentado o una lista de pestañas, con un indicador que se desliza hacia la opción elegida; `TabStyle` se combina sobre el estilo de cada pestaña, su `Selected` sobre el de la pestaña elegida y `IndicatorStyle` sobre el del indicador. Las pestañas subrayadas pueden establecer `IndicatorColor` y ajustar la línea a su contenido con `IndicatorFit: TabsIndicatorContent`. `ui.Sidebar` contiene secciones y filas para una barra lateral de escritorio, y `ui.TabShell` mantiene secciones de nivel superior junto a una barra de pestañas (abajo por defecto, o vertical en el borde inicial o final con `Edge` / `EdgeFunc`, incluido `mobile.TabBarAutomatic` para seguir `cx.Env().VerticalBarEdge()`) en cualquier ancho de ventana. Usa la barra del sistema cuando cada pestaña tiene un `SystemIcon` o un `Icon` SVG, en cualquier borde en iOS y en el borde inferior en Android, mostrando el SVG siempre que `SystemIcon` no nombre ningún SF Symbol en iOS ni drawable en Android; de lo contrario, dibuja la barra con los iconos SVG. Establece `BarHidden` mientras sea visible una página de detalle apilada para ocultar cualquiera de las dos barras y restaurar el área segura de la sección; `TabBarInset` pasa a ser 0 mientras está oculta o colocada verticalmente.

`mobile.TabBar` es la barra de pestañas del sistema en iOS y Android, con su aspecto Liquid Glass o Material; recibe `Items` de `mobile.TabBarItem{Title, Icon, SVG}`, el índice seleccionado, `OnChange` y un `Edge` opcional (`mobile.TabBarBottom`, `mobile.TabBarLeading`, `mobile.TabBarTrailing` o `mobile.TabBarAutomatic`, que sigue a `cx.Env().VerticalBarEdge()` en iOS 27.1 y vuelve al borde inferior en otros lugares) o `EdgeFunc` reactivo. Los roles de paleta de Android `tabBarBackground`, `tabBarIdle`, `tabBarSelected` y `tabBarIndicator` establecen el fondo de la barra, los iconos y etiquetas inactivos, el icono y etiqueta seleccionados y la píldora seleccionada. Las paletas personalizadas usan como respaldo `Surface`, `TextMuted`, `Accent` y `accentContainer` respectivamente, con una píldora de acento al 12 % cuando `accentContainer` no está definido. La paleta predeterminada conserva los colores de fondo e inactivos de Material, con la pestaña seleccionada teñida por el acento. En otras plataformas no ocupa espacio, de modo que una aplicación muestra allí `ui.Tabs`. `mobile.IsAvailable(cx)` indica si la barra se muestra. `Icon` nombra un icono de plataforma; `SVG`, como `lucide.House()`, se muestra en iOS y Android siempre que `Icon` no nombre ningún SF Symbol o drawable, teñido por la barra igual que un icono del sistema.

Establece `TabBarItem.Badge` en `mobile.TabBadge{Text: "3"}` para texto o `mobile.TabBadge{Dot: true}` para un punto. Actualízalo mediante `ItemsFunc`, y devuelve el valor cero para borrarlo. `ui.ShellTab.Badge` acepta `func() mobile.TabBadge` tanto para la barra del sistema como para la barra dibujada. El texto tiene prioridad cuando ambos campos están establecidos. UIKit muestra una insignia nativa en blanco para un punto. La tecnología de asistencia lee el texto de la insignia o la palabra localizada para contenido nuevo. TSX acepta `badge: { text: "3" }` o `badge: { dot: true }` en un elemento nativo, y una función que devuelva ese valor en un elemento de `TabShell`.

```go
mobile.TabBar{
	Items: []mobile.TabBarItem{{Title: "Home", Icon: "house"}, {Title: "Search", Icon: "magnifyingglass"}},
	Selected: func() int { return tab.Get() },
	OnChange: func(i int) { tab.Set(i) },
}
```

## Siguiente

- [Formularios](forms.md): lo que contiene una página de una pila.
- [Animación](animation.md): cómo se mueven las páginas y los modales.
- [Disposición](layout.md): el área segura alrededor de un encabezado.
