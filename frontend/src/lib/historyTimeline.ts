import type { FileVersion } from './api'

export interface HistoryLane {
  id: string
  label: string
  branch: boolean
  sourceVersionID: string
  versions: FileVersion[]
}

export function buildHistoryLanes(versions: readonly FileVersion[]): HistoryLane[] {
  const ordered = [...versions].sort((a, b) => a.version_number - b.version_number || a.version_id.localeCompare(b.version_id))
  const lanes: HistoryLane[] = [{ id: 'main', label: 'Main line', branch: false, sourceVersionID: '', versions: [] }]
  let currentLane = lanes[0]
  for (const version of ordered) {
    if (version.change_kind === 'reversion' && version.restores_version_id) {
      const source = versions.find(item => item.version_id === version.restores_version_id)
      if (source) {
        currentLane = {
          id: `branch-${version.version_id}`,
          label: `Branch ${lanes.length} from V${source.version_number}`,
          branch: true,
          sourceVersionID: source.version_id,
          versions: [],
        }
        lanes.push(currentLane)
      }
    }
    currentLane.versions.push(version)
  }

  return lanes
}
