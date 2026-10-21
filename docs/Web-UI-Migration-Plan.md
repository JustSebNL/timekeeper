# TimeKeeper — Wails + Svelte Migration Plan

## 1. Objective

Migrate TimeKeeper from its current increasingly complex browser-oriented UI into a proper desktop application using:

```text
Wails
Go
Svelte
TypeScript
SQLite
```

The migration must **not** rewrite the existing TimeKeeper core.

The goal is to place a modern desktop UI on top of the existing Go architecture while retaining:

* `tk` CLI
* SQLite database
* existing project/task logic
* agent integration
* pulse/heartbeat engine
* Trail of Thought board
* metrics
* Git integration
* Shadowbox integration
* plugin interfaces
* backup systems
* headless operation
* API functionality

The GUI becomes another interface into TimeKeeper rather than becoming TimeKeeper itself.

---

# 2. Target Architecture

```text
                         TIMEKEEPER
                              ¦
                     +-----------------+
                     ¦                 ¦
                TimeKeeper Core       ¦
                     Go               ¦
                     ¦                 ¦
        +------------+------------+    ¦
        ¦            ¦            ¦    ¦
      SQLite      Services      Events ¦
        ¦            ¦            ¦    ¦
        ¦        +---+----+       ¦    ¦
        ¦        ¦   ¦    ¦       ¦    ¦
        ¦      Git Agents Pulse   ¦    ¦
        ¦            ¦            ¦    ¦
        ¦        Shadowbox        ¦    ¦
        ¦                         ¦    ¦
        +-------------------------+    ¦
                     ¦                 ¦
            Application API           ¦
                     ¦                 ¦
       +-------------+-------------+   ¦
       ¦             ¦             ¦   ¦
      CLI          Wails          API  ¦
       ¦             ¦                 ¦
   Hermes/Agents    Svelte              ¦
                     ¦                 ¦
                  Human UI             ¦
                                       ¦
                              Plugins / Integrations
```

The architectural rule is:

> UI code must never contain TimeKeeper business logic.

Svelte displays state and sends commands.

Go remains authoritative.

---

# 3. Recommended Technology Stack

## Backend

```text
Go
SQLite
existing TimeKeeper packages
goroutines
context.Context
```

Where appropriate:

```text
fsnotify       filesystem monitoring
go-git/git CLI Git integration
database/sql   persistence
WebSocket/SSE  remote/event integrations
```

Do not replace existing working packages simply because the GUI is changing.

---

## Desktop Shell

```text
Wails v2
```

Use Wails primarily for:

* desktop window
* Go ? frontend bindings
* event communication
* native dialogs
* file selection
* notifications
* system tray
* application lifecycle
* packaging

Wails must remain an adapter around TimeKeeper.

---

## Frontend

```text
Svelte
TypeScript
Vite
```

Optional:

```text
TailwindCSS
Lucide icons
```

Avoid adding a large UI framework initially.

Develop the TimeKeeper visual language directly from reusable Svelte components.

---

# 4. Repository Structure

Restructure gradually toward:

