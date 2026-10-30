import { configDefaults, defineConfig, mergeConfig } from 'vitest/config'
import viteConfig from './vite.config'

/**
 * Unit tests reuse the vite config; Playwright specs under e2e/ run with `cd e2e && npx playwright test`.
 * fsModuleCache is off: it keys transforms on file content only, so eager `import.meta.glob` module discovery
 * (HttpServicesFactory) went stale when new `api/http/<domain>/module.ts` files were added.
 */
export default mergeConfig(viteConfig, defineConfig({
  test: { exclude: [...configDefaults.exclude, 'e2e/**'], fsModuleCache: false },
}))
