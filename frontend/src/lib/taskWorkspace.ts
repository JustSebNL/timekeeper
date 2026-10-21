import type { ProjectExecutionTree, TaskDetail } from './api'

export interface TaskWorkspaceReads {
  getTaskDetail: (taskID: string) => Promise<TaskDetail>
  getExecutionTree: (projectID: string) => Promise<ProjectExecutionTree>
}

export interface TaskWorkspaceSnapshot {
  detail: TaskDetail
  tree: ProjectExecutionTree
}

export class TaskWorkspaceRequestGuard {
  private generation = 0

  begin(taskID: string, projectID: string) {
    const generation = ++this.generation
    return {
      isCurrent: (currentTaskID: string | null, currentProjectID: string | null) =>
        generation === this.generation && currentTaskID === taskID && currentProjectID === projectID,
    }
  }

  invalidate() {
    this.generation++
  }
}

/**
 * Read both authoritative views before exposing either one to the workspace.
 * A Task must remain scoped to the Project from which it was opened.
 */
export async function loadTaskWorkspaceSnapshot(
  api: TaskWorkspaceReads,
  taskID: string,
  projectID: string,
): Promise<TaskWorkspaceSnapshot> {
  const [detail, tree] = await Promise.all([
    api.getTaskDetail(taskID),
    api.getExecutionTree(projectID),
  ])

  if (detail.task.project_id && detail.task.project_id !== projectID) {
    throw new Error(`Task ${taskID} does not belong to the selected Project ${projectID}.`)
  }
  if (tree.project.project_id !== projectID) {
    throw new Error(`Project hierarchy ${tree.project.project_id} does not match the selected Project ${projectID}.`)
  }

  return { detail, tree }
}
