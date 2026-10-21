export type WorkspaceSection = 'overview' | 'projects' | 'queue' | 'files' | 'settings' | 'agents' | 'metrics' | 'memory' | 'plugins'

export interface WorkspaceNavigationItem {
  id: string
  label: string
  icon: string
  enabled: boolean
  hint?: string
}

export const workspaceNavigation: readonly WorkspaceNavigationItem[] = [
  { id: 'overview', label: 'Overview', icon: '⌂', enabled: true },
  { id: 'projects', label: 'Projects', icon: '▤', enabled: true },
  { id: 'queue', label: 'Registration Queue', icon: '⇢', enabled: true },
  { id: 'files', label: 'Files & History', icon: '▣', enabled: true },
  { id: 'settings', label: 'Settings', icon: '⚙', enabled: true },
  { id: 'tasks', label: 'Tasks', icon: '✓', enabled: false, hint: 'next' },
  { id: 'agents', label: 'Agents', icon: '◉', enabled: true },
  { id: 'metrics', label: 'Metrics', icon: '▥', enabled: true },
  { id: 'memory', label: 'Memory', icon: '⌁', enabled: true },
  { id: 'plugins', label: 'Plugins', icon: '🔌', enabled: true },
]

const enabledSections = new Set<string>(workspaceNavigation.filter((item) => item.enabled).map((item) => item.id))

export function resolveWorkspaceSection(value: string): WorkspaceSection {
  return enabledSections.has(value) ? (value as WorkspaceSection) : 'overview'
}

export function workspaceSectionTitle(section: WorkspaceSection): string {
  if (section === 'projects') return 'Projects'
  if (section === 'queue') return 'Registration Queue'
  if (section === 'files') return 'Files & History'
  if (section === 'settings') return 'Settings'
  if (section === 'agents') return 'Agent Pulse'
  if (section === 'metrics') return 'Metrics'
  if (section === 'memory') return 'Memory Galaxy'
  if (section === 'plugins') return 'Plugins'
  return 'Overview'
}
