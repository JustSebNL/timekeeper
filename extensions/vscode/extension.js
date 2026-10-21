// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

'use strict';

const { TimeKeeperClient } = require('./lib/client');
const { summarizeProject, renderProjectContext } = require('./lib/context');
const { editorReference, formatEditorReference, taskDescription } = require('./lib/editor');
const { repositoryStatus, formatRepositoryStatus } = require('./lib/git');
const { createHistoryDocumentProvider } = require('./lib/history');
const { createHistoryCommands } = require('./lib/history-commands');
const { executionNodes } = require('./lib/tree');

function statusContext(status) {
  return String(status || 'Open').toLowerCase().replace(/[^a-z]+/g, '-').replace(/^-|-$/g, '');
}

function buildTreeItem(vscode, node) {
  const hasChildren = node.kind !== 'sprint';
  const item = new vscode.TreeItem(
    node.label,
    hasChildren ? vscode.TreeItemCollapsibleState.Collapsed : vscode.TreeItemCollapsibleState.None,
  );
  const parts = [];
  if (node.status) parts.push(node.status);
  if (typeof node.progress === 'number') parts.push(`${node.progress.toFixed(1)}%`);
  parts.push(node.id);
  item.description = parts.join(' · ');
  item.tooltip = `${node.kind[0].toUpperCase()}${node.kind.slice(1)}\n${item.description}`;
  item.contextValue = node.kind === 'sprint' ? `timekeeper.sprint.${statusContext(node.status)}` : `timekeeper.${node.kind}`;
  item.node = node;
  return item;
}

class TimeKeeperTreeProvider {
  constructor(vscode, clientFactory, getSelectedProjectID) {
    this.vscode = vscode;
    this.clientFactory = clientFactory;
    this.getSelectedProjectID = getSelectedProjectID;
    this.cache = new Map();
    this.onDidChangeTreeDataEmitter = new vscode.EventEmitter();
    this.onDidChangeTreeData = this.onDidChangeTreeDataEmitter.event;
  }

  refresh() {
    this.cache.clear();
    this.onDidChangeTreeDataEmitter.fire(undefined);
  }

  dispose() {
    this.onDidChangeTreeDataEmitter.dispose();
  }

  async getChildren(element) {
    if (!element) {
      const projects = await this.clientFactory().projects();
      const selectedProjectID = this.getSelectedProjectID();
      const items = selectedProjectID ? (projects.items || []).filter(project => project.project_id === selectedProjectID) : (projects.items || []);
      return items.map(project => buildTreeItem(this.vscode, {
        kind: 'project',
        id: project.project_id,
        parentID: null,
        label: project.name || project.project_name || project.project_id,
        status: project.status || '',
        progress: project.calculated_completion_pct,
      }));
    }
    if (element.node.kind === 'project') {
      await this.loadProject(element.node.id);
    }
    const nodes = this.cache.get(this.projectIDFor(element.node)) || [];
    return nodes.filter(node => node.parentID === element.node.id).map(node => buildTreeItem(this.vscode, node));
  }

  getTreeItem(element) {
    return element;
  }

  async loadProject(projectID) {
    if (this.cache.has(projectID)) return;
    const tree = await this.clientFactory().projectTree(projectID);
    this.cache.set(projectID, executionNodes(tree));
  }

  async categories(projectID) {
    await this.loadProject(projectID);
    return (this.cache.get(projectID) || []).filter(node => node.kind === 'category');
  }

  projectIDFor(node) {
    if (node.kind === 'project') return node.id;
    for (const [projectID, nodes] of this.cache.entries()) {
      if (nodes.some(candidate => candidate.id === node.id)) return projectID;
    }
    return '';
  }
}

