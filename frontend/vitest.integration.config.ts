import { defineConfig, mergeConfig } from 'vitest/config'
import viteConfig from './vite.config.ts'

// Интеграционные тесты против живого бэкенда (docker compose): make db && make backend-up.
export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      include: ['src/**/*.integration.test.{ts,tsx}'],
      environment: 'jsdom',
      setupFiles: ['src/test/integration/setup.ts'],
      globalSetup: ['src/test/integration/global-setup.ts'],
      // Один бэкенд и общие лимиты логина — файлы по очереди.
      fileParallelism: false,
      testTimeout: 15_000,
    },
  }),
)
