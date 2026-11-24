---
title: La base de código
description: Dónde vive cada parte de NSGo y los documentos que explican por qué.
group: Contribuir
order: 81
---

NSGo es un solo módulo de Go. Las aplicaciones importan sus paquetes desde arriba; el backend de cada plataforma se sitúa en la base y recibe lo que cambió como vistas nativas para crear, actualizar y eliminar.

## Paquetes

| Paquete | Qué contiene |
| --- | --- |
| `nsgo` (la raíz) | `nsgo.Run` y `nsgo.NewView`, eligiendo un backend mediante etiquetas de compilación |
| `core` | La API de la aplicación: elementos, estilos, clases, temas, señales y servicios; diseño y preparación de texto |
| `base` | Componentes sin estilo, un archivo cada uno, y los complejos como paquetes (`base/dock`, `base/editor`) |
| `ui` | Los componentes de `base` con estilo, y paquetes como `ui/chat` |
| `mobile`, `desktop` | Lo que solo tienen los teléfonos o solo los escritorios |
| `tsx` | Aplicaciones TSX, su empaquetador y compilador, y los módulos que importan |
| `i18n`, `*/locales` | Catálogos de mensajes y el código generado a partir de ellos |
| `nsgotest` | Pruebas que actúan como un usuario, sin interfaz gráfica o en una plataforma |
| `internal/engine`, `internal/driver`, `internal/layout` | La goroutine del motor, el protocolo de montaje y el diseño |
| `internal/platform/<target>` | Un backend: vistas nativas, texto y entrada para un destino |
| `cmd/nsgo` | La herramienta `nsgo` |
| `examples/` | La galería y las demás aplicaciones de ejemplo |

Los paquetes solo importan hacia abajo; `internal/moduletest/layering_test.go` comprueba el grafo en cada destino.

## Documentos de diseño

[ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) mapea los paquetes, los hilos y los flujos desde un commit hasta la pantalla, y enumera los temas en `docs/architecture/`: uno por parte, como diseño, texto, desplazamiento, temas o la herramienta. Lea el tema de lo que vaya a cambiar antes de modificarlo; registra cómo funciona esa parte y en qué se aparta de [Native Shell](https://github.com/longbridge/nsh), cuyo diseño porta NSGo.

[AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md) contiene las reglas que sigue cada cambio, tanto para personas como para agentes de programación. Algunas dan forma a la mayoría de los cambios:

- Cada API tiene un único hogar: `core` para elementos y el entorno, `base` para controles, `ui` para su aspecto, `mobile` y `desktop` para un solo tipo de dispositivo.
- `base` no tiene aspecto; un componente de `ui` es su forma de `base` con uno.
- Nada cambia implícitamente según el dispositivo: ni controles más grandes en escritorios ni más pequeños en teléfonos dentro de `core` o `base`.
- Vistas nativas en todas partes, iguales en todas las plataformas: lo que una plataforma tiene de forma nativa, las demás lo imitan.
- Sin C escrito a mano, y nada más allá de macOS, Xcode y Go para compilar todos los destinos salvo el del emulador de Android x86_64.

Lo que falta, plataforma por plataforma, está en la [hoja de ruta](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md).
