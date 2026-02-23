# Codex-Inspired Features — Design

## Context

OpenAI's Codex desktop app ships four UX patterns that Hivemind should match:
**Review Queue**, **Inline Diff Comments**, **Skills / Task Templates**, and **Automations**.
This document is the design spec for all four.

Hivemind is exceptionally well-positioned: workflow DAGs, brain IPC, tmux send-keys, and
`ui/overlay/` (TextInputOverlay, PickerOverlay) are already shipped. The new work is
primarily additive.

---

## Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Skills storage | `~/.hivemind/skills/` only (global) | Works across all repos, never needs gitignore, shared across projects |
| Review Queue trigger | Automation-triggered instances only | Avoids noise from interactive sessions |
| Schedule format | Human-friendly strings | `"hourly"`, `"daily"`, `"every 4h"`, `"@06:00"` cover 95% of cases; better TUI UX |
| Inline comment injection | Formatted text via existing tmux send-keys | No new infrastructure; reuses `session/tmux/tmux_io.go` |
| Skill `context_files` paths | Absolute or `~`-relative | No repo-relative paths since skills are global |

---

## Milestone Map

```
M1: Review Queue        ── no deps, ships standalone
M3: Skills Templates    ── no deps, ships standalone (parallel with M1)
         ↓                         ↓
M2: Inline Diff Comments  ── no hard deps, enhances review workflow
M4: Automations           ── depends on M1 (review queue) + M3 (skills)
```

---

## Feature 1: Review Queue

### What makes Codex great here

Completed work lands in a dedicated review space with diff, file stats, and clear
actions. Users are never hunting through a list for "which agent finished."

### Data model changes

Add to `session.Instance` (`session/instance.go`):

```go
// PendingReview is true when this instance was started by an automation and has
// finished (Running → Ready). It stays true until the user takes an action.
PendingReview bool      `json:"pending_review,omitempty"`
CompletedAt   *time.Time `json:"completed_at,omitempty"`
// AutomationID is set when this instance was created by an automation.
// Empty for manually created instances.
AutomationID  string    `json:"automation_id,omitempty"`
```

When an automation-triggered instance transitions `Running → Ready`:
- `SetStatus(Ready)` also sets `PendingReview = true` and `CompletedAt = now`

### UI: sidebar section

The sidebar gains a **Review Queue** section above the running instances list.
Only shown when at least one instance has `PendingReview = true`.

```
┌─ REVIEW QUEUE ─────────────────────────────┐
│ ✓ dep-audit         +142/-87   2m ago      │
│ ✓ security-scan     +12/-3     14m ago     │
├─ RUNNING ──────────────────────────────────┤
│ ▶ add-tests         Running                │
└────────────────────────────────────────────┘
```

Rendered by a new `renderReviewSection()` method in `ui/list_renderer.go`, styled
with a distinct amber header to make it visually pop.

### UI: review actions

When a review-queue instance is selected, the key hint bar shows:

```
c: commit  p: create PR  s: send back  o: checkout  d: discard
```

| Key | Action |
|-----|--------|
| `c` | Prompt for commit message → `git commit` in worktree |
| `p` | Enter PR title/body flow (existing `statePRTitle`) |
| `s` | Open `TextInputOverlay` → comment injected into agent; clears `PendingReview` |
| `o` | Checkout worktree branch locally |
| `d` | Confirm discard → remove worktree, set `PendingReview = false` |

After any action except `s`, `PendingReview` is set to `false` and the instance
leaves the review queue.

### Key files

| File | Change |
|------|--------|
| `session/instance.go` | Add `PendingReview`, `CompletedAt`, `AutomationID` fields; update `SetStatus` |
| `config/state.go` | Fields persist automatically (JSON) |
| `ui/list_renderer.go` | `renderReviewSection()` |
| `ui/list.go` | Render review section before running instances |
| `app/app_input.go` | New review action key handlers (c/p/s/o/d in review context) |
| `app/app_actions.go` | `discardReviewInstance()`, `sendBackToAgent()` |

---

## Feature 2: Inline Diff Comments

### What makes Codex great here

You annotate the actual line that's wrong, right in the diff view. Feedback is
*spatially anchored* to the code — no context switching to type a separate message.

### Data model changes

Extend `ui.DiffPane` (`ui/diff.go`):

```go
// Comment mode fields
commentMode    bool
commentCursor  int                     // index into rendered diff lines
comments       map[string][]LineComment // filePath → comments
```

```go
type LineComment struct {
    File    string // relative file path
    Line    int    // line number in the diff output
    Marker  string // "+", "-", or " " (context line)
    Code    string // the line content being commented on
    Comment string // user's comment text
}
```

