// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/svelte'
import RegistrationQueueWorkspace from './RegistrationQueueWorkspace.svelte'

const counts = { tasks: 1, subtasks: 0, sprints: 1, total_objects: 2 }
const registered = {
  queue_id: 'RQ-1',
  project_id: 'P-1',
  kind: 'execution_batch',
  status: 'Registered',
  payload_hash: 'a'.repeat(64),
  counts,
  attempts: 1,
  created_at: '2026-10-14T10:00:00Z',
  updated_at: '2026-10-14T10:00:01Z',
  processed_at: '2026-10-14T10:00:01Z',
  payload: { version: 'timekeeper-execution-batch/v1', tasks: [{ category_id: 'C-1', name: 'Batch task' }] },
  result: {
    tasks: [{ task_id: 'T-10001', project_id: 'P-1', name: 'Batch task', status: 'Open' }],
    subtasks: [],
    sprints: [{ sprint_id: 'SP-10002', project_id: 'P-1', name: 'Batch sprint', status: 'Open' }],
    counts,
  },
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('RegistrationQueueWorkspace', () => {
  it('shows stacked API offers and reads created Core entities through the ordinary queue detail', async () => {
    const api = {
      getRegistrationQueue: vi.fn().mockResolvedValue({ items: [registered], limit: 50, offset: 0, has_more: false, next_offset: null }),
      getRegistrationQueueItem: vi.fn().mockResolvedValue(registered),
    }
    render(RegistrationQueueWorkspace, {
      props: {
        projects: [{ project_id: 'P-1', project_name: 'Queue project', status: 'Open' }],
        initialProjectID: 'P-1',
        api,
      },
    })

    expect(await screen.findByText('RQ-1')).toBeTruthy()
    await fireEvent.click(screen.getByRole('button', { name: /RQ-1/ }))
    expect(await screen.findByText(/Task · Batch task/)).toBeTruthy()
    expect(await screen.findByText(/T-10001/)).toBeTruthy()
    expect(screen.getByText(/SP-10002/)).toBeTruthy()
    expect(screen.getByText(/Registration completed/)).toBeTruthy()
    expect(api.getRegistrationQueueItem).toHaveBeenCalledWith('P-1', 'RQ-1')
  })

  it('renders sparse registration results when absent entity arrays decode as null', async () => {
    const sparse = {
      ...registered,
      queue_id: 'RQ-2',
      result: {
        tasks: null,
        subtasks: null,
        sprints: [{ sprint_id: 'SP-10003', project_id: 'P-1', name: 'Only sprint', status: 'Open' }],
        counts,
      },
    }
    const api = {
      getRegistrationQueue: vi.fn().mockResolvedValue({ items: [sparse], limit: 50, offset: 0, has_more: false, next_offset: null }),
      getRegistrationQueueItem: vi.fn().mockResolvedValue(sparse),
    }
    render(RegistrationQueueWorkspace, {
      props: {
        projects: [{ project_id: 'P-1', project_name: 'Queue project', status: 'Open' }],
        initialProjectID: 'P-1',
        api,
      },
    })
    await fireEvent.click(await screen.findByRole('button', { name: /RQ-2/ }))
    expect(await screen.findByText(/SP-10003/)).toBeTruthy()
  })

  it('loads older Core offers in FIFO pages instead of hiding everything after the first page', async () => {
    const older = { ...registered, queue_id: 'RQ-older', created_at: '2026-10-13T10:00:00Z' }
    const api = {
      getRegistrationQueue: vi.fn()
        .mockResolvedValueOnce({ items: [registered], limit: 1, offset: 0, has_more: true, next_offset: 1 })
        .mockResolvedValueOnce({ items: [older], limit: 1, offset: 1, has_more: false, next_offset: null }),
      getRegistrationQueueItem: vi.fn().mockResolvedValue(registered),
    }
    render(RegistrationQueueWorkspace, {
      props: {
        projects: [{ project_id: 'P-1', project_name: 'Queue project', status: 'Open' }],
        initialProjectID: 'P-1',
        api,
      },
    })

    expect(await screen.findByText('RQ-1')).toBeTruthy()
    await fireEvent.click(screen.getByRole('button', { name: 'Load older offers' }))
    expect(await screen.findByText('RQ-older')).toBeTruthy()
    expect(api.getRegistrationQueue).toHaveBeenNthCalledWith(2, 'P-1', '', 50, 1)
  })
})
