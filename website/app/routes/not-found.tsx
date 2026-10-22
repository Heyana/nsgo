import { NotFound } from "~/components/not-found"

export function meta() {
  return [{ title: "Not found | NSGo" }]
}

export default function NotFoundRoute() {
  return <NotFound />
}
