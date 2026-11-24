---
title: Textos, pruebas y publicación
description: Traduce los textos de un componente, pruébalo como lo haría un usuario y ofrécelo a aplicaciones Go y TSX.
group: Escribir componentes
order: 74
---

## Textos

Un componente expresa sus propios textos, como el nombre de un botón para lectores de pantalla, a través de los catálogos de su paquete en lugar de usar cadenas literales. Incluye un paquete `locales/` en la biblioteca con el catálogo del idioma de origen y un archivo que genere el código:

```yaml
# locales/en.yaml
clear:
  message: Clear
  description: What screen readers say of the button that empties a chip input.
```

```go
// Package locales says the chips' words in the language the app shows.
package locales

//go:generate go tool nsgo i18n
```

`go generate ./locales` genera una función por mensaje, que el componente llama con su ámbito: `locales.Clear(cx)` lo devuelve en el idioma que muestra la aplicación. Haz commit del archivo `catalog_gen.go` generado para que las aplicaciones compilen solo con Go. Añade un archivo por cada idioma al que traduzcas (`zh-Hans.yaml`); una aplicación proporciona otros en sus propios catálogos, bajo una clave que indique la ruta de importación de tu paquete, tal como ocurre con [los textos propios de NSGo](../localization.md#las-palabras-propias-de-nsgo).

## Pruebas

`nsgotest` ejecuta un componente tal como lo usaría un usuario, sin interfaz gráfica. Móntalo en una pequeña raíz y actúa sobre él por rol y texto:

```go
func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestToggle(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		on := core.NewSignal(cx, false)
		return chips.Toggle{On: on.Get, OnChange: on.Set, Label: "Bold"}
	})
	app.Press("role:switch Bold")
	if !app.Find("role:switch Bold").Props.Checked {
		t.Fatal("the toggle did not turn on")
	}
}
```

Desde la raíz de la biblioteca, `go tool nsgo test` ejecuta todos los paquetes, y `-target macos` u otro destino ejecuta las mismas pruebas en una ventana nativa. Prueba ambas apariencias y una configuración regional simulada (`-lang en-XA`) para detectar colores y textos fijos en código. [Pruebas](../testing.md) cubre el resto.

## Publicación

Una biblioteca de componentes es un módulo Go: las aplicaciones ejecutan `go get` e importan sus paquetes. Mantén sus versiones sin estilos y con estilos en paquetes separados cuando existan ambas, para que una aplicación con diseño propio no importe estilos que no usa.

La página [Componentes](https://nsgo.dev/components/) del sitio enumera bibliotecas que otros pueden utilizar; añade un documento y una captura de pantalla en `docs/public/components/` para publicar la tuya.

Las aplicaciones TSX acceden a los componentes de Go una vez registrados en una función `init`:

```go
func init() {
	tsx.Register("example.com/chips", map[string]core.Node{"Toggle": Toggle{}})
}
```

`import { Toggle } from "example.com/chips"` toma entonces los campos como propiedades ([Aplicaciones TSX](../tsx.md#go-desde-tsx)).
