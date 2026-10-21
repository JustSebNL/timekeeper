// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from '@testing-library/svelte'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { KnowledgeProviderStatus, KnowledgeResult } from './api'
import MemoryWorkspace from './MemoryWorkspace.svelte'

afterEach(cleanup)

const status: KnowledgeProviderStatus = {
  available: true,
  provider: 'obsidian',
  protocol_version: 'timekeeper-knowledge/v1',
  capabilities: ['knowledge.search'],
  last_checked_at: '2026-10-18T19:00:00Z',
}
const results: KnowledgeResult[] = [{
  id: 'MG-1',
  title: 'Durable context',
  type: 'pattern',
  summary: 'Store useful context across sessions.',
  topics: ['memory'],
  tags: ['core'],
  review_status: 'verified',
  visibility: 'private',
}]

describe('MemoryWorkspace', () => {
  it('shows a safe unavailable state without searching when Core cannot reach the provider', async () => {
    const api = {
      getKnowledgeStatus: vi.fn().mockRejectedValue(new Error('Core request failed (503 Service Unavailable).')),
      searchKnowledge: vi.fn().mockResolvedValue([]),
    }
    render(MemoryWorkspace, { props: { api } })

    expect((await screen.findByRole('alert')).textContent).toContain('Memory Galaxy is unavailable')
    expect(screen.queryByLabelText('Search Memory Galaxy')).toBeNull()
    expect(api.searchKnowledge).not.toHaveBeenCalled()
  })

  it('searches only after Core reports the capability and renders bounded result metadata', async () => {
    const api = {
      getKnowledgeStatus: vi.fn().mockResolvedValue(status),
      searchKnowledge: vi.fn().mockResolvedValue(results),
    }
    render(MemoryWorkspace, { props: { api } })

    const input = await screen.findByLabelText('Search Memory Galaxy')
    await fireEvent.input(input, { target: { value: '  durable context  ' } })
    await fireEvent.click(screen.getByRole('button', { name: 'Search knowledge' }))

    expect(await screen.findByText('Durable context')).not.toBeNull()
    expect(screen.getByText('Store useful context across sessions.')).not.toBeNull()
    expect(screen.getByText('verified')).not.toBeNull()
    expect(api.searchKnowledge).toHaveBeenCalledWith('durable context', 8)
  })

  it('clears the searching state when a provider refresh invalidates an in-flight search', async () => {
    let resolveSearch!: (value: KnowledgeResult[]) => void
    const pendingSearch = new Promise<KnowledgeResult[]>((resolve) => { resolveSearch = resolve })
    const api = {
      getKnowledgeStatus: vi.fn().mockResolvedValue(status),
      searchKnowledge: vi.fn().mockReturnValueOnce(pendingSearch).mockResolvedValue(results),
    }
    render(MemoryWorkspace, { props: { api } })

    await fireEvent.input(await screen.findByLabelText('Search Memory Galaxy'), { target: { value: 'durable' } })
    await fireEvent.click(screen.getByRole('button', { name: 'Search knowledge' }))
    expect(await screen.findByRole('button', { name: 'Searching…' })).not.toBeNull()

    await fireEvent.click(screen.getByRole('button', { name: 'Refresh Memory Galaxy status' }))
    await screen.findByLabelText('Search Memory Galaxy')
    resolveSearch(results)

    expect(await screen.findByRole('button', { name: 'Search knowledge' })).not.toBeNull()
    expect(api.searchKnowledge).toHaveBeenCalledTimes(1)
  })
})
