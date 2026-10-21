// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte'
import { afterEach, describe, expect, it, vi } from 'vitest'
import HistoryWorkspace from './HistoryWorkspace.svelte'
import type { ProjectSummary } from './api'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('HistoryWorkspace', () => {
  it('restores an older version as a new branch and refreshes the selected file', async () => {
    const first = { version_id: 'FV-1', file_id: 'F-1', version_number: 1, change_kind: 'file_added', content_hash: 'a', size_bytes: 1 }
    const second = { version_id: 'FV-2', file_id: 'F-1', version_number: 2, change_kind: 'feature', parent_version_id: 'FV-1', content_hash: 'b', size_bytes: 2 }
    const restored = { version_id: 'FV-3', file_id: 'F-1', version_number: 3, change_kind: 'reversion', parent_version_id: 'FV-2', restores_version_id: 'FV-1', content_hash: 'a', size_bytes: 1 }
    let currentVersionID = 'FV-2'
    const versions = [first, second]
    const api = {
      getProjectFileRoot: vi.fn().mockResolvedValue({ project_id: 'P-7', root_path: 'D:/workspace' }),
      getProjectFiles: vi.fn(async () => [{ file_id: 'F-1', project_id: 'P-7', current_path: 'src/main.go', file_type: 'go', current_version_id: currentVersionID, tracking_policy: 'project_scan' }]),
      setProjectFileRoot: vi.fn(),
      scanProjectFiles: vi.fn(),
      getFileVersions: vi.fn(async () => versions.slice()),
      getFileVersion: vi.fn(async (_fileID: string, versionID: string) => ({ version: versions.find(item => item.version_id === versionID)!, content: versionID })),
      compareFileVersions: vi.fn().mockResolvedValue({ from: first, to: second, from_content: 'old', to_content: 'new', hunks: [{ from_start: 1, from_count: 1, to_start: 1, to_count: 1, removed: ['old'], added: ['new'] }] }),
      restoreFileVersion: vi.fn(async () => {
        currentVersionID = restored.version_id
        versions.push(restored)
        return { file: { file_id: 'F-1', current_version_id: restored.version_id }, version: restored }
      }),
      hasNativeDirectoryPicker: () => false,
      chooseProjectSourceDirectory: vi.fn().mockResolvedValue(''),
    }
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    render(HistoryWorkspace, {
      props: {
        projects: [{ project_id: 'P-7', project_name: 'TimeKeeper', status: 'Open' }],
        initialProjectID: 'P-7',
        api: api as never,
      },
    })

    await fireEvent.click(await screen.findByRole('button', { name: /Open src\/main\.go/ }))
    await waitFor(() => {
      expect((screen.getByLabelText('Compare from version') as HTMLSelectElement).value).toBe('FV-1')
      expect((screen.getByLabelText('Compare to version') as HTMLSelectElement).value).toBe('FV-2')
    })
    await fireEvent.click(screen.getByRole('button', { name: 'Compare' }))
    await waitFor(() => expect(api.compareFileVersions).toHaveBeenCalledWith('F-1', 'FV-1', 'FV-2'))
    expect(screen.getByText('+ new')).not.toBeNull()
    await fireEvent.click(screen.getByRole('button', { name: /V1/ }))
    await fireEvent.click(screen.getByRole('button', { name: 'Restore selected version' }))

    await waitFor(() => expect(api.restoreFileVersion).toHaveBeenCalledWith('F-1', {
      version_id: 'FV-1', expected_current_version_id: 'FV-2',
    }))
    expect(await screen.findByRole('button', { name: /V3 · current/ })).not.toBeNull()
    expect(screen.getByRole('region', { name: 'File version history' })).not.toBeNull()
  })

  it('keeps scanning explicit, configures a Core source root, then opens saved history', async () => {
    let sourceRoot = ''
    let scanned = false
    const file = {
      file_id: 'F-1', project_id: 'P-7', current_path: 'src/main.go', file_type: 'go',
      current_version_id: 'FV-1', tracking_policy: 'project_scan',
    }
    const version = {
      version_id: 'FV-1', file_id: 'F-1', version_number: 1, change_kind: 'file_added',
      snapshot_type: 'full', size_bytes: 12, created_at: '2026-10-12T10:00:00Z',
    }
    const api = {
      getProjectFileRoot: vi.fn(async () => ({ project_id: 'P-7', root_path: sourceRoot })),
      getProjectFiles: vi.fn(async () => scanned ? [file] : []),
      setProjectFileRoot: vi.fn(async (_projectID: string, path: string) => {
        sourceRoot = path
        return { project_id: 'P-7', root_path: path }
      }),
      scanProjectFiles: vi.fn(async () => {
        scanned = true
        return { project_id: 'P-7', root_path: sourceRoot, files_discovered: 1, files_added: 1, versions_added: 1, files_skipped: 0 }
      }),
      getFileVersions: vi.fn().mockResolvedValue([version]),
      getFileVersion: vi.fn().mockResolvedValue({ version, content: 'package main\n' }),
      compareFileVersions: vi.fn(),
      restoreFileVersion: vi.fn(),
      hasNativeDirectoryPicker: () => false,
      chooseProjectSourceDirectory: vi.fn().mockResolvedValue(''),
    }
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    render(HistoryWorkspace, {
      props: {
        projects: [{ project_id: 'P-7', project_name: 'TimeKeeper', status: 'Open' }] satisfies ProjectSummary[],
        initialProjectID: 'P-7',
        api: api as never,
      },
    })

    expect(await screen.findByText(/Source folder is not configured/)).not.toBeNull()
    expect(api.scanProjectFiles).not.toHaveBeenCalled()
    expect((screen.getByRole('button', { name: 'Scan and index' }) as HTMLButtonElement).disabled).toBe(true)

    await fireEvent.input(screen.getByLabelText('Project source folder path'), { target: { value: 'D:/Dev/TimeKeeper' } })
    expect((screen.getByRole('button', { name: 'Scan and index' }) as HTMLButtonElement).disabled).toBe(true)
    await fireEvent.click(screen.getByRole('button', { name: 'Set source folder' }))
    await waitFor(() => expect(api.setProjectFileRoot).toHaveBeenCalledWith('P-7', 'D:/Dev/TimeKeeper'))
    expect((screen.getByRole('button', { name: 'Scan and index' }) as HTMLButtonElement).disabled).toBe(false)

    await fireEvent.click(screen.getByRole('button', { name: 'Scan and index' }))
    expect(window.confirm).toHaveBeenCalledOnce()
    await waitFor(() => expect(api.scanProjectFiles).toHaveBeenCalledWith('P-7'))
    expect(await screen.findByText(/Scanned 1 text files/)).not.toBeNull()
    const openFile = await screen.findByRole('button', { name: /Open src\/main\.go/ })
    await fireEvent.click(openFile)
    expect(await screen.findByText('package main')).not.toBeNull()
    expect(api.getFileVersions).toHaveBeenCalledWith('F-1')
    expect(api.getFileVersion).toHaveBeenCalledWith('F-1', 'FV-1')
  })

  it('keeps scan disabled when Core cannot read the configured root', async () => {
    const api = {
      getProjectFileRoot: vi.fn().mockRejectedValue(new Error('Core request failed (500 Internal Server Error).')),
      getProjectFiles: vi.fn().mockResolvedValue([]),
      setProjectFileRoot: vi.fn(),
      scanProjectFiles: vi.fn(),
      hasNativeDirectoryPicker: () => false,
      chooseProjectSourceDirectory: vi.fn(),
    }
    render(HistoryWorkspace, {
      props: {
        projects: [{ project_id: 'P-7', project_name: 'TimeKeeper', status: 'Open' }],
        initialProjectID: 'P-7',
        api: api as never,
      },
    })

    expect(await screen.findByText('File history is unavailable.')).not.toBeNull()
    await fireEvent.input(screen.getByLabelText('Project source folder path'), { target: { value: 'D:/Dev/TimeKeeper' } })
    expect((screen.getByRole('button', { name: 'Scan and index' }) as HTMLButtonElement).disabled).toBe(true)
    expect(api.scanProjectFiles).not.toHaveBeenCalled()
  })
})
