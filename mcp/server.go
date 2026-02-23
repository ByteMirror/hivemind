package mcp

import (
	"github.com/ByteMirror/hivemind/memory"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

const serverInstructions = "You are running inside Hivemind, a multi-agent orchestration system. " +
	"You may be one of several agents working in parallel on the same codebase. " +
	"IMPORTANT: At the start of every task, call update_status with your feature, files, and role. " +
	"Call get_brain regularly to check for messages and avoid conflicts. " +
	"Use send_message to coordinate with teammates when working on related areas. " +
	"You can spawn new agents with create_instance, inject urgent messages into other agents' " +
	"terminals with inject_message, and manage agent lifecycles with pause/resume/kill_instance. " +
	"For complex multi-step tasks, use define_workflow to create a task DAG with dependencies, " +
	"and complete_task to mark tasks as done (which auto-triggers dependent tasks). " +
	"To wait for sub-agents to finish, use wait_for_events instead of polling get_brain in a loop. " +
	"It long-polls for real-time events (status changes, messages, workflow triggers) with no missed events." +
	"\n\nThis IDE has a persistent memory store shared across all sessions and projects.\n" +
	"Rules:\n" +
	"- Call memory_search at the start of every session and before answering questions about the user's preferences, setup, past decisions, or active projects.\n" +
	"- Call memory_write whenever you learn something durable: user setup, preferences, project decisions, recurring patterns.\n" +
	"- Write stable facts (hardware, OS, global preferences) with scope=\"global\" — this writes to system/global.md which is always in agent context. Write project decisions with scope=\"repo\" (dated files default to repo).\n" +
	"- Never assume you know the user's preferences — search first.\n" +
	"- Files in system/ are always injected into CLAUDE.md — use memory_pin to promote important files there.\n" +
	"- Use memory_tree to see the file structure. All memory changes are git-versioned; use memory_history to review."

// HivemindMCPServer wraps an MCP server with Hivemind-specific state.
type HivemindMCPServer struct {
	server      *mcpserver.MCPServer
	stateReader *StateReader
	brainClient BrainClient
	instanceID  string // used by Tier 2 introspection tools
	repoPath    string // scopes brain and instance listing to this repo
	tier        int    // gates tool registration: 1=read, 2=+introspect, 3=+write
	memoryMgr    *memory.Manager
	repoMemoryMgr *memory.Manager // scoped to the current repo (may be nil)
}

// NewHivemindMCPServer creates a new MCP server for a Hivemind agent.
func NewHivemindMCPServer(brainClient BrainClient, hivemindDir, instanceID, repoPath string, tier int, memMgr *memory.Manager, repoMemMgr *memory.Manager) *HivemindMCPServer {
	s := mcpserver.NewMCPServer(
		"hivemind",
		"0.1.0",
		mcpserver.WithInstructions(serverInstructions),
	)

	h := &HivemindMCPServer{
		server:      s,
		stateReader: NewStateReader(hivemindDir),
		brainClient: brainClient,
		instanceID:    instanceID,
		repoPath:      repoPath,
		tier:          tier,
		memoryMgr:     memMgr,
		repoMemoryMgr: repoMemMgr,
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

	// --- Read-only tools ---

	memRead := gomcp.NewTool("memory_read",
		gomcp.WithDescription("Read full body of a memory file (frontmatter stripped)."),
		gomcp.WithReadOnlyHintAnnotation(true),
		gomcp.WithString("path",
			gomcp.Required(),
			gomcp.Description("Relative path within ~/.hivemind/memory/."),
		),
	)
	h.server.AddTool(memRead, handleMemoryRead(mgr))

	memSearch := gomcp.NewTool("memory_search",
		gomcp.WithDescription(
			"Search IDE-wide memory before answering questions about prior work, "+
				"user preferences, project setups, or past decisions. "+
				"Returns ranked snippets with file path and line numbers.",
		),
		gomcp.WithReadOnlyHintAnnotation(true),
		gomcp.WithString("query",
			gomcp.Required(),
			gomcp.Description("Natural language search query."),
		),
		gomcp.WithNumber("max_results",
			gomcp.Description("Maximum results to return (default 10)."),
		),
	)
	h.server.AddTool(memSearch, handleMemorySearch(mgr))

	memGet := gomcp.NewTool("memory_get",
		gomcp.WithDescription(
			"Read specific lines from a memory file. "+
				"Use after memory_search to pull only the relevant lines.",
		),
		gomcp.WithReadOnlyHintAnnotation(true),
		gomcp.WithString("path",
			gomcp.Required(),
			gomcp.Description("Relative path within ~/.hivemind/memory/ (from memory_search results)."),
		),
		gomcp.WithNumber("from",
			gomcp.Description("Start line number (1-indexed, default 1)."),
		),
		gomcp.WithNumber("lines",
			gomcp.Description("Number of lines to read (default: entire file)."),
		),
	)
	h.server.AddTool(memGet, handleMemoryGet(mgr))

	memList := gomcp.NewTool("memory_list",
		gomcp.WithDescription("List all IDE-wide memory files with metadata."),
		gomcp.WithReadOnlyHintAnnotation(true),
	)
	h.server.AddTool(memList, handleMemoryList(mgr))

	memTree := gomcp.NewTool("memory_tree",
		gomcp.WithDescription(
			"View the memory file tree with descriptions from YAML frontmatter. "+
				"Files in system/ are always injected into agent context.",
		),
		gomcp.WithReadOnlyHintAnnotation(true),
	)
	h.server.AddTool(memTree, handleMemoryTree(mgr))

	memHistory := gomcp.NewTool("memory_history",
		gomcp.WithDescription("View git history of memory changes. Omit path for all files."),
		gomcp.WithReadOnlyHintAnnotation(true),
		gomcp.WithString("path",
			gomcp.Description("Optional: filter history to a specific file."),
		),
		gomcp.WithNumber("count",
			gomcp.Description("Number of entries to return (default 10)."),
		),
	)
	h.server.AddTool(memHistory, handleMemoryHistory(mgr))

	// --- Write tools ---

	memWrite := gomcp.NewTool("memory_write",
		gomcp.WithDescription(
			"Write to IDE-wide memory. "+
				"Use scope=\"global\" for cross-project facts (OS, hardware, user preferences) — writes to system/global.md. "+
				"Use scope=\"repo\" (or omit for dated files) for project-specific decisions. "+
				"Use this whenever you discover something worth remembering across sessions: "+
				"user preferences, project facts, environment setup, API keys configured, "+
				"decisions made and their rationale.",
		),
		gomcp.WithString("content",
			gomcp.Required(),
			gomcp.Description("The fact or note to save. Plain text or Markdown."),
		),
		gomcp.WithString("file",
			gomcp.Description("Target filename (default: YYYY-MM-DD.md). Named files (e.g. global.md) default to global scope."),
		),
		gomcp.WithString("scope",
			gomcp.Description("Storage scope: \"global\" (cross-project, OS/preferences) or \"repo\" (this project). Dated files default to repo scope."),
		),
		gomcp.WithString("commit_message",
			gomcp.Description("Optional git commit message for this change."),
		),
	)
	h.server.AddTool(memWrite, handleMemoryWrite(mgr, repoMgr))

	memAppend := gomcp.NewTool("memory_append",
		gomcp.WithDescription("Append content to an existing memory file."),
		gomcp.WithString("path",
			gomcp.Required(),
			gomcp.Description("Relative path to the memory file."),
		),
		gomcp.WithString("content",
			gomcp.Required(),
			gomcp.Description("Content to append."),
		),
	)
	h.server.AddTool(memAppend, handleMemoryAppend(mgr))

	memMove := gomcp.NewTool("memory_move",
		gomcp.WithDescription("Rename or move a memory file. Use \"/\" in the path to organize into topics."),
		gomcp.WithString("from",
			gomcp.Required(),
			gomcp.Description("Current relative path."),
		),
		gomcp.WithString("to",
			gomcp.Required(),
			gomcp.Description("New relative path."),
		),
	)
	h.server.AddTool(memMove, handleMemoryMove(mgr))

	memDelete := gomcp.NewTool("memory_delete",
		gomcp.WithDescription("Delete a memory file permanently."),
		gomcp.WithString("path",
			gomcp.Required(),
			gomcp.Description("Relative path to delete."),
		),
	)
	h.server.AddTool(memDelete, handleMemoryDelete(mgr))

	memPin := gomcp.NewTool("memory_pin",
		gomcp.WithDescription(
			"Pin a memory file by moving it to system/. "+
				"System files are always injected into agent context at startup.",
		),
		gomcp.WithString("path",
			gomcp.Required(),
			gomcp.Description("Relative path of file to pin."),
		),
	)
	h.server.AddTool(memPin, handleMemoryPin(mgr))

	memUnpin := gomcp.NewTool("memory_unpin",
		gomcp.WithDescription("Unpin a memory file by moving it out of system/ to root."),
		gomcp.WithString("path",
			gomcp.Required(),
			gomcp.Description("Relative path of file in system/ to unpin."),
		),
	)
	h.server.AddTool(memUnpin, handleMemoryUnpin(mgr))
}

// registerMemorySkills registers subagent-based memory maintenance skills (Tier 3 only).
func (h *HivemindMCPServer) registerMemorySkills() {
	mgr := h.memoryMgr

	memInit := gomcp.NewTool("memory_init",
		gomcp.WithDescription(
			"Bootstrap memory from codebase analysis. Spawns a specialized agent that "+
				"analyzes the codebase and creates system/global.md, system/conventions.md, "+
				"and project-specific memory files.",
		),
	)
	h.server.AddTool(memInit, handleMemoryInit(mgr, h.brainClient, h.repoPath, h.instanceID))

	memReflect := gomcp.NewTool("memory_reflect",
		gomcp.WithDescription(
			"Review recent memory activity and persist insights. Spawns an agent that "+
				"consolidates duplicates, identifies patterns, and writes a reflection.",
		),
	)
	h.server.AddTool(memReflect, handleMemoryReflect(mgr, h.brainClient, h.repoPath, h.instanceID))

	memDefrag := gomcp.NewTool("memory_defrag",
		gomcp.WithDescription(
			"Reorganize aging memory files for clarity. Spawns an agent that merges "+
				"duplicates, splits large files, and ensures clear frontmatter descriptions.",
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
			"Spawn a new agent instance in Hivemind. The new agent will start in the same "+
				"repository with its own worktree. Use this to delegate subtasks or create "+
				"specialized agents (reviewers, testers, etc.).",
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
			"Define a workflow DAG: a set of tasks with dependencies. Tasks whose dependencies "+
				"are already satisfied will be triggered immediately (spawning new agent instances). "+
				"Use complete_task to mark tasks as done, which triggers dependent tasks.",
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
