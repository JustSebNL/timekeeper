import { render } from 'svelte/server'
import { describe, expect, it } from 'vitest'
import StatCard from './StatCard.svelte'

describe('StatCard', () => {
  it('renders reusable metric data and muted future-state styling', () => {
    const { body } = render(StatCard, {
      props: { icon: '✓', tone: 'blue', caption: 'COMING NEXT', value: '—', label: 'Active tasks', muted: true },
    })

    expect(body).toContain('class="stat-card muted-card"')
    expect(body).toContain('COMING NEXT')
    expect(body).toContain('Active tasks')
    expect(body).toContain('—')
  })
})
