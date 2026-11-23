import { getEntry, type CollectionEntry } from 'astro:content';
import { fromMarkdown } from 'mdast-util-from-markdown';
import { docGroups, englishPages, type DocPage } from './docs';

// The English docs as Markdown for language models: /llms.txt lists the
// pages, /llms-full.txt holds them all and /docs/<id>.md holds one. Links
// are absolute, to other pages' Markdown and to the repository on GitHub,
// and code blocks lose the site's marks (`preview=`, `program`).

const site = 'https://nsgo.dev';
const repo = 'https://github.com/nsgo-dev/nsgo';
const links = {
  roadmap: `${repo}/blob/main/docs/roadmap.md`,
  releases: `${repo}/releases`,
  architecture: `${repo}/blob/main/ARCHITECTURE.md`,
};

/** A page with its entry of the docs collection. */
export interface Doc {
  page: DocPage;
  entry: CollectionEntry<'docs'>;
}

/** The Markdown copy of a page by its file below en/, such as https://nsgo.dev/docs/testing.md. */
export function markdownUrl(file: string): string {
  return `${site}/docs/${file === 'index' ? file : file.replace(/\/index$/, '')}.md`;
}

/** The English docs in the sidebar's order. */
export async function llmsDocs(): Promise<Doc[]> {
  return Promise.all(
    englishPages().map(async (page) => {
      const entry = await getEntry('docs', page.slug);
      if (!entry) throw new Error(`No docs entry for ${page.slug}`);
      return { page, entry };
    }),
  );
}

/** A page as Markdown: its title, description and body. */
export function pageMarkdown(doc: Doc): string {
  const { title, description } = doc.entry.data;
  const head = [`# ${title}`];
  if (description) head.push(`> ${description}`);
  return `${head.join('\n\n')}\n\n${rewrite(doc.entry.body ?? '', doc.entry.filePath!.replace(/^.*docs\/public\//, '')).trim()}\n`;
}

/** /llms.txt: what NSGo is, then every page by group. */
export function llmsIndex(): string {
  const { pages, groups } = docGroups();
  const sections = [{ title: 'Overview', pages }, ...groups.map((group) => ({ title: group.label.en, pages: group.pages }))].map((group) => {
    const items = group.pages.map(({ file, title, description }) => `- [${title}](${markdownUrl(file)})${description ? `: ${description}` : ''}`);
    return `## ${group.title}\n\n${items.join('\n')}`;
  });
  return [
    '# NSGo',
    '> A cross-platform native UI engine with a Go backend: app logic, the reactive graph and layout run in Go, and each platform draws with its own native views, on iOS, Android, macOS, Windows, Linux, the web, tvOS, watchOS, HarmonyOS as a preview and mini programs.',
    [
      `NSGo is early, with no stable API: use the latest version (\`go get github.com/nsgo-dev/nsgo@latest\`) and read its source, \`go list -m -f '{{.Dir}}' github.com/nsgo-dev/nsgo\`, before relying on a signature. Each page below is Markdown; ${site}/llms-full.txt holds them all.`,
      `Coding agents can install NSGo's skill, which keeps an app on NSGo's conventions: \`bunx skills add nsgo-dev/nsgo\`.`,
    ].join('\n\n'),
    ...sections,
    [
      '## Optional',
      '',
      `- [Live demo](${site}/gallery/): every component, running in the browser`,
      `- [Roadmap](${links.roadmap}): what is not implemented yet, by target`,
      `- [Releases](${links.releases})`,
      `- [Architecture](${links.architecture}): how the engine works, for contributors`,
    ].join('\n'),
  ].join('\n\n') + '\n';
}

/** /llms-full.txt: every page, each after its URL. */
export function llmsFull(docs: Doc[]): string {
  const pages = docs.map((doc) => `---\n\nSource: ${markdownUrl(doc.page.file)}\n\n${pageMarkdown(doc).trimEnd()}`);
  return `# NSGo documentation\n\n${pages.join('\n\n')}\n`;
}

/** rewrite makes the links of file, such as en/authoring/index.md, absolute and drops its code blocks' marks. */
function rewrite(source: string, file: string): string {
  const edits: { start: number; end: number; text: string }[] = [];
  const visit = (node: any) => {
    const start = node.position?.start.offset;
    const end = node.position?.end.offset;
    if (node.type === 'code' && start !== undefined && node.meta) {
      // Keep the fence and the language, as GitHub shows them.
      const line = source.slice(start, source.indexOf('\n', start));
      const fence = line.match(/^\s*(`{3,}|~{3,})\S*/);
      if (fence) edits.push({ start, end: start + line.length, text: fence[0] });
      return;
    }
    if ((node.type === 'link' || node.type === 'image' || node.type === 'definition') && start !== undefined) {
      const url = absolute(node.url, file);
      if (url !== node.url) {
        const text = source.slice(start, end);
        const at = text.lastIndexOf(node.url);
        if (at >= 0) edits.push({ start: start + at, end: start + at + node.url.length, text: url });
      }
    }
    node.children?.forEach(visit);
  };
  visit(fromMarkdown(source));
  let out = source;
  for (const edit of edits.sort((a, b) => b.start - a.start)) {
    out = out.slice(0, edit.start) + edit.text + out.slice(edit.end);
  }
  return out;
}

/** absolute turns a link in file, below docs/public/, into a URL. */
function absolute(href: string, from: string): string {
  if (/^[a-z]+:|^#|^\/\//i.test(href)) return href;
  if (href.startsWith('/')) return `${site}${href}`;
  const [target, hash] = href.split('#');
  const file = resolve(`docs/public/${from}`, target);
  const anchor = hash ? `#${hash}` : '';
  if (file.startsWith('docs/public/en/') && file.endsWith('.md')) {
    return `${markdownUrl(file.slice('docs/public/en/'.length, -'.md'.length))}${anchor}`;
  }
  return `${repo}/blob/main/${file}${anchor}`;
}

/** resolve joins a relative path to the directory of the file at from. */
function resolve(from: string, relative: string): string {
  const parts = from.split('/').slice(0, -1);
  for (const part of relative.split('/')) {
    if (part === '..') parts.pop();
    else if (part !== '.' && part !== '') parts.push(part);
  }
  return parts.join('/');
}
