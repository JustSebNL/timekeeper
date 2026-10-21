import type { SprintAction } from './api'

const actionsBySprintStatus: Readonly<Record<string, readonly SprintAction[]>> = {
  Open: ['start', 'hold', 'cancel'],
  Active: ['hold', 'complete', 'cancel'],
  'On Hold': ['resume', 'cancel'],
  Completed: [],
  Cancelled: [],
  TimedOut: [],
}

export function sprintActionsForStatus(status: string): SprintAction[] {
  return [...(actionsBySprintStatus[status] ?? [])]
}

export function sprintActionLabel(action: SprintAction): string {
  return action[0].toUpperCase() + action.slice(1)
}

export function sprintActionRequiresReason(action: SprintAction): boolean {
  return action === 'hold' || action === 'cancel'
}

export function taskStatusRequiresConfirmation(status: string): boolean {
  return status === 'Cancelled'
}

export function retainAvailableSprintAction(
  pending: SprintAction | null,
  available: readonly SprintAction[],
): SprintAction | null {
  return pending && available.includes(pending) ? pending : null
}

export async function runAndRefreshTaskWorkspace(
  action: () => Promise<unknown>,
  refresh: () => Promise<boolean>,
  refreshFailureMessage: string,
): Promise<void> {
  try {
    await action()
  } catch (cause) {
    try {
      await refresh()
    } catch {
      // Preserve the authoritative action error; the caller still offers retry.
    }
    throw cause
  }

  if (!await refresh()) {
    throw new Error(refreshFailureMessage)
  }
}