---
title: Docs and the website
description: Write a page of these docs, translate it, run the website and keep the agents' skill current.
group: Contributing
order: 83
---

## A page

These docs are the Markdown in `docs/public/`, readable on GitHub as they are. `en/` and `zh/` mirror each other path for path; a page missing in Chinese shows the English one with a notice. Each page starts with front matter:

```yaml
---
title: Testing
description: Test an app the way a user uses it, headless or in a native window.
group: Toolchain
order: 21
---
```

- `group` is the page's group in the sidebar, in the page's language, and `order` places it. The groups follow a reader's way in: tutorials, the toolchain, components, core concepts, then writing components and contributing. A page without a group, such as the introduction, comes before them.
- The layout shows the title and the description, so the body starts at `##`.

Link between pages with relative `.md` links (`[Testing](testing.md#run-on-a-platform)`); the site turns them into its URLs. Each page answers one question; how NSGo works inside stays in `docs/architecture/`.

## The website

The website in `website/` is built by [nsgo-website](https://github.com/nsgo-dev/nsgo-website), an Astro integration, from the settings in `website/site.config.ts`. It needs [Bun](https://bun.com) 1.4 or later:

```sh
cd website
bun install
bun run dev    # at http://localhost:4321/
bun run build  # type-check and build, as the deployment does
```

`website/README.md` says how the site is built and published.

## For agents

The website also publishes the English pages for language models: `/llms.txt` lists them, `/llms-full.txt` holds them all and `/docs/<page>.md` holds one, with absolute links and without the code blocks' marks (`src/lib/llms.ts`). They follow the pages with no step of their own.

`skills/nsgo/` is the skill agents install with `bunx skills add nsgo-dev/nsgo`: `SKILL.md` with the rules an app follows, and `references/` read when a task needs them. Change it in the same change as the docs when what an app writes, the tool's commands, testing or packaging change; keep it to what an agent must follow and link the docs for the rest. `go tool nsgo test -run TestSkills .` checks its front matter, its links and every name of NSGo's it mentions.
