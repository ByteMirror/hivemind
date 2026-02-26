# Hivemind TUI Feature Roadmap (Codex Gap-Oriented)

Date: 2026-02-26

## Goal
Prioritize Codex-inspired improvements that fit Hivemind's architecture (Bubble Tea TUI, tmux sessions, git worktrees, local automation config) without forcing desktop-UI assumptions.

## Guiding Constraints
- Keep interactions keyboard-first and pane-based.
- Reuse existing state machine (`state*` in app), overlays, and list/sidebar abstractions.
- Prefer additive behavior over workflow rewrites.
- Keep integrations scriptable and local-first before native OAuth complexity.

## Milestone 1: Review Inbox

### Problem
Automation-completed work is not separated into a clear, action-oriented review queue.

### Features
- Add `PendingReview` and `CompletedAt` to automation-spawned instances.
- Add a dedicated Review section/filter in list/sidebar for pending review items.
- Add contextual review actions for pending items:
  - commit
  - create PR
  - send feedback back to agent
  - checkout
  - discard
- Add review-specific menu hints when a pending-review instance is selected.

### Why this fits Hivemind
- Uses existing instance model, list renderer, key handlers, and git actions.
- No new infrastructure required.

### Acceptance Criteria
- Finished automation instances land in Review Queue automatically.
- User can complete review loop without leaving TUI.
- Review state clears consistently after action completion.

## Milestone 2: Diff Triage Upgrade

### Problem
Diff comments exist, but review triage depth is limited.

### Features
- Diff source mode toggle: working, staged, combined (and optional untracked).
- Hunk navigation (`next`/`prev` hunk).
- Send feedback scope: selected file comments or all comments.
- Keep existing inline comment mode (`v`, `c`, `enter`) and extend it.

### Why this fits Hivemind
- Builds directly on existing `DiffPane` and comment mode.
- Avoids changing core session lifecycle.

### Acceptance Criteria
- Reviewer can triage large diffs faster with mode/hunk controls.
- Feedback payload preserves line context and scope.

## Milestone 3: Automations v2

### Problem
Current automations are schedule-based and lack inbox/review policy semantics.

### Features
- Automation type: one-time or recurring.
- Per-automation completion policy:
  - queue for review
  - no review queue
- Lightweight run history in automations UI:
  - last run time
  - last status
  - next run

### Why this fits Hivemind
- Extends current `config.Automation` and automations modal.
- Works with existing periodic automation check and spawn flow.

### Acceptance Criteria
- One-time tasks run once and stop.
- Recurring tasks remain existing behavior.
- Review policy affects queue behavior deterministically.

## Milestone 4: Worktree Manager Screen

### Problem
Worktrees are central but lack dedicated visibility and management UX.

### Features
- New worktree manager view listing:
  - branch
  - path
  - attached instance (if any)
  - dirty/clean status
- Actions:
  - open shell
  - checkout branch
  - prune stale
  - delete safe-to-remove

### Why this fits Hivemind
- Hivemind already depends on git worktrees; this surfaces existing internals.

### Acceptance Criteria
- Users can inspect and maintain worktrees without leaving app.
- Unsafe destructive actions require confirmation.

## Milestone 5: Integration Hooks (Local-First)

### Problem
Codex-style service integrations are powerful, but native OAuth UI is heavy for TUI now.

### Features
- Event hook system: run command/webhook on key lifecycle events:
  - instance finished
  - review required
  - PR created
  - automation triggered
- Configurable in settings/config file.
- Structured event payload for external scripts.

### Why this fits Hivemind
- Keeps integration model composable and script-driven.
- Avoids fragile in-TUI auth flows initially.

### Acceptance Criteria
- Hooks fire reliably with event payload.
- Failures are surfaced in logs/toasts without breaking core flow.

## Milestone 6: Settings Profiles

### Problem
Multi-agent CLI orchestration benefits more from launch profiles than desktop-only app preferences.

### Features
- Named launch profiles per program/flags/env.
- Profile picker during new instance flow.
- Optional default profile by repo/topic.

### Why this fits Hivemind
- Aligns with existing support for multiple agent CLIs and topic-based workflow.

### Acceptance Criteria
- Users can start instances with predictable profile behavior in one step.
- Existing default-program behavior remains backward-compatible.

## Implementation Order
1. Review Inbox
2. Diff Triage Upgrade
3. Automations v2
4. Worktree Manager Screen
5. Integration Hooks
6. Settings Profiles

## Out of Scope (for now)
- IDE side-panel parity.
- Native Slack/Linear OAuth UI.
- Desktop-specific settings that do not map to TUI workflows.