```text
timekeeper/
¦
+-- cmd/
¦   +-- tk/
¦   ¦   +-- main.go
¦   ¦
¦   +-- timekeeper/
¦       +-- main.go
¦
+-- internal/
¦   ¦
¦   +-- app/
¦   ¦   +-- app.go
¦   ¦   +-- lifecycle.go
¦   ¦   +-- events.go
¦   ¦
¦   +-- project/
¦   +-- category/
¦   +-- task/
¦   +-- sprint/
¦   +-- agent/
¦   +-- pulse/
¦   +-- memory/
¦   +-- totb/
¦   +-- metrics/
¦   +-- shadowbox/
¦   +-- git/
¦   +-- backup/
¦   +-- plugins/
¦   +-- storage/
¦
+-- pkg/
¦   +-- timekeeper/
¦
+-- adapters/
¦   ¦
¦   +-- cli/
¦   +-- api/
¦   +-- wails/
¦
+-- frontend/
¦   ¦
¦   +-- src/
¦   ¦   +-- lib/
¦   ¦   ¦   +-- components/
¦   ¦   ¦   +-- stores/
¦   ¦   ¦   +-- api/
¦   ¦   ¦   +-- utils/
¦   ¦   ¦   +-- types/
¦   ¦   ¦
¦   ¦   +-- views/
¦   ¦   ¦   +-- Dashboard/
¦   ¦   ¦   +-- Projects/
¦   ¦   ¦   +-- Tasks/
¦   ¦   ¦   +-- Agents/
¦   ¦   ¦   +-- Memory/
¦   ¦   ¦   +-- Shadowbox/
¦   ¦   ¦   +-- Metrics/
¦   ¦   ¦   +-- Plugins/
¦   ¦   ¦   +-- Settings/
¦   ¦   ¦
¦   ¦   +-- App.svelte
¦   ¦   +-- main.ts
¦   ¦
¦   +-- package.json
¦
+-- migrations/
+-- tests/
+-- scripts/
¦
+-- go.mod
+-- wails.json
+-- README.md
```

This structure does not need to be created in one commit.

Move packages only when doing so produces a clear architectural benefit.

---

# 5. Migration Principle

Do **not** perform:

```text
OLD TIMEKEEPER
      ?
 delete everything
      ?
NEW TIMEKEEPER
```

Instead:

```text
                    Existing TimeKeeper
                           ¦
              +--------------------------+
              ¦                          ¦
          Existing UI              New Wails UI
              ¦                          ¦
              +--------------------------+
                           ¦
                      Same Go Core
                           ¦
                         SQLite
```

Both interfaces should temporarily work against the same underlying system.

Only remove the old UI when feature parity has been reached.

---

# 6. Phase 0 — Freeze the Architecture

Before changing UI code, identify:

### Business logic

Anything responsible for:

* creating projects
* creating tasks
* status transitions
* starting/stopping work
* timers
* sprints
* heartbeat handling
* crash recovery
* database operations
* agent registration
* token accounting
* metrics
* ToTb
* Shadowbox
* backups

must live outside UI code.

Create a list:

```text
UI-only
Business logic
Storage logic
Agent logic
System/infrastructure logic
```

Anything currently sitting inside web handlers or JavaScript that belongs in business logic must first be moved into Go.

### Deliverable

```text
docs/architecture/current-system.md
```

Document:

* packages
* dependencies
* database access
* HTTP endpoints
* UI dependencies
* global state
* background workers
* agent interfaces

---

# 7. Phase 1 — Establish a TimeKeeper Application Service

Create a central application layer.

Example:

```go
type TimeKeeper struct {
    Projects  *project.Service
    Tasks     *task.Service
    Agents    *agent.Service
    Memory    *memory.Service
    Metrics   *metrics.Service
    Shadowbox *shadowbox.Service
    Events    *events.Bus
}
```

Both CLI and GUI use this object.

Do not let Wails instantiate separate implementations.

Target:

```text
             TimeKeeper
                 ¦
       +---------+---------+
       ¦         ¦         ¦
      CLI       GUI       API
```

rather than:

```text
CLI implementation

GUI implementation

API implementation
```

---

# 8. Phase 2 — Introduce an Internal Command Layer

Create clean commands for UI operations.

Examples:

```go
CreateProject()
UpdateProject()
DeleteProject()

CreateTask()
StartTask()
PauseTask()
ResumeTask()
CompleteTask()
CancelTask()

CreateSprint()
CloseSprint()

PostToTBMessage()

StartAgent()
StopAgent()

CreateShadowbox()
StopShadowbox()

SearchMemory()

GetMetrics()
```

The UI should never manipulate SQLite directly.

Bad:

```text
Svelte
  ?
UPDATE tasks SET status...
```

Correct:

```text
Svelte
  ?
StartTask("T-1042")
  ?
TimeKeeper task service
  ?
validation
  ?
SQLite
  ?
event
```

---

# 9. Phase 3 — Introduce the TimeKeeper Event Bus

