---
title: Menú
description: Un menú desde un disparador, un menú contextual y sus elementos.
group: Componentes
order: 42
---

`ui.Menu` abre elementos `ui.MenuItem` desde un botón: junto a él, o como una hoja desde la parte inferior con `Presentation: base.PresentSheet`, sea cual sea el dispositivo. `ui.ContextMenu` abre los mismos elementos en el puntero.

```go preview=menu/basic height=280
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", Icon: lucide.Pencil, OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", Shortcut: "mod+d", OnSelect: duplicate},
	ui.MenuSeparator{},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

El menú se abre y se cierra por sí solo; `Open` y `OnOpenChange` permiten que la aplicación lo controle. `Trigger` es el aspecto y el contenido del botón, y el menú recibe sus pulsaciones. Un menú emergente nunca sobrepasa el espacio junto a su disparador: se abre en el lado donde cabe completo y sus elementos se desplazan más allá de ese espacio, o más allá de `MaxHeight`.

## Elementos

Un `ui.MenuItem` muestra `Text`, un `Icon` opcional y un `Shortcut` (`"mod+c"`, escrito al estilo de la plataforma en un menú emergente), e informa `OnSelect`, cerrando el menú salvo que se indique `NoCloseOnSelect`. `Disabled` lo desactiva, `Checked` muestra una marca junto a él y `Variant: ui.MenuItemDanger` señala una acción destructiva. `ui.MenuLabel` encabeza un grupo y `ui.MenuSeparator` los divide. Un `ui.Submenu` contiene más elementos y se abre junto al elemento o, en una hoja, en lugar de los que lo rodean.

```go preview=menu/checked height=320
ui.Menu{Trigger: ui.Button{Text: "View", Variant: ui.ButtonGhost}, Children: []core.Node{
	ui.MenuLabel{Text: "Sort by"},
	ui.MenuItem{Text: "Name", Checked: func() bool { return sort.Get() == "name" }, OnSelect: func() { sort.Set("name") }},
	ui.MenuItem{Text: "Date", Checked: func() bool { return sort.Get() == "date" }, OnSelect: func() { sort.Set("date") }},
	ui.MenuSeparator{},
	ui.Submenu{Text: "Share", Children: []core.Node{
		ui.MenuItem{Text: "Copy link", OnSelect: copyLink},
		ui.MenuItem{Text: "Email", OnSelect: emailLink},
	}},
}}
```

## Menús contextuales

`ui.ContextMenu` abre sus elementos con un clic secundario, o una pulsación larga en pantallas táctiles, en la posición del puntero sobre su `Trigger`.

```go preview=menu/context height=220
ui.ContextMenu{Trigger: fileRow, Children: []core.Node{
	ui.MenuItem{Text: "Open", OnSelect: open},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## Comportamiento

Escape, atrás y una pulsación fuera cierran el menú. El menú es una ventana emergente colocada según `Placement` y `Align`, o una hoja desde la parte inferior cuando se presenta así. En un escritorio, Abajo, Arriba, Inicio, Fin, Av Pág y Re Pág se mueven por los elementos habilitados empezando por aquel sobre el que está el puntero, Derecha abre un submenú e Izquierda lo cierra, e Intro elige. Un menú abierto con el teclado enfoca su primer elemento, y el foco vuelve al disparador cuando el menú se cierra.

## Accesibilidad

El menú informa `RoleMenu` y cada elemento `RoleMenuItem`. `Label` nombra el disparador cuando su contenido no lo hace. Un lector de pantalla lee el texto de un elemento y si está deshabilitado.

## Plataforma

El disparador y los elementos son de la aplicación; la hoja, la ventana emergente, el foco, Escape y atrás son de la plataforma. La aplicación elige entre la ventana emergente o la hoja; NSGo no escoge ninguna según el dispositivo.

## Relacionado

- [Selector](select.md): un valor de una lista.
- [Diálogo](dialog.md): una superficie modal.
- [Pestañas](tabs.md): secciones mostradas a la vez.
- Las páginas Menu, ContextMenu y Command de la galería.
- [Componentes sin estilos](../authoring/base.md#menu): `base.Menu`, para un menú con un aspecto propio.
