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
- strict numeric-loopback HTTP configuration validation

## Run locally

1. Start Time Keeper from the repository root:

   ```text
   TIMEKEEPER_GO='/path/to/go' ./scripts/run-local.sh
   ```

2. In VS Code, set `timekeeper.serverUrl` to the exact local server origin. The default is:

   ```text
   http://127.0.0.1:1618
   ```

3. Open the Explorer view, expand **Time Keeper**, and run **Time Keeper: Connect**.

The extension accepts only `http://127.0.0.1:<port>` or `http://[::1]:<port>` origins. It intentionally refuses hostnames, LAN addresses, HTTPS, paths, credentials, query strings, and fragments because the current Time Keeper server is an unauthenticated local-only API.

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

## Deliberately not present yet

- code references / create Task from editor selection
- Git integration
- Project Authority prompt, Harness, Skill, and code-context UI
- remote Time Keeper server support
- direct SQLite access
- automatic server startup or background timers

Those features must preserve the local API authority and explicit user-consent boundaries.
