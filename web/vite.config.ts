import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Build output goes straight into the Go embed directory.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': { target: 'http://127.0.0.1:18443', changeOrigin: true },
      '/install': { target: 'http://127.0.0.1:18443', changeOrigin: true },
      '/download': { target: 'http://127.0.0.1:18443', changeOrigin: true },
    },
  },
})
