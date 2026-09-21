// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

const path = require('node:path');
const { Plugin, PluginSettingTab, Setting, Notice } = require('obsidian');
const { GalaxyRepository } = require('./src/repository');
const { GalaxyApiServer } = require('./src/api/server');

const DEFAULT_SETTINGS = Object.freeze({ galaxyFolder: 'Memory Galaxy' });

class TimeKeeperMemoryGalaxyPlugin extends Plugin {
  async onload() {
    this.settings = Object.assign({}, DEFAULT_SETTINGS, await this.loadData());
    this.vaultPath = this.app.vault.adapter.getBasePath();
    this.pluginState = path.join(this.vaultPath, '.obsidian', 'plugins', this.manifest.id);
    this.addSettingTab(new MemoryGalaxySettingTab(this.app, this));
    await this.startServer();
    this.addCommand({
      id: 'show-memory-galaxy-status',
      name: 'Show Memory Galaxy status',
      callback: () => new Notice(this.descriptor ? `Memory Galaxy available on ${this.descriptor.base_url}` : 'Memory Galaxy is unavailable')
    });
    this.addCommand({
      id: 'open-memory-galaxy-inbox',
      name: 'Open Memory Galaxy Inbox',
      callback: () => this.app.workspace.openLinkText(`${this.settings.galaxyFolder}/Inbox`, '', false)
    });
  }

  async startServer() {
    this.repository = new GalaxyRepository(this.vaultPath, this.settings.galaxyFolder);
    this.apiServer = new GalaxyApiServer(this.repository, {
      descriptorPath: path.join(this.pluginState, 'endpoint.json'),
      tokenPath: path.join(this.pluginState, 'capability.token')
    });
    try {
      this.descriptor = await this.apiServer.start();
    } catch (error) {
      this.descriptor = null;
      console.error('TimeKeeper Memory Galaxy failed to start', error);
      new Notice('TimeKeeper Memory Galaxy could not start. Check the console.');
    }
  }

  async reconnect() {
    await this.apiServer?.stop();
    await this.startServer();
    this.displayNotice(this.descriptor ? 'Memory Galaxy reconnected.' : 'Memory Galaxy is unavailable.');
  }

  async saveSettings() {
    await this.saveData(this.settings);
  }

  displayNotice(message) {
    new Notice(message);
    this.settingTab?.setStatus(message);
  }

  async onunload() {
    await this.apiServer?.stop();
  }
}

class MemoryGalaxySettingTab extends PluginSettingTab {
  constructor(app, plugin) {
    super(app, plugin);
    plugin.settingTab = this;
  }

  display() {
    const { containerEl } = this;
    containerEl.replaceChildren();
    containerEl.createEl('h2', { text: 'TimeKeeper Memory Galaxy' });
    new Setting(containerEl)
      .setName('Galaxy folder')
      .setDesc('Folder inside the current vault where cross-project knowledge is stored.')
      .addText(text => text
        .setPlaceholder(DEFAULT_SETTINGS.galaxyFolder)
        .setValue(this.plugin.settings.galaxyFolder)
        .onChange(async value => {
          const folder = value.trim() || DEFAULT_SETTINGS.galaxyFolder;
          this.plugin.settings.galaxyFolder = folder;
          await this.plugin.saveSettings();
        }));
    new Setting(containerEl)
      .setName('Connection')
      .setDesc(this.plugin.descriptor ? `Connected on ${this.plugin.descriptor.base_url}` : 'Disconnected')
      .addButton(button => button.setButtonText('Reconnect').onClick(() => this.plugin.reconnect()));
    this.statusEl = containerEl.createDiv({ cls: 'timekeeper-memory-galaxy-status' });
  }

  setStatus(message) {
    if (this.statusEl) this.statusEl.setText(message);
  }
}

module.exports = TimeKeeperMemoryGalaxyPlugin;
module.exports.DEFAULT_SETTINGS = DEFAULT_SETTINGS;
module.exports.MemoryGalaxySettingTab = MemoryGalaxySettingTab;
