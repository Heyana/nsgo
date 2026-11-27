# NSGo website

The site at <https://nsgo.dev/>, in English and Chinese, built by
[nsgo-website](https://github.com/nsgo-dev/nsgo-website), the Astro
integration every NSGo project's site shares: its header, footer, home page
and Starlight docs come from `site.config.ts`. This folder keeps only what is
NSGo's own: the settings, the home page's platforms, screens and flow, the
Benchmarks page, the docs' live previews, the gallery and `llms.txt`. Its
Live demo links open the gallery's web build, compiled by `nsgo`, full page
in a new tab.

```sh
bun install
bun run dev          # build the gallery and the docs' apps, then serve the site at /
bun run build        # build them, typecheck, then build dist/ and its search index
bun run preview      # serve dist/
bun run gallery      # rebuild only the gallery
bun run live         # rebuild only the docs' apps
bun run screenshots  # retake the home page's screens of the gallery
bun run share-cards  # redraw public/share/, the cards a shared link shows
```

`astro.config.mjs` fixes the base at `/` for nsgo.dev on GitHub Pages, so the
dev server also runs at <http://localhost:4321/>.

What the package draws, its words and its look change in nsgo-website, not
here; a setting this site needs that the package lacks belongs there too.

## Pages

| Page | Path | Source |
| --- | --- | --- |
| Home | `/` | `site.config.ts`'s `home`, with `src/components/home/` |
| Docs | `/docs/` | `docs/public/` |
| Benchmarks | `/benchmarks/` | `src/components/Benchmarks.astro` |
| Sponsors | `/sponsors/` | `site.config.ts`'s `sponsors`, with `src/assets/sponsors/` |

English is at the base and Chinese under `zh/`; the package's language menu
switches them and follows the browser's language on a first visit. Add every
word in English and Chinese together.

The header's GitHub link shows the repository's stars, read from GitHub as
the site builds, with `GITHUB_TOKEN` when it is set. The website workflow
rebuilds daily to keep them current.

`scripts/web-build.mjs` builds an app with `go tool nsgo build -target web`
from the repository root into a folder of `public/`, keeping its wasm only
as a `.gzip` copy (and dropping the tool's `.br` and `.gz` siblings); its
`tags` adds build tags. `scripts/build-gallery.mjs` builds the gallery once
per demo group with `-tags gallery_group,gallery_<group>` — each group's
demo list lives behind that tag in `examples/gallery` — and writes the
groups to `public/gallery/<group>/`, each behind `scripts/gallery-loader.html`,
which reports its download, decompression, compilation and start progress
and follows the site's appearance, `starlight-theme`. It writes the group
list into `scripts/gallery-frame.html`, which serves `/gallery/`: the shell
loads one group's app on demand behind the platform frames, keeps
`?group=` and `?platform=` in the address, and `app.html` and `loader.html`
redirect to it. All of `public/`'s builds are generated and ignored.

## Home

`site.config.ts` gives the hero's words, the features, the API and the
closing card. Four parts are NSGo's own components:
`Platforms.astro`, the hero's board of every platform in its device's shape,
from `src/platforms.ts`, over `HeroField.astro`, a faint text art in
WebGL2 behind the whole hero; `Screens.astro`, real screens of the gallery;
`How.astro`, the flow from the app to the engine to the platform; and
`Workflow.astro`, the tool's four steps with their commands. Their styles
are in `src/styles/site.css`.

The screens are `src/assets/screens/`, the gallery's desktop and phone
layouts in each language and in light and dark, named
`<layout>-<lang>-<theme>.jpg`; the page shows the pair in the site's
appearance. `bun run screenshots` retakes them all from `public/gallery/`
(run `bun run gallery` first) with Google Chrome and macOS's `sips`; retake
them when the gallery's look changes or a language is added, giving it a
locale in `scripts/screenshots.mjs`.

A shared link shows `public/share/<lang>.png`, which `bun run share-cards`
draws from the settings with Google Chrome; redraw them when the headline
changes.

## Benchmarks

The Benchmarks page shows `benchmark/results/`, which `benchmark/runner`
writes on each desktop ([benchmark](../benchmark/README.md)): bar charts a
case, with a switch beside each case's title that shows every case on one
desktop, and a table of where each framework runs, whose rows are in
`src/lib/benchmarks.ts`. A desktop without results says so. Its words are in
`src/lib/benchmark-copy.ts`.

## Docs

The docs are the Markdown in the repository's `docs/public/`, whose README
says how to write them; the package serves `en/` at `docs/` and `zh/` at
`zh/docs/`, and a page Chinese lacks shows the English one with a notice.
`src/lib/docs.ts` builds the sidebar from the pages' `group` and `order`.
`astro.config.mjs` gives Starlight's Markdown two Sätteri plugins:
`src/lib/doc-links.mjs` turns the relative `.md` links between pages into
the site's URLs, and `src/lib/live.mjs` puts a box before each code block
marked `preview=<demo>` or `program` (`docs/public/README.md`).

`scripts/build-live.mjs` builds what the boxes run as one app in
`public/live/`, so a reader downloads NSGo once for the whole site:
`examples/previews`, the component examples, and each program block's code,
built once when a translation shares it, each a package whose `Root` the app
runs when its launch parameters name it. Programs thus share one storage. It
also bundles `src/scripts/live.ts` as `public/live/boxes.js`, which each box
loads: it mounts a box through the tool's loader once it nears the window,
compiling the wasm once a page, in the site's appearance, which it follows,
and the page's language.

`src/lib/llms.ts` publishes the English pages for language models:
`/llms.txt` lists them by group, `/llms-full.txt` holds them all and
`src/pages/docs/[...slug].md.ts` serves each at `/docs/<page>.md`, its
relative links made absolute and its code blocks' marks dropped.

Search is Starlight's Pagefind, built with the site, so it works in
`bun run preview` but not in the dev server.

## Docs: goals

- **Versions from 1.0.** Until 1.0 the docs show `main`. From 1.0 each
  minor release keeps its docs, built from its release tag so
  `docs/public/` stays the only copy: `/docs/` is the latest release,
  `/docs/1.2/` an older one and `/docs/next/` `main`, with a version switch
  that keeps the page when it exists.
- **Translations kept current.** The build lists Chinese pages that are
  missing or older than the English.
- **Search by version**, once versions exist.
- **Checks.** The build fails on a broken link or anchor, and code blocks
  marked as programs compile through `go tool nsgo check`.

## Deployment

The [Website workflow](../.github/workflows/website.yml) builds `website/dist`
and publishes it to GitHub Pages.
