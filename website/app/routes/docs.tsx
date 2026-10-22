import { DocsLayout } from "@fumadocs/base-ui/layouts/docs"
import {
  DocsBody,
  DocsDescription,
  DocsPage,
  DocsTitle,
  EditOnGitHub,
} from "@fumadocs/base-ui/layouts/docs/page"
import { useFumadocsLoader } from "fumadocs-core/source/client"
import { use } from "react"

import type { Route } from "./+types/docs"
import { useMDXComponents } from "~/components/mdx"
import { i18n, localeOf, type Locale } from "~/lib/i18n"
import { baseOptions } from "~/lib/layout.shared"
import { site } from "~/lib/site"
import { docs, source } from "~/lib/source"

export async function loader({ params }: Route.LoaderArgs) {
  const locale = localeOf(params.lang)
  if (!locale) throw new Response("Not found", { status: 404 })
  const slugs = params["*"].split("/").filter((v) => v.length > 0)
  const page = source.getPage(slugs, locale)
  if (!page) throw new Response("Not found", { status: 404 })

  return {
    locale,
    path: page.path,
    title: page.data.title,
    description: page.data.description,
    pageTree: await source.serializePageTree(source.getPageTree(locale)),
  }
}

export function meta({ loaderData }: Route.MetaArgs) {
  if (!loaderData) return [{ title: site.name }]
  const title = `${loaderData.title} | ${site.name}`
  return [
    { title },
    { name: "description", content: loaderData.description },
    { property: "og:title", content: title },
    { property: "og:description", content: loaderData.description },
    { property: "og:type", content: "article" },
  ]
}

const editText: Record<Locale, string> = {
  en: "Edit this page on GitHub",
  zh: "在 GitHub 上编辑此页",
}

function Content({ path, locale }: { path: string; locale: Locale }) {
  const page = docs.getPage(path)
  if (!page) throw new Error(`unknown page: ${path}`)

  // The page's content loads lazily, so the docs share no single bundle.
  const { toc } = use(page.load())
  const Mdx = page.body

  return (
    <DocsPage toc={toc} tableOfContent={{ style: "clerk" }}>
      <DocsTitle>{page.title}</DocsTitle>
      <DocsDescription>{page.description}</DocsDescription>
      <DocsBody>
        <Mdx components={useMDXComponents()} />
      </DocsBody>
      <EditOnGitHub href={`${site.blob}website/content/docs/${path}`}>
        {editText[locale]}
      </EditOnGitHub>
    </DocsPage>
  )
}

export default function Page({ loaderData }: Route.ComponentProps) {
  const { pageTree } = useFumadocsLoader(loaderData)
  const locale = loaderData.locale ?? i18n.defaultLanguage

  return (
    <DocsLayout {...baseOptions(locale)} tree={pageTree}>
      <Content path={loaderData.path} locale={locale} />
    </DocsLayout>
  )
}
