import { render } from 'svelte/server'
import { describe, expect, it } from 'vitest'
import Topbar from './Topbar.svelte'

describe('Topbar', () => {
  it('renders the active workspace, bound search state, Core status, and refresh action', () => {
    const { body } = render(Topbar, {
      props: {
        sectionTitle: 'Projects',
        coreOnline: true,
        search: 'memory',
        refreshing: false,
        onSearch: () => undefined,
        onRefresh: () => undefined,
      },
    })

    expect(body).toContain('<strong>Projects</strong>')
    expect(body).toContain('value="memory"')
    expect(body).toContain('Core connected')
    expect(body).toContain('aria-label="Refresh data"')
    expect(body).toContain('<kbd>Esc</kbd>')
  })

  it('shows an accessible clear action only while the filter has a value', () => {
    const baseProps = {
      sectionTitle: 'Projects',
      coreOnline: true,
      refreshing: false,
      onSearch: () => undefined,
      onRefresh: () => undefined,
    }

    const filtered = render(Topbar, { props: { ...baseProps, search: 'memory' } })
    const empty = render(Topbar, { props: { ...baseProps, search: '' } })

    expect(filtered.body).toContain('aria-label="Clear search"')
    expect(empty.body).not.toContain('aria-label="Clear search"')
  })
})
