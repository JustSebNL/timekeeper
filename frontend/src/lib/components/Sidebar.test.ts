import { render } from 'svelte/server'
import { describe, expect, it } from 'vitest'
import Sidebar from './Sidebar.svelte'

describe('Sidebar', () => {
  it('exposes accessible active Overview/Projects navigation and marks future sections unavailable', () => {
    const { body } = render(Sidebar, {
      props: {
        activeSection: 'projects',
        onNavigate: () => undefined,
      },
    })

    expect(body).toContain('aria-label="Main navigation"')
    expect(body).toContain('data-workspace-section="overview"')
    expect(body).toContain('data-workspace-section="projects"')
    expect(body).toContain('data-workspace-section="files"')
    expect(body).toContain('data-workspace-section="settings"')
    expect(body).toContain('Settings')
    expect(body).toContain('Files &amp; History')
    expect(body).toContain('aria-current="page"')
    expect(body).toContain('aria-disabled="true"')
    expect(body).toContain('Tasks')
    expect(body).toContain('Agents')
    expect(body).toContain('Memory')
  })
})