The new GUI will require considerably more live state than the old interface.

Introduce an internal event system.

Example events:

```text
project.created
project.updated

task.created
task.started
task.paused
task.completed
task.failed

sprint.started
sprint.completed

agent.connected
agent.started
agent.idle
agent.working
agent.stopped

pulse.received
pulse.missed

totb.created

memory.created
memory.updated

shadowbox.started
shadowbox.stopped

git.changed

metrics.updated
```

Example:

```go
type Event struct {
    Type      string
    EntityID  string
    Timestamp time.Time
    Data      any
}
```

The same bus can eventually feed:

```text
Wails UI
WebSocket clients
plugins
logging
Needle
remote TimeKeeper clients
```

This becomes an important piece of TimeKeeper infrastructure.

---

# 10. Phase 4 — Add Wails Without Replacing Anything

Create the Wails desktop application.

Initially expose only:

```go
GetVersion()
GetSystemStatus()
GetProjects()
GetActiveTasks()
```

Target:

```text
Wails launches
     ?
Svelte loads
     ?
calls Go
     ?
receives real TimeKeeper data
```

Do not add editing yet.

This phase proves:

* Wails packaging
* Svelte development flow
* database access
* application startup
* Go bindings
* generated frontend models

---

# 11. Phase 5 — Create the New Application Shell

Build the permanent UI shell before migrating individual screens.

Recommended layout:

```text
+--------------------------------------------------------------+
¦ TimeKeeper            Search         Agent ? DB ? Git ?      ¦
+--------------------------------------------------------------¦
¦             ¦                                                ¦
¦ Dashboard   ¦                                                ¦
¦ Projects    ¦                                                ¦
¦ Tasks       ¦                 WORKSPACE                      ¦
¦ Agents      ¦                                                ¦
¦ Memory      ¦                                                ¦
¦ Shadowbox   ¦                                                ¦
¦ Metrics     ¦                                                ¦
¦ Plugins     ¦                                                ¦
¦             ¦                                                ¦
¦ Settings    ¦                                                ¦
+--------------------------------------------------------------¦
¦ Active: T-1042 · Cipher-03 · 00:42:18 · Pulse ?             ¦
+--------------------------------------------------------------+
```

Permanent areas:

### Top bar

* global search
* connection status
* active agent
* Git state
* database state
* notifications

### Sidebar

* Dashboard
* Projects
* Tasks
* Agents
* Memory
* Shadowbox
* Metrics
* Plugins
* Settings

### Main workspace

Changes depending on current context.

### Status bar

Display:

```text
active project
active task
timer
active agent
pulse
token use
background jobs
```

---

# 12. Phase 6 — Build the Svelte Component System

Before recreating every feature, build reusable primitives.

## Core components

```text
Button
IconButton
Badge
Input
Select
Checkbox
Modal
Drawer
Tooltip
ContextMenu
Tabs
Tree
Table
VirtualList
ProgressBar
StatusIndicator
SearchBox
SplitPane
Toast
```

TimeKeeper-specific components:

```text
ProjectTree
TaskTree
TaskCard
SprintCard
AgentBadge
AgentActivity
PulseIndicator
Timer
TokenCounter
ToTBMessage
ShadowboxStatus
GitStatus
MemoryItem
MetricCard
```

Do not duplicate UI structures per page.

---

# 13. Phase 7 — Svelte State Model

Do not create one gigantic global store.

Suggested stores:

```text
projects
tasks
agents
activeTask
workspace
memory
shadowbox
metrics
notifications
settings
```

Example conceptual flow:

```text
Go event

task.started
     ?
Wails event bridge
     ?
task store
     ?
UI components automatically update
```

Avoid frontend polling wherever possible.

---

# 14. Phase 8 — Project Workspace

Migrate Projects first.

Target hierarchy:

```text
Project
+-- Category
    +-- Task
        +-- Sprint
        +-- Subtask
            +-- Sprint
```

The UI should support:

* tree expand/collapse
* filtering
* search
* drag/drop where appropriate
* context menus
* keyboard navigation
* status icons
* progress
* active indicators

