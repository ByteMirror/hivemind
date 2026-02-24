# Hivemind (claude-squad)

TUI for managing multiple AI coding agents (Claude Code, Aider, Codex, Amp) in parallel. Built with Go, Bubble Tea, and tmux.

## Build / Test / Lint

```bash
go build ./...          # build all packages
go test ./...           # run all tests
go vet ./...            # static analysis
goimports -w .          # fix import ordering
```

## Project Structure

| Package | Responsibility |
|---------|---------------|
| `main.go` | CLI entry point (cobra commands) |
| `app/` | Bubble Tea application: model, update loop, input handling, actions |
| `session/` | Instance lifecycle, storage, topics, embedded terminal |
| `session/git/` | Git worktree management, diff, PR creation |
| `session/tmux/` | Tmux session management, PTY I/O |
| `ui/` | Rendering: list, preview, diff pane, sidebar, menus, overlays |
| `ui/overlay/` | Modal overlays (confirmation, text input, picker, context menu) |
| `keys/` | Key bindings and global keymap |
| `config/` | User configuration and persistent state |
| `daemon/` | Background daemon for instance monitoring |
| `log/` | Logging utilities |
| `cmd/` | CLI subcommands |
| `web/` | Next.js web dashboard (separate from Go codebase) |

## Code Conventions

- **Imports**: stdlib, blank line, external, blank line, internal. Use `goimports`.
- **Errors**: Use sentinel errors from `session/errors.go` (`ErrInstanceNotStarted`, etc.) for known conditions. Use `fmt.Errorf("...: %w", err)` for wrapping.
- **Error handling**: Check `errors.Is()` against sentinel errors, not string comparison.
- **Input handling**: Each app state has a dedicated `handle<State>Keys(msg) (tea.Model, tea.Cmd)` method in `app/app_input.go`. The `handleKeyPress` dispatcher routes by `m.state`.
- **Concurrency**: `Instance.started` is an `atomic.Bool`. Set `tmuxSession`/`gitWorktree` before storing `true`. Any field read from multiple goroutines must use atomics or a mutex.
- **File writes**: Use `atomicWriteFile` (in `config/fileutil.go`) for state/config — write to temp file, then rename.
- **File permissions**: Use `0600` for all user-private files (config, state, logs, PID). Never `0644`/`0666`.
- **Naming**: Follow Effective Go. Unexported fields/methods by default; export only what other packages need.
- **Testing**: Table-driven tests preferred. Test files live next to source (`foo_test.go`).

## Security

- **Shell injection**: Never concatenate user input into shell command strings. Use `exec.Command` with separate args. For tmux `new-session`, split the program string with `strings.Fields`.
- **Sanitization**: Tmux session names use allowlist `[a-zA-Z0-9_-]`. Branch names use `[a-z0-9\-_/.]`.
- **Git hooks**: `--no-verify` is configurable via `skip_git_hooks` in `~/.hivemind/config.json` (default: true). Set to `false` for repos with mandatory pre-commit hooks (e.g. secret scanning).
- **Path validation**: Before `os.RemoveAll`, validate the path is under the expected directory.

## Things to Avoid

- Don't use `fmt.Errorf` for error conditions that callers check — use sentinel errors.
- Don't add methods to `handleKeyPress` directly — extract a `handle<State>Keys` method.
- Don't import `session` from `ui` or `keys` — dependency flows: `app` → `session`, `ui`, `keys`.
- Don't create subpackages prematurely — this is a CLI tool, not a library.
- Don't commit `worktrees/` directory contents.
- Don't use `os.WriteFile` directly for config/state — use `atomicWriteFile`.

<!-- hivemind-memory-start -->
## Hivemind Memory

Hivemind maintains an IDE-wide persistent memory store across all sessions and projects.

### Rules

- **Before answering** any question about the user's preferences, setup, past decisions, or active projects: call `memory_search` first.
- **After every session** where you learn something durable: call `memory_write` to persist it.
- Write **stable facts** (hardware, OS, global preferences) to `global.md` using `scope="global"`. Write **project decisions** with `scope="repo"` (default for dated files).
- **When asked to write memory at session end**: Do it immediately. Call memory_write with a concise summary of: (1) what was built/changed, (2) key decisions made, (3) any user preferences expressed.

### What is worth writing to memory

- User's OS, hardware, terminal and editor setup
- API keys, services, and credentials configured
- Project tech stack decisions and the reasoning behind them
- Recurring patterns the user likes or dislikes
- Anything you had to look up or figure out that the user will likely ask again

### Tools

| Tool | When to use |
|------|-------------|
| `memory_search(query)` | Start of session, before answering questions about prior context |
| `memory_write(content, file?, scope?)` | scope="repo" for this project's decisions; scope="global" for user preferences/hardware |
| `memory_get(path, from?, lines?)` | Read specific lines from a memory file |
| `memory_list()` | Browse all memory files |

### Global context

**[global.md L3]** ## Hardware & OS
- **Machine**: Apple M4 Pro, 24 GB RAM
- **OS**: macOS 26.2 (Darwin 25.2.0, kernel arm64) — codename Tahoe
- **Shell**: zsh
- **Hostname**: DE08-M0079
- **User**: fabian.urbanek

**[global.md L1]** # Global Setup

### Repo context (memory_1896e0b419b5aa60)

*(no repo memory yet)*

<!-- hivemind-memory-end -->
