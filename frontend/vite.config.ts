import { svelte } from '@sveltejs/vite-plugin-svelte'
import { svelteTesting } from '@testing-library/svelte/vite'
import { defineConfig, loadEnv } from 'vite'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const coreOrigin = env.TIMEKEEPER_CORE_ORIGIN || 'http://timekeeper.local'

  return {
    plugins: [svelte(), svelteTesting()],
    base: './',
    build: {
      outDir: '../cmd/desktop/assets',
      emptyOutDir: true,
    },
    server: {
      allowedHosts: ['vite.timekeeper.local'],
      proxy: {
        '/health': coreOrigin,
        '/api': coreOrigin,
      },
    },
  }
})