Selecting an entity opens it in the workspace.

Example:

```text
Project: TimeKeeper

Overview
Tasks
ToTb
Memory
Agents
Files
Git
Metrics
Settings
```

---

# 15. Phase 9 — Task Workspace

A task should become a first-class workspace.

Example:

```text
T-1042 — Implement agent event stream

Status          Active
Project         TimeKeeper
Category        Agent Runtime
Progress        62%
Started         13:42
Worked          02:18:32
Agent           Cipher-03
Model           ...
```

Tabs:

```text
Overview
Sprints
Activity
Agents
Files
History
ToTb
Metrics
```

Actions:

```text
Start
Pause
Resume
Stop
Complete
Fail
Cancel
```

All actions call Go commands.

---

# 16. Phase 10 — Agent Control Center

This should be a major part of the new interface.

Example:

```text
AGENTS

Cipher-01      WORKING
Project        TimeKeeper
Task           T-1042
Runtime        01:42:18
Pulse          2s ago
Tokens         83,421

Cipher-02      IDLE
Cipher-03      SHADOWBOX
Cipher-04      OFFLINE
```

Provide:

* status
* current project
* current task
* model
* provider
* token consumption
* estimated cost
* last heartbeat
* task history
* recent actions

Do not expose hidden model reasoning to other agents.

Logging and audit information should remain separately controlled.

---

# 17. Phase 11 — Trail of Thought Board

Preserve the existing project-level ToTb concept.

Improve the interface around it.

```text
TimeKeeper / ToTb

[Agent] Cipher-03
14:33

Database migration completed.
Remaining issue is restoring active heartbeat state...

--------------------------

[Human]
14:36

Don't change database IDs during migration.
```

Keep the existing maximum message size unless deliberately changed elsewhere.

Support:

* project filtering
* agent filtering
* timestamps
* Post ID
* copying addresses
* search
* jump-to-task

---

# 18. Phase 12 — Shadowbox Integration

Build Shadowbox as a dedicated workspace.

```text
Shadowbox

SB-104
Status       Running
Task         T-1042
Agent        Cipher-03
Created      14:03

Containers

Original
Modified

Diff
History
Processes
Files
Logs
```

Do not embed Shadowbox logic into the frontend.

The UI calls:

```text
ShadowboxService
```

through the Go core.

---

# 19. Phase 13 — Memory Galaxy

Treat Memory Galaxy as a separate subsystem.

UI capabilities:

```text
Search
Browse
Filter
Link
Inspect
Tag
Project associations
Task associations
Agent associations
Source/provenance
```

Conceptual interface:

```text
Memory Galaxy
¦
+- Long Horizon
+- RAG
+- Knowledge
+- Lessons
+- Skills
+- Harnesses
+- Cross-project
```

Do not tightly couple Memory Galaxy navigation to project navigation.

Cross-project discovery is one of its primary purposes.

---

# 20. Phase 14 — Metrics

Create a proper metrics workspace.

Include existing metrics such as:

```text
total work time
sessions
tasks completed
tokens
estimated cost
agent performance
model performance
```

Agent/model ranking:

```text
Best 5
Worst 5
Economy class
```

Track:

```text
token efficiency
cost
completion time
bug corrections
failure rate
```

Charts should be computed from Go/SQLite data.

The browser should only render them.

---

# 21. Phase 15 — System Tray and Background Mode

Desktop TimeKeeper should be able to continue running without its main window open.

Tray menu example:

```text
TimeKeeper

Active: T-1042
Worked: 01:38:17

Open TimeKeeper
Pause Task
Stop Task
Agents
-------------
Exit
```

Closing the main window should optionally:

```text
hide application
```

rather than:

```text
terminate TimeKeeper
```

This is important because the core may be supervising:

* agents
* timers
* heartbeats
* Shadowboxes
* backups
* file monitoring

---

# 22. Phase 16 — Startup and Recovery

Preserve TimeKeeper's recovery behavior.

Startup flow:

