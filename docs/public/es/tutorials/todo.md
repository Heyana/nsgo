---
title: "Ejemplo: una aplicación de tareas"
description: Crea paso a paso una pequeña aplicación de tareas, con señales, una lista y una prueba.
group: Primeros pasos
order: 11
---

Este tutorial construye una pequeña aplicación de tareas: agrega elementos, márcalos como completados y elimínalos. Toma unos diez minutos y asume la [configuración](../getting-started.md).

## Crea la aplicación

```sh
mkdir todo && cd todo
go mod init example.com/todo
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Construye la aplicación de tareas

La aplicación generada es un contador. Reemplaza todo el contenido de `app.go` con esta aplicación:

```go program height=480
package main

import (
	"fmt"
	"strings"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type todo struct {
	id   int
	text string
	done bool
}

func main() { nsgo.Run(Root) }

func Root(cx *core.Scope) core.Node {
	items := core.NewSignalFunc(cx, []todo{}, nil)
	draft := core.NewSignal(cx, "")
	nextID := core.NewSignal(cx, 1)
	colors := ui.ColorsOf(cx)

	add := func() {
		text := strings.TrimSpace(draft.Get())
		if text == "" {
			return
		}
		id := nextID.Get()
		nextID.Set(id + 1)
		items.Update(func(list []todo) []todo {
			return append(list, todo{id: id, text: text})
		})
		draft.Set("")
	}
	setDone := func(id int, done bool) {
		items.Update(func(list []todo) []todo {
			next := append([]todo(nil), list...)
			for i := range next {
				if next[i].id == id {
					next[i].done = done
				}
			}
			return next
		})
	}
	remove := func(id int) {
		items.Update(func(list []todo) []todo {
			out := list[:0:0]
			for _, it := range list {
				if it.id != id {
					out = append(out, it)
				}
			}
			return out
		})
	}
	left := func() int {
		n := 0
		for _, it := range items.Get() {
			if !it.done {
				n++
			}
		}
		return n
	}

	return core.View{
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, Padding: core.All(24), PaddingEnv: core.EnvAll(core.EnvSafeArea), Gap: 16},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Background()}
		},
		Children: []core.Node{
			ui.Text("Todos", 28, ui.Bold, colors.Text),
			core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
				ui.Input{Value: draft.Get, OnChange: draft.Set, Placeholder: "What needs doing?",
					OnSubmit: func(string) { add() }, TestID: "draft", Style: core.Style{FlexGrow: 1}},
				ui.Button{Text: "Add", Icon: lucide.Plus, OnPress: add, TestID: "add"},
			}},
			ui.Textf(func() string { return fmt.Sprintf("%d left", left()) }, 13, ui.Regular, colors.Muted),
			core.List{
				Count:         func() int { return len(items.Get()) },
				Key:           func(i int) any { return items.Get()[i].id },
				EstimatedSize: 44,
				Style:         core.Style{FlexGrow: 1},
				Empty:         ui.Text("Nothing yet. Add one above.", 13, ui.Regular, colors.Muted),
				Row: func(cx *core.Scope, row core.ListRow) core.Node {
					index := row.Index()
					it := items.Get()[index]
					return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(8, 0)},
						Children: []core.Node{
							ui.Checkbox{
								Checked: func() bool {
									for _, x := range items.Get() {
										if x.id == it.id {
											return x.done
										}
									}
									return false
								},
								OnChange: func(done bool) { setDone(it.id, done) },
								Text:     it.text,
								TestID:   fmt.Sprintf("todo-%d", it.id),
								Style:    core.Style{FlexGrow: 1, FlexShrink: 1},
							},
							ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost,
								TestID: fmt.Sprintf("delete-%d", it.id), OnPress: func() { remove(it.id) }},
						}}
				},
			},
		},
	}
}
```

Guarda el archivo y la aplicación se reinicia. Escribe una tarea, presiona **Add** y márcala como completada.

## Cómo funciona

**El estado son señales.** `items`, `draft` y `nextID` son señales. Una señal almacena un valor y rastrea qué lo lee: cuando `items` cambia, solo el texto y las filas que la leen se actualizan. `core.NewSignal` compara los valores con `==`, por lo que un slice usa `core.NewSignalFunc` sin función de igualdad.

**Las actualizaciones reemplazan el slice.** `items.Update` devuelve un nuevo slice, por lo que la identidad de la lista y las claves de fila deciden qué permanece en pantalla. `nextID` da a cada tarea un ID, y `Key` de `core.List` lo usa; sin una clave estable, las filas reutilizarían sus celdas para otros elementos.

**La pantalla es un árbol de cajas.** `Root` devuelve una `core.View` con relleno, una separación y los márgenes del área segura. `FlexGrow: 1` y `FlexShrink: 1` hacen que llene la ventana y nada más, de modo que una lista larga se desplace dentro de ella en lugar de estirarla más allá del borde de la ventana. La fila de entrada establece `Direction: core.Row`; todo lo demás es la columna predeterminada.

**La entrada está controlada.** `draft.Get` es el valor del campo y `draft.Set` recibe cada cambio. `OnSubmit` agrega una tarea al presionar Intro, y `add` limpia el borrador, de modo que el campo se vacía solo.

**La lista solo construye lo que se muestra.** `core.List` solicita `Count` filas y construye una fila solo cuando se acerca a la pantalla. `Empty` se muestra mientras no hay elementos, y la casilla de verificación y el botón de eliminar de cada fila llevan el ID de su tarea, de modo que una pulsación siga encontrando el elemento correcto después de que la lista cambie.

**El aspecto es el de `ui`.** `ui.Text`, `ui.Input`, `ui.Button`, `ui.Checkbox` y `ui.IconButton` toman sus colores del tema y siguen automáticamente los modos claro y oscuro. `ui.ColorsOf(cx)` proporciona los colores para las partes que la propia aplicación dibuja, y una `StyleFunc` sigue un cambio de tema en el sitio.

## Pruébala

La prueba generada presiona el botón del contador, así que reemplaza `app_test.go` con una prueba de la aplicación de tareas:

```go
package main

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestAddCompleteAndDelete(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Find("text:Nothing yet. Add one above.")
	app.Type("id:draft", "Buy milk")
	app.Press("id:add")
	app.WaitFor("text:Buy milk", time.Second)
	app.Press("id:todo-1")
	app.WaitFor("text:0 left", time.Second)
	app.Press("id:delete-1")
	app.WaitFor("text:Nothing yet. Add one above.", time.Second)
}
```

La prueba inicia la aplicación sin interfaz gráfica, busca elementos por su texto o ID de prueba y actúa como lo haría un usuario. Ejecútala con:

```sh
go tool nsgo test
```

`TestMain` cede a la plataforma el hilo principal cuando las pruebas se ejecutan en un destino, y `go tool nsgo test -target ios_simulator` ejecuta la misma prueba en el simulador con su entrada nativa.

## Distribúyela

```sh
go tool nsgo build
```

`build` escribe un paquete de lanzamiento para cada destino que enumera `nsgo.yaml`, dentro de `dist/<name>-<version>-<target>`: un `.app`, un `.ipa`, un `.apk`, los directorios de Windows y Linux, o un sitio web. Todo lo que se va a distribuir también queda en `dist/releases`: la imagen de disco, el programa de instalación, los paquetes de Debian y Arch, los IPA, los APK, el sitio, los miniprogramas y los scripts que instalan la aplicación desde tus lanzamientos de GitHub. [Compilación y distribución](../building.md) cubre instaladores, firma y notarización, publicación, íconos, widgets, App Clips y la App Store.

## Siguiente

- [Ejemplo: una aplicación de notas](notes.md) y [Ejemplo: repositorios populares de GitHub](repos.md): dos aplicaciones más, paso a paso.
- [Componentes y estado](../components.md): elementos, componentes, señales y regiones en más detalle.
- [Diseño](../layout.md) y [Estilos y temas](../styles.md): la estructura y el aspecto de la pantalla.
- [Listas y desplazamiento](../lists.md): listas largas, encabezados fijos y carga adicional.
- [Pruebas](../testing.md): busca controles, presiónalos y toma capturas de pantalla.
- [Incrustación](../embedding.md): coloca la aplicación dentro de otra aplicación.
