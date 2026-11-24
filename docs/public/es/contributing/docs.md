---
title: Documentación y sitio web
description: Escribir una página de esta documentación, traducirla, ejecutar el sitio web y mantener al día la habilidad de los agentes.
group: Contribuir
order: 83
---

## Una página

Esta documentación es el Markdown en `docs/public/`, legible en GitHub tal cual está. `en/` y `zh/` se reflejan ruta a ruta; una página que falta en chino muestra la versión en inglés con un aviso. Cada página comienza con metadatos iniciales:

```yaml
---
title: Testing
description: Test an app the way a user uses it, headless or in a native window.
group: Toolchain
order: 21
---
```

- `group` es el grupo de la página en la barra lateral, en el idioma de la página, y `order` la ubica. Los grupos siguen el recorrido del lector: tutoriales, herramientas, componentes, conceptos básicos y luego escribir componentes y contribuir. Una página sin grupo, como la introducción, va antes que ellos.
- El diseño ya muestra el título y la descripción, por lo que el cuerpo empieza en `##`.

Enlace entre páginas con enlaces `.md` relativos (`[Pruebas](testing.md#run-on-a-platform)`); el sitio los convierte en sus URLs. Cada página responde una pregunta; cómo funciona NSGo por dentro permanece en `docs/architecture/`.

## El sitio web

El sitio web en `website/` se compila con [nsgo-website](https://github.com/nsgo-dev/nsgo-website), una integración de Astro, a partir de los ajustes en `website/site.config.ts`. Necesita [Bun](https://bun.com) 1.4 o posterior:

```sh
cd website
bun install
bun run dev    # at http://localhost:4321/
bun run build  # type-check and build, as the deployment does
```

`website/README.md` indica cómo se compila y publica el sitio.

## Para agentes

El sitio web también publica las páginas en inglés para modelos de lenguaje: `/llms.txt` las enumera, `/llms-full.txt` las contiene todas y `/docs/<page>.md` contiene una sola, con enlaces absolutos y sin las marcas de los bloques de código (`src/lib/llms.ts`). Siguen a las páginas sin un paso adicional propio.

`skills/nsgo/` es la habilidad que los agentes instalan con `bunx skills add nsgo-dev/nsgo`: `SKILL.md` con las reglas que sigue una aplicación y `references/` que se leen cuando una tarea las necesita. Modifíquela en el mismo cambio que la documentación cuando cambie lo que escribe una aplicación, los comandos de la herramienta, las pruebas o el empaquetado; manténgala limitada a lo que un agente debe seguir y enlace la documentación para el resto. `go tool nsgo test -run TestSkills .` comprueba sus metadatos iniciales, sus enlaces y cada nombre de NSGo que menciona.
