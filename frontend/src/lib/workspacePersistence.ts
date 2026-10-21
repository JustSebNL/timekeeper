import { resolveWorkspaceSection, type WorkspaceSection } from './navigation'

export const workspaceStorageKey = 'timekeeper.desktop.activeWorkspace'
export const selectedProjectStorageKey = 'timekeeper.desktop.selectedProject'

export interface WorkspaceStorage {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
  removeItem?(key: string): void
}

function browserStorage(override?: WorkspaceStorage): WorkspaceStorage | undefined {
  if (override) return override
  try {
    return typeof window === 'undefined' ? undefined : window.localStorage
  } catch {
    return undefined
  }
}

export function readWorkspaceSection(storage?: WorkspaceStorage): WorkspaceSection {
  const source = browserStorage(storage)
  if (!source) return 'overview'
  try {
    return resolveWorkspaceSection(source.getItem(workspaceStorageKey) ?? '')
  } catch {
    return 'overview'
  }
}

export function saveWorkspaceSection(section: string, storage?: WorkspaceStorage): void {
  const resolved = resolveWorkspaceSection(section)
  if (resolved !== section) return
  const destination = browserStorage(storage)
  if (!destination) return
  try {
    destination.setItem(workspaceStorageKey, resolved)
  } catch {
    // Workspace restoration is best-effort; a blocked storage backend must not break navigation.
  }
}

export function readSelectedProjectID(storage?: WorkspaceStorage): string | null {
  const source = browserStorage(storage)
  if (!source) return null
  try {
    const projectID = source.getItem(selectedProjectStorageKey)?.trim()
    return projectID || null
  } catch {
    return null
  }
}

export function saveSelectedProjectID(projectID: string | null, storage?: WorkspaceStorage): void {
  const destination = browserStorage(storage)
  if (!destination) return
  try {
    const normalized = projectID?.trim() || ''
    if (normalized) destination.setItem(selectedProjectStorageKey, normalized)
    else if (destination.removeItem) destination.removeItem(selectedProjectStorageKey)
    else destination.setItem(selectedProjectStorageKey, '')
  } catch {
    // A missing or blocked browser store must not block Project navigation.
  }
}
