import { describe, expect, it } from 'vitest'
import { flattenExecutionTree } from './executionTree'

const tree = {
  project: { project_id: 'P-7', project_name: 'Example', status: 'Open' },
  categories: [{
    category: { category_id: 'C-1', name: 'Delivery', status: 'Open', item_address: '1' },
    categories: [{ category: { category_id: 'C-2', name: 'Quality', status: 'Open', item_address: '1.2' }, categories: [], tasks: [] }],
    tasks: [{
      task: { task_id: 'T-3', name: 'Build', status: 'Active', item_address: '1.3' },
      sprints: [{ sprint_id: 'SP-4', name: 'Implementation', status: 'Active', item_address: '1.3.4' }],
      subtasks: [{
        subtask: { subtask_id: 'ST-5', name: 'Verify', status: 'Open', item_address: '1.3.5' },
        sprints: [{ sprint_id: 'SP-6', name: 'Acceptance', status: 'Open', item_address: '1.3.5.6' }],
      }],
    }],
  }],
}

describe('Project execution hierarchy', () => {
  it('flattens nested categories and executable ownership without losing IDs or depth', () => {
    expect(flattenExecutionTree(tree).map(({ kind, id, name, depth }) => [kind, id, name, depth])).toEqual([
      ['category', 'C-1', 'Delivery', 0],
      ['task', 'T-3', 'Build', 1],
      ['sprint', 'SP-4', 'Implementation', 2],
      ['subtask', 'ST-5', 'Verify', 2],
      ['sprint', 'SP-6', 'Acceptance', 3],
      ['category', 'C-2', 'Quality', 1],
    ])
  })

  it('returns an empty, non-null list for an empty Project tree', () => {
    expect(flattenExecutionTree({ project: tree.project, categories: [] })).toEqual([])
  })
})
