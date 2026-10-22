import { zhCN } from "@fumadocs/language/zh-cn"
import { defineI18n } from "fumadocs-core/i18n"
import { uiTranslations } from "@fumadocs/base-ui/i18n"

// English is served without a prefix and Chinese under /zh. A page's
// translation sits beside it, as quick-start.zh.mdx next to quick-start.mdx.
export const i18n = defineI18n({
  defaultLanguage: "en",
  languages: ["en", "zh"],
  hideLocale: "default-locale",
})

export type Locale = (typeof i18n.languages)[number]

export const translations = i18n
  .translations()
  .extend(uiTranslations())
  .preset("zh", zhCN())
  .add({ en: { displayName: "English" } })

/** The locale of a route's optional :lang parameter, or undefined if unknown. */
export function localeOf(lang: string | undefined): Locale | undefined {
  if (lang === undefined) return i18n.defaultLanguage
  return (i18n.languages as string[]).includes(lang) &&
    lang !== i18n.defaultLanguage
    ? (lang as Locale)
    : undefined
}

/** Prefixes a site path with the locale's segment. */
export function localized(locale: Locale, path: string): string {
  if (locale === i18n.defaultLanguage) return path
  return path === "/" ? `/${locale}` : `/${locale}${path}`
}
