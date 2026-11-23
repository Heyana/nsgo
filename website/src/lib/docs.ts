// The docs' pages in the sidebar's order, read from docs/public/'s front
// matter: each page's `order` places it and its `group`, in the page's
// language, names the sidebar group it is in. Pages without a group come
// before the groups. site.config.ts builds the sidebar from it and the
// llms.txt pages list the English pages by it. Runs in Node, at build time,
// from website/, which Astro and the share-cards command run in.
import { readFileSync, readdirSync } from 'node:fs';
import path from 'node:path';

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
  /** In English and, where a page of the group has it, in Chinese. */
  label: { en: string; zh?: string };
  pages: DocPage[];
}

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
  const zh = pagesOf('zh');
  const pages: DocPage[] = [];
  const groups: DocGroup[] = [];
  // Each group's Chinese names by how many of its pages use them, as a
  // page may keep the English one.
  const names = new Map<string, Map<string, number>>();
  for (const page of englishPages()) {
    if (!page.group) {
      pages.push(page);
      continue;
    }
    let group = groups.find((g) => g.label.en === page.group);
    if (!group) groups.push((group = { label: { en: page.group }, pages: [] }));
    group.pages.push(page);
    const name = zh.get(page.file)?.group;
    if (name && name !== page.group) {
      const counts = names.get(page.group) ?? new Map<string, number>();
      counts.set(name, (counts.get(name) ?? 0) + 1);
      names.set(page.group, counts);
    }
  }
  for (const group of groups) {
    const counts = [...(names.get(group.label.en) ?? new Map<string, number>()).entries()];
    if (counts.length) group.label.zh = counts.sort((a, b) => b[1] - a[1])[0][0];
  }
  return { pages, groups };
}
