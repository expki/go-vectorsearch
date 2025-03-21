import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:7500',
        changeOrigin: true,
        secure: false,
      },
    },
  },
  build: {
    outDir: '../static',
    emitAssets: false,
    cssCodeSplit: false,
  },
})
