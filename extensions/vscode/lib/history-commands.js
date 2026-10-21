// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

'use strict';

const { historyFileChoices, historyVersionChoices } = require('./history');

function createHistoryCommands({ vscode, getClient, getSelectedProjectID, historyDocuments, refreshTree, reportError }) {
  async function chooseHistoryFile() {
    const projectID = getSelectedProjectID();
    if (!projectID) {
      vscode.window.showErrorMessage('Time Keeper: select a Project first.');
      return null;
    }
    const response = await getClient().projectFiles(projectID);
    const files = response.items || [];
    if (files.length === 0) {
      vscode.window.showInformationMessage('Time Keeper: no tracked files yet. Configure a Project source folder and explicitly scan it from Files & History.');
      return null;
    }
    const choice = await vscode.window.showQuickPick(historyFileChoices(files), {
      placeHolder: `Choose a tracked file in ${projectID}`,
      matchOnDescription: true,
    });
    return choice?.file || null;
  }

  async function chooseHistoryVersion(file, includeCurrent = true) {
    const response = await getClient().fileVersions(file.file_id);
    const versions = response.items || [];
    if (versions.length === 0) {
      vscode.window.showInformationMessage(`Time Keeper: ${file.current_path} has no saved versions.`);
      return null;
    }
    const choices = historyVersionChoices(file, versions)
      .filter(choice => includeCurrent || choice.version.version_id !== file.current_version_id);
    if (choices.length === 0) {
      vscode.window.showInformationMessage(`Time Keeper: ${file.current_path} has no earlier version to restore.`);
      return null;
    }
    return vscode.window.showQuickPick(choices, {
      placeHolder: `${includeCurrent ? 'Open' : 'Restore'} a saved version of ${file.current_path}`,
      matchOnDescription: true,
      matchOnDetail: true,
    });
  }

  async function browseFileHistory() {
    try {
      const file = await chooseHistoryFile();
      if (!file) return;
      const choice = await chooseHistoryVersion(file, true);
      if (!choice) return;
      const saved = await getClient().fileVersion(file.file_id, choice.version.version_id);
      const uri = historyDocuments.open(file, saved.version, saved.content);
      const document = await vscode.workspace.openTextDocument(uri);
      await vscode.window.showTextDocument(document, { preview: true });
    } catch (error) {
      reportError(error);
    }
  }

  async function restoreFileHistoryVersion() {
    try {
      const file = await chooseHistoryFile();
      if (!file) return;
      if (!file.current_version_id) {
        vscode.window.showErrorMessage(`Time Keeper: ${file.current_path} has no current version; refresh or scan the Project first.`);
        return;
      }
      const choice = await chooseHistoryVersion(file, false);
      if (!choice) return;
      const confirmation = await vscode.window.showWarningMessage(
        `Restore ${file.current_path} to V${choice.version.version_number} as a new branch? The current file will change; all history remains preserved.`,
        { modal: true },
        'Restore as new branch',
      );
      if (confirmation !== 'Restore as new branch') return;
      const result = await getClient().restoreFileVersion(file.file_id, choice.version.version_id, file.current_version_id);
      refreshTree();
      vscode.window.showInformationMessage(`Time Keeper restored ${file.current_path} as ${result.version.version_id}.`);
    } catch (error) {
      reportError(error);
    }
  }

  async function captureFileHistorySnapshot() {
    try {
      const file = await chooseHistoryFile();
      if (!file) return;
      const phase = await vscode.window.showQuickPick([
        { label: 'Code start — before a deliberate change', phase: 'code_start' },
        { label: 'Code end — after a deliberate change', phase: 'code_end' },
      ], { placeHolder: `Choose a history boundary for ${file.current_path}` });
      if (!phase) return;
      const summary = await vscode.window.showInputBox({
        prompt: 'Describe this explicit IDE history checkpoint',
        placeHolder: 'For example: Add retry handling to the API client',
        ignoreFocusOut: true,
      });
      if (summary === undefined) return;
      if (!summary.trim()) {
        vscode.window.showErrorMessage('Time Keeper: enter a short checkpoint summary.');
        return;
      }
      const result = await getClient().captureFileSnapshot(file.file_id, {
        phase: phase.phase,
        change_summary: summary.trim(),
        change_kind: 'code_change',
      });
      refreshTree();
      vscode.window.showInformationMessage(`Time Keeper captured ${file.current_path} as ${result.version.version_id}.`);
    } catch (error) {
      reportError(error);
    }
  }

  return { browseFileHistory, captureFileHistorySnapshot, restoreFileHistoryVersion };
}

module.exports = { createHistoryCommands };
