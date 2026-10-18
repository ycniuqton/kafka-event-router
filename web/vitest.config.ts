import { configDefaults, defineConfig, mergeConfig } from 'vitest/config'
import viteConfig from './vite.config'

/** Unit tests reuse the vite config; Playwright specs under e2e/ run with `cd e2e && npx playwright test`. */
export default mergeConfig(viteConfig, defineConfig({
  test: { exclude: [...configDefaults.exclude, 'e2e/**'] },
}))
