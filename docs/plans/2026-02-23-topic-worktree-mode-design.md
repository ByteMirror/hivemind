# Topic Worktree Mode Design

**Date:** 2026-02-23
**Branch:** fabian.urbanek/worktree-manageent

## Problem

Topics currently support two worktree modes, expressed as `SharedWorktree bool`:

- `false` (default) — each instance in the topic creates its own git worktree and branch
- `true` — all instances share one git worktree and branch

There is no way to run instances directly in the main repository directory without any worktree. This is useful when the user wants agents to work on the current branch as-is, without creating new branches or directories.

## Solution

Replace `SharedWorktree bool` with a `TopicWorktreeMode` string enum with three values:

| Mode | Value | Behaviour |
|------|-------|-----------|
| Per-instance worktrees | `"per_instance"` | Each instance gets its own branch + worktree directory (existing default) |
| Shared worktree | `"shared"` | All instances share one branch + worktree directory (existing "yes" option) |
| Main repo | `"main_repo"` | Instances run directly in the repo directory; no worktree, no new branch |

## Architecture

### Data model changes

**`session/topic.go`**
- Add `TopicWorktreeMode` string type with three constants
- Replace `SharedWorktree bool` with `WorktreeMode TopicWorktreeMode` in `Topic` and `TopicOptions`
- Helper methods: `IsSharedWorktree() bool`, `IsMainRepo() bool`
- `Setup()`: only create git worktree when `WorktreeMode == TopicWorktreeModeShared`

**`session/topic_storage.go`**
- `TopicData.WorktreeMode` (new, JSON: `"worktree_mode"`)
- `TopicData.SharedWorktree` (legacy, JSON: `"shared_worktree"`) — kept as read-only migration field
- `FromTopicData`: if `worktree_mode` is empty, derive from `shared_worktree` (true → `shared`, false → `per_instance`)
- `ToTopicData`: write `worktree_mode` only; drop `shared_worktree` from new output

### Instance lifecycle changes

**`session/instance.go`**
- Add `mainRepo bool` field (not persisted; re-derived at start time from topic mode)
- Add `GetWorkingPath() string`: returns `gitWorktree.GetWorktreePath()` when a worktree exists, else `i.Path`

**`session/instance_lifecycle.go`**
- New `StartInMainRepo()`: skips git worktree creation, starts tmux session at `i.Path`, sets `mainRepo = true`
- `Kill()`: no worktree cleanup when `gitWorktree == nil`
- `Pause()`: skip worktree commit/remove when `mainRepo == true`; only detach tmux
- `Resume()`: skip worktree setup when `mainRepo == true`; just restart tmux at `i.Path`

**`session/instance_session.go`**
- `GetGitWorktree()` already returns `(nil, nil)` when unset — callers updated to use `GetWorkingPath()`

### App layer changes

**`app/app_input.go`**
- `handleNewTopicConfirmKeys`: replace `ConfirmationOverlay` (Y/N) with `PickerOverlay` showing three mode options
- All `topic.SharedWorktree` checks → `topic.IsSharedWorktree()`
- Instance start dispatch: add `MainRepo` branch calling `StartInMainRepo()`
- Move prevention: only block moves for `IsSharedWorktree()` topics

**`app/app_actions.go`**
- Context menu "Push branch": gate on `topic.IsSharedWorktree()`

**`app/app_brain.go`**
- Instance start dispatch: add `MainRepo` branch calling `StartInMainRepo()`

**`app/app_state.go`**
- `topicMeta()`: build `WorktreeMode` map instead of `shared` bool map; pass to sidebar
- Callers of `selected.GetGitWorktree()` → `selected.GetWorkingPath()`

### UI changes

**`ui/sidebar.go`**
- `SidebarItem.SharedWorktree bool` → `SidebarItem.WorktreeMode session.TopicWorktreeMode`
- Keep `\ue727` icon for `Shared` topics; no icon for `PerInstance` and `MainRepo`
- `SetItems` / `SetGroupedItems`: accept mode map instead of bool map

## UX flow

```
[T] New topic
    ↓
Enter topic name
    ↓
Picker: "Worktree mode for '<name>'"
  ▸ Per-instance worktrees   (each agent gets its own branch + directory)
    Shared worktree           (all agents share one branch + directory)
    Main repo (no worktree)   (agents work directly in the repo)
    ↓
Topic created
```

## Backward compatibility

Existing stored topics (`~/.hivemind/topics.json`) with `"shared_worktree": true` load as `Shared`, `false` loads as `PerInstance`. No manual migration required.

## Files changed

| File | Change |
|------|--------|
| `session/topic.go` | Add enum, replace bool, update `Setup()`, add helpers |
| `session/topic_storage.go` | Add `worktree_mode` field, migration shim in `FromTopicData` |
| `session/instance.go` | Add `mainRepo bool`, add `GetWorkingPath()` |
| `session/instance_lifecycle.go` | Add `StartInMainRepo()`, update Pause/Resume/Kill |
| `session/instance_session.go` | Update callers of `GetGitWorktree()` where relevant |
| `app/app_input.go` | Replace Y/N confirm with picker, update all mode checks |
| `app/app_actions.go` | Update `SharedWorktree` check |
| `app/app_brain.go` | Add `MainRepo` dispatch branch |
| `app/app_state.go` | Update `topicMeta()`, replace `GetGitWorktree()` callers |
| `ui/sidebar.go` | Replace `SharedWorktree bool` with `WorktreeMode` |
