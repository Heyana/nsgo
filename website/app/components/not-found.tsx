import { HomeLayout } from "@fumadocs/base-ui/layouts/home"
import { Link, useLocation } from "react-router"

import { localized, type Locale } from "~/lib/i18n"
import { baseOptions } from "~/lib/layout.shared"

const text = {
  en: {
    title: "Page not found",
    body: "This page does not exist, or has moved.",
    back: "Back to the docs",
  },
  zh: {
    title: "页面不存在",
    body: "这个页面不存在，或者已经移动。",
    back: "返回文档",
  },
}

export function NotFound() {
  const { pathname } = useLocation()
  const locale: Locale = /^\/zh(\/|$)/.test(pathname) ? "zh" : "en"
  const t = text[locale]
  return (
    <HomeLayout {...baseOptions(locale)}>
      <main className="flex flex-1 flex-col items-center justify-center gap-3 px-6 py-24 text-center">
        <p className="font-mono text-sm text-fd-primary">404</p>
        <h1 className="text-3xl font-semibold tracking-tight">{t.title}</h1>
        <p className="text-fd-muted-foreground">{t.body}</p>
        <Link
          to={localized(locale, "/docs")}
          className="mt-4 rounded-full bg-fd-primary px-4 py-2 text-sm font-medium text-fd-primary-foreground"
        >
          {t.back}
        </Link>
      </main>
    </HomeLayout>
  )
}
