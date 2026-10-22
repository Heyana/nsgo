# NSGo website

The project's public site: a home page in English and Simplified Chinese, and a Fumadocs documentation area whose guides are still being written. It is a static React Router site that deploys to GitHub Pages at <https://oneo-me.github.io/nsgo/>.

The site is independent of the Go module. It has no live WebAssembly examples yet; the windows on the home page are page-side recreations of `examples/hello` and the README counter.

## Requirements

- [Bun](https://bun.sh) 1.4 or newer.

## Commands

Run these from `website/`:

```sh
bun install          # install dependencies
bun run dev          # serve at http://localhost:5173/nsgo/
bun run check        # check that every page has a Chinese translation
bun run typecheck    # generate route types and typecheck
bun run build        # prerender the static site into build/pages
bun run preview      # serve build/pages the way GitHub Pages does
bun run format       # format with Prettier
```

## Adding a page

1. Add `name.mdx` under `content/docs/` (or a subject folder) with a `title` and `description`.
2. Add `name.zh.mdx` beside it with the Simplified Chinese copy.
3. List the pages in that folder's `meta.json` and `meta.zh.json`, which order the sidebar.

Pages can use Fumadocs' `Steps`, `Tabs`, `Accordion`, `Accordions` and `TypeTable`, registered in [`app/components/mdx.tsx`](app/components/mdx.tsx). `bun run check` fails when a translation is missing.

## Structure

| Path | Purpose |
| --- | --- |
| `content/docs/` | Documentation pages, their translations and sidebar order |
| `app/routes/` | The home page, the docs pages, the static search index and the 404 page |
| `app/components/home/` | The home page, its copy and the window recreations |
| `app/lib/` | Site constants, i18n, the Fumadocs source and the home page's data |
| `scripts/prepare-pages.mjs` | Merges the prerendered pages and assets into the Pages artifact |
| `scripts/check-docs.ts` | The translation check behind `bun run check` |

`app/assets/icon.png` is copied from `docs/images/logo.png`.

## Design

The site follows the gallery's Onevra design language: warm neutral surfaces, one terracotta accent and 8-point controls. [`app/app.css`](app/app.css) holds the tokens for both appearances; use them instead of hardcoded colors. The home page's window recreations mirror `examples/hello` and the same control metrics.

## Deployment

The [Website workflow](../.github/workflows/website.yml) checks, builds and publishes the site whenever `website/**` changes on `main`. Enable it once under **Settings → Pages → Build and deployment → Source → GitHub Actions**.

`BASE_PATH` overrides the deploy path in `vite.config.ts` and `react-router.config.ts`. It defaults to `/nsgo/`; set it to `/` for a custom domain. React Router writes prerendered pages under `build/client/nsgo/` while Vite keeps assets at the root of `build/client`, so `scripts/prepare-pages.mjs` merges both into `build/pages` and adds a `404.html` fallback.
