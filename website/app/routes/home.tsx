import type { Route } from "./+types/home"
import { Landing } from "~/components/home/landing"
import { homeText } from "~/components/home/text"
import { homeData } from "~/lib/home.server"
import { localeOf } from "~/lib/i18n"

export async function loader({ params }: Route.LoaderArgs) {
  const locale = localeOf(params.lang)
  if (!locale) throw new Response("Not found", { status: 404 })
  return { locale, ...(await homeData()) }
}

export function meta({ loaderData }: Route.MetaArgs) {
  const t = homeText[loaderData?.locale ?? "en"]
  return [
    { title: t.title },
    { name: "description", content: t.description },
    { property: "og:title", content: t.title },
    { property: "og:description", content: t.description },
    { property: "og:type", content: "website" },
  ]
}

export default function Home({ loaderData }: Route.ComponentProps) {
  return <Landing {...loaderData} />
}
