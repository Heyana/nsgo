import { glob } from "node:fs/promises"

import type { Config } from "@react-router/dev/config"

// GitHub Pages serves the site from https://<owner>.github.io/<repo>/.
// Override BASE_PATH to deploy under a custom domain or path.
const basePath = process.env.BASE_PATH ?? "/nsgo/"

/**
 * The URL of a documentation file: index.mdx is the folder itself, a
 * translation is name.zh.mdx, and (group) folders add no segment.
 */
function docsPath(file: string): string {
  let name = file.replace(/\.mdx$/, "")
  let prefix = ""
  if (name.endsWith(".zh")) {
    name = name.slice(0, -".zh".length)
    prefix = "/zh"
  }
  const segments = name.split("/").filter((s) => !/^\(.+\)$/.test(s))
  if (segments.at(-1) === "index") segments.pop()
  return [prefix + "/docs", ...segments].join("/")
}

// The build prerenders every page as static files. Development renders on
// request instead, so that a page added while the server runs opens at once
// rather than waiting for the prerender list to be read again.
const building = process.argv.includes("build")

export default {
  basename: basePath,
  ssr: !building,
  // Every page ships as its own static HTML file, so a shared link opens
  // directly with its own title instead of through the 404 fallback.
  async prerender() {
    const paths = ["/", "/zh", "/api/search"]
    for await (const file of glob("**/*.mdx", { cwd: "content/docs" })) {
      paths.push(docsPath(file))
    }
    return paths
  },
} satisfies Config
