import type { BaseLayoutProps } from "@fumadocs/base-ui/layouts/shared"

import { Logo } from "~/components/logo"
import { localized, type Locale } from "~/lib/i18n"
import { site } from "~/lib/site"

const text = {
  en: { docs: "Docs", changelog: "Changelog" },
  zh: { docs: "文档", changelog: "更新日志" },
} satisfies Record<Locale, Record<string, string>>

/** The header shared by the home page and the documentation. */
export function baseOptions(locale: Locale): BaseLayoutProps {
  const t = text[locale]
  return {
    nav: {
      title: (
        <>
          <Logo className="size-6 rounded-[6px]" />
          <span className="font-semibold tracking-tight">{site.name}</span>
        </>
      ),
      url: localized(locale, "/"),
    },
    githubUrl: site.github,
    links: [
      {
        text: t.docs,
        url: localized(locale, "/docs"),
        active: "nested-url",
        on: "nav",
      },
      { text: t.changelog, url: site.changelog, external: true },
    ],
  }
}
