# Icons

`core.Icon` draws an SVG as an icon in the text color it inherits, and `icons/lucide` is the bundled Lucide pack. Read this before changing SVG parsing, the icon element or the generated pack.

An icon is a vector element, not an image: `core.SVG` turns a document into canvas operations, and every backend already plays those. NSH instead rasterizes SVG files on the image path (`nsh/docs/dev/images.md`); NSGo's icons need no rasterizer, no per-platform SVG engine and no pixel-density variants. Loading SVG as an `Image` stays on the [roadmap](../roadmap.md).

## SVG documents

`ParseSVG` reads the parts icons use and nothing else:

| Read | Not read |
| --- | --- |
| `path`, `rect`, `circle`, `ellipse`, `line`, `polyline`, `polygon` inside `svg` and `g` | Transforms, gradients, patterns, text, images, clip paths, masks, filters, markers, animations, style sheets |
| `fill`, `stroke`, `stroke-width`, `stroke-linecap`, `stroke-linejoin`, inherited through groups | Alpha, `fill-rule`, dashes |
| `none`, `currentColor`, `#rgb`, `#rgba`, `#rrggbb`, `#rrggbbaa` colors | Names, `rgb()`, `var()` |

Parsing records `DrawOp`s in the `viewBox` with the color zero standing for `currentColor`. `SVG.Draw` substitutes the icon's color, scales the document uniformly to fit its box, centers it and leaves it as one immutable `Drawing` the platform replays. A non-square box keeps the aspect ratio.

- Path data supports every SVG 1.1 command; elliptical arcs become cubic curves of at most a quarter turn each.
- A rounded `rect` takes elliptical corner radii; only equal radii become one `DrawRoundRect`.
- Unknown elements are skipped and an unread paint keeps the inherited one, so a document that starts using gradients draws its fallbacks instead of failing the build.

## Icon element

`core.Icon` mounts as a `KindCanvas` view, so no backend knows about icons. It takes the text color of its ancestors and, without a style size, is as high as the inherited font size, as wide as the SVG's aspect ratio makes it. `Measure` sizes it like an image, and `SVG.Draw` fits it into the result.

- Each commit's style resolution writes the inherited color into the icon's signal; a changed color redraws without a relayout.
- `SVGFunc` re-reads a document from a signal, as `Image.SourceFunc` does, so a list's cell follows its row without being rebuilt. A document whose intrinsic size changes marks the icon for layout.
- `Label` gives the icon the image role and a name for assistive technology; without it the icon is a decoration that technology skips.

## Lucide pack

`icons/lucide` has one accessor per icon of a pinned Lucide release. `upstream.json` records the repository, the full commit and a digest of the license and icons. Each accessor parses its document on first use and returns the same `*core.SVG` afterwards; a zero-value cache keeps unused icons out of package initializers, so a binary carries only the icons it uses.

There is no runtime registry. The exhaustive catalog lives in `icons_gen_test.go` and in the gallery's generated `icons_gen.go`, so the package carries only what an app imports.

Regenerate from the repository root with a clean Lucide checkout at the commit:

```sh
git -C /path/to/lucide checkout <40-character-commit>
go run ./tools/lucidegen -upstream /path/to/lucide -commit <40-character-commit>
```

- The generator requires a clean checkout at exactly that commit, parses every icon with `core.ParseSVG` and rejects any icon that is not 24 by 24.
- It writes `icons/lucide/icons_gen.go`, `icons_gen_test.go`, `LICENSE` and `upstream.json`, and the gallery's catalog.
- It reports accessors added and removed. Review removals and renames before accepting an update, since they change the public API.
- `-check` verifies the files without writing them.
- Names are upstream kebab-case in PascalCase; upstream aliases and lab icons are excluded.

The pack keeps Lucide's ISC license and Feather's MIT notice in `icons/lucide/LICENSE`; `NOTICE` records the attribution. Never edit the generated SVG strings: fidelity to upstream is the point.

## Verification

`go test ./core -run SVG` covers the parser, the path grammar and fitting. `go test ./icons/lucide` parses every icon, checks its size and that accessors share their document. The gallery's Icons section reviews the pack by eye: sizes, colors, buttons and the filtered browser, in both appearances on a running target.