```text
Launch TimeKeeper
      ?
Open SQLite
      ?
Check previous session
      ?
Clean shutdown?
   +----------------+
  YES               NO
   ¦                 ¦
normal start     crash recovery
                     ¦
              inspect state
                     ¦
              restore services
```

Recover:

* active project
* active task
* timers
* agent assignments
* pulse state
* Shadowbox references
* pending work
* previous UI workspace

UI state should be secondary to core state.

---

# 23. Phase 17 — CLI Compatibility

This is a hard requirement.

The following must continue working without Wails:

```text
tk project ...
tk task ...
tk sprint ...
tk agent ...
tk tracker list
```

The application architecture must permit:

```text
tk
```

to operate independently of:

```text
timekeeper.exe
```

where technically possible.

Both use:

```text
same core
same rules
same models
same database
```

---

# 24. Phase 18 — Agent Compatibility

Agents must not have to interact with the graphical interface.

Preferred:

```text
Hermes
  ?
tk CLI
  ?
TimeKeeper Core
```

or:

```text
Hermes
  ?
TimeKeeper API
  ?
TimeKeeper Core
```

Not:

```text
Hermes
  ?
UI automation
  ?
Wails
```

The UI exists primarily for humans.

---

# 25. Phase 19 — Plugin Boundary

Do not make Wails itself the plugin system.

Plugins should attach to the TimeKeeper core.

```text
                TimeKeeper Core
                     ¦
         +-----------+-----------+
         ¦           ¦           ¦
      Wails        CLI        Plugins
                                 ¦
                     +-----------+----------+
                   Obsidian    Backup     Future
```

The UI can expose plugin configuration and status.

Plugin execution belongs to Go.

---

# 26. Phase 20 — Database Rules

Do not redesign SQLite as part of the UI migration.

Database changes should only happen where necessary to support new functionality.

Introduce proper migrations if not already present:

```text
migrations/
001_initial.sql
002_agents.sql
003_memory.sql
...
```

Wails should never contain database-specific assumptions.

---

# 27. Phase 21 — Testing

Create three levels of tests.

## Core tests

```text
Go unit tests
```

Test:

* task transitions
* IDs
* timers
* recovery
* projects
* agents
* sprints
* metrics

## Integration tests

Test:

```text
command
 ?
service
 ?
database
 ?
event
```

## UI tests

Test only UI behavior.

Do not use UI tests to prove core business rules.

---

# 28. Phase 22 — Performance Requirements

Set explicit targets.

Suggested:

```text
Cold startup       < 2 seconds
Idle CPU           ~0%
Idle RAM           < 150 MB target
UI response        < 100 ms
DB operations      non-blocking UI
Agent events       < 250 ms UI update
```

These are targets rather than architectural assumptions.

Measure them before removing the old frontend.

---

# 29. Phase 23 — Migration Feature Matrix

Maintain:

```text
docs/migration-ui.md
```

Example:

| Feature   | Old UI | Wails UI | Core API | Tested |
| --------- | ------ | -------- | -------- | ------ |
| Projects  | ?      | ?        | ?        | ?      |
| Tasks     | ?      | ?        | ?        | ?      |
| Sprints   | ?      | ?        | ?        | ?      |
| ToTb      | ?      | ?        | ?        | ?      |
| Agents    | ?      | ?        | ?        | ?      |
| Pulse     | ?      | ?        | ?        | ?      |
| Metrics   | ?      | ?        | ?        | ?      |
| Shadowbox | ?      | ?        | ?        | ?      |
| Memory    | ?      | ?        | ?        | ?      |

The old UI cannot be retired until required features reach parity.

---

# 30. Phase 24 — Remove Old UI

Only after parity:

```text
Old web UI
     ?
mark deprecated
     ?
one release cycle
     ?
remove
```

Do not immediately delete old code.

Tag the last release containing the old interface.

Example:

```text
legacy-ui-final
```

This gives a clean rollback point.

---

# 31. Migration Branch Strategy

Recommended:

