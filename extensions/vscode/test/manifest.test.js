// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');

const manifest = require('../package.json');

test('extension manifest exposes a loopback server setting and execution commands', () => {
  assert.equal(manifest.contributes.configuration.properties['timekeeper.serverUrl'].default, 'http://127.0.0.1:1618');
  const commands = manifest.contributes.commands.map(command => command.command);
  for (const command of [
    'timekeeper.connect',
    'timekeeper.selectProject',
    'timekeeper.refresh',
    'timekeeper.startSprint',
    'timekeeper.holdSprint',
    'timekeeper.resumeSprint',
    'timekeeper.completeSprint',
    'timekeeper.copyEditorReference',
    'timekeeper.createTaskFromSelection',
    'timekeeper.showProjectContext',
    'timekeeper.showGitStatus',
    'timekeeper.browseFileHistory',
    'timekeeper.restoreFileHistoryVersion',
    'timekeeper.captureFileHistorySnapshot',
    'timekeeper.openDashboard',
  ]) {
    assert.ok(commands.includes(command), `missing ${command}`);
  }
  assert.ok(manifest.contributes.views.explorer.some(view => view.id === 'timekeeper.projects'));
  const sprintMenus = manifest.contributes.menus['view/item/context'];
  for (const [command, contextValue] of [
    ['timekeeper.startSprint', 'timekeeper.sprint.open'],
    ['timekeeper.holdSprint', 'timekeeper.sprint.active'],
    ['timekeeper.resumeSprint', 'timekeeper.sprint.on-hold'],
    ['timekeeper.completeSprint', 'timekeeper.sprint.active'],
  ]) {
    assert.ok(sprintMenus.some(item => item.command === command && item.when.includes(contextValue)), `missing Sprint menu ${command}`);
  }
  const editorMenus = manifest.contributes.menus['editor/context'];
  assert.ok(editorMenus.some(item => item.command === 'timekeeper.copyEditorReference' && item.when === 'editorTextFocus'));
  assert.ok(editorMenus.some(item => item.command === 'timekeeper.createTaskFromSelection' && item.when === 'editorHasSelection'));
  const viewMenus = manifest.contributes.menus['view/title'];
  assert.ok(viewMenus.some(item => item.command === 'timekeeper.showProjectContext' && item.when === 'view == timekeeper.projects'));
  assert.ok(viewMenus.some(item => item.command === 'timekeeper.showGitStatus' && item.when === 'view == timekeeper.projects'));
  assert.ok(viewMenus.some(item => item.command === 'timekeeper.browseFileHistory' && item.when === 'view == timekeeper.projects'));
  assert.ok(viewMenus.some(item => item.command === 'timekeeper.restoreFileHistoryVersion' && item.when === 'view == timekeeper.projects'));
  assert.ok(viewMenus.some(item => item.command === 'timekeeper.captureFileHistorySnapshot' && item.when === 'view == timekeeper.projects'));
});
