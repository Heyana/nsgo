import { createFromSource } from "fumadocs-core/search/server"

import { source } from "~/lib/source"

// Prerendered into a static index that the search dialog downloads.
const server = createFromSource(source)

export function loader() {
  return server.staticGET()
}
