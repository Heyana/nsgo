import { existsSync } from 'node:fs';
import { defineConfig } from 'astro/config';
import { satteri } from '@astrojs/markdown-satteri';
import nsgoWebsite from 'nsgo-website';
import site from './site.config.ts';
import { docLinks } from './src/lib/doc-links.mjs';
import { livePreviews } from './src/lib/live.mjs';

const base = '/';

// GitHub Pages answers a folder of public/ with its index.html, as at
// /gallery/; the dev server does not, so it is taught to.
const publicIndexes = {
  name: 'public-indexes',
  configureServer(server) {
    const publicDir = server.config.publicDir;
    server.middlewares.use((req, res, next) => {
      const url = new URL(req.url ?? '/', 'http://localhost');
      const dir = url.pathname.replace(/\/$/, '');
      if (dir && existsSync(`${publicDir}${decodeURIComponent(dir)}/index.html`)) {
        if (!url.pathname.endsWith('/')) {
          res.writeHead(301, { Location: `${dir}/${url.search}` }).end();
          return;
        }
        req.url = `${dir}/index.html${url.search}`;
      }
      next();
    });
  },
};

// The site is deployed at the root of nsgo.dev on GitHub Pages and built by
// nsgo-website from site.config.ts: English at the base and Chinese under
// zh/. The docs are the repository's docs/public/, whose relative .md links
// become the site's and whose marked code blocks run above themselves.
export default defineConfig({
  site: 'https://nsgo.dev',
  base,
  vite: { plugins: [publicIndexes] },
  markdown: {
    processor: satteri({ mdastPlugins: [docLinks({ base }), livePreviews({ base })] }),
  },
  integrations: [nsgoWebsite(site)],
});
