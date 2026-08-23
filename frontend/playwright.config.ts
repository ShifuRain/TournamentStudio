import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  webServer: {
    command: 'sh e2e/run-server.sh',
    url: 'http://localhost:8080/healthz',
    reuseExistingServer: false,
    timeout: 30_000,
  },
  use: {
    baseURL: 'http://localhost:8080',
  },
})
