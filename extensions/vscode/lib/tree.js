// Copyright (c) 2026 Seb. All rights reserved.

'use strict';

function executionNodes(tree) {
  const result = [];
  if (!tree?.project?.project_id) {
    return result;
  }
  const project = tree.project;
  result.push({
    kind: 'project',
    id: project.project_id,
    parentID: null,
    label: project.name || project.project_name || project.project_id,
    status: project.status || '',
    progress: project.calculated_completion_pct,
  });

  for (const category of tree.categories || []) {
    appendCategory(result, category, project.project_id);
  }
  return result;
}

function appendCategory(result, categoryNode, parentID) {
  const category = categoryNode.category;
  if (!category?.category_id) {
    return;
  }
  result.push({
    kind: 'category',
    id: category.category_id,
    parentID,
    label: category.name || category.category_name || category.category_id,
    status: category.status || '',
  });
  for (const childCategory of categoryNode.categories || []) {
    appendCategory(result, childCategory, category.category_id);
  }
  for (const taskNode of categoryNode.tasks || []) {
    appendTask(result, taskNode, category.category_id);
  }
}

function appendTask(result, taskNode, parentID) {
  const task = taskNode.task;
  if (!task?.task_id) {
    return;
  }
  result.push({
    kind: 'task',
    id: task.task_id,
    parentID,
    label: task.name || task.task_name || task.task_id,
    status: task.status || '',
  });
  for (const sprint of taskNode.sprints || []) {
    appendSprint(result, sprint, task.task_id);
  }
  for (const subtaskNode of taskNode.subtasks || []) {
    appendSubtask(result, subtaskNode, task.task_id);
  }
}

function appendSubtask(result, subtaskNode, parentID) {
  const subtask = subtaskNode.subtask;
  if (!subtask?.subtask_id) {
    return;
  }
  result.push({
    kind: 'subtask',
    id: subtask.subtask_id,
    parentID,
    label: subtask.name || subtask.subtask_name || subtask.subtask_id,
    status: subtask.status || '',
  });
  for (const sprint of subtaskNode.sprints || []) {
    appendSprint(result, sprint, subtask.subtask_id);
  }
}

function appendSprint(result, sprint, parentID) {
  if (!sprint?.sprint_id) {
    return;
  }
  result.push({
    kind: 'sprint',
    id: sprint.sprint_id,
    parentID,
    label: sprint.name || sprint.sprint_name || sprint.sprint_id,
    status: sprint.status || '',
  });
}

module.exports = { executionNodes };
