# Documentation index

New to TimeKeeper? Start with the [README](../README.md), then follow [Installation](install.md). This index groups the rest of the repository's guides by what you are trying to do.

The repository currently holds detailed command, API, and operator references. A combined user-facing documentation and plugin Store portal at `time.mngr.eu.org` is planned but is not implemented in this checkout. Keep these references until the portal is published and its content and links are verified.

The repository contains both operating guides and engineering notes. Documents under **Design, audit, and history** describe proposals, implementation boundaries, or dated evidence; they are not promises that every optional feature is available in a release. Check each document's status note and the current CLI/API before relying on a feature.

## Get started and use TimeKeeper

- [README](../README.md) — what TimeKeeper does, who it may help, and a first project workflow.
- [Installation](install.md) — Windows/Linux service paths, the macOS build-only path, verification, and uninstall behavior.
- [CLI help](../HELP.md) — terminal commands, examples, service operations, and current limits.
- [Friendly local URLs](friendly-url.md) — primary `timekeeper.local` contact point, loopback fallback, and troubleshooting.
- [Administrator and groups](administrator.md) — local user/group permissions and the important authentication boundary.
- [Security review](../THREAT_REVIEW.md) — the local-only security model and accepted limitations.

## Agents and work continuity

- [Agent first-use guide](HARNESS.md) — the bounded workflow for an agent using a TimeKeeper checkout.
- [Agent harness router](agent.md) — choose the right role Skill and see how planning, investigation, fixing, mining, review, and autonomous delivery log into TimeKeeper.
- [Agent instructions](../AGENTS.md) and [harness contract](../harness/SKILL.md) — repository instructions and operational rules.
- [Agent/API integration](../AGENT_INTEGRATION.md) — framework and direct HTTP integration.
- [Agent wake adapter protocol](agent-wake-adapter-protocol.md) — how a trusted runtime adapter can receive a bounded continuation request.
- [Anti Slack Engine](anti-slack-engine.md) — optional Guardian/wake workflow and its limits.
- [Current Guardian input flow](current-guardian-input-flow.md) — deployment-specific walkthrough for the configured agent-side input path; not a generic setup guide.
- [Guardian self-nudge](guardian-self-nudge.md) — design and implementation notes for the local receiver.
- [Execution safeguards](execution-safeguards.md) — supervised Codex runs, limits, evidence, and usage attribution.
- [Queue logic](queue-logic.md) — Task/Sprint states, eligibility, and queue rules.
- [Batch import format](batch-import-format.md) — canonical Task batch schemas for CSV, TSV, XLSX, JSON, YAML, SQL, and archives, with the validation-before-apply gate and unsupported formats.
- [Import and export](import-export.md) — multi-format batch task import, task export, and full project import/export.
- [FASE identity](fase-identity-design.md) — identity design and implementation/deployment status; identity is attribution, not authentication.

Project-local agent Skills are under [`harness/skills/`](../harness/skills/): [onboarding](../harness/skills/onboarding/SKILL.md), [autonomous coder](../harness/skills/autonomous-coder/SKILL.md), [project landscape](../harness/skills/project-landscape/SKILL.md), [project planner](../harness/skills/project-planner/SKILL.md), [backlog sync](../harness/skills/backlog-sync/SKILL.md), [codebase audit](../harness/skills/codebase-audit/SKILL.md), [bug hunter](../harness/skills/bug-hunter/SKILL.md), [bug fixer](../harness/skills/bug-fixer/SKILL.md), [deep audit/fixer](../harness/skills/deep-audit-fixer/SKILL.md), [gem mining](../harness/skills/gem-mining/SKILL.md), [code reviewer](../harness/skills/code-reviewer/SKILL.md), [timeboxing](../harness/skills/timeboxing/SKILL.md), [service recovery](../harness/skills/service-recovery/SKILL.md), and [troubleshooting](../harness/skills/troubleshooting/SKILL.md).

