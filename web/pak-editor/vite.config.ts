/// <reference types="vitest" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// Development: start the Go backend with
//   go run ./cmd/pak-editor --port 5174 --no-browser
// then `npm run dev` here. Vite proxies /api to it. Open the dev URL with
// ?t=<token printed by pak-editor> so requests carry the session token.
// Use `npm run dev:mock` (VITE_MOCK=1, see .env.mock) to work without the backend.
export default defineConfig({
  base: './',
  plugins: [react()],
  build: {
    // The Go binary embeds this folder (go:embed in cmd/pak-editor).
    outDir: '../../cmd/pak-editor/ui',
    emptyOutDir: true,
    sourcemap: false,
    chunkSizeWarningLimit: 1500,
  },
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:5174',
        // The Go server only accepts Host 127.0.0.1:<port> (DNS-rebinding guard).
        changeOrigin: true,
      },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    css: false,
  },
});
