import { reactRouter } from "@react-router/dev/vite"
import tailwindcss from "@tailwindcss/vite"
import { fumadocsMdx } from "fumadocs-mdx/vite"
import { defineConfig } from "vite"

// Must match the React Router `basename` in react-router.config.ts so the
// generated asset URLs also resolve under the GitHub Pages project path.
const basePath = process.env.BASE_PATH ?? "/nsgo/"

export default defineConfig({
  base: basePath,
  resolve: { tsconfigPaths: true },
  server: { watch: { ignored: ["**/build/**", "**/.source/**"] } },
  plugins: [
    fumadocsMdx({
      globalOptions: {
        mdxOptions: {
          rehypeCodeOptions: {
            themes: { light: "github-light", dark: "github-dark" },
          },
        },
      },
    }),
    tailwindcss(),
    reactRouter(),
  ],
})