All harness documents are also available through the shared HTML viewer at `http://timekeeper.local/harness.html`. The menu is built dynamically from files under `harness/`; each item loads its Markdown/YAML source into a read-only textarea with a **Copy file** button. `tk harness list` prints direct selected-item HTML links (`http://timekeeper.local/harness.html?item=<id>`), while `tk harness show <id>` prints the raw source in the terminal.

## API and integrations

- [Project lifecycle and deadlines](project-lifecycle-and-deadlines.md) — Project phases, planned/actual dates, deadline state, milestone contracts, closeout evidence, and agent CLI/API.
- [Project file history and editor](file-history-and-editor.md) — tracked-file browser, typed revision events, revert branches, and the current capture boundary.
- [HTTP API reference](../API.md) — routes, local security boundary, project records, Sprint lifecycle, messages, usage, planning, and optional providers.
- [Bug reporter API](api-bug-reporter.md) — bounded defect submissions for TimeKeeper alpha testing.
- [Model pricing](model-pricing.md) — server-owned, optional model usage price catalogue.
- [VS Code companion](../extensions/vscode/README.md) — local extension setup, commands, and development checks.
- [Obsidian Memory Galaxy specification](TimeKeeper—Obsidian-Memory-Galaxy-Implementation.md) — provider and integration scope; check the implementation status before using it.
- [Jev/TypeSafe integration concept](jev-typesafe-timekeeper-integration.md) — original exploration; not an enabled product integration.
- [Jev Category/Sprint decision protocol](jev-category-sprint-decision-protocol.md) — implementation blueprint; Jev routing is not enabled by this document.

## Extensions and Store

These guides are for extension developers and operators, not prerequisites for using local project tracking. The Store client is optional, its default URL is a reserved placeholder, and staged packages are not executed by Core.

- [Store documentation hub](store.md)
- [Extension model](extensions.md)
- [Store API](store-api.md)
- [Store authentication](auth-protocol.md)
- [Store developer guide](store-developer-guide.md)
- [Store system architecture](store-system-architecture.md)
- [Package format](package-format.md)
- [Plugin architecture](plugin-architecture.md)
- [Plugin candidate triage](plugin-candidate-triage.md) — approved, deferred, and non-goal extension areas.
- [Plugin security scanning](plugin-security-scanning.md)
- [Two-box architecture](two-box-architecture.md)
- [PicoClaw Needle plugin notes](picoclaw-needle-plugin.md)
- [PicoClaw strip audit](picoclaw-strip-audit.md)

## Design, audit, and history

These are engineering records rather than first-use instructions. Their status sections distinguish implemented behavior from proposals and historical verification.

- [Updated Wails, Svelte, and Store migration plan](updated-Web-UI-Migration-Plan.md) — current phased migration gates; the original Web UI plan is preserved separately as historical input.
- [Local SQLite shadow copies](shadow-copy.md) — verified A/B snapshots, primary health gating, and offline restore safety.
- [Guardian control-plane/port consolidation](guardian-port-consolidation.md) — architecture decision and staged migration plan.
- [Rollout evidence (2026-09-08)](rollout-2026-09-08.md) — dated installation and verification record, not current service status.
- [Implementation plans](superpowers/plans/2026-09-26-fase-identity.md) — FASE identity work plan and checkpoints.
- [ASE completion ledger](superpowers/plans/ase-completion-ledger.md) — implementation tracking notes.

## For contributors

- [Build a Windows candidate without installing](build-windows-candidate.md) — run tests and stage the CLI/Core without changing the installed service or database.

From the repository root, run `go test ./...` and `go vet ./...` for the Go source, `node --test web/trail_of_thought_board.test.mjs web/file_editor_history.test.mjs` for dashboard behavior, and `npm test --prefix web/editor` for the CodeMirror adapter contract. The installed app and the source checkout may be different versions; do not assume a successful local health check means every source change is deployed.

For project-level agent conventions, see [TIMEKEEPER.md](../TIMEKEEPER.md). For the repository's operational contract, see [harness/SKILL.md](../harness/SKILL.md).
