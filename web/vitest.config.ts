import { defineConfig, mergeConfig } from 'vitest/config'
import viteConfig from './vite.config'

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      environment: 'jsdom',
      include: ['src/**/*.test.ts'],
      coverage: {
        provider: 'v8',
        include: ['src/lib/**', 'src/themes/**', 'src/i18n/**'],
        exclude: ['src/lib/**/*.test.ts', 'src/lib/types.ts', 'src/themes/**/*.test.ts', 'src/i18n/**/*.test.ts'],
        reporter: ['text', 'html'],
        thresholds: { lines: 85, functions: 85, branches: 85, statements: 85 },
      },
    },
  }),
)
