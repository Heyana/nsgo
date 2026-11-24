---
title: Introducción
description: Qué es NSGo, en qué se ejecuta y cómo funciona.
order: 1
---

NSGo es un framework multiplataforma de interfaz de usuario nativa escrito en Go. Escribes una aplicación una sola vez, en Go o en TSX, y se ejecuta en iOS y sus App Clips, tvOS, watchOS, Android, macOS, Windows, Linux y la web, dibujada por las propias vistas de cada plataforma. Las aplicaciones de Go también se ejecutan en miniprogramas de WeChat, QQ, Alipay y Douyin, de forma experimental: esas plataformas limitan las aplicaciones a 2 MB, límite que las aplicaciones más grandes superan. HarmonyOS ejecuta aplicaciones de Go en versión preliminar, dibujadas mediante ArkUI.

## Qué escribes

Una aplicación es un paquete `main` de Go. Sus componentes devuelven elementos: vistas, texto, imágenes, entradas y los controles de `ui`. El estado reside en señales (`signals`), y solo lo que lee una señal se actualiza cuando esta cambia.

```go
func Root(cx *core.Scope) core.Node {
	count := core.NewSignal(cx, 0)
	return core.View{Style: core.Style{Padding: core.All(24), Gap: 12}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }},
		ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }},
	}}
}
```

La misma aplicación puede escribirse en TSX y estilizarse con clases de Tailwind ([aplicaciones TSX](tsx.md)).

## Cómo funciona

Un motor en Go gestiona la aplicación: el grafo reactivo, los estilos, la disposición flexbox y grid, y la preparación de texto. Cada actualización produce un lote de cambios, y un backend ligero lo aplica a las vistas nativas de la plataforma. Las animaciones se ejecutan en el reloj de cuadros de la plataforma, por lo que nada realiza commits más de una vez por cuadro.

| Plataforma | Dibuja con |
| --- | --- |
| iOS | UIKit y CoreText |
| Android | Vistas de Android |
| macOS | AppKit y CoreText |
| Windows | Direct2D y DirectWrite |
| Linux | GTK 4, Pango y Cairo |
| Web | WebAssembly y elementos del DOM |

Todos los destinos se compilan en una Mac con Go; iOS también necesita Xcode. La herramienta `nsgo` crea, ejecuta, prueba y empaqueta aplicaciones.

## Estado

NSGo está en una etapa temprana: 0.1.0 es su primera versión, su API aún no es estable y lo que falta está en la [hoja de ruta](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md). Adapta a Go el diseño de [Native Shell](https://github.com/longbridge/nsh).

## Cómo leer la documentación

Cada página responde a una pregunta. Un ejemplo de código es un fragmento de un componente salvo que la página indique lo contrario: `cx` es el `*core.Scope` que recibe un componente. Las aplicaciones usan los componentes de `ui`, que tienen una apariencia definida; `base` contiene los mismos controles sin apariencia, para autores de componentes.

- **Primeros pasos**: [configuración](getting-started.md), luego aplicaciones de ejemplo construidas paso a paso: [una aplicación de tareas](tutorials/todo.md), [una aplicación de notas](tutorials/notes.md) y [repositorios populares de GitHub](tutorials/repos.md).
- **Herramientas**: [ejecución y depuración](debugging.md), [pruebas](testing.md), [compilación y distribución](building.md) e [integración](embedding.md).
- **Componentes**: [los componentes más usados](ui.md), sus variantes, estados y accesibilidad.
- **Conceptos básicos**: [componentes y estado](components.md), [disposición](layout.md), [estilos](styles.md), [texto](text.md), [listas](lists.md), [navegación](navigation.md), [formularios](forms.md), [localización](localization.md), [TSX](tsx.md) y el resto.
- **Funciones de plataforma**: [servicios de plataforma](services.md), [notificaciones](notifications.md), [respuesta háptica](haptics.md), [widgets](widgets.md), [Actividades en vivo](live-activities.md) y [permisos](permissions.md).
- **Escribir componentes**: [componentes para aplicaciones de otras personas](authoring/index.md), sin estilo y con estilo.
- **Contribuir**: [compila NSGo desde el código fuente](contributing.md), oriéntate en el proyecto y envía un cambio.
- La [demostración en vivo](https://nsgo.dev/gallery/): cada componente, en tu navegador.
