import { HomeLayout } from "@fumadocs/base-ui/layouts/home"
import {
  ArrowRight,
  Boxes,
  Check,
  Code,
  FlaskConical,
  PanelsTopLeft,
  Ruler,
  Zap,
} from "lucide-react"
import { Link } from "react-router"

import { GithubIcon } from "~/components/github-icon"
import { InstallCommand } from "~/components/home/install-command"
import { homeText } from "~/components/home/text"
import { CounterWindow, HelloWindow } from "~/components/home/window-mocks"
import { WindowFrame } from "~/components/home/window-frame"
import { localized, type Locale } from "~/lib/i18n"
import { baseOptions } from "~/lib/layout.shared"
import { site } from "~/lib/site"
import { cn } from "~/lib/utils"

const featureIcons = [Code, PanelsTopLeft, Zap, Ruler, FlaskConical, Boxes]

const primaryLink =
  "inline-flex h-11 items-center gap-2 rounded-full bg-fd-primary px-6 text-sm font-medium text-fd-primary-foreground shadow-sm transition-colors hover:bg-fd-primary/90"
const secondaryLink =
  "inline-flex h-11 items-center gap-2 rounded-full border bg-fd-background px-6 text-sm font-medium transition-colors hover:bg-fd-accent"

// A running target is green, a build-only one amber, work in progress blue,
// and a plan neutral.
const statusTones = [
  "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
  "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
  "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
  "bg-amber-500/10 text-amber-700 dark:text-amber-300",
  "bg-sky-500/10 text-sky-700 dark:text-sky-300",
  "bg-fd-muted text-fd-muted-foreground",
]

function Section({
  id,
  title,
  lead,
  className,
  children,
}: {
  id?: string
  title: string
  lead: string
  className?: string
  children: React.ReactNode
}) {
  return (
    <section id={id} className={cn("scroll-mt-20 border-t", className)}>
      <div className="mx-auto w-full max-w-6xl px-6 py-24">
        <div className="mx-auto mb-12 max-w-2xl text-center">
          <h2 className="text-3xl font-semibold tracking-tight text-balance sm:text-4xl">
            {title}
          </h2>
          <p className="mt-4 text-lg text-pretty text-fd-muted-foreground">
            {lead}
          </p>
        </div>
        {children}
      </div>
    </section>
  )
}

