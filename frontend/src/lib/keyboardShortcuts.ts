export interface ShortcutKeyEvent {
  key: string
  target: unknown
  defaultPrevented?: boolean
  altKey?: boolean
  ctrlKey?: boolean
  metaKey?: boolean
  shiftKey?: boolean
  isComposing?: boolean
  repeat?: boolean
  preventDefault(): void
}

export interface FocusableFilterInput {
  focus(): void
}

export interface ClearableFilterInput extends FocusableFilterInput {
  value: string
}

function isEditableTarget(target: unknown): boolean {
  if (target === null || typeof target !== 'object') return false
  const element = target as { tagName?: unknown; isContentEditable?: unknown }
  const tagName = typeof element.tagName === 'string' ? element.tagName.toUpperCase() : ''
  return tagName === 'INPUT' || tagName === 'TEXTAREA' || tagName === 'SELECT' || element.isContentEditable === true
}

export function focusProjectFilterShortcut(event: ShortcutKeyEvent, input: FocusableFilterInput | null): boolean {
  if (!input || event.key !== '/' || event.defaultPrevented || event.isComposing || event.repeat) return false
  if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey || isEditableTarget(event.target)) return false

  event.preventDefault()
  input.focus()
  return true
}

export function clearProjectFilterShortcut(
  event: ShortcutKeyEvent,
  input: ClearableFilterInput | null,
  clearValue: () => void,
): boolean {
  if (!input || event.key !== 'Escape' || event.target !== input || input.value.length === 0) return false
  if (event.defaultPrevented || event.isComposing || event.repeat) return false
  if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return false

  event.preventDefault()
  clearValue()
  return true
}
