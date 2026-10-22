// Checks that every documentation page has its Chinese translation, so a
// translation cannot be forgotten as pages are added.
//
//   bun run check

import { glob } from "node:fs/promises"

const pages = new Set<string>()
for await (const file of glob("**/*.mdx", { cwd: "content/docs" })) {
  pages.add(file)
}

const problems: string[] = []
for (const page of pages) {
  if (page.endsWith(".zh.mdx")) {
    if (!pages.has(page.replace(/\.zh\.mdx$/, ".mdx"))) {
      problems.push(`${page} has no English original`)
    }
  } else if (!pages.has(page.replace(/\.mdx$/, ".zh.mdx"))) {
    problems.push(`${page} has no Chinese translation`)
  }
}

if (problems.length > 0) {
  console.error(problems.map((p) => `check-docs: ${p}`).join("\n"))
  process.exit(1)
}
console.log("check-docs: every page has a translation")
