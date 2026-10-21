import { describe, expect, it } from 'vitest'
import { clearProjectFilterShortcut, focusProjectFilterShortcut, type ShortcutKeyEvent } from './keyboardShortcuts'

function makeEvent(overrides: Partial<ShortcutKeyEvent> = {}) {
  let prevented = false
  const event: ShortcutKeyEvent = {
    key: '/',
    target: { tagName: 'BUTTON' },
    preventDefault: () => { prevented = true },
    ...overrides,
  }
  return { event, wasPrevented: () => prevented }
}

describe('Project filter keyboard shortcut', () => {
  it('focuses the filter and prevents the slash from being typed into the page', () => {
    const { event, wasPrevented } = makeEvent()
    let focused = false

    const handled = focusProjectFilterShortcut(event, { focus: () => { focused = true } })

    expect(handled).toBe(true)
    expect(focused).toBe(true)
    expect(wasPrevented()).toBe(true)
  })

  it('does not intercept slash while the user is typing in editable controls', () => {
    const editableTargets = [
      { tagName: 'INPUT' },
      { tagName: 'TEXTAREA' },
      { tagName: 'SELECT' },
      { isContentEditable: true },
    ]

    for (const target of editableTargets) {
      const { event, wasPrevented } = makeEvent({ target })
      let focused = false

      expect(focusProjectFilterShortcut(event, { focus: () => { focused = true } })).toBe(false)
      expect(focused).toBe(false)
      expect(wasPrevented()).toBe(false)
    }
  })

  it('ignores other keys, modified/composition/repeat events, and already handled events', () => {
    const ignoredEvents: Partial<ShortcutKeyEvent>[] = [
      { key: 'a' },
      { altKey: true },
      { ctrlKey: true },
      { metaKey: true },
      { shiftKey: true },
      { isComposing: true },
      { repeat: true },
      { defaultPrevented: true },
    ]

    for (const overrides of ignoredEvents) {
      const { event, wasPrevented } = makeEvent(overrides)
      let focused = false

      expect(focusProjectFilterShortcut(event, { focus: () => { focused = true } })).toBe(false)
      expect(focused).toBe(false)
      expect(wasPrevented()).toBe(false)
    }
  })

  it('does not claim the shortcut when the filter input is unavailable', () => {
    const { event, wasPrevented } = makeEvent()

    expect(focusProjectFilterShortcut(event, null)).toBe(false)
    expect(wasPrevented()).toBe(false)
  })

  it('clears a non-empty filter only when Escape is pressed on that filter', () => {
    const input = { value: 'agent', focus: () => undefined }
    const { event, wasPrevented } = makeEvent({ key: 'Escape', target: input })
    let cleared = false

    expect(clearProjectFilterShortcut(event, input, () => { cleared = true })).toBe(true)
    expect(cleared).toBe(true)
    expect(wasPrevented()).toBe(true)
  })

  it('does not intercept empty filters, other controls, or unavailable inputs', () => {
    const emptyInput = { value: '', focus: () => undefined }
    const otherInput = { value: 'keep me', focus: () => undefined }
    const cases = [
      { input: emptyInput, target: emptyInput },
      { input: otherInput, target: { tagName: 'INPUT' } },
      { input: null, target: { tagName: 'BODY' } },
    ]

    for (const { input, target } of cases) {
      const { event, wasPrevented } = makeEvent({ key: 'Escape', target })
      let cleared = false

      expect(clearProjectFilterShortcut(event, input, () => { cleared = true })).toBe(false)
      expect(cleared).toBe(false)
      expect(wasPrevented()).toBe(false)
    }
  })

  it('ignores modified, repeated, composing, or already-handled Escape events', () => {
    const input = { value: 'agent', focus: () => undefined }
    const ignoredEvents: Partial<ShortcutKeyEvent>[] = [
      { key: 'a' },
      { altKey: true },
      { ctrlKey: true },
      { metaKey: true },
      { shiftKey: true },
      { isComposing: true },
      { repeat: true },
      { defaultPrevented: true },
    ]

    for (const overrides of ignoredEvents) {
      const { event, wasPrevented } = makeEvent({ key: 'Escape', target: input, ...overrides })
      let cleared = false

      expect(clearProjectFilterShortcut(event, input, () => { cleared = true })).toBe(false)
      expect(cleared).toBe(false)
      expect(wasPrevented()).toBe(false)
    }
  })
})