function activate(context) {
  const vscode = require('vscode');
  const state = { client: undefined, selectedProjectID: context.workspaceState.get('timekeeper.selectedProjectID') };
  const statusBar = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100);
  statusBar.name = 'Time Keeper';
  statusBar.command = 'timekeeper.connect';
  statusBar.text = '$(circle-outline) Time Keeper: disconnected';
  statusBar.show();

  const getClient = () => {
    if (!state.client) {
      state.client = new TimeKeeperClient(vscode.workspace.getConfiguration('timekeeper').get('serverUrl'));
    }
    return state.client;
  };
  const provider = new TimeKeeperTreeProvider(vscode, getClient, () => state.selectedProjectID);
  const historyDocuments = createHistoryDocumentProvider(vscode);
  const historyDocumentRegistration = vscode.workspace.registerTextDocumentContentProvider('timekeeper-history', historyDocuments);
  const reportError = error => vscode.window.showErrorMessage(`Time Keeper: ${error.message}`);
  const historyCommands = createHistoryCommands({
    vscode,
    getClient,
    getSelectedProjectID: () => state.selectedProjectID,
    historyDocuments,
    refreshTree: () => provider.refresh(),
    reportError,
  });
  const setConnected = suffix => {
    statusBar.text = `$(check) Time Keeper${suffix ? `: ${suffix}` : ': connected'}`;
  };

  async function connect() {
    try {
      const health = await getClient().health();
      if (health.status !== 'ok') throw new Error('health endpoint did not return ok');
      setConnected(state.selectedProjectID || 'connected');
      provider.refresh();
      vscode.window.showInformationMessage('Time Keeper connected.');
    } catch (error) {
      statusBar.text = '$(error) Time Keeper: unavailable';
      reportError(error);
    }
  }

  async function selectProject() {
    try {
      const projects = (await getClient().projects()).items || [];
      const choice = await vscode.window.showQuickPick(projects.map(project => ({
        label: project.name || project.project_name || project.project_id,
        description: `${project.status || 'Open'} · ${project.project_id}`,
        projectID: project.project_id,
      })), { placeHolder: 'Select the Time Keeper Project for this workspace' });
      if (!choice) return;
      state.selectedProjectID = choice.projectID;
      await context.workspaceState.update('timekeeper.selectedProjectID', choice.projectID);
      setConnected(choice.projectID);
      provider.refresh();
    } catch (error) {
      reportError(error);
    }
  }

  async function showProjectContext() {
    if (!state.selectedProjectID) {
      vscode.window.showErrorMessage('Time Keeper: select a Project first.');
      return;
    }
    try {
      const [tree, agentFiles, harnessFiles, devinFiles, agentsFiles, harnessDocs] = await Promise.all([
        getClient().projectTree(state.selectedProjectID),
        vscode.workspace.findFiles('AGENTS.md', null, 10),
        vscode.workspace.findFiles('harness/**/SKILL.md', null, 50),
        vscode.workspace.findFiles('.devin/**/SKILL.md', null, 50),
        vscode.workspace.findFiles('.agents/**/SKILL.md', null, 50),
        vscode.workspace.findFiles('docs/HARNESS.md', null, 10),
      ]);
      const paths = [...new Set([...agentFiles, ...harnessFiles, ...devinFiles, ...agentsFiles, ...harnessDocs]
        .map(uri => vscode.workspace.asRelativePath(uri, false)))].sort();
      const panel = vscode.window.createWebviewPanel(
        'timekeeper.projectContext',
        `Time Keeper: ${state.selectedProjectID}`,
        vscode.ViewColumn.Beside,
        { enableScripts: false },
      );
      panel.webview.html = renderProjectContext(summarizeProject(tree), paths);
    } catch (error) {
      reportError(error);
    }
  }

  async function showGitStatus() {
    try {
      const gitExtension = vscode.extensions.getExtension('vscode.git');
      const api = gitExtension?.exports?.getAPI?.(1);
      if (!api) {
        vscode.window.showErrorMessage('Time Keeper: the built-in VS Code Git extension is unavailable.');
        return;
      }
      const activeURI = vscode.window.activeTextEditor?.document?.uri;
      const folder = activeURI ? vscode.workspace.getWorkspaceFolder(activeURI) : vscode.workspace.workspaceFolders?.[0];
      const repository = folder?.uri && typeof api.getRepository === 'function' ? api.getRepository(folder.uri) : api.repositories?.[0];
      const status = repositoryStatus(repository);
      if (!status) {
        vscode.window.showErrorMessage('Time Keeper: no Git repository is available for this workspace.');
        return;
      }
      const message = `Time Keeper Git status: ${formatRepositoryStatus(status)}`;
      const choice = await vscode.window.showInformationMessage(message, 'Copy');
      if (choice === 'Copy') {
        await vscode.env.clipboard.writeText(message);
      }
    } catch (error) {
      reportError(error);
    }
  }

  async function copyEditorReference() {
    const reference = editorReference(vscode, vscode.window.activeTextEditor);
    if (!reference) {
      vscode.window.showErrorMessage('Time Keeper: open a text editor first.');
      return;
    }
    await vscode.env.clipboard.writeText(formatEditorReference(reference));
    vscode.window.showInformationMessage('Time Keeper copied the editor reference.');
  }

  async function createTaskFromSelection() {
    const reference = editorReference(vscode, vscode.window.activeTextEditor);
    if (!reference) {
      vscode.window.showErrorMessage('Time Keeper: open a text editor first.');
      return;
    }
    if (!state.selectedProjectID) {
      vscode.window.showErrorMessage('Time Keeper: select a Project first.');
      return;
    }
    try {
      const categories = await provider.categories(state.selectedProjectID);
      if (categories.length === 0) {
        vscode.window.showErrorMessage('Time Keeper: the selected Project has no Category for the new Task.');
        return;
      }
      const name = await vscode.window.showInputBox({
        prompt: 'Name the Time Keeper Task for this editor selection',
        value: formatEditorReference(reference),
        ignoreFocusOut: true,
      });
      if (!name?.trim()) return;
      const category = await vscode.window.showQuickPick(categories.map(node => ({
        label: node.label,
        description: node.id,
        categoryID: node.id,
      })), { placeHolder: 'Choose the Category for this Task', ignoreFocusOut: true });
      if (!category) return;
      const estimate = await vscode.window.showQuickPick([
        { label: '15 minutes', seconds: 15 * 60 },
        { label: '30 minutes', seconds: 30 * 60 },
        { label: '1 hour', seconds: 60 * 60 },
        { label: '2 hours', seconds: 2 * 60 * 60 },
      ], { placeHolder: 'Choose an initial Task estimate', ignoreFocusOut: true });
      if (!estimate) return;
      const task = await getClient().createTask(state.selectedProjectID, {
        category_id: category.categoryID,
        name: name.trim(),
        description: taskDescription(reference),
        goal: `Review the selected code at ${formatEditorReference(reference)}.`,
        priority: 'Normal',
        estimated_duration_seconds: estimate.seconds,
      });
      provider.refresh();
      vscode.window.showInformationMessage(`Time Keeper created ${task.task_id}.`);
    } catch (error) {
      reportError(error);
    }
  }

  async function sprintAction(action, node) {
    if (!node?.id || node.kind !== 'sprint') {
      vscode.window.showErrorMessage('Time Keeper: select a Sprint first.');
      return;
    }
    try {
      const sprint = await getClient().sprintAction(node.id, action);
      vscode.window.showInformationMessage(`Time Keeper: ${sprint.sprint_id} is ${sprint.status}.`);
      provider.refresh();
    } catch (error) {
      reportError(error);
    }
  }

  context.subscriptions.push(
    statusBar,
    provider,
    historyDocuments,
    historyDocumentRegistration,
    vscode.window.registerTreeDataProvider('timekeeper.projects', provider),
    vscode.commands.registerCommand('timekeeper.connect', connect),
    vscode.commands.registerCommand('timekeeper.selectProject', selectProject),
    vscode.commands.registerCommand('timekeeper.refresh', () => provider.refresh()),
    vscode.commands.registerCommand('timekeeper.startSprint', node => sprintAction('start', node)),
    vscode.commands.registerCommand('timekeeper.holdSprint', node => sprintAction('hold', node)),
    vscode.commands.registerCommand('timekeeper.resumeSprint', node => sprintAction('resume', node)),
    vscode.commands.registerCommand('timekeeper.completeSprint', node => sprintAction('complete', node)),
    vscode.commands.registerCommand('timekeeper.copyEditorReference', copyEditorReference),
    vscode.commands.registerCommand('timekeeper.createTaskFromSelection', createTaskFromSelection),
    vscode.commands.registerCommand('timekeeper.showProjectContext', showProjectContext),
    vscode.commands.registerCommand('timekeeper.showGitStatus', showGitStatus),
    vscode.commands.registerCommand('timekeeper.browseFileHistory', historyCommands.browseFileHistory),
    vscode.commands.registerCommand('timekeeper.restoreFileHistoryVersion', historyCommands.restoreFileHistoryVersion),
    vscode.commands.registerCommand('timekeeper.captureFileHistorySnapshot', historyCommands.captureFileHistorySnapshot),
    vscode.commands.registerCommand('timekeeper.openDashboard', () => vscode.env.openExternal(vscode.Uri.parse(`${getClient().serverURL}/`))),
    vscode.workspace.onDidChangeConfiguration(event => {
      if (event.affectsConfiguration('timekeeper.serverUrl')) {
        state.client = undefined;
        provider.refresh();
        statusBar.text = '$(circle-outline) Time Keeper: reconnect required';
      }
    }),
  );
}

function deactivate() {}

module.exports = { activate, deactivate, buildTreeItem, TimeKeeperTreeProvider };