### Interaction flow

1. In diff view, press `v` → enter **comment mode**. A cursor (`▶`) appears on the
   first diff line.
2. `j`/`k` move cursor through diff lines. The current line is highlighted.
3. Press `c` on any line → `TextInputOverlay` opens. Comment is saved to `comments`.
4. Commented lines render with an amber annotation below them:
   ```
    + token := r.Header.Get("Auth")
      │ ★ should be "Authorization", not "Auth"
   ```
5. Press `Esc` to exit comment mode (comments are preserved).
6. Press `Enter` (or a dedicated `s` key) to **send feedback** → formats all
   comments and injects via `instance.SendKeys(formatted)`.
7. Press `x` to clear all comments.

The status bar in comment mode shows:
```
comment mode  ▶ j/k: move  c: add comment  Enter: send  x: clear  Esc: exit
```

### Comment message format

```
Code review feedback on your changes:

[auth/handler.go +42] `token := r.Header.Get("Auth")`
  → Should be "Authorization", not "Auth"

[utils/parse.go -18] `return nil`
  → Don't remove this nil check, it guards against empty slices

Please address these comments and continue.
```

### Key files

| File | Change |
|------|--------|
| `ui/diff.go` | Add comment mode, cursor tracking, `LineComment`, comment rendering |
| `ui/overlay/textInput.go` | Reused as-is for comment input |
| `app/app_input.go` | Comment mode key handlers routed from diff focus state |
| `session/tmux/tmux_io.go` | `SendKeys()` — already exists, used for injection |

---

## Feature 3: Skills / Task Templates

### What makes Codex great here

Skills make Hivemind feel like a *platform*. Users build reusable patterns once
(code review, security audit, test writing) and invoke them by name across any project.

### Storage

Skills live exclusively in `~/.hivemind/skills/`. No per-project scope.
Each skill is a Markdown file with YAML frontmatter:

```
~/.hivemind/skills/
  code-review.md
  security-audit.md
  test-writer.md
  dep-update.md
```

Skill format:

```yaml
---
name: "security-audit"
description: "Check for OWASP Top 10 vulnerabilities"
context_files:
  - "~/work/security-checklist.md"   # absolute or ~-relative paths only
setup_script: "git diff main...HEAD --name-only"
---
You are performing a security audit. Focus on:
- SQL injection, XSS, command injection
- Improper authentication/authorization
- Secrets or credentials in code

When done, summarize findings as a bullet list with severity (high/medium/low).
```

### Loading

New `config/skills.go`:

```go
type Skill struct {
    Name         string
    Description  string
    ContextFiles []string
    SetupScript  string
    Instructions string // the markdown body
}

func LoadSkills() ([]Skill, error) // scans ~/.hivemind/skills/*.md
```

### Instance creation integration

When creating a new instance (`stateNew`), after the user enters title + prompt:
- `Tab` opens a `PickerOverlay` listing all loaded skills
- Selecting a skill:
  1. Prepends skill instructions to the prompt
  2. Reads `context_files` and appends content as `<context filename="...">` blocks
  3. Runs `setup_script` (via `exec.Command("sh", "-c", script)`) before the agent
     starts; output is appended to the context

Built-in personality meta-skills (no file required, selected via shortcut `t`/`T` in
instance creation):
- **Terse**: appends `"Be extremely concise. Use bullet points. No preamble."`
- **Verbose**: appends `"Explain your reasoning step by step. Walk through changes."`

### Key files

| File | Change |
|------|--------|
| `config/skills.go` | New — `Skill` struct, `LoadSkills()`, YAML frontmatter parser |
| `ui/overlay/pickerOverlay.go` | Reused as-is for skill selection |
| `app/app.go` | Add optional skill selection step to `stateNew` |
| `session/instance_lifecycle.go` | Run `setup_script`, prepend skill instructions to prompt |

---

## Feature 4: Automations / Scheduled Tasks

### What makes Codex great here

Users define recurring background tasks once. Agents run on a schedule with zero
interaction. Results wait in the review queue.

### Storage

New file: `~/.hivemind/automations.json`

```go
// config/automations.go

type Automation struct {
    ID           string     `json:"id"`
    Name         string     `json:"name"`
    Instructions string     `json:"instructions"`
    SkillName    string     `json:"skill_name,omitempty"`
    Schedule     string     `json:"schedule"` // see schedule formats below
    RepoPath     string     `json:"repo_path"`
    Enabled      bool       `json:"enabled"`
    LastRun      *time.Time `json:"last_run,omitempty"`
    NextRun      time.Time  `json:"next_run"`
    CreatedAt    time.Time  `json:"created_at"`
}
```

