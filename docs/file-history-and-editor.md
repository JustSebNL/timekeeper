# Project File Explorer and History

The legacy browser dashboard's **File history & editor** panel combines a project-scoped tracked-file browser, source editor, revision preview, compare/restore controls, and a branch-aware revision timeline.

## Timeline semantics

Each immutable file version has two distinct relationships:

- `parent_version_id` is the immediately preceding chronological version. It preserves the complete event order.
- `restores_version_id` identifies the earlier version whose bytes were restored. This creates a side branch in the timeline while the prior line remains intact.

A restore is a new version with `change_kind: reversion`; it never moves the current pointer backward or deletes later versions. The restore target must belong to the same tracked file. Existing history entries whose summary is the legacy `Restored FV-…` form are backfilled to the structured reversion fields during migration.

Every version also has a validated `change_kind`. The initial icon legend includes milestone, bugfix, regular code change, reversion, feature, release, feature end-of-life, refactor, test, documentation, security, performance, dependency, configuration, and other. The legend and dropdown use the same browser-side mapping; the stored enum is the durable classification.

## Capturing and browsing

Choose a Project in the explorer to list its tracked files. Search by relative path or file ID, then open a file to see its current contents and version timeline. Selecting a timeline event previews its exact saved contents. Compare loads the selected saved versions into a read-only, side-by-side CodeMirror MergeView with changed lines highlighted; restore remains a separate explicit action. Restore requires confirmation and appends a new reversion event.

Configure a per-Project source folder with `PUT /api/v1/projects/{projectID}/file-root`, then press **Scan and index** (or call `POST /api/v1/projects/{projectID}/files/scan`). The bounded scanner indexes UTF-8 text files under that root and captures changed contents as immutable versions. It skips symlinks, binary/oversized files, common generated/dependency folders, and the history blob directory. The path is resolved by the server on its own host; the TimeKeeper service account must be able to read and write the folder. Existing tracked records retain the source root they were created against.

## Completeness boundary

The UI shows tracked files and scans the explicitly configured Project root. This is not a continuous filesystem watcher: edits that occur and disappear between scans are not guaranteed to be recorded. Agent integrations should still call the snapshot API at work boundaries. Deleted files are not yet represented by a tombstone event. Do not claim byte-for-byte history for unobserved writes or file deletions.

The scanner is bounded to 10,000 files and 512 MiB total, with a 10 MiB per-file limit. A file omitted by these limits, binary detection, or excluded directories must be reviewed separately. An empty file list can mean the source root is unset or no supported text files have been indexed; it does not prove the Project directory is empty.

## Current client surfaces

- **Legacy browser dashboard:** retains the source editor plus file/version browsing, compare, and restore while migration parity is incomplete.
- **Wails/Svelte Files & History workspace:** configures an explicit per-Project source root, scans only on the user's explicit request, browses immutable versions, compares two saved versions, and restores a selected version as a new branch through Core. It is not yet a source editor.
- **VS Code companion:** offers explicit commands to browse a tracked file's saved versions in a read-only `timekeeper-history:` document, capture a required-summary `code_start`/`code_end` checkpoint, and restore a saved version after modal confirmation. Restore carries the expected current version ID for optimistic concurrency. The extension uses Core APIs only; it does not scan in the background or upload editor contents.

To capture an external change for an already tracked file, use `POST /api/v1/files/{fileID}/snapshots` with `phase: code_start` or `code_end`, a summary, and an optional `change_kind`. The server also needs a valid `-history-root` for content-addressed versions; without it, file-history routes are unavailable.

## Verification contract

- Store migration preserves legacy versions and adds `change_kind` and `restores_version_id` safely.
- Version reads round-trip the event kind and restore target.
- Reversion targets cannot point to a different file.
- UI renders all recorded versions in chronological lanes, puts revert-created branches beside the existing line, and loads selected historical bytes as text.
- A green source test is not proof of live availability: verify the installed server's history-root configuration and the served dashboard assets separately.
