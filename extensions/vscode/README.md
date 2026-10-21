# Time Keeper for VS Code

Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

The official Time Keeper VS Code companion is a thin client for a locally running Time Keeper server. It never opens Time Keeper's SQLite database directly and does not maintain a second task store.

```text
VS Code extension → numeric-loopback Time Keeper HTTP API → SQLite authority
```

## Implemented MVP

- Explorer sidebar showing Projects and their recursive execution hierarchy
- status bar connection state
- workspace-scoped Project selection
- Command Palette connection, selection, refresh, and dashboard commands
- state-aware Sprint actions: start, hold, resume, and complete
- explicit editor references (`file:Lx-Ly`) and Task creation from a selected editor range
- command-only Git status summary through VS Code's built-in Git extension
- read-only tracked file/version browsing with saved versions opened through a virtual read-only text document
- explicit Restore File History Version command, Core optimistic-version checked and guarded by a modal confirmation
- explicit Code Start/Code End history checkpoints for a selected tracked file
- command-only Project Context panel for project authority, execution counts, and workspace Harness/Skill paths
- strict numeric-loopback HTTP configuration validation

## Run locally

1. Start Time Keeper from the repository root:

   ```text
   TIMEKEEPER_GO='/path/to/go' ./scripts/run-local.sh
   ```

2. In VS Code, set `timekeeper.serverUrl` to the exact local server origin. The companion deliberately restricts this setting to numeric loopback origins as a client security boundary; unlike the general CLI/dashboard contact point, its current default remains:

   ```text
   http://127.0.0.1:1618
   ```

   The human/agent-facing default contact point elsewhere is `http://timekeeper.local/`.

3. Open the Explorer view, expand **Time Keeper**, and run **Time Keeper: Connect**.

The extension accepts only `http://127.0.0.1:<port>` or `http://[::1]:<port>` origins. It intentionally refuses hostnames, LAN addresses, HTTPS, paths, credentials, query strings, and fragments because the current Time Keeper server is an unauthenticated local-only API.

## Create work from an editor selection

1. Select a workspace Project with **Time Keeper: Select Project**.
2. Open a file and place the caret or select a range.
3. Use **Time Keeper: Copy Editor Reference** to copy `path:Lx-Ly`, or **Time Keeper: Create Task from Selection** to choose a name, Category, and estimate.

Task creation sends the explicit file/line reference and your chosen Task fields to the local API. It does not send the selected source text, Git data, or unrelated workspace files.

## Project Context panel

**Time Keeper: Show Project Context** opens a read-only panel for the selected Project. It shows the API's project goal, description, status, progress, and execution-tree counts, plus workspace paths for `AGENTS.md`, Harness/Skill files, `.devin` Skills, `.agents` Skills, and `docs/HARNESS.md`. It lists paths only; it does not read or upload file contents.

## File history in the IDE

Use **Time Keeper: Browse File History** to choose an already tracked Project file and saved version. Historical content opens through a read-only `timekeeper-history:` document. If the list is empty, configure that Project's source folder and explicitly run **Scan and index** from the TimeKeeper Files & History workspace first.

**Time Keeper: Restore File History Version** asks for a tracked file and earlier version, then requires a modal confirmation. The extension sends the selected version plus the current version ID to Core; Core performs the restore as a new branch or rejects stale state. The extension never opens or writes SQLite directly, and it never silently scans the workspace.

**Time Keeper: Capture File History Snapshot** records a user-invoked `code_start` or `code_end` boundary for a tracked file. The summary is required; Core reads the file from its configured Project root and writes the immutable version. The extension does not upload source text or watch files in the background.

## Consented Git status

**Time Keeper: Show Git Status** reads the current repository snapshot exposed by VS Code's built-in Git extension and can copy the summary. It reports the branch, upstream ref, local ahead/behind counts when VS Code already has them, and staged/unstaged/merge/untracked counts. The command only runs when invoked; it does not poll in the background, run `git fetch`, send remotes, or include file contents.

## Package locally

```text
npx --yes @vscode/vsce package --allow-missing-repository --out ../../.timekeeper/timekeeper-vscode-0.1.0.vsix
```

Install the resulting `.vsix` through **Extensions: Install from VSIX...**. This repository does not publish the extension to the VS Code Marketplace.

## Verify

```text
npm run lint
npm test
```

## Roadmap and security boundaries

The following are deliberately outside the current security model, not promised features: remote TimeKeeper servers, direct SQLite access, automatic server startup/background timers, and background workspace scanning. Any reconsideration would need a separate security and consent design. All future extension work must preserve the local API as authority and keep workspace access or context submission explicit.
