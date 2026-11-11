import { mkdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import type { Plugin } from 'vite';
import { defineConfig } from 'vitest/config';

// The Go binary embeds this folder (internal/webui/dist).
const outDir = fileURLToPath(new URL('../internal/webui/dist', import.meta.url));

// emptyOutDir removes everything, but `go build` needs the folder to hold at
// least one file, so put the placeholder back after every build.
function keepPlaceholder(): Plugin {
  return {
    name: 'keep-placeholder',
    apply: 'build',
    closeBundle() {
      mkdirSync(outDir, { recursive: true });
      writeFileSync(path.join(outDir, '.gitkeep'), '');
    },
  };
}

export default defineConfig({
  plugins: [svelte(), keepPlaceholder()],
  build: {
    outDir,
    emptyOutDir: true,
    target: 'es2022',
    chunkSizeWarningLimit: 1500, // pdf.js is lazy-loaded and large
  },
  server: {
    port: 5173,
    // `npm run dev` talks to a Go server started with `make run`.
    // changeOrigin must stay false: the server compares the browser's Origin
    // header with the Host header, and rewriting Host to :8080 would make
    // every login and other POST look like a cross-site request.
    proxy: {
      '/api': { target: 'http://127.0.0.1:8080', changeOrigin: false },
      '/healthz': { target: 'http://127.0.0.1:8080', changeOrigin: false },
    },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts'],
  },
});
