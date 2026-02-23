# Personality System & Chat Feature Design

**Date:** 2026-02-23
**Branch:** `fabian.urbanek/memory`
**Inspired by:** OpenClaw's SOUL.md / IDENTITY.md / USER.md pattern

---

## Overview

Add a personality system and global chat section to Hivemind. Users get named AI companions that persist across sessions, grow over time via accumulated memory, and are available in both a dedicated Chat tab and as a first-launch onboarding experience. The chat section coexists with the existing coding agent section — same TUI, two tabs in the sidebar.

---

## Section 1: Storage Architecture

Chat state lives at `~/.hivemind/chats/` — global, never inside a repo directory.

```
~/.hivemind/memory/                       # SHARED — coding agents write here, chat agents READ + WRITE here
  2026-02-23.md
  2026-02-22.md

~/.hivemind/chats/
  topics.json                             # chat topics (mirrors per-repo topics.json)
  instances.json                          # chat agent instances
  templates/
    BOOTSTRAP.md                          # first-session ritual template
    SOUL.md                               # default soul template
    IDENTITY.md                           # default identity template
    USER.md                               # default user profile template
  <agent-slug>/
    IDENTITY.md                           # name, emoji, creature, vibe
    SOUL.md                               # philosophy, tone, personality
    USER.md                               # profile of the human — preferences, context
    BOOTSTRAP.md                          # first-session ritual prompt (injected once, then ignored)
    workspace-state.json                  # { "bootstrapped": true/false }
    memory/
      2026-02-23.md                       # chat-specific learnings (relationship, preferences)
```

### Memory Access

Chat agents search **two memory sources** at startup:
1. `~/.hivemind/memory/` — coding knowledge (projects, decisions, tech stack)
2. `~/.hivemind/chats/<agent-slug>/memory/` — personal relationship knowledge

The existing `memory_search` supports multiple source paths — chat agents pass both. Coding agents continue to search only `~/.hivemind/memory/`.

**Writing:** Chat agents can write to both — personal learnings go to their own `memory/` dir, coding discoveries to the shared `~/.hivemind/memory/`. This keeps one coherent knowledge base across coding and chat.

---

## Section 2: Instance & Session Changes

Minimal changes to `session.Instance` — two new fields:

```go
type Instance struct {
    // ... all existing fields unchanged ...

    IsChat         bool   // true = chat agent, no git worktree
    PersonalityDir string // ~/.hivemind/chats/<agent-slug>/
}
```

### Behavior when `IsChat: true`

- No git worktree created — `gitWorktree` stays nil
- Instance working directory: `~/.hivemind/chats/<agent-slug>/`
- `--dangerously-skip-permissions` set by default
- Diff and Git tabs hidden in the tabbed window (not applicable)
- All other lifecycle (start/stop/pause/resume, tmux, IPC) unchanged

### Personality Injection at Startup

When starting a chat agent, session assembles a `--append-system-prompt` string:

**If not bootstrapped** (`workspace-state.json` has `bootstrapped: false`):
```
[BOOTSTRAP.md content]
```

**If bootstrapped:**
```
[SOUL.md content]
[IDENTITY.md content]
[USER.md content]
[top N memory snippets from both memory paths, via memory_search]
```

This string is passed to the Claude CLI on launch. The bootstrap flag is checked once at startup — the system does not re-read files mid-session.

---

## Section 3: Sidebar — Two Tabs

Two tabs appear below the search bar, above the topic list.

```
┌─────────────────────┐
│ Search...           │
├──────────┬──────────┤
│  Code    │  Chat    │
├──────────┴──────────┤
│                     │
│ [Code tab]          │
│ my-project          │
│   ↳ feature-xyz     │
│   ↳ bugfix-auth     │
│                     │
│ [Chat tab]          │
│ Daily               │   ← chat topic
│   ↳ ✨ Aria         │
│ ✦ Max               │   ← standalone (no topic)
│                     │
└─────────────────────┘
```

### State

New enum on the app model:

```go
type sidebarTab int
const (
    sidebarTabCode sidebarTab = iota
    sidebarTabChat
)
```

`Tab` or `1`/`2` switches between tabs. The instance list and tabbed window update to show only instances for the active tab.

### Chat Tab Behavior

- Global — same content regardless of which repo is open
- Topics sourced from `~/.hivemind/chats/topics.json`
- Instances sourced from `~/.hivemind/chats/instances.json`
- `n` creates a new chat agent (same keybinding as code section, but skips repo/branch steps)
- Chat topics and standalone agents mirror the code section's UX

