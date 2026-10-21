import type { ExecutionCategory, ProjectExecutionTree } from './api'

export type HierarchyKind = 'category' | 'task' | 'subtask' | 'sprint'

export interface ProjectHierarchyRow {
  id: string
  kind: HierarchyKind
  name: string
  status: string
  itemAddress: string
  depth: number
  completionPercent?: number
}

export function flattenExecutionTree(tree: ProjectExecutionTree): ProjectHierarchyRow[] {
  const rows: ProjectHierarchyRow[] = []
  const addCategory = (node: ExecutionCategory, depth: number): void => {
    rows.push({
      id: node.category.category_id,
      kind: 'category',
      name: node.category.name,
      status: node.category.status,
      itemAddress: node.category.item_address,
      depth,
    })
    for (const taskNode of node.tasks || []) {
      rows.push({
        id: taskNode.task.task_id,
        kind: 'task',
        name: taskNode.task.name,
        status: taskNode.task.status,
        itemAddress: taskNode.task.item_address,
        depth: depth + 1,
        completionPercent: taskNode.task.calculated_completion_pct,
      })
      for (const sprint of taskNode.sprints || []) {
        rows.push({
          id: sprint.sprint_id,
          kind: 'sprint',
          name: sprint.name,
          status: sprint.status,
          itemAddress: sprint.item_address,
          depth: depth + 2,
        })
      }
      for (const subtaskNode of taskNode.subtasks || []) {
        rows.push({
          id: subtaskNode.subtask.subtask_id,
          kind: 'subtask',
          name: subtaskNode.subtask.name,
          status: subtaskNode.subtask.status,
          itemAddress: subtaskNode.subtask.item_address,
          depth: depth + 2,
          completionPercent: subtaskNode.subtask.calculated_completion_pct,
        })
        for (const sprint of subtaskNode.sprints || []) {
          rows.push({
            id: sprint.sprint_id,
            kind: 'sprint',
            name: sprint.name,
            status: sprint.status,
            itemAddress: sprint.item_address,
            depth: depth + 3,
          })
        }
      }
    }
    for (const child of node.categories || []) addCategory(child, depth + 1)
  }
  for (const category of tree.categories || []) addCategory(category, 0)
  return rows
}
