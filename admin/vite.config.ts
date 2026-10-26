import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import tailwindcss from '@tailwindcss/vite';

// `bun run dev` proxies the API to a server on :8080 (run it with DIALCATZ_DATA_DIR=./tmp go run ./cmd/dialcatz-den).
export default defineConfig({
  plugins: [vue(), tailwindcss()],
  build: { outDir: '../internal/web/dist', emptyOutDir: true },
  server: { proxy: { '/api': 'http://localhost:8080' } },
});
