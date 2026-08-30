import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  server: {
    port: 5174,
    proxy: {
      '/api': 'http://127.0.0.1:16789',
      '/uploads': 'http://127.0.0.1:16789',
    },
  },
})
