import { describe, expect, it } from 'vitest'
import { buildHistoryLanes } from './historyTimeline'
import type { FileVersion } from './api'

const version = (version_id: string, version_number: number, extras: Partial<FileVersion> = {}): FileVersion => ({
  version_id,
  file_id: 'F-1',
  version_number,
  change_kind: 'code_change',
  ...extras,
})

describe('branch-aware file history timeline', () => {
  it('keeps chronological history on the main lane and starts a branch at each reversion', () => {
    const versions = [
      version('FV-1', 1, { change_kind: 'file_added' }),
      version('FV-2', 2, { parent_version_id: 'FV-1', change_kind: 'feature' }),
      version('FV-3', 3, { parent_version_id: 'FV-2', restores_version_id: 'FV-1', change_kind: 'reversion' }),
      version('FV-4', 4, { parent_version_id: 'FV-3', change_kind: 'bugfix' }),
    ]

    expect(buildHistoryLanes(versions)).toEqual([
      { id: 'main', label: 'Main line', branch: false, sourceVersionID: '', versions: versions.slice(0, 2) },
      { id: 'branch-FV-3', label: 'Branch 1 from V1', branch: true, sourceVersionID: 'FV-1', versions: versions.slice(2) },
    ])
  })

  it('does not create a branch for a reversion whose target is missing', () => {
    const versions = [version('FV-1', 1), version('FV-2', 2, { change_kind: 'reversion', restores_version_id: 'FV-missing' })]
    expect(buildHistoryLanes(versions)).toHaveLength(1)
    expect(buildHistoryLanes(versions)[0].versions).toHaveLength(2)
  })
})
