import { describe, expect, it } from 'vitest'
import { resolveWorkspaceSection, workspaceNavigation, workspaceSectionTitle } from './navigation'

describe('Svelte workspace navigation contract', () => {
  it('allows implemented workspaces while keeping unsupported workspaces unavailable', () => {
    expect(resolveWorkspaceSection('overview')).toBe('overview')
    expect(resolveWorkspaceSection('projects')).toBe('projects')
    expect(resolveWorkspaceSection('queue')).toBe('queue')
    expect(workspaceSectionTitle('queue')).toBe('Registration Queue')
    expect(resolveWorkspaceSection('files')).toBe('files')
    expect(resolveWorkspaceSection('settings')).toBe('settings')
    expect(workspaceSectionTitle('settings')).toBe('Settings')
    expect(resolveWorkspaceSection('agents')).toBe('agents')
    expect(workspaceSectionTitle('agents')).toBe('Agent Pulse')
    expect(resolveWorkspaceSection('metrics')).toBe('metrics')
    expect(workspaceSectionTitle('metrics')).toBe('Metrics')
    expect(resolveWorkspaceSection('memory')).toBe('memory')
    expect(workspaceSectionTitle('memory')).toBe('Memory Galaxy')
    expect(resolveWorkspaceSection('plugins')).toBe('plugins')
    expect(workspaceSectionTitle('plugins')).toBe('Plugins')
    for (const reserved of ['tasks']) {
      expect(resolveWorkspaceSection(reserved)).toBe('overview')
    }
    expect(resolveWorkspaceSection('unrecognized')).toBe('overview')
  })

  it('keeps implemented workspaces enabled and future workspaces explicitly unavailable', () => {
    expect(workspaceNavigation.map(({ id, enabled }) => [id, enabled])).toEqual([
      ['overview', true],
      ['projects', true],
      ['queue', true],
      ['files', true],
      ['settings', true],
      ['tasks', false],
      ['agents', true],
      ['metrics', true],
      ['memory', true],
      ['plugins', true],
    ])
  })
})
