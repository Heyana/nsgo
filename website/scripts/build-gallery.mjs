// Builds the gallery for the web, one group at a time, and puts them in
// public/gallery/: the shell (index.html) lists the groups and loads one's
// app.html on demand, so a first visit fetches the shell and one group
// instead of the whole gallery. Each group's demo list lives in
// examples/gallery/demos_<group>.go behind a build tag, so the linker
// leaves the other groups' demos out.
//
// public/gallery/ is generated: do not edit it.
import { readFileSync, rmSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { buildWeb, website } from './web-build.mjs';

const galleryDir = path.join(website, 'public', 'gallery');

// The groups, in the gallery's order, named as its index names them.
export const groups = [
  { id: 'apps', title: 'Apps' },
  { id: 'actions', title: 'Actions' },
  { id: 'inputs', title: 'Inputs' },
  { id: 'display', title: 'Display' },
  { id: 'data', title: 'Data' },
  { id: 'containers', title: 'Containers' },
  { id: 'feedback', title: 'Feedback' },
  { id: 'popups', title: 'Popups' },
  { id: 'navigation', title: 'Navigation' },
  { id: 'documents', title: 'Documents' },
  { id: 'primitives', title: 'Primitives' },
  { id: 'styling', title: 'Styling' },
  { id: 'interaction', title: 'Interaction' },
  { id: 'platform', title: 'Platform' },
];

rmSync(galleryDir, { recursive: true, force: true });

const index = { groups: [] };
let icon = '';
for (const group of groups) {
  console.log(`Building the gallery's ${group.title}...`);
  const app = buildWeb({
    app: 'examples/gallery',
    name: group.title,
    output: path.join(galleryDir, group.id),
    tags: ['gallery_group', `gallery_${group.id}`],
  });
  icon ||=
    readFileSync(path.join(galleryDir, group.id, 'index.html'), 'utf8').match(
      /<link rel="icon" href="([^"]+)">/,
    )?.[1] ?? '';
  writeFileSync(
    path.join(galleryDir, group.id, 'app.html'),
    readFileSync(path.join(website, 'scripts', 'gallery-loader.html'), 'utf8')
      .replaceAll('__EXEC__', app.exec)
      .replaceAll('__GZIP__', app.gzip)
      .replaceAll('__RESOURCES__', JSON.stringify(app.resources ?? {}))
      .replaceAll('__GZIP_SIZE__', String(app.gzipSize))
      .replaceAll('__ICON__', icon),
  );
  rmSync(path.join(galleryDir, group.id, 'index.html'));
  index.groups.push({ id: group.id, title: group.title, gzip: app.gzip, gzipSize: app.gzipSize });
}

writeFileSync(path.join(galleryDir, 'index.json'), `${JSON.stringify(index, null, 2)}\n`);
const frame = readFileSync(path.join(website, 'scripts', 'gallery-frame.html'), 'utf8').replaceAll('__ICON__', icon);
writeFileSync(path.join(galleryDir, 'index.html'), frame);
// Old links to the full bundle open the shell, which loads a group.
writeFileSync(
  path.join(galleryDir, 'app.html'),
  '<!doctype html><meta charset="utf-8"><script>location.replace("./" + location.search + location.hash)</script>\n',
);
writeFileSync(
  path.join(galleryDir, 'loader.html'),
  '<!doctype html><meta charset="utf-8"><link rel="canonical" href="./">' +
    '<script>location.replace("./" + location.search + location.hash)</script>\n',
);
