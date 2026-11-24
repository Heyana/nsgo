// The docs' pages in the sidebar's order, read from docs/public/'s front
// matter: each page's `order` places it and its `group`, in the page's
// language, names the sidebar group it is in. Pages without a group come
// before the groups. site.config.ts builds the sidebar from it and the
// llms.txt pages list the English pages by it. Runs in Node, at build time,
// from website/, which Astro and the share-cards command run in.
import { existsSync, readFileSync, readdirSync } from 'node:fs';
import path from 'node:path';
import { NON_DEFAULT_LANGS, type SiteLang } from '../locales';

export const docsDir = path.resolve('../docs/public');

export interface DocPage {
  /** The page's path below its language, without .md, such as ui/button or authoring/index. */
  file: string;
  /** Its slug on the site, as Starlight's sidebar names it: docs, docs/ui/button, docs/authoring. */
  slug: string;
  title: string;
  description?: string;
  group?: string;
  order: number;
}

export interface DocGroup {
  /** In English and each supported site language. */
  label: Partial<Record<SiteLang, string>> & { en: string };
  pages: DocPage[];
}

// Fallback translations for sidebar group labels when a locale has no translated docs folder yet.
const GROUP_LABELS: Record<string, Partial<Record<SiteLang, string>>> = {
  'Get started': {
    'zh-tw': '快速開始',
    ja: 'はじめに',
    ko: '시작하기',
    es: 'Primeros pasos',
    fr: 'Démarrer',
    de: 'Erste Schritte',
    pt: 'Primeiros passos',
    ru: 'Начало работы',
  },
  'Core concepts': {
    'zh-tw': '核心概念',
    ja: 'コアコンセプト',
    ko: '핵심 개념',
    es: 'Conceptos básicos',
    fr: 'Concepts clés',
    de: 'Kernkonzepte',
    pt: 'Conceitos principais',
    ru: 'Основные концепции',
  },
  'Writing components': {
    'zh-tw': '撰寫元件',
    ja: 'コンポーネントを書く',
    ko: '컴포넌트 작성',
    es: 'Escribir componentes',
    fr: 'Écrire des composants',
    de: 'Komponenten schreiben',
    pt: 'Escrevendo componentes',
    ru: 'Написание компонентов',
  },
  Components: {
    'zh-tw': '元件',
    ja: 'コンポーネント',
    ko: '컴포넌트',
    es: 'Componentes',
    fr: 'Composants',
    de: 'Komponenten',
    pt: 'Componentes',
    ru: 'Компоненты',
  },
  'Platform features': {
    'zh-tw': '平台功能',
    ja: 'プラットフォーム機能',
    ko: '플랫폼 기능',
    es: 'Funciones de plataforma',
    fr: 'Fonctionnalités plateforme',
    de: 'Plattformfunktionen',
    pt: 'Recursos de plataforma',
    ru: 'Возможности платформ',
  },
  Toolchain: {
    'zh-tw': '工具鏈',
    ja: 'ツールチェーン',
    ko: '툴체인',
    es: 'Herramientas',
    fr: 'Chaîne d’outils',
    de: 'Toolchain',
    pt: 'Ferramentas',
    ru: 'Инструменты',
  },
  Contributing: {
    'zh-tw': '參與貢獻',
    ja: 'コントリビュート',
    ko: '기여하기',
    es: 'Contribuir',
    fr: 'Contribuer',
    de: 'Mitwirken',
    pt: 'Contribuir',
    ru: 'Участие в разработке',
  },
};

/** The fields of a page's front matter, whose values are single lines. */
function frontMatter(source: string): Record<string, string> {
  const match = source.match(/^---\r?\n([\s\S]*?)\r?\n---/);
  if (!match) return {};
  return Object.fromEntries(
    match[1]
      .split(/\r?\n/)
      .map((line) => line.match(/^(\w+):\s*(.*)$/))
      .filter((m): m is RegExpMatchArray => m !== null)
      .map(([, key, value]) => [key, value.replace(/^(['"])(.*)\1$/, '$2')]),
  );
}

/** A language's pages, by their path below it. */
function pagesOf(lang: string): Map<string, DocPage> {
  const dir = path.join(docsDir, lang);
  const pages = new Map<string, DocPage>();
  if (!existsSync(dir)) return pages;
  for (const name of readdirSync(dir, { recursive: true, encoding: 'utf8' })) {
    if (!name.endsWith('.md')) continue;
    const file = name.split(path.sep).join('/').replace(/\.md$/, '');
    const data = frontMatter(readFileSync(path.join(dir, name), 'utf8'));
    const page = file.replace(/(^|\/)index$/, '');
    pages.set(file, {
      file,
      slug: page ? `docs/${page}` : 'docs',
      title: data.title ?? file,
      description: data.description,
      group: data.group,
      order: Number(data.order ?? 1000),
    });
  }
  return pages;
}

/** The English pages in the sidebar's order. */
export function englishPages(): DocPage[] {
  return [...pagesOf('en').values()].sort((a, b) => a.order - b.order || a.file.localeCompare(b.file));
}

/** The pages without a group, then each group in the order of its first page. */
export function docGroups(): { pages: DocPage[]; groups: DocGroup[] } {
  const translated = Object.fromEntries(NON_DEFAULT_LANGS.map((lang) => [lang, pagesOf(lang)])) as Record<SiteLang, Map<string, DocPage>>;
  const pages: DocPage[] = [];
  const groups: DocGroup[] = [];
  const names = new Map<string, Map<SiteLang, Map<string, number>>>();
  for (const page of englishPages()) {
    if (!page.group) {
      pages.push(page);
      continue;
    }
    let group = groups.find((g) => g.label.en === page.group);
    if (!group) {
      group = { label: { en: page.group, ...(GROUP_LABELS[page.group] ?? {}) }, pages: [] };
      groups.push(group);
    }
    group.pages.push(page);
    for (const lang of NON_DEFAULT_LANGS) {
      const name = translated[lang].get(page.file)?.group;
      if (name && name !== page.group) {
        let byLang = names.get(page.group);
        if (!byLang) names.set(page.group, (byLang = new Map()));
        let counts = byLang.get(lang);
        if (!counts) byLang.set(lang, (counts = new Map()));
        counts.set(name, (counts.get(name) ?? 0) + 1);
      }
    }
  }
  for (const group of groups) {
    const byLang = names.get(group.label.en);
    if (!byLang) continue;
    for (const [lang, counts] of byLang) {
      const top = [...counts.entries()].sort((a, b) => b[1] - a[1])[0]?.[0];
      if (top) group.label[lang] = top;
    }
  }
  return { pages, groups };
}
