// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ConnectionWorkspace from './ConnectionWorkspace.svelte'

afterEach(cleanup)

const diagnostic = {
  client_surface: 'desktop',
  selected_endpoint: 'http://127.0.0.1:1618',
  used_fallback: true,
  connection_info: {
    service: 'TimeKeeper',
    instance_id: 'tk_instance_0123456789abcdef0123456789abcdef',
    version: '0.0.1-alpha',
    api_version: 'v1',
    capabilities: ['projects.list', 'tasks.lifecycle'],
    friendly_urls: ['http://timekeeper.local/'],
  },
}

describe('ConnectionWorkspace', () => {
  it('runs the read-only connection test only on demand and explains local fallback selection', async () => {
    const api = { testConnection: vi.fn().mockResolvedValue(diagnostic) }
    render(ConnectionWorkspace, { props: { api } })

    expect(api.testConnection).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Test connection' })).toBeTruthy()
    expect(screen.getByText(/read-only/i)).toBeTruthy()

    await fireEvent.click(screen.getByRole('button', { name: 'Test connection' }))
    await waitFor(() => expect(api.testConnection).toHaveBeenCalledOnce())

    expect(screen.getByRole('status').textContent).toContain('loopback fallback')
    expect(screen.getByText('Selected Core endpoint')).toBeTruthy()
    expect(screen.getAllByText('http://127.0.0.1:1618')).toHaveLength(2)
    expect(screen.getByText('tk_instance_0123456789abcdef0123456789abcdef')).toBeTruthy()
    expect(screen.getByText('v1')).toBeTruthy()
    expect(screen.getByText(/projects\.list/)).toBeTruthy()
  })

  it('labels browser-facing origin and avoids claiming native proxy or fallback control', async () => {
    const browserDiagnostic = { ...diagnostic, client_surface: 'browser' as const, selected_endpoint: 'http://127.0.0.1:5173', used_fallback: false }
    const api = { testConnection: vi.fn().mockResolvedValue(browserDiagnostic) }
    render(ConnectionWorkspace, { props: { api } })

    await fireEvent.click(screen.getByRole('button', { name: 'Test connection' }))
    expect(await screen.findByText('Browser API origin')).toBeTruthy()
    expect(screen.getByText(/native proxy and DNS routing are controlled by the browser and host/i)).toBeTruthy()
  })

  it('shows a retryable error without exposing arbitrary response details', async () => {
    const api = { testConnection: vi.fn().mockRejectedValue(new Error('Core request failed (500 Internal Server Error).')) }
    render(ConnectionWorkspace, { props: { api } })

    await fireEvent.click(screen.getByRole('button', { name: 'Test connection' }))
    const alert = await screen.findByRole('alert')
    expect(alert.textContent).toContain('Core request failed (500 Internal Server Error).')
    expect(screen.getByRole('button', { name: 'Test connection' })).toBeTruthy()
  })
})
