import { describe, expect, it } from 'vitest'
import { readSelectedProjectID, readWorkspaceSection, saveSelectedProjectID, saveWorkspaceSection, selectedProjectStorageKey, workspaceStorageKey } from './workspacePersistence'

function memoryStorage(initial: Record<string, string> = {}) {
  const values = new Map(Object.entries(initial))
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => { values.set(key, value) },
    removeItem: (key: string) => { values.delete(key) },
    value: (key: string) => values.get(key),
  }
}

describe('desktop workspace restoration', () => {
  it('restores the last supported workspace across app restarts', () => {
    const storage = memoryStorage({ [workspaceStorageKey]: 'projects' })
    expect(readWorkspaceSection(storage)).toBe('projects')
    expect(readWorkspaceSection(memoryStorage({ [workspaceStorageKey]: 'settings' }))).toBe('settings')
  })

  it('falls back to Overview for absent, unknown, or not-yet-enabled workspaces', () => {
    expect(readWorkspaceSection(memoryStorage())).toBe('overview')
    expect(readWorkspaceSection(memoryStorage({ [workspaceStorageKey]: 'tasks' }))).toBe('overview')
    expect(readWorkspaceSection(memoryStorage({ [workspaceStorageKey]: 'made-up' }))).toBe('overview')
  })

  it('persists only enabled workspace selections', () => {
    const storage = memoryStorage()
    saveWorkspaceSection('projects', storage)
    expect(storage.value(workspaceStorageKey)).toBe('projects')
    saveWorkspaceSection('tasks' as 'projects', storage)
    expect(storage.value(workspaceStorageKey)).toBe('projects')
  })

  it('uses browser localStorage by default when available', () => {
    const storage = memoryStorage()
    const previousWindow = globalThis.window
    Object.defineProperty(globalThis, 'window', {
      configurable: true,
      value: { localStorage: storage },
    })

    try {
      saveWorkspaceSection('projects')
      expect(storage.value(workspaceStorageKey)).toBe('projects')
      expect(readWorkspaceSection()).toBe('projects')
    } finally {
      Object.defineProperty(globalThis, 'window', {
        configurable: true,
        value: previousWindow,
      })
    }
  })

  it('restores and clears the last selected Project independently of the active section', () => {
    const storage = memoryStorage()
    saveSelectedProjectID('P-10053', storage)
    expect(readSelectedProjectID(storage)).toBe('P-10053')
    expect(storage.value(workspaceStorageKey)).toBeUndefined()
    saveSelectedProjectID('', storage)
    expect(readSelectedProjectID(storage)).toBeNull()
    expect(storage.value(selectedProjectStorageKey)).toBeUndefined()
  })

  it('degrades to Overview when browser storage is unavailable', () => {
    const brokenStorage = {
      getItem: () => { throw new Error('storage blocked') },
      setItem: () => { throw new Error('storage blocked') },
    }
    expect(readWorkspaceSection(brokenStorage)).toBe('overview')
    expect(() => saveWorkspaceSection('projects', brokenStorage)).not.toThrow()
  })
})
