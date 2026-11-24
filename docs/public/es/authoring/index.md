---
title: Escribir componentes
description: Crea componentes que otras personas integren en sus aplicaciones, tal como están hechos base y ui en NSGo.
group: Escribir componentes
order: 70
---

Estas páginas están dirigidas a autores de componentes: una biblioteca de controles, un sistema de diseño o una vista compleja que las aplicaciones importan como paquete Go. Dan por conocidos [Componentes y estado](../components.md) y [Estilos y temas](../styles.md).

## Un componente

Un componente es un tipo con un método `Render`, como en una aplicación. Sus campos son lo que configura la aplicación; todo lo que la aplicación no establezca toma el valor cero, por lo que el valor cero de cada campo debe corresponder al caso habitual.

```go
package chips

import "github.com/nsgo-dev/nsgo/core"

// Chip is a short label in a pill.
type Chip struct {
	Text string
}

// Render implements core.Node.
func (c Chip) Render(*core.Scope) core.Node {
	return core.View{Class: "self-start rounded-full px-3 py-1", Children: []core.Node{
		core.Text{Content: c.Text, Class: "text-sm"},
	}}
}
```

Una aplicación lo utiliza como cualquier otro elemento: `chips.Chip{Text: "New"}`.

## Comportamiento y apariencia

NSGo mantiene cada control por duplicado. `base` contiene su comportamiento, estado y accesibilidad sin apariencia visual, y recibe cada estilo de la aplicación parte por parte; `ui` aplica una apariencia sobre la forma `base`, de modo que el comportamiento vive en un único lugar. Una biblioteca puede seguir la misma división:

- Un componente **sin estilos** gestiona las pulsaciones, el teclado, el foco y lo que oyen los lectores de pantalla, y expone el estilo de cada parte, como hace `base.Switch`. Las aplicaciones y otras bibliotecas le dan su propia apariencia ([Partes y estados](parts.md)).
- Un componente **con estilos** envuelve a uno sin estilos y únicamente elige estilos a partir del tema activo, como hace `ui.Switch` ([Apariencia y temas](looks.md)).

Un componente sin comportamiento reutilizable, como una insignia, puede tener estilos directamente.

## Un paquete

- Mantén un componente por archivo, con su mismo nombre (`chip.go`), junto con los tipos que solo él utiliza; lo que comparten varios va en un archivo propio.
- Da a un componente complejo un paquete propio cuya API omita el nombre del componente, como `dock.Area` en `base/dock`.
- Coloca los textos que pronuncia un componente en el subpaquete `locales/` del paquete ([Textos, pruebas y publicación](shipping.md)).
- No hagas nada implícito según el dispositivo: un tamaño o una forma que cambie entre teléfono y escritorio es un campo que configura la aplicación.

## Siguiente

- [Partes y estados](parts.md): valores controlados, partes con estilo y accesibilidad.
- [Apariencia y temas](looks.md): colores del tema, variantes y tamaños.
- [Componentes sin estilos](base.md): los controles propios de NSGo sin apariencia, para estilizar o ampliar.
- [Textos, pruebas y publicación](shipping.md): traducciones, pruebas y TSX.
