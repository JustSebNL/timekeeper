// Copyright (c) 2026 Seb. All rights reserved.

'use strict';

const { TimeKeeperClient } = require('./lib/client');
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
  const reportError = error => vscode.window.showErrorMessage(`Time Keeper: ${error.message}`);
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
    vscode.window.registerTreeDataProvider('timekeeper.projects', provider),
    vscode.commands.registerCommand('timekeeper.connect', connect),
    vscode.commands.registerCommand('timekeeper.selectProject', selectProject),
    vscode.commands.registerCommand('timekeeper.refresh', () => provider.refresh()),
    vscode.commands.registerCommand('timekeeper.startSprint', node => sprintAction('start', node)),
    vscode.commands.registerCommand('timekeeper.holdSprint', node => sprintAction('hold', node)),
    vscode.commands.registerCommand('timekeeper.resumeSprint', node => sprintAction('resume', node)),
    vscode.commands.registerCommand('timekeeper.completeSprint', node => sprintAction('complete', node)),
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
