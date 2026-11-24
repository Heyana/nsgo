---
title: Apariencia y temas
description: Toma colores del tema activo y dota a un componente de variantes y tamaños.
group: Escribir componentes
order: 72
---

Un componente con estilos elige estilos y nada más: envuelve a uno sin estilos en un paquete propio como `chipsui` y mantiene allí su comportamiento. Sus colores proceden del tema, de modo que sigue el tema de la aplicación, el tema propio de un panel y la apariencia clara u oscura sin volver a construirse.

## Colores del tema

Lee los colores dentro de una función `StyleFunc`: vuelve a ejecutarse cuando cambia aquello que leyó, por lo que un cambio de tema o apariencia rediseña el componente en su sitio. `cx.Palette()` devuelve la paleta que se muestra donde está el componente, y `ui.ColorsOf(cx)` nombra los roles que utiliza `ui`.

```go
// Toggle is chips.Toggle in the theme's colors.
type Toggle struct {
	On       func() bool
	OnChange func(bool)
	Label    string
}

func (t Toggle) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	on := func() bool { return t.On != nil && t.On() }
	return chips.Toggle{On: t.On, OnChange: t.OnChange, Label: t.Label,
		Class: "flex-row items-center gap-2 rounded-full px-3 py-1",
		StyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: colors.Accent()}
			}
			return core.Style{Background: colors.Inset()}
		},
		DotClass: "size-2 rounded-full",
		DotStyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: colors.OnAccent()}
			}
			return core.Style{Background: colors.Faint()}
		},
		Children: []core.Node{ui.Text(t.Label, 13, ui.Medium, colors.Text)},
	}
}
```

Los colores fijos se rompen en modo oscuro y bajo el tema de una aplicación; toma cada color de un rol.

## Roles propios

Un color para el que la paleta no tiene rol se coloca en `Palette.Colors` bajo un nombre propio, definido en ambas apariencias. Las aplicaciones lo sobrescriben en código o en su archivo de tema, bajo `light` y `dark`:

```go
theme := core.DefaultTheme()
theme.Light.SetColor("chip", core.RGB(0xeef5ff))
theme.Dark.SetColor("chip", core.RGB(0x1b2a3d))
cx.SetTheme(theme)
```

Un componente lo lee con `cx.Palette().Color("chip")`, que indica si el tema lo define, de modo que el componente pueda recurrir a un rol alternativo si no existe. Los degradados, sombras y otros acabados no son colores: mantenlos en el código, elegidos según `cx.Dark()`.

## Variantes

Un componente con varias apariencias admite `Variant`, un tipo propio cuyo valor cero corresponde a la apariencia predeterminada. Nombra únicamente apariencias que el componente dibuja, incluido su color, en lugar de campos booleanos o un tipo de color compartido:

```go
// ToggleVariant is a toggle's look.
type ToggleVariant uint8

const (
	// ToggleSolid fills the pill with the accent when on.
	ToggleSolid ToggleVariant = iota
	// ToggleOutline draws its edge in the accent instead.
	ToggleOutline
)
```

## Tamaños

La mayoría de los componentes necesitan un solo tamaño, lo que mantiene los controles alineados sin que la aplicación tenga que elegir. Uno que necesite más admite `Size`, un `ui.Size` con valores predeterminado, pequeño o grande, para coincidir con los 28, 24 y 32 puntos de `ui.Button`; `Size.Height()` devuelve la altura con la que alinear otros controles. Un componente que conserva la misma forma a cualquier escala, como un avatar, recibe `Size` en puntos en su lugar.
