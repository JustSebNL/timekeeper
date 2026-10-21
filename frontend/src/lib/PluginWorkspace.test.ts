// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from '@testing-library/svelte'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { PluginCatalog } from './api'
import PluginWorkspace from './PluginWorkspace.svelte'

afterEach(cleanup)

const sampleCatalog: PluginCatalog = {
  schema_version: 'timekeeper-plugin-index/v1',
  generated_at: '2026-10-18T20:00:00Z',
  plugins: [
    {
      id: 'extension.example',
      version: '1.2.0',
      name: 'Example Extension',
      description: 'A sample extension running in WASI.',
      runtime: 'wasi',
      entrypoint: 'main.wasm',
      protocol: 'timekeeper-plugin/v1',
      capabilities: ['knowledge.search', 'files.read'],
      requires: ['storage.local'],
    },
    {
      id: 'obsidian.bridge',
      version: '0.9.1',
      name: 'Obsidian Bridge',
      description: 'Bridge plugin running as subprocess.',
      runtime: 'process',
      entrypoint: './bin/obsidian',
      protocol: 'timekeeper-knowledge/v1',
      capabilities: ['knowledge.export'],
    },
  ],
}

describe('PluginWorkspace', () => {
  it('shows an alert when Core cannot load the plugin catalog', async () => {
    const api = {
      getPluginCatalog: vi.fn().mockRejectedValue(new Error('Core request failed (503 Service Unavailable).')),
    }
    render(PluginWorkspace, { props: { api } })

    expect((await screen.findByRole('alert')).textContent).toContain('Plugin catalog is unavailable')
    expect(screen.getByText('Core could not load the plugin catalog.')).not.toBeNull()
    expect(api.getPluginCatalog).toHaveBeenCalledOnce()
  })

  it('renders registered plugins with metadata and badges', async () => {
    const api = {
      getPluginCatalog: vi.fn().mockResolvedValue(sampleCatalog),
    }
    render(PluginWorkspace, { props: { api } })

    expect(await screen.findByText('Example Extension')).not.toBeNull()
    expect(screen.getByText('v1.2.0')).not.toBeNull()
    expect(screen.getByText('extension.example')).not.toBeNull()
    expect(screen.getAllByText('wasi').length).toBeGreaterThan(0)
    expect(screen.getByText('knowledge.search')).not.toBeNull()
    expect(screen.getByText('Obsidian Bridge')).not.toBeNull()
    expect(screen.getAllByText('process').length).toBeGreaterThan(0)
  })

  it('filters plugins by text query', async () => {
    const api = {
      getPluginCatalog: vi.fn().mockResolvedValue(sampleCatalog),
    }
    render(PluginWorkspace, { props: { api } })

    await screen.findByText('Example Extension')
    const filterInput = screen.getByLabelText(/Filter plugins/)
    await fireEvent.input(filterInput, { target: { value: 'Obsidian' } })

    expect(screen.getByText('Obsidian Bridge')).not.toBeNull()
    expect(screen.queryByText('Example Extension')).toBeNull()
  })

  it('filters plugins by runtime when runtime filter buttons are clicked', async () => {
    const api = {
      getPluginCatalog: vi.fn().mockResolvedValue(sampleCatalog),
    }
    render(PluginWorkspace, { props: { api } })

    await screen.findByText('Example Extension')
    await fireEvent.click(screen.getByRole('button', { name: 'process' }))

    expect(screen.getByText('Obsidian Bridge')).not.toBeNull()
    expect(screen.queryByText('Example Extension')).toBeNull()

    await fireEvent.click(screen.getByRole('button', { name: 'All' }))
    expect(screen.getByText('Example Extension')).not.toBeNull()
    expect(screen.getByText('Obsidian Bridge')).not.toBeNull()
  })

  it('shows empty state when no plugins are returned', async () => {
    const api = {
      getPluginCatalog: vi.fn().mockResolvedValue({
        schema_version: 'timekeeper-plugin-index/v1',
        plugins: [],
      }),
    }
    render(PluginWorkspace, { props: { api } })

    expect(await screen.findByText('No plugins configured')).not.toBeNull()
    expect(screen.getByText(/No extensions or plugins are currently registered/)).not.toBeNull()
  })
})
