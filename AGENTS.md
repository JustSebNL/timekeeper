# TimeKeeper agent instructions

When the user says “Use TimeKeeper in the terminal and start using the app,” treat that as authorization to bootstrap the workflow. Do not ask the user to explain the steps again.

From the repository root, do this automatically before project work:

1. Read `harness/SKILL.md`.
2. On first use, read `harness/skills/onboarding/SKILL.md`; otherwise read the one Skill relevant to the next action under `harness/skills/`.
3. Locate the local CLI: on Linux/WSL prefer `.timekeeper/app/tk`, then `.timekeeper/app/bin/tk`; on Windows use `.timekeeper/app/bin/tk.exe`. Fall back to `tk` on `PATH` only when no local CLI is present.
4. Run `<cli> doctor` against the local TimeKeeper service.
5. If health fails, follow the failure-only recovery procedure in `harness/SKILL.md`, then run `doctor` once more. Never start a second server or database.
6. Run `<cli> list`, inspect the relevant `<cli> tree <project-id>` and `<cli> summary <project-id>`, and resume existing work when present.
7. If no suitable record exists, create the Project/Category/Task and one bounded Sprint for the next concrete action.
8. Use TimeKeeper notes or Trail of Thought checkpoints for decisions, blockers, evidence, and handoffs.
9. Verify real work before completing a Sprint; never treat model text alone as proof.

Keep the active context compact by retaining IDs and M-refs rather than copying long history. Never store credentials, private prompts, or unbounded logs in TimeKeeper.

References:

- First-use quick guide: `docs/HARNESS.md`
- Operational harness contract: `harness/SKILL.md`
- Project-local Skills: `harness/skills/`
- Full API/integration contract: `AGENT_INTEGRATION.md`
