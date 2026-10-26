import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// The site is a single static page. Relative URLs let it live under a
// repository path on GitHub Pages or at a domain root without a rebuild.
export default defineConfig({
  base: './',
  plugins: [react()],
  build: {
    target: 'es2020',
    // The gallery's wasm and its gzip copy are generated into public/ by
    // scripts/build-gallery.mjs; Vite only copies them.
    chunkSizeWarningLimit: 700,
  },
});
