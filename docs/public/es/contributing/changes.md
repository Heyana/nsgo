---
title: Realizar un cambio
description: Lo que necesita una pull request a NSGo, desde el mismo cambio en cada backend hasta pruebas, documentación y una línea en el registro de cambios.
group: Contribuir
order: 82
---

## Antes de empezar

Abra un issue o una discusión para cualquier cosa más grande que una corrección, de modo que el diseño quede acordado antes del código. Las preguntas que el diseño deja abiertas se enumeran en [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md#open-questions); sus respuestas se deciden allí primero.

## Cada backend

Una corrección o una funcionalidad en un backend normalmente corresponde también a los demás. Haga el mismo cambio en cada destino que lo necesite, o registre lo que falta en la [hoja de ruta](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md). Los componentes se ven y se comportan igual en todas las plataformas.

## Comprobaciones

Use la herramienta `nsgo` en lugar de `go vet`, `go test` o `go build`:

```sh
go tool nsgo check   # vet every package
go tool nsgo test    # every package's tests
```

Añada una prueba para lo que cambie: `nsgotest` para lo que ve un usuario, las pruebas propias de un paquete para el resto. Ejecute la galería en las plataformas que haya tocado ([Configuración de desarrollo](../contributing.md#the-gallery)).

Formatee los archivos Go con `gofmt` de la versión de Go en `go.mod`; `gofmt -l .` enumera los archivos que lo necesitan. CI comprueba cada archivo Go rastreado y falla con los nombres de los que no estén formateados.

## La pull request

Titúlela como un Conventional Commit, como `fix(linux): keep the caret after a paste`: una fusión por aplastamiento convierte el título en el asunto del commit, y una comprobación falla hasta que encaja. La plantilla pide los destinos que toca el cambio, capturas de pantalla de antes y después de cualquier elemento visible y las comprobaciones ejecutadas.

## Documentación y registro de cambios

- Actualice el documento cuyos hechos cambian: un tema en `docs/architecture/` sobre cómo funciona NSGo, una página en `docs/public/` sobre cómo lo usan las aplicaciones y los componentes ([Documentación y sitio web](docs.md)).
- Mantenga `README.md` y `README.zh-CN.md` equivalentes; todo lo demás está en inglés, con páginas en chino en `docs/public/zh/`.
- Registre un cambio visible para los usuarios como una línea, con una referencia como `(#12 by @you)`, en un archivo propio dentro de `changelog.d/` bajo el encabezado de su tipo, como `### Fixed`, en lugar de en `CHANGELOG.md`, que un lanzamiento rellena a partir de esos archivos ([formato](https://github.com/nsgo-dev/nsgo/blob/main/changelog.d/README.md)).