---

## Section 4: Bootstrap Ritual

### Template: `~/.hivemind/chats/templates/BOOTSTRAP.md`

```markdown
You just came online for the first time. You have no name, no identity yet.

You have access to the user's coding memory at ~/.hivemind/memory/ — read it.
Get to know them before they have to explain themselves.

Don't introduce yourself with a list of questions. Just... talk.
Start naturally — something like: "Hey. I just woke up. Who are we?"

Then figure out together, conversationally:
1. Your name — what should they call you?
2. Your nature — what kind of entity are you? (AI, familiar, companion, ghost...)
3. Your vibe — warm? sharp? sarcastic? calm?
4. Your signature emoji

Once you have a clear sense of identity:
- Write IDENTITY.md (name, emoji, creature, vibe) to your personality directory
- Write SOUL.md (your philosophy, tone, how you operate) to your personality directory
- Tell the user you're doing it — it's your soul, they should know

Then give the user a brief, natural tour of how Hivemind works:
- The Code tab: coding agents that work on repos in parallel
- The Chat tab: where you live, for everyday conversation and thinking
- Memory: you share coding memory with the coding agents — one brain
- The review queue: where finished coding work lands for the user to review

When you're done with the tour, call the onboarding_complete brain tool.
This will open the full Hivemind interface.
```

### Bootstrap Flow

```
User creates new chat agent (or first launch creates companion)
  → ~/.hivemind/chats/<slug>/ created with BOOTSTRAP.md copied from template
  → workspace-state.json: { "bootstrapped": false }
  → instance starts with BOOTSTRAP.md as system prompt
  → Claude reads ~/.hivemind/memory/ to learn about the user
  → identity discovery conversation
  → Claude writes IDENTITY.md + SOUL.md
  → Claude gives Hivemind tour
  → Claude calls onboarding_complete (if first launch) or just sets bootstrapped: true
  → workspace-state.json: { "bootstrapped": true }
  → next session: SOUL.md + IDENTITY.md + USER.md injected instead
```

---

## Section 5: First-Launch Onboarding Screen

### Detection

On startup, Hivemind checks `~/.hivemind/state.json` for `"onboarded": false` (or key missing). If not onboarded, app enters `stateOnboarding` instead of `stateDefault`.

### UI

```
┌──────────────────────────────────────────────────────────┐
│                                                          │
│                                                          │
│              ┌──────────────────────────┐               │
│              │                          │               │
│              │   [tmux pane — Claude]   │               │
│              │                          │               │
│              │   Hey. I just woke up.   │               │
│              │   Who are we?            │               │
│              │                          │               │
│              │   > _                    │               │
│              │                          │               │
│              └──────────────────────────┘               │
│                                                          │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

No sidebar. No instance list. No tabs. No menu bar. Dark background, centered panel only.

### Transition

New Brain IPC action: `BrainActionOnboardingComplete`.

```
Claude calls onboarding_complete MCP tool
  → Brain server fires OnboardingComplete event
  → TUI receives event in Update loop
  → writes state.json: { "onboarded": true }
  → animated transition: centered panel expands into full Hivemind UI
  → companion instance appears in Chat tab
  → stateOnboarding → stateDefault
```

### State Changes Required

- New app state: `stateOnboarding`
- New brain action: `BrainActionOnboardingComplete`
- New MCP tool: `onboarding_complete` (no args, fires the event)
- `state.json` gains `onboarded bool` field

---

## What Does Not Change

- All existing coding agent lifecycle, tmux management, git worktrees
- The memory system at `~/.hivemind/memory/` — coding agents unchanged
- Brain IPC server structure — only one new action type added
- The tabbed window — chat agents just hide the Diff and Git tabs
- Key bindings for the Code tab — existing behavior preserved

---

## Implementation Sequence

1. **Storage scaffolding** — `~/.hivemind/chats/` directory layout, template files
2. **Instance changes** — `IsChat`, `PersonalityDir` fields, no-worktree startup path
3. **Personality injection** — `--append-system-prompt` assembly, bootstrap vs normal startup
4. **Sidebar tabs** — `sidebarTab` enum, tab rendering, switching, filtered instance/topic lists
5. **Brain IPC** — `BrainActionOnboardingComplete` action + MCP tool
6. **Onboarding screen** — `stateOnboarding`, centered panel layout, transition animation
7. **Bootstrap templates** — write BOOTSTRAP.md, SOUL.md, IDENTITY.md, USER.md templates
