---
title: Publicación en la web
description: Publica una aplicación de NSGo con semántica de documento, metadatos de página y recursos desde una CDN.
group: Herramientas
order: 24
---

## Compilar la aplicación

```sh
go tool nsgo build -target web
```

Sirve el directorio generado mediante HTTP. Los recursos de lanzamiento tienen hashes de contenido en sus nombres, así que almacénalos en caché como inmutables; revalida `index.html`, `nsgo.json` y `manifest.webmanifest`. La aplicación web conserva el comportamiento de disposición, desplazamiento, foco y puntero de NSGo.

## Encabezados y enlaces

Un `core.View` o `core.Text` con `Role: core.RoleHeader` se convierte en `h1` hasta `h6`, seleccionado según `AccessibilityLevel`; sin nivel es `h2`. Los encabezados Markdown de `base/textview` conservan sus niveles. Los márgenes y las fuentes de encabezado del navegador no modifican los estilos de la aplicación.

`base.Link` y `ui.Link` publican su `Href` como un enlace de ancla (`anchor`). Un clic normal ejecuta `OnOpen` y `OnPress` de la aplicación; un clic central o un clic con una tecla modificadora usa la acción de enlace del navegador. El navegador proporciona su menú contextual de enlace salvo que la aplicación gestione `OnContextMenu`. Un enlace deshabilitado no tiene dirección activa. Para una vista personalizada con rol de enlace, asigna `core.WebViewProps{Href: "/notes"}` a `core.View.Web` o vincula `HrefFunc`. TSX usa `web={{href: "/notes"}}`; los encabezados usan `accessibilityLevel={1}`. Se rechazan los esquemas de URL ejecutables inseguros. El contenido confinado mantiene la navegación en sus manejadores de pulsación con ámbito restringido y no expone ninguna dirección del navegador.

## Metadatos de la página

Importa `github.com/nsgo-dev/nsgo/web` y coloca un `web.Head` junto al contenido de una página:

```go
core.Group{
	web.Head{Title: "Notes", Description: "Your saved notes"},
	pageContent,
}
```

`TitleFunc` y `DescriptionFunc` siguen señales. Los `Head` visibles posteriores sobrescriben los campos que establecen y heredan los demás. Ocultar o eliminar una página restaura los `Head` anteriores; eliminar todos los `Head` restaura el documento original. Un campo estático vacío hereda el valor; una función que devuelve una cadena vacía lo borra. Un `Head` no añade ningún nodo de disposición. Los destinos nativos, las aplicaciones incrustadas y el contenido confinado dejan intacto el documento de su host.

En TSX, importa `Head` desde `nsgo/web` y escribe `<Head title={title()} description="Your saved notes" />`. Para vistas previas en HTML estático, prerrenderiza cada URL como parte del despliegue del sitio; `Head` actualiza el documento en ejecución.

## Servir recursos desde una CDN

El cargador resuelve todos los archivos de la aplicación de forma relativa a `nsgo.json`. Una página puede cargar la aplicación desde otro origen:

```js
const url = new URL("https://cdn.example.com/notes/nsgo.json");
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url).href);
await mount(null, url.href, { config });
```

Permite que el origen de la página obtenga los archivos de la CDN mediante CORS y sirve WebAssembly como `application/wasm`. Sirve el HTML de entrada de la aplicación para cualquier ruta profunda que utilice tu aplicación. [Incrustación](embedding.md#web) muestra cómo montarla dentro de un elemento en su lugar.
