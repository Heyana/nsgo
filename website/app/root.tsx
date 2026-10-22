import { RootProvider } from "@fumadocs/base-ui/provider/react-router"
import { i18nProvider } from "@fumadocs/base-ui/i18n"
import {
  Links,
  Meta,
  Outlet,
  Scripts,
  ScrollRestoration,
  isRouteErrorResponse,
  useParams,
} from "react-router"

import type { Route } from "./+types/root"
import iconUrl from "~/assets/icon.png"
import { NotFound } from "~/components/not-found"
import { i18n, localeOf, translations } from "~/lib/i18n"
import "./app.css"

export function Layout({ children }: { children: React.ReactNode }) {
  const { lang } = useParams()
  const locale = localeOf(lang) ?? i18n.defaultLanguage

  return (
    <html lang={locale === "zh" ? "zh-CN" : "en"} suppressHydrationWarning>
      <head>
        <meta charSet="utf-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <link rel="icon" type="image/png" href={iconUrl} />
        <Meta />
        <Links />
      </head>
      <body className="flex min-h-screen flex-col">
        <RootProvider
          i18n={i18nProvider(translations, locale)}
          search={{
            options: {
              type: "static",
              api: `${import.meta.env.BASE_URL}api/search`,
            },
          }}
        >
          {children}
        </RootProvider>
        <ScrollRestoration />
        <Scripts />
      </body>
    </html>
  )
}

export default function App() {
  return <Outlet />
}

export function ErrorBoundary({ error }: Route.ErrorBoundaryProps) {
  if (isRouteErrorResponse(error) && error.status === 404) return <NotFound />

  let details = "An unexpected error occurred."
  let stack: string | undefined
  if (isRouteErrorResponse(error)) {
    details = error.statusText || details
  } else if (import.meta.env.DEV && error instanceof Error) {
    details = error.message
    stack = error.stack
  }
  return (
    <main className="mx-auto flex w-full max-w-3xl flex-1 flex-col justify-center gap-3 px-6 py-16">
      <h1 className="text-3xl font-semibold">Error</h1>
      <p className="text-fd-muted-foreground">{details}</p>
      {stack ? (
        <pre className="overflow-x-auto rounded-xl border p-4 text-sm">
          <code>{stack}</code>
        </pre>
      ) : null}
    </main>
  )
}
