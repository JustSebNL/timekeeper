import { resolve } from 'node:path'
import { expect, it } from 'vitest'
import { loadConfigFromFile } from 'vite'

it('allows the explicit Vite development hostname', async () => {
  const loaded = await loadConfigFromFile(
    { command: 'serve', mode: 'development' },
    resolve(process.cwd(), 'vite.config.ts'),
  )

  expect(loaded?.config.server?.allowedHosts).toContain('vite.timekeeper.local')
})
