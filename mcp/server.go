package mcp

import (
	"github.com/ByteMirror/hivemind/memory"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

const serverInstructions = `You are running inside Hivemind, a multi-agent orchestration system.
You may be one of several agents working in parallel on the same codebase.
Each agent runs in its own git worktree with its own tmux session.

## Startup Protocol

At the start of every task, perform these steps in order:

1. Call update_status with the feature you're working on, the files you'll touch, and your role.
   This registers you with the coordination brain so other agents can see what you're doing.
2. Call get_brain to read the shared state: what other agents are working on, which files they're
   touching, and any messages waiting for you.
3. Call memory_search with a query relevant to your task. Memory contains prior decisions,
   user preferences, architecture notes, and environment facts from previous sessions.

If update_status returns file conflicts, coordinate with the other agent via send_message before
proceeding. Do not silently work on the same files — this causes merge conflicts.

## Multi-Agent Coordination

You are part of a team. Other agents may be running concurrently in the same repository.

### Staying in Sync
- Call get_brain periodically (every few minutes during long tasks) to check for new messages
  and detect if another agent has started working on files near yours.
- After changing your focus or switching files, call update_status again so others see the change.

### Communication
- send_message(to, message): Send a targeted message to a specific agent by title, or broadcast
  to all agents by leaving "to" empty. Messages appear in the recipient's next get_brain call.
  Keep messages concise and actionable: what changed, what they should know.
- inject_message(to, message): For urgent coordination only. This types directly into another
  agent's terminal input, bypassing the polling-based message system. Use sparingly — the target
  agent may be mid-thought.

### Spawning Sub-Agents
Use create_instance to delegate independent subtasks. Each new agent gets its own worktree and
tmux session. Provide a clear, self-contained prompt — the sub-agent has no context from your
conversation.

Common patterns:
- Spawn a "reviewer" agent to review your changes before creating a PR.
- Spawn a "tester" agent to write tests for a feature you just implemented.
- Spawn a "coder" agent for an independent subtask while you continue on your own work.

### Lifecycle Management
- pause_instance(target): Suspend an agent. Its tmux session is preserved; execution stops.
- resume_instance(target): Resume a paused agent.
- kill_instance(target): Terminate an agent permanently. The tmux session is destroyed and
  the worktree is cleaned up.

## Workflow Orchestration

For complex multi-step tasks, define a workflow DAG instead of spawning agents manually.

1. Call define_workflow with a JSON array of tasks. Each task has an id, title, prompt, role,
   and a depends_on list of task IDs. Tasks whose dependencies are already satisfied will be
   triggered immediately (each spawning a new agent instance).
2. When a sub-agent finishes its work, it calls complete_task(task_id, status). If status is
   "done", any tasks that depended on it (and whose other dependencies are also complete)
   will be triggered automatically.
3. Use get_workflow to inspect the current DAG state: which tasks are pending, running, or done.

### Waiting for Sub-Agents
Use wait_for_events instead of polling get_brain in a loop. It long-polls for real-time events
with server-side buffering so no events are missed between polls.

On first call, omit subscriber_id to create a subscription. Filter by event types
(task_completed, instance_killed, message_received, etc.) and/or by instance titles.
On subsequent calls, pass the returned subscriber_id to continue receiving events.

Call unsubscribe_events when you no longer need the subscription.

## Persistent Memory System

Hivemind maintains an IDE-wide persistent memory store backed by git. Memory survives across
sessions, projects, and agent instances. Use it to build institutional knowledge about the
user's environment, preferences, and project decisions.

### Architecture
- Memory lives in ~/.hivemind/memory/ (global) and ~/.hivemind/memory/repos/<slug>/ (per-repo).
- Files are Markdown (.md) with optional YAML frontmatter for descriptions.
- The system/ directory contains pinned files that are always injected into every agent's
  CLAUDE.md at startup. This is the highest-priority context.
- All changes are automatically git-committed for versioning and history.

### When to Read Memory
- **Start of session**: Call memory_search with a query relevant to your task.
- **Before answering questions** about the user's preferences, setup, past decisions, or
  active projects: search first, never assume.
- **When exploring the memory store**: Use memory_tree to see the file structure with
  descriptions, then memory_read or memory_get to read specific files.

### When to Write Memory
- **After completing a significant task**: Record what was built, key decisions made, and
  any user preferences observed. Do not wait to be asked.
- **When you discover something durable**: User setup, environment facts, project conventions,
  API configurations, recurring patterns the user likes or dislikes.
- **At the end of a working session**: Summarize what was accomplished.
- **When asked to write memory**: Do it immediately without asking for confirmation.

### Scope and Organization
- scope="global": Cross-project facts (hardware, OS, shell, editor, global preferences).
  When no explicit file is given, writes to system/global.md which is always in agent context.
- scope="repo": Project-specific decisions. Dated files (YYYY-MM-DD.md) default to repo scope.
- Parameter semantics:
  - file: write target path for memory_write.
  - path: existing file path for read/get/append/move/delete/pin/unpin/history/diff filters.
  - Repo path form: repos/<repo-slug>/<file>.md (explicit repo targeting).
  - For memory_read/memory_get/memory_append/memory_diff with bare paths, lookup tries global first,
    then repo fallback when the file/ref is missing.
- Use YAML frontmatter (---\ndescription: ...\n---) to describe what each file contains.
  These descriptions appear in the memory tree and help future agents find relevant context.

### File Management
- memory_pin(path): Move a file into system/ so it's always injected into agent context.
  Use this for high-value reference files (conventions, architecture decisions).
- memory_unpin(path): Move a file out of system/ back to root.
- memory_move(from, to): Reorganize files. Use "/" in paths to create topic directories.
- memory_delete(path): Remove a file permanently.

### History and Maintenance
- memory_history(path?, scope?, count?): View git log of memory changes across global/repo memory.
- memory_init: (Skill) Spawns a sub-agent to bootstrap memory from codebase analysis.
- memory_reflect: (Skill) Spawns a sub-agent to review recent changes and consolidate insights.
- memory_defrag: (Skill) Spawns a sub-agent to reorganize aging memory files.

## Tool Reference

### Coordination (Tier 1-2)
| Tool | Purpose |
|------|---------|
| get_brain | Read shared state: agent statuses, file ownership, messages for you |
| list_instances | See all agents, their status, branch, and activity |
| update_status | Declare your feature, files, and role; detect conflicts |
| send_message | Message another agent or broadcast to all |
| get_my_session_summary | Your session: changed files, commits, diff stats |
| get_my_diff | Full git diff of your changes since base commit |

### Agent Lifecycle (Tier 3)
| Tool | Purpose |
|------|---------|
| create_instance | Spawn a new agent with its own worktree |
| inject_message | Type directly into another agent's terminal (urgent) |
| pause_instance | Suspend an agent, preserving its tmux session |
| resume_instance | Resume a paused agent |
| kill_instance | Terminate an agent and clean up its worktree |

### Workflows (Tier 3)
| Tool | Purpose |
|------|---------|
| define_workflow | Create a task DAG with dependencies |
| complete_task | Mark a task done/failed; triggers dependents |
| get_workflow | Inspect the current DAG state |
| wait_for_events | Long-poll for real-time events (replaces polling) |
| unsubscribe_events | Remove an event subscription |

### Memory
| Tool | Purpose |
|------|---------|
| memory_search | Search memory by natural language query; returns ranked snippets |
| memory_read | Read full file body (frontmatter stripped) |
| memory_get | Read specific lines from a memory file |
| memory_list | List all memory files with metadata |
| memory_tree | View file tree with descriptions from frontmatter |
| memory_history | View git history of memory changes |
| memory_write | Write or overwrite a memory file |
| memory_append | Append content to an existing memory file |
| memory_move | Rename or reorganize a memory file |
| memory_delete | Delete a memory file |
| memory_pin | Move file to system/ (always-in-context) |
| memory_unpin | Move file out of system/ to root |

### Memory Skills (Tier 3)
| Tool | Purpose |
|------|---------|
| memory_init | Bootstrap memory from codebase analysis (spawns agent) |
| memory_reflect | Review recent changes and consolidate insights (spawns agent) |
| memory_defrag | Reorganize aging memory files (spawns agent) |`

// HivemindMCPServer wraps an MCP server with Hivemind-specific state.
type HivemindMCPServer struct {
	server           *mcpserver.MCPServer
	stateReader      *StateReader
	brainClient      BrainClient
	instanceID       string // used by Tier 2 introspection tools
	repoPath         string // scopes brain and instance listing to this repo
	tier             int    // gates tool registration: 1=read, 2=+introspect, 3=+write
	memoryMgr        *memory.Manager
	repoMemoryMgr    *memory.Manager // canonical repo-scoped memory manager (may be nil)
	legacyRepoMemMgr *memory.Manager // legacy worktree-slug repo manager during migration
}

// NewHivemindMCPServer creates a new MCP server for a Hivemind agent.
func NewHivemindMCPServer(brainClient BrainClient, hivemindDir, instanceID, repoPath string, tier int, memMgr *memory.Manager, repoMemMgr *memory.Manager, legacyRepoMemMgr *memory.Manager) *HivemindMCPServer {
	s := mcpserver.NewMCPServer(
		"hivemind",
		"0.1.0",
		mcpserver.WithInstructions(serverInstructions),
	)

	h := &HivemindMCPServer{
		server:           s,
		stateReader:      NewStateReader(hivemindDir),
		brainClient:      brainClient,
		instanceID:       instanceID,
		repoPath:         repoPath,
		tier:             tier,
		memoryMgr:        memMgr,
		repoMemoryMgr:    repoMemMgr,
		legacyRepoMemMgr: legacyRepoMemMgr,
	}

	h.registerTier1Tools()
	if tier >= 2 {
		h.registerTier2Tools()
	}
	if tier >= 3 {
		h.registerTier3Tools()
	}
	if memMgr != nil {
		h.registerMemoryTools()
		if tier >= 3 && brainClient != nil {
			h.registerMemorySkills()
		}
	}

	Log("server created: tier=%d tools registered", tier)
	return h
}

// registerMemoryTools registers the IDE-wide memory tools (all tiers).
func (h *HivemindMCPServer) registerMemoryTools() {
	mgr := h.memoryMgr
	repoMgr := h.repoMemoryMgr
	legacyRepoMgr := h.legacyRepoMemMgr

	// --- Read-only tools ---

	memRead := gomcp.NewTool("memory_read",
		gomcp.WithDescription(
			"Use this when you know an existing memory file path and need the full file body. "+
				"Example: memory_read(path=\"system/global.md\").",
		),
		gomcp.WithReadOnlyHintAnnotation(true),
		gomcp.WithString("path",
			gomcp.Required(),
			gomcp.Description("Existing file path. Global example: \"system/global.md\". Repo example: \"repos/<repo-slug>/notes.md\". Bare paths try global first, then repo fallback if missing."),
		),
		gomcp.WithString("ref",
			gomcp.Description("Optional git ref (branch, tag, or SHA). Example: ref=\"main\"."),
		),
	)
	h.server.AddTool(memRead, handleMemoryRead(mgr, repoMgr, legacyRepoMgr))

	memSearch := gomcp.NewTool("memory_search",
		gomcp.WithDescription(
			"Use this first when you need prior decisions, preferences, or setup context. "+
				"Example: memory_search(query=\"commit message conventions\").",
		),
		gomcp.WithReadOnlyHintAnnotation(true),
		gomcp.WithString("query",
			gomcp.Required(),
			gomcp.Description("Natural-language search query."),
		),
		gomcp.WithNumber("max_results",
			gomcp.Description("Maximum results to return (default 10). Example: max_results=5."),
		),
	)
	h.server.AddTool(memSearch, handleMemorySearch(mgr, repoMgr, legacyRepoMgr))

	memGet := gomcp.NewTool("memory_get",
		gomcp.WithDescription(
			"Use this when you need specific line ranges from an existing memory file. "+
				"Example: memory_get(path=\"system/global.md\", from=1, lines=20).",
		),
		gomcp.WithReadOnlyHintAnnotation(true),
		gomcp.WithString("path",
			gomcp.Required(),
			gomcp.Description("Existing file path. Global example: \"system/global.md\". Repo example: \"repos/<repo-slug>/notes.md\". Bare paths try global first, then repo fallback if missing."),
		),
		gomcp.WithNumber("from",
			gomcp.Description("Start line number (1-indexed, default 1)."),
		),
		gomcp.WithNumber("lines",
			gomcp.Description("Number of lines to read (default: entire file)."),
		),
		gomcp.WithString("ref",
			gomcp.Description("Optional git ref (branch, tag, or SHA). Example: ref=\"main\"."),
		),
	)
	h.server.AddTool(memGet, handleMemoryGet(mgr, repoMgr, legacyRepoMgr))

	memList := gomcp.NewTool("memory_list",
		gomcp.WithDescription(
			"Use this to enumerate available memory files before reading or editing. "+
				"Example: memory_list() or memory_list(ref=\"main\").",
		),
		gomcp.WithReadOnlyHintAnnotation(true),
		gomcp.WithString("ref",
			gomcp.Description("Optional git ref (branch, tag, or SHA)."),
		),
	)
	h.server.AddTool(memList, handleMemoryList(mgr, repoMgr, legacyRepoMgr))

	memTree := gomcp.NewTool("memory_tree",
		gomcp.WithDescription(
			"Use this when you need a structured memory view (paths + descriptions). "+
				"Example: memory_tree() or memory_tree(ref=\"main\").",
		),
		gomcp.WithReadOnlyHintAnnotation(true),
		gomcp.WithString("ref",
			gomcp.Description("Optional git ref (branch, tag, or SHA)."),
		),
	)
	h.server.AddTool(memTree, handleMemoryTree(mgr, repoMgr, legacyRepoMgr))

	memHistory := gomcp.NewTool("memory_history",
		gomcp.WithDescription(
			"Use this when you need commit history for memory changes. "+
				"Example: memory_history(path=\"smoke.md\", scope=\"repo\", count=10).",
		),
		gomcp.WithReadOnlyHintAnnotation(true),
		gomcp.WithString("path",
			gomcp.Description("Optional existing file path filter. Use repos/<repo-slug>/... for explicit repo targeting."),
		),
		gomcp.WithString("scope",
			gomcp.Description("History scope: \"all\" (default), \"global\", or \"repo\"."),
		),
		gomcp.WithNumber("count",
			gomcp.Description("Number of history entries to return (default 10)."),
		),
		gomcp.WithString("branch",
			gomcp.Description("Optional branch/ref filter. Example: branch=\"main\"."),
		),
	)
	h.server.AddTool(memHistory, handleMemoryHistory(mgr, repoMgr, legacyRepoMgr))

	// --- Write tools ---

	memWrite := gomcp.NewTool("memory_write",
		gomcp.WithDescription(
			"Use this when you want to create or overwrite memory content. "+
				"Example: memory_write(content=\"Use lowercase conventional commits.\", scope=\"repo\", file=\"conventions.md\"). "+
				"`file` is the write target path.",
		),
		gomcp.WithString("content",
			gomcp.Required(),
			gomcp.Description("Content to save (Markdown recommended)."),
		),
		gomcp.WithString("file",
			gomcp.Description("Write target path. Examples: \"system/conventions.md\", \"auth-decisions.md\". Default: today's YYYY-MM-DD.md."),
		),
		gomcp.WithString("scope",
			gomcp.Description("Scope: \"global\" for cross-project facts, \"repo\" for project-specific facts. Default behavior uses dated repo files when file is omitted."),
		),
		gomcp.WithString("commit_message",
			gomcp.Description("Optional custom commit message."),
		),
		gomcp.WithString("branch",
			gomcp.Description("Optional branch to commit to. When omitted, writes to the default memory branch."),
		),
	)
	h.server.AddTool(memWrite, handleMemoryWrite(mgr, repoMgr))

	memAppend := gomcp.NewTool("memory_append",
		gomcp.WithDescription(
			"Use this when you want to add text to an existing file without overwriting it. "+
				"Example: memory_append(path=\"notes.md\", content=\"new finding\").",
		),
		gomcp.WithString("path",
			gomcp.Required(),
			gomcp.Description("Existing file path. Use repos/<repo-slug>/... for explicit repo targeting. Bare paths try global first, then repo fallback if missing."),
		),
		gomcp.WithString("content",
			gomcp.Required(),
			gomcp.Description("Content to append to the end of the file."),
		),
		gomcp.WithString("branch",
			gomcp.Description("Optional branch to commit to. When omitted, uses default memory branch."),
		),
	)
	h.server.AddTool(memAppend, handleMemoryAppend(mgr, repoMgr, legacyRepoMgr))

	memMove := gomcp.NewTool("memory_move",
		gomcp.WithDescription(
			"Use this when you want to rename or reorganize an existing memory file. "+
				"Example: memory_move(from=\"notes.md\", to=\"architecture/notes.md\").",
		),
		gomcp.WithString("from",
			gomcp.Required(),
			gomcp.Description("Current existing file path."),
		),
		gomcp.WithString("to",
			gomcp.Required(),
			gomcp.Description("Target path. Parent directories are created automatically."),
		),
		gomcp.WithString("branch",
			gomcp.Description("Optional branch to commit to. When omitted, uses default memory branch."),
		),
	)
	h.server.AddTool(memMove, handleMemoryMove(mgr, repoMgr, legacyRepoMgr))

	memDelete := gomcp.NewTool("memory_delete",
		gomcp.WithDescription("Use this when you want to permanently remove an existing file. Example: memory_delete(path=\"notes.md\")."),
		gomcp.WithString("path",
			gomcp.Required(),
			gomcp.Description("Existing file path to delete."),
		),
		gomcp.WithString("branch",
			gomcp.Description("Optional branch to commit to. When omitted, uses default memory branch."),
		),
	)
	h.server.AddTool(memDelete, handleMemoryDelete(mgr, repoMgr, legacyRepoMgr))

	memPin := gomcp.NewTool("memory_pin",
		gomcp.WithDescription(
			"Use this when a file should always be injected into agent context. "+
				"Example: memory_pin(path=\"conventions.md\").",
		),
		gomcp.WithString("path",
			gomcp.Required(),
			gomcp.Description("Existing file path to pin. Example: \"conventions.md\" -> \"system/conventions.md\"."),
		),
		gomcp.WithString("branch",
			gomcp.Description("Optional branch to commit to. When omitted, uses default memory branch."),
		),
	)
	h.server.AddTool(memPin, handleMemoryPin(mgr, repoMgr, legacyRepoMgr))

	memUnpin := gomcp.NewTool("memory_unpin",
		gomcp.WithDescription(
			"Use this when you want to stop always-injected context for a pinned file. "+
				"Example: memory_unpin(path=\"system/conventions.md\").",
		),
		gomcp.WithString("path",
			gomcp.Required(),
			gomcp.Description("Existing pinned path under system/. Example: \"system/conventions.md\" -> \"conventions.md\"."),
		),
		gomcp.WithString("branch",
			gomcp.Description("Optional branch to commit to. When omitted, uses default memory branch."),
		),
	)
	h.server.AddTool(memUnpin, handleMemoryUnpin(mgr, repoMgr, legacyRepoMgr))

	memBranches := gomcp.NewTool("memory_branches",
		gomcp.WithDescription("Use this to inspect memory branches before ref-based reads/writes. Example: memory_branches(scope=\"repo\")."),
		gomcp.WithReadOnlyHintAnnotation(true),
		gomcp.WithString("scope",
			gomcp.Description("Branch scope: \"repo\" (default), \"global\", or \"all\"."),
		),
	)
	h.server.AddTool(memBranches, handleMemoryBranches(mgr, repoMgr, legacyRepoMgr))

	memBranchCreate := gomcp.NewTool("memory_branch_create",
		gomcp.WithDescription("Use this to create a memory branch for isolated edits. Example: memory_branch_create(name=\"feature/memory\", from_ref=\"main\", scope=\"repo\")."),
		gomcp.WithString("name",
			gomcp.Required(),
			gomcp.Description("New branch name."),
		),
		gomcp.WithString("from_ref",
			gomcp.Description("Optional source ref. Default is the memory default branch."),
		),
		gomcp.WithString("scope",
			gomcp.Description("Branch scope: \"repo\" (default) or \"global\"."),
		),
	)
	h.server.AddTool(memBranchCreate, handleMemoryBranchCreate(mgr, repoMgr))

	memBranchDelete := gomcp.NewTool("memory_branch_delete",
		gomcp.WithDescription("Use this to remove a memory branch after merge or cleanup. Example: memory_branch_delete(name=\"feature/memory\", force=true, scope=\"repo\")."),
		gomcp.WithString("name",
			gomcp.Required(),
			gomcp.Description("Branch name to delete."),
		),
		gomcp.WithBoolean("force",
			gomcp.Description("Force delete (uses -D)."),
		),
		gomcp.WithString("scope",
			gomcp.Description("Branch scope: \"repo\" (default) or \"global\"."),
		),
	)
	h.server.AddTool(memBranchDelete, handleMemoryBranchDelete(mgr, repoMgr))

	memBranchMerge := gomcp.NewTool("memory_branch_merge",
		gomcp.WithDescription("Use this to merge memory branch changes. Example: memory_branch_merge(source=\"feature/memory\", target=\"main\", strategy=\"ff-only\", scope=\"repo\")."),
		gomcp.WithString("source",
			gomcp.Required(),
			gomcp.Description("Source branch/ref to merge from."),
		),
		gomcp.WithString("target",
			gomcp.Description("Target branch/ref. Defaults to the memory default branch."),
		),
		gomcp.WithString("strategy",
			gomcp.Description("Merge strategy: \"ff-only\" (default) or \"no-ff\"."),
		),
		gomcp.WithString("scope",
			gomcp.Description("Branch scope: \"repo\" (default) or \"global\"."),
		),
	)
	h.server.AddTool(memBranchMerge, handleMemoryBranchMerge(mgr, repoMgr))

	memDiff := gomcp.NewTool("memory_diff",
		gomcp.WithDescription("Use this to compare memory content between refs. Example: memory_diff(base_ref=\"main\", head_ref=\"feature/memory\", path=\"notes.md\", scope=\"repo\")."),
		gomcp.WithReadOnlyHintAnnotation(true),
		gomcp.WithString("base_ref",
			gomcp.Required(),
			gomcp.Description("Base ref for diff."),
		),
		gomcp.WithString("head_ref",
			gomcp.Required(),
			gomcp.Description("Head ref for diff."),
		),
		gomcp.WithString("path",
			gomcp.Description("Optional existing file path filter. Use repos/<repo-slug>/... for explicit repo targeting."),
		),
		gomcp.WithString("scope",
			gomcp.Description("Diff scope: \"repo\" (default) or \"global\"."),
		),
	)
	h.server.AddTool(memDiff, handleMemoryDiff(mgr, repoMgr, legacyRepoMgr))
}

// registerMemorySkills registers subagent-based memory maintenance skills (Tier 3 only).
func (h *HivemindMCPServer) registerMemorySkills() {
	mgr := h.memoryMgr

	memInit := gomcp.NewTool("memory_init",
		gomcp.WithDescription(
			"Bootstrap the memory store from codebase analysis. Spawns a specialized sub-agent that "+
				"examines the codebase, user environment, and project structure, then creates organized "+
				"memory files: system/global.md (hardware, OS, tools), system/conventions.md (code patterns), "+
				"and project-specific files. Call this once when memory is empty or after major project changes.",
		),
	)
	h.server.AddTool(memInit, handleMemoryInit(mgr, h.brainClient, h.repoPath, h.instanceID))

	memReflect := gomcp.NewTool("memory_reflect",
		gomcp.WithDescription(
			"Review recent memory activity and persist insights. Spawns a sub-agent that reads "+
				"the git history of recent memory changes, identifies patterns and duplicates, "+
				"consolidates related notes, and writes a dated reflection summary. "+
				"Useful after a busy session or when memory feels cluttered.",
		),
	)
	h.server.AddTool(memReflect, handleMemoryReflect(mgr, h.brainClient, h.repoPath, h.instanceID))

	memDefrag := gomcp.NewTool("memory_defrag",
		gomcp.WithDescription(
			"Reorganize aging memory files for clarity and efficiency. Spawns a sub-agent that "+
				"reviews the entire memory store, merges small related files, splits overly large ones, "+
				"ensures every file has a descriptive YAML frontmatter header, and pins critical reference "+
				"material to system/. Target: 15-25 focused files in logical directories.",
		),
	)
	h.server.AddTool(memDefrag, handleMemoryDefrag(mgr, h.brainClient, h.repoPath, h.instanceID))
}

// registerTier1Tools registers read-only Tier 1 tools.
func (h *HivemindMCPServer) registerTier1Tools() {
	listInstances := gomcp.NewTool("list_instances",
		gomcp.WithDescription(
			"See all Hivemind instances, their status, current activity, and branch. "+
				"Use this to understand what the swarm is working on before starting work.",
		),
		gomcp.WithReadOnlyHintAnnotation(true),
	)
	h.server.AddTool(listInstances, handleListInstances(h.stateReader, h.repoPath))

	getBrain := gomcp.NewTool("get_brain",
		gomcp.WithDescription(
			"Read the shared coordination state: what each agent is working on, which files "+
				"they're touching, and any messages for you. Call this at the start of every task "+
				"and periodically to stay in sync with the team.",
		),
		gomcp.WithReadOnlyHintAnnotation(true),
	)
	h.server.AddTool(getBrain, handleGetBrain(h.brainClient, h.repoPath, h.instanceID))
}

// registerTier2Tools registers self-introspection and coordination tools.
func (h *HivemindMCPServer) registerTier2Tools() {
	getSessionSummary := gomcp.NewTool("get_my_session_summary",
		gomcp.WithDescription(
			"Get a summary of your own session: changed files, commit history, and diff stats. "+
				"Use this to understand your progress or prepare a PR description.",
		),
		gomcp.WithReadOnlyHintAnnotation(true),
	)
	h.server.AddTool(getSessionSummary, handleGetMySessionSummary(h.stateReader, h.instanceID))

	getMyDiff := gomcp.NewTool("get_my_diff",
		gomcp.WithDescription(
			"Get the full git diff of your session's changes since the base commit. "+
				"Use this to review your own work before submitting.",
		),
		gomcp.WithReadOnlyHintAnnotation(true),
	)
	h.server.AddTool(getMyDiff, handleGetMyDiff(h.stateReader, h.instanceID))

	updateStatus := gomcp.NewTool("update_status",
		gomcp.WithDescription(
			"Declare what feature you're working on and which files you're touching. "+
				"Call this at the start of every task and when you switch files. "+
				"Returns warnings if another agent is already working on the same files.",
		),
		gomcp.WithString("feature",
			gomcp.Required(),
			gomcp.Description("Short description of the feature or task you're working on."),
		),
		gomcp.WithString("files",
			gomcp.Description("Comma-separated list of file paths you're actively editing."),
		),
		gomcp.WithString("role",
			gomcp.Description("Your role: coder, reviewer, architect, tester, or custom. Visible to other agents."),
		),
	)
	h.server.AddTool(updateStatus, handleUpdateStatus(h.brainClient, h.repoPath, h.instanceID))

	sendMessage := gomcp.NewTool("send_message",
		gomcp.WithDescription(
			"Send a message to another agent or broadcast to all. "+
				"Use this to coordinate work, warn about breaking changes, or share discoveries. "+
				"Leave 'to' empty to broadcast to all agents.",
		),
		gomcp.WithString("to",
			gomcp.Description("Instance title of the target agent. Leave empty to broadcast to all."),
		),
		gomcp.WithString("message",
			gomcp.Required(),
			gomcp.Description("The message content. Be concise and actionable."),
		),
	)
	h.server.AddTool(sendMessage, handleSendMessage(h.brainClient, h.repoPath, h.instanceID))
}

// registerTier3Tools registers write/action tools for agent lifecycle, coordination, and workflows.
func (h *HivemindMCPServer) registerTier3Tools() {
	createInstance := gomcp.NewTool("create_instance",
		gomcp.WithDescription(
			"Spawn a new agent instance in Hivemind. The new agent starts in the same repository "+
				"with its own git worktree and tmux session. Use this to delegate independent subtasks "+
				"or create specialized agents (reviewers, testers, architects). "+
				"The sub-agent has no context from your conversation — provide a clear, self-contained prompt.",
		),
		gomcp.WithString("title",
			gomcp.Required(),
			gomcp.Description("Unique name for the new instance (alphanumeric, hyphens, underscores)."),
		),
		gomcp.WithString("program",
			gomcp.Description("Agent program to run (e.g., 'claude', 'aider'). Defaults to the TUI's configured program."),
		),
		gomcp.WithString("prompt",
			gomcp.Description("Initial prompt to send to the agent after it starts."),
		),
		gomcp.WithString("role",
			gomcp.Description("Agent role: coder, reviewer, architect, tester, or custom. Visible to other agents via get_brain."),
		),
		gomcp.WithString("topic",
			gomcp.Description("Topic to assign the new instance to. If omitted, uses the creating agent's topic."),
		),
		gomcp.WithBoolean("skip_permissions",
			gomcp.Description("Run the new agent with --dangerously-skip-permissions for autonomous operation. Defaults to true."),
		),
	)
	h.server.AddTool(createInstance, handleCreateInstance(h.brainClient, h.repoPath, h.instanceID))

	injectMessage := gomcp.NewTool("inject_message",
		gomcp.WithDescription(
			"Inject a message directly into another agent's terminal input, bypassing the polling-based "+
				"message system. Use this for urgent coordination — the target agent will see the message "+
				"immediately as if it were typed into their terminal.",
		),
		gomcp.WithString("to",
			gomcp.Required(),
			gomcp.Description("Instance title of the target agent."),
		),
		gomcp.WithString("message",
			gomcp.Required(),
			gomcp.Description("The message content to inject."),
		),
	)
	h.server.AddTool(injectMessage, handleInjectMessage(h.brainClient, h.repoPath, h.instanceID))

	pauseInstance := gomcp.NewTool("pause_instance",
		gomcp.WithDescription("Pause another agent instance. The agent's tmux session is preserved but execution stops."),
		gomcp.WithString("target",
			gomcp.Required(),
			gomcp.Description("Instance title of the agent to pause."),
		),
	)
	h.server.AddTool(pauseInstance, handlePauseInstance(h.brainClient, h.repoPath, h.instanceID))

	resumeInstance := gomcp.NewTool("resume_instance",
		gomcp.WithDescription("Resume a paused agent instance."),
		gomcp.WithString("target",
			gomcp.Required(),
			gomcp.Description("Instance title of the agent to resume."),
		),
	)
	h.server.AddTool(resumeInstance, handleResumeInstance(h.brainClient, h.repoPath, h.instanceID))

	killInstance := gomcp.NewTool("kill_instance",
		gomcp.WithDescription("Terminate another agent instance. This kills the tmux session and cleans up the worktree."),
		gomcp.WithString("target",
			gomcp.Required(),
			gomcp.Description("Instance title of the agent to kill."),
		),
	)
	h.server.AddTool(killInstance, handleKillInstance(h.brainClient, h.repoPath, h.instanceID))

	defineWorkflow := gomcp.NewTool("define_workflow",
		gomcp.WithDescription(
			"Define a workflow as a directed acyclic graph (DAG) of tasks with dependencies. "+
				"Tasks whose dependencies are already satisfied are triggered immediately, each spawning "+
				"a new agent instance. When a task completes via complete_task, downstream dependents "+
				"are automatically triggered. Use this for multi-step tasks that have a clear dependency structure.",
		),
		gomcp.WithString("tasks_json",
			gomcp.Required(),
			gomcp.Description("JSON array of task objects: [{\"id\": \"task-1\", \"title\": \"Implement feature\", \"depends_on\": [], \"prompt\": \"...\", \"role\": \"coder\"}, ...]"),
		),
	)
	h.server.AddTool(defineWorkflow, handleDefineWorkflow(h.brainClient, h.repoPath, h.instanceID))

	completeTask := gomcp.NewTool("complete_task",
		gomcp.WithDescription(
			"Mark a workflow task as completed or failed. If completed, dependent tasks in the "+
				"DAG will be automatically triggered (spawning new agent instances).",
		),
		gomcp.WithString("task_id",
			gomcp.Required(),
			gomcp.Description("The ID of the task to complete."),
		),
		gomcp.WithString("status",
			gomcp.Description("Task status: 'done' (default) or 'failed'."),
		),
		gomcp.WithString("error",
			gomcp.Description("Error message if the task failed."),
		),
	)
	h.server.AddTool(completeTask, handleCompleteTask(h.brainClient, h.repoPath, h.instanceID))

	getWorkflow := gomcp.NewTool("get_workflow",
		gomcp.WithDescription("Get the current workflow DAG: all tasks, their statuses, and dependencies."),
		gomcp.WithReadOnlyHintAnnotation(true),
	)
	h.server.AddTool(getWorkflow, handleGetWorkflow(h.brainClient, h.repoPath, h.instanceID))

	waitForEvents := gomcp.NewTool("wait_for_events",
		gomcp.WithDescription(
			"Long-poll for real-time events (status changes, instance lifecycle, messages, workflow triggers). "+
				"On first call, omit subscriber_id to create a subscription with your filter. "+
				"On subsequent calls, pass the returned subscriber_id to continue receiving events. "+
				"Events are buffered server-side so none are missed between polls. "+
				"Use this instead of polling get_brain in a loop.",
		),
		gomcp.WithString("subscriber_id",
			gomcp.Description("Subscription ID from a previous call. Omit on first call to create a new subscription."),
		),
		gomcp.WithString("types",
			gomcp.Description("Comma-separated event types to filter: status_changed, message_received, agent_removed, "+
				"workflow_defined, task_completed, task_triggered, instance_status_changed, instance_created, instance_killed. "+
				"Leave empty for all types."),
		),
		gomcp.WithString("instances",
			gomcp.Description("Comma-separated instance titles to filter events by source. Leave empty for all."),
		),
		gomcp.WithString("parent_title",
			gomcp.Description("Only receive events about children of this parent agent."),
		),
		gomcp.WithNumber("timeout",
			gomcp.Description("How long to wait for events in seconds (1-25, default 15)."),
		),
	)
	h.server.AddTool(waitForEvents, handleWaitForEvents(h.brainClient, h.repoPath, h.instanceID))

	unsubscribeEvents := gomcp.NewTool("unsubscribe_events",
		gomcp.WithDescription("Remove an event subscription. Call this when you no longer need to receive events."),
		gomcp.WithString("subscriber_id",
			gomcp.Required(),
			gomcp.Description("The subscription ID to remove."),
		),
	)
	h.server.AddTool(unsubscribeEvents, handleUnsubscribeEvents(h.brainClient, h.repoPath, h.instanceID))
}

// Serve starts the MCP server using stdio transport.
func (h *HivemindMCPServer) Serve() error {
	return mcpserver.ServeStdio(h.server)
}