export function Landing({
  locale,
  counter,
}: {
  locale: Locale
  counter: string
}) {
  const t = homeText[locale]
  const docs = localized(locale, "/docs")
  const install = `go get ${site.module}`

  return (
    <HomeLayout {...baseOptions(locale)} className="overflow-x-clip">
      {/* Hero */}
      <section className="relative isolate">
        <div
          aria-hidden
          className="absolute inset-0 -z-10 bg-[radial-gradient(ellipse_80%_60%_at_50%_0%,color-mix(in_oklab,var(--color-fd-primary)_16%,transparent),transparent_70%)]"
        />
        <div
          aria-hidden
          className="absolute inset-0 -z-10 bg-[radial-gradient(var(--color-fd-border)_1px,transparent_1px)] [mask-image:radial-gradient(ellipse_70%_55%_at_50%_20%,black,transparent)] bg-size-[22px_22px] opacity-70"
        />
        <div className="mx-auto flex max-w-6xl flex-col items-center px-6 pt-20 pb-10 text-center sm:pt-28">
          <a
            href={site.changelog}
            target="_blank"
            rel="noreferrer"
            className="mb-8 inline-flex items-center gap-2 rounded-full border bg-fd-card/80 px-3 py-1 text-xs font-medium text-fd-muted-foreground backdrop-blur transition-colors hover:text-fd-foreground"
          >
            <span className="size-1.5 rounded-full bg-fd-primary" />
            {t.badge}
            <ArrowRight className="size-3" aria-hidden />
          </a>
          <h1 className="text-5xl font-semibold tracking-tight text-balance sm:text-7xl">
            {t.headline[0]}
            <br />
            <span className="bg-linear-to-r from-fd-primary via-[color-mix(in_oklab,var(--color-fd-primary)_65%,#b7d84a)] to-fd-primary bg-clip-text text-transparent">
              {t.headline[1]}
            </span>
          </h1>
          <p className="mt-6 max-w-2xl text-lg text-pretty text-fd-muted-foreground sm:text-xl">
            {t.lead}
          </p>
          <div className="mt-10 flex flex-wrap justify-center gap-3">
            <Link to={docs} className={primaryLink}>
              {t.start}
              <ArrowRight className="size-4" aria-hidden />
            </Link>
            <a href="#example" className={secondaryLink}>
              {t.browse}
            </a>
          </div>
          <div className="mt-6 max-w-full">
            <InstallCommand command={install} copy={t.copy} copied={t.copied} />
          </div>
        </div>
        <div className="mx-auto max-w-5xl px-4 pb-6 sm:px-6">
          <WindowFrame title={t.windowTitle} className="mx-auto max-w-3xl">
            <div className="h-[360px]">
              <HelloWindow locale={locale} />
            </div>
          </WindowFrame>
          <p className="mx-auto mt-5 max-w-xl text-center text-sm text-fd-muted-foreground">
            {t.windowCaption}
          </p>
        </div>
      </section>

      {/* Code and its result */}
      <Section id="example" title={t.code.title} lead={t.code.lead}>
        <div className="grid items-stretch gap-6 lg:grid-cols-[minmax(0,1.15fr)_minmax(0,0.85fr)]">
          <div className="flex min-w-0 flex-col overflow-hidden rounded-2xl border bg-fd-card">
            <div className="flex h-10 items-center border-b px-4 font-mono text-xs text-fd-muted-foreground">
              {t.code.file}
            </div>
            <div
              className="min-w-0 flex-1 overflow-auto py-3 text-[13px] leading-relaxed [tab-size:4] [&_pre]:bg-transparent! [&_pre]:[tab-size:4]"
              dangerouslySetInnerHTML={{ __html: counter }}
            />
          </div>
          <div className="flex flex-col gap-5">
            <WindowFrame title={t.code.result}>
              <div className="h-[190px]">
                <CounterWindow locale={locale} />
              </div>
            </WindowFrame>
            <ul className="flex flex-col gap-3">
              {t.code.points.map((point) => (
                <li key={point} className="flex gap-3 text-fd-muted-foreground">
                  <Check
                    className="mt-0.5 size-5 shrink-0 text-fd-primary"
                    aria-hidden
                  />
                  {point}
                </li>
              ))}
            </ul>
          </div>
        </div>
      </Section>

      {/* Features */}
      <Section title={t.features.title} lead={t.features.lead}>
        <div className="grid gap-px overflow-hidden rounded-2xl border bg-fd-border sm:grid-cols-2 lg:grid-cols-3">
          {t.features.items.map((item, i) => {
            const Icon = featureIcons[i]
            return (
              <div key={item.title} className="bg-fd-card p-7">
                <div className="mb-5 inline-flex size-10 items-center justify-center rounded-xl bg-fd-primary/10 text-fd-primary">
                  <Icon className="size-5" aria-hidden />
                </div>
                <h3 className="font-semibold">{item.title}</h3>
                <p className="mt-2 text-sm leading-relaxed text-fd-muted-foreground">
                  {item.body}
                </p>
              </div>
            )
          })}
        </div>
      </Section>

      {/* Platforms */}
      <Section title={t.platforms.title} lead={t.platforms.lead}>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {t.platforms.statuses.map((platform, i) => (
            <div
              key={platform.name}
              className="flex flex-col gap-3 rounded-2xl border bg-fd-card p-5"
            >
              <div className="flex items-center justify-between gap-3">
                <h3 className="font-semibold">{platform.name}</h3>
                <span
                  className={cn(
                    "rounded-full px-2.5 py-0.5 text-xs font-medium whitespace-nowrap",
                    statusTones[i]
                  )}
                >
                  {platform.state}
                </span>
              </div>
              <p className="text-sm leading-relaxed text-fd-muted-foreground">
                {platform.body}
              </p>
            </div>
          ))}
        </div>
        <p className="mx-auto mt-8 max-w-xl text-center text-sm text-fd-muted-foreground">
          {t.platforms.note}
        </p>
      </Section>

      {/* Architecture */}
      <Section title={t.architecture.title} lead={t.architecture.lead}>
        <div className="grid items-stretch gap-4 lg:grid-cols-[1fr_auto_1fr] lg:items-center">
          <div className="rounded-2xl border bg-fd-card p-6">
            <p className="font-mono text-xs tracking-wide text-fd-muted-foreground uppercase">
              {t.architecture.engine}
            </p>
            <div className="mt-5 flex flex-wrap items-center gap-2">
              {t.architecture.steps.map((step, i) => (
                <div key={step} className="flex items-center gap-2">
                  {i > 0 ? (
                    <ArrowRight
                      className="size-3.5 shrink-0 text-fd-muted-foreground"
                      aria-hidden
                    />
                  ) : null}
                  <span className="rounded-lg border bg-fd-background px-3 py-1.5 text-sm">
                    {step}
                  </span>
                </div>
              ))}
            </div>
          </div>
          <div className="flex items-center justify-center gap-2 py-1 text-fd-muted-foreground lg:flex-col lg:px-2 lg:py-0">
            <ArrowRight className="size-4 rotate-90 lg:rotate-0" aria-hidden />
            <span className="rounded-full border bg-fd-card px-2.5 py-1 font-mono text-[11px]">
              {t.architecture.batch}
            </span>
          </div>
          <div className="rounded-2xl border bg-fd-card p-6">
            <p className="font-mono text-xs tracking-wide text-fd-muted-foreground uppercase">
              {t.architecture.platform}
            </p>
            <div className="mt-5 rounded-lg border bg-fd-background px-3 py-2.5 text-sm">
              {t.architecture.platformBody}
            </div>
            <p className="mt-3 font-mono text-[11px] text-fd-muted-foreground">
              macOS · iOS · Web · Windows · Android · Linux
            </p>
          </div>
        </div>
        <p className="mx-auto mt-8 max-w-2xl text-center text-sm text-fd-muted-foreground">
          {t.architecture.note}
        </p>
      </Section>

      {/* Call to action */}
      <section className="border-t">
        <div className="mx-auto flex max-w-6xl flex-col items-center px-6 py-24 text-center">
          <h2 className="text-3xl font-semibold tracking-tight sm:text-4xl">
            {t.cta.title}
          </h2>
          <p className="mt-4 text-lg text-fd-muted-foreground">{t.cta.lead}</p>
          <div className="mt-8 flex flex-wrap justify-center gap-3">
            <Link to={docs} className={primaryLink}>
              {t.cta.start}
              <ArrowRight className="size-4" aria-hidden />
            </Link>
            <a
              href={site.github}
              target="_blank"
              rel="noreferrer"
              className={secondaryLink}
            >
              <GithubIcon className="size-4" />
              {t.cta.github}
            </a>
          </div>
        </div>
      </section>

      <footer className="border-t">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-4 px-6 py-8 text-sm text-fd-muted-foreground">
          <p>
            {t.footer.copyright} · {t.footer.design}{" "}
            <a
              href={site.nsh}
              target="_blank"
              rel="noreferrer"
              className="hover:text-fd-foreground"
            >
              Native Shell
            </a>
          </p>
          <a
            href={`${site.blob}ARCHITECTURE.md`}
            target="_blank"
            rel="noreferrer"
            className="hover:text-fd-foreground"
          >
            {t.footer.reference}
          </a>
        </div>
      </footer>
    </HomeLayout>
  )
}
