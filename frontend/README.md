# TimeKeeper Svelte UI

This Svelte 5 + TypeScript UI is a read-only client of the existing TimeKeeper Core. It provides Overview and Projects workspaces, reads Project execution trees (Categories, Tasks, Subtasks, Sprints) from Core, and keeps the current supported workspace/selected Project in WebView local storage so a desktop restart can restore that view. Unsupported navigation destinations remain visibly disabled. The UI never opens SQLite or implements Core business rules.

The Project filter can be focused with `/` from non-editable areas and cleared with `Esc` while focused. Neither shortcut intercepts unrelated editable controls; modified/composition keys are ignored.

## Run locally

From this directory:

```powershell
npm install
npm run dev
```

Vite proxies `/health` and `/api` to `http://timekeeper.local` by default. Set `TIMEKEEPER_CORE_ORIGIN` in the environment to target another Core URL. The browser uses same-origin relative API paths, so the development server avoids a separate CORS configuration.

When hosted by Wails, the API client uses the desktop's read-only `Health`, `Projects`, and `ProjectExecutionTree` Go bindings instead of browser HTTP. Those bindings call the same loopback `CoreClient` used by the desktop lifecycle adapter; the UI still has no database access or write commands.

## Build in Wails

From `cmd/desktop`, run the pinned Wails v2 CLI build:

```powershell
go run github.com/wailsapp/wails/v2/cmd/wails@v2.15.0 build -m -nopackage -nosyncgomod
```

The Wails config installs the frontend from `frontend/package-lock.json`, runs the Vite production build, and embeds its output into `cmd/desktop/assets/`. A standalone `npm run build` from this directory updates those embedded assets too.

## Verify

```powershell
npm test
npm run check
npm run build
```

Overview reads Core health and the project list; Projects can open a read-only Category/Task/Subtask/Sprint hierarchy. Remaining navigation entries are placeholders until their mapped migration tasks begin. Wails ownership and production asset embedding are tracked separately from later native-feature and packaging tasks.
