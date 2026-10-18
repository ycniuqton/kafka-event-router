import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

/** Dev-server proxy target for the Go API (`ROUTER_HTTP_ADDR`); override with VITE_PROXY_TARGET. */
const apiTarget = process.env.VITE_PROXY_TARGET ?? 'http://localhost:8080'

/** Dev-server bind address/port (deploy/dev/run.sh sets 0.0.0.0:5173); `vite build` ignores them. */
const devHost = process.env.VITE_DEV_HOST || undefined
const devPort = process.env.VITE_DEV_PORT ? Number(process.env.VITE_DEV_PORT) : undefined

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    host: devHost,
    port: devPort,
    proxy: {
      '/api': { target: apiTarget, changeOrigin: false, ws: true },
      '/auth': { target: apiTarget, changeOrigin: false },
    },
  },
})