```text
main
¦
+-- migration/wails
¦
+-- feature/wails-shell
+-- feature/project-workspace
+-- feature/task-workspace
+-- feature/agent-workspace
+-- ...
```

Avoid a migration branch containing months of unrelated work.

Merge infrastructure changes incrementally where safe.

---

# 32. Implementation Order

Recommended exact order:

```text
01 Audit current architecture

02 Extract remaining UI business logic into Go

03 Establish shared TimeKeeper application service

04 Establish commands

05 Establish event bus

06 Add Wails

07 Add Svelte shell

08 Connect read-only projects

09 Connect task data

10 Project workspace

11 Task workspace

12 Editing/actions

13 Live events

14 Agent center

15 Pulse/heartbeat

16 ToTb

17 Metrics

18 Shadowbox

19 Memory Galaxy

20 Plugins

21 Settings

22 Tray/background mode

23 Startup recovery

24 Packaging

25 Feature parity audit

26 Deprecate old UI
```

---

# 33. Things NOT to Rewrite During Migration

Unless existing code is genuinely broken, do not simultaneously replace:

```text
SQLite
ID system
CLI
task model
project model
agent protocol
pulse protocol
Shadowbox protocol
Memory Galaxy architecture
plugin architecture
backup architecture
```

Every extra rewrite massively increases migration risk.

The project should primarily be:

> Replace the presentation/application shell while cleaning the boundary around the existing Go core.

---

# 34. Wails Version Strategy

Start the production migration using:

```text
Wails v2 stable
```

Structure the Wails adapter so that framework-specific code remains isolated:

```text
adapters/wails/
```

When Wails v3 reaches stable GA and has proven suitable:

```text
Wails v2
    ?
adapter migration
    ?
Wails v3
```

The core should not notice this migration.

This prevents TimeKeeper from becoming coupled to a beta desktop framework.

---

# 35. Definition of Done

The migration is finished when:

### Core

* TimeKeeper core runs independently of GUI.
* CLI retains full functionality.
* Existing SQLite databases load without conversion failures.
* Existing IDs remain unchanged.
* Business logic is independent of Svelte/Wails.

### Desktop

* TimeKeeper runs as native desktop application.
* Closing/minimizing behavior is correct.
* Background services remain operational.
* Tray functionality works.
* Crash recovery works.
* Application can restore previous workspace.

### UI

* Project hierarchy works.
* Tasks and subtasks work.
* Sprints work.
* ToTb works.
* Agents are visible.
* Agent activity updates live.
* Metrics work.
* Shadowbox works.
* Memory Galaxy works.
* Plugins can expose configuration/status.

### Compatibility

* `tk` CLI works.
* agents can work without GUI.
* API clients continue working.
* existing TimeKeeper databases work.

### Quality

* no UI direct SQLite access
* no duplicated business rules
* no hidden second TimeKeeper implementation in Wails
* no dependency on GUI for background execution

---

# 36. Final Architecture

The finished system should look like:

```text
                           TIMEKEEPER

                       +-------------+
                       ¦    CORE     ¦
                       ¦     GO      ¦
                       +-------------+
                              ¦
            +-----------------+-----------------+
            ¦                 ¦                 ¦
         Storage          Services           Events
         SQLite              ¦                 ¦
                     +-------+-------+         ¦
                     ¦       ¦       ¦         ¦
                   Tasks   Agents  Memory      ¦
                     ¦       ¦       ¦         ¦
                   Pulse Shadowbox Plugins     ¦
                              ¦                 ¦
                              +-----------------+
                                       ¦
                             Application Layer
                                       ¦
                  +--------------------+-------------------+
                  ¦                    ¦                   ¦
                 CLI                  API                Wails
                  ¦                    ¦                   ¦
               Agents             Integrations          Svelte
                                                          ¦
                                                         User
```

The important result is not merely:

> TimeKeeper uses Wails.

It is:

> **TimeKeeper has one Go core with multiple interchangeable interfaces.**

That gives the project room to grow without allowing the GUI framework to dictate the architecture.