### Schedule formats

| String | Meaning |
|--------|---------|
| `"hourly"` | Every 60 minutes |
| `"daily"` | Every 24 hours from first run |
| `"weekly"` | Every 7 days |
| `"every 30m"` | Every N minutes |
| `"every 4h"` | Every N hours |
| `"@06:00"` | Daily at a specific local time |

`config.ParseSchedule(s string) (interval time.Duration, dailyAt *time.Time, err error)`
`config.NextRunTime(a *Automation) time.Time`

### Daemon integration

The existing tick loop in `daemon/daemon.go` is extended:

```go
// On each tick, after existing AutoYes logic:
for _, auto := range automations {
    if auto.Enabled && time.Now().After(auto.NextRun) {
        triggerAutomation(auto, brainServer)
        auto.LastRun = ptr(time.Now())
        auto.NextRun = config.NextRunTime(auto)
        config.SaveAutomations(automations)
    }
}
```

`triggerAutomation` sends a `CreateInstance` action to the TUI brain server
(same IPC path used by `create_instance` MCP tool), with:
- `Title` derived from automation name + timestamp
- `Prompt` = resolved skill instructions + automation instructions
- `AutomationID` set to `auto.ID`
- `AutoYes = true` (runs unattended)
- `SkipPermissions = true`

When the instance finishes → `PendingReview = true` → lands in Review Queue (Feature 1).

### TUI Automation Manager

New app state `stateAutomations`. Accessible via `A` key (or command palette).

```
┌─ AUTOMATIONS ──────────────────────────────────────────────────────┐
│  Name              Schedule     Last Run      Next Run    Status   │
│  dep-audit         daily        2h ago        tomorrow    ● on     │
│  security-scan     weekly       3d ago        in 4d       ● on     │
│  test-coverage     every 6h     30m ago       in 5.5h     ○ off    │
├────────────────────────────────────────────────────────────────────┤
│  n: new  e: edit  space: toggle  r: run now  d: delete  Esc: back  │
└────────────────────────────────────────────────────────────────────┘
```

Creating an automation is a multi-step flow using existing overlay components:
1. `TextInputOverlay` → name
2. Multiline text input → instructions
3. `PickerOverlay` → skill (optional, can skip)
4. `TextInputOverlay` → schedule string (with hint: "hourly, daily, every 4h, @06:00")
5. `PickerOverlay` → repo path (from recent repos list)

### Future extension: MCP `create_automation` tool

A future brain MCP tool `create_automation` would let a Claude Code agent set up
automations programmatically:

```
create_automation(
  name="nightly-dep-audit",
  instructions="Check for outdated dependencies and create a PR with updates",
  skill="dep-update",
  schedule="@02:00",
  repo_path="/current/repo"
)
```

This allows Claude Code to say "I'll set up a nightly dependency scan for you" and
wire it directly into Hivemind's daemon without the user touching the TUI.

### Key files

| File | Change |
|------|--------|
| `config/automations.go` | New — `Automation` struct, CRUD, `ParseSchedule`, `NextRunTime`, `LoadAutomations`, `SaveAutomations` |
| `daemon/daemon.go` | Load automations on start; check schedule in tick loop; call `triggerAutomation` |
| `brain/protocol.go` | Automation-triggered `CreateInstanceParams` (add `AutomationID` field) |
| `brain/server.go` | Handle automation `CreateInstance` → set `AutomationID` on new instance |
| `app/app.go` | Add `stateAutomations` to state enum |
| `app/app_input.go` | `handleAutomationsKeys()` for list navigation + CRUD |
| `ui/automations_list.go` | New — renders the automation manager table |

---

## Testing strategy

- **Review Queue**: unit test `SetStatus` transition sets `PendingReview` when
  `AutomationID != ""`; integration test review actions (commit/discard/send back).
- **Inline Comments**: unit test `DiffPane` comment storage and formatted output.
- **Skills**: unit test `LoadSkills()` with fixture `.md` files; test YAML parse errors.
- **Automations**: unit test `ParseSchedule` and `NextRunTime` for all format variants;
  unit test daemon tick fires `triggerAutomation` when `NextRun` is past.
  Existing `brain/workflow_test.go` pattern as reference.

---

## Non-goals

- Per-project skills (`.hivemind/skills/` in repo) — global-only by design
- Full cron syntax for automations — human-friendly formats only (for now)
- Real-time automation logs in TUI — instances are visible in the running list as usual
- Skill sharing/marketplace — out of scope; file-based is sufficient
