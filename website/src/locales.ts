import type { Lang } from 'nsgo-website/config';

export const SITE_LANGS = ['en', 'zh', 'zh-tw', 'ja', 'ko', 'es', 'fr', 'de', 'pt', 'ru', 'it', 'vi', 'id', 'tr'] as const satisfies readonly Lang[];
export type SiteLang = (typeof SITE_LANGS)[number];

export const NON_DEFAULT_LANGS = ['zh', 'zh-tw', 'ja', 'ko', 'es', 'fr', 'de', 'pt', 'ru', 'it', 'vi', 'id', 'tr'] as const satisfies readonly SiteLang[];
export type NonDefaultLang = (typeof NON_DEFAULT_LANGS)[number];

export const DATE_LOCALES: Record<SiteLang, string> = {
  en: 'en-US',
  zh: 'zh-CN',
  'zh-tw': 'zh-TW',
  ja: 'ja-JP',
  ko: 'ko-KR',
  es: 'es-ES',
  fr: 'fr-FR',
  de: 'de-DE',
  pt: 'pt-BR',
  ru: 'ru-RU',
  it: 'it-IT',
  vi: 'vi-VN',
  id: 'id-ID',
  tr: 'tr-TR',
};

const LANG_SET = new Set<string>(SITE_LANGS);

export function asSiteLang(lang: string): SiteLang {
  return LANG_SET.has(lang) ? (lang as SiteLang) : 'en';
}
