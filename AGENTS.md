# Development Rules

Abah is the user and sole author of this repo.

## Agent skills

### Workflow

Route every task that changes this repo's code or docs through Matt Pocock's skills. `ask-matt` is the router: read it before starting, pick the stage the task sits at, then run that stage's skill.

- **Running a skill**: the ones Abah normally types (`ask-matt`, `grill-with-docs`, `to-spec`, `to-tickets`, `implement`, `implement-spec`, `triage`, `wayfinder`, `improve-codebase-architecture`, `handoff`): read `~/.claude/plugins/cache/mattpocock/mattpocock-skills/<highest version>/skills/*/<name>/SKILL.md` and follow it as if he had typed the command. All others: load them through your agent's skill mechanism.
- **Entry point by size**: small fix, one file, obvious behaviour: `tdd` (or `diagnosing-bugs` for a bug), then `code-review`. Multi-file feature or design change: `grill-with-docs`, then the main flow. Foggy effort spanning sessions: `wayfinder`.
- **Overrides** (this file wins where a skill disagrees):
  - `implement` / `implement-spec`: commit only when Abah asks; run the affected tests, leave the full suite to Abah.
  - `to-spec`, `to-tickets`, `triage`: draft each issue, show it, publish after Abah approves.
  - Between tickets, dispatch each one to a subagent (`implement-spec`) in place of `/clear`.
  - `retro` runs only when Abah types it.
- **Shared checkout**: when another agent may be using this checkout, work in your own `git worktree` off `origin/main` and leave its uncommitted changes in place.
- **Attribution**: Abah is the sole author of commits, PRs, issues and comments; add no agent credit lines.

### Issue tracker

Issues and specs live as GitHub issues (`H4fizWasabie/cave-lms`), managed via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default vocabulary: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one root `GLOSSARY.md` + `docs/adr/`. See `docs/agents/domain.md`.
