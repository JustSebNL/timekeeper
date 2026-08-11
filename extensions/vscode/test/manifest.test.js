// Copyright (c) 2026 Seb. All rights reserved.

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
});
