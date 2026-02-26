# Memory Git Features — Agent Smoke Test

Paste the prompt below into a single agent panel.

---

You are running a smoke test for Hivemind memory git features.

## Rules
- Use only memory MCP tools for all memory operations.
- Do not skip steps.
- If a step fails, continue and mark it failed in the final report.
- Use this exact file path: `smoke.md`.
- Use these exact commit messages when writing:
  - `memory: smoke main seed`
  - `memory: smoke branch change`

## Steps
1. Call `memory_branches(scope="repo")`.
2. From the response, capture:
   - `default_branch` as `DEFAULT_BRANCH`
   - create `TEST_BRANCH = "smoke-e2e"`
3. Call `memory_branch_delete(name=TEST_BRANCH, force=true, scope="repo")` to clean old runs (ignore errors).
4. Seed default branch:
   - Call `memory_write(content="# Smoke Test\n\nbase: main branch", file="smoke.md", scope="repo", commit_message="memory: smoke main seed", branch=DEFAULT_BRANCH)`.

5. Call `memory_branch_create(name=TEST_BRANCH, from_ref=DEFAULT_BRANCH, scope="repo")`.

6. Write on test branch (must not touch default branch yet):
   - Call `memory_write(content="# Smoke Test\n\nbase: main branch\nbranch: smoke-e2e change", file="smoke.md", scope="repo", commit_message="memory: smoke branch change", branch=TEST_BRANCH)`.
   - Call `memory_append(path="smoke.md", content="branch: appended line", branch=TEST_BRANCH)`.

7. Validate ref-aware reads:
   - Call `memory_read(path="smoke.md", ref=DEFAULT_BRANCH)` and verify it does **not** include `branch: appended line`.
   - Call `memory_read(path="smoke.md", ref=TEST_BRANCH)` and verify it **does** include `branch: appended line`.

8. Validate ref-aware listing/tree:
   - Call `memory_list(ref=DEFAULT_BRANCH)`.
   - Call `memory_tree(ref=TEST_BRANCH)`.
   - Confirm `smoke.md` appears.

9. Validate branch-filtered history metadata:
   - Call `memory_history(path="smoke.md", scope="repo", branch=TEST_BRANCH, count=10)`.
   - Confirm entries include fields: `sha`, `parent_sha`, `message`, `date`, `scope`, `branch`, `author_name`, `author_email`, `additions`, `files`. (`deletions` may be omitted when 0)
   - Confirm one commit message is exactly `memory: smoke branch change`.

10. Validate diff:
   - Call `memory_diff(base_ref=DEFAULT_BRANCH, head_ref=TEST_BRANCH, path="smoke.md", scope="repo")`.
   - Confirm diff contains at least one added line with `+branch:`.

11. Merge and verify:
   - Call `memory_branch_merge(source=TEST_BRANCH, target=DEFAULT_BRANCH, strategy="ff-only", scope="repo")`.
   - Call `memory_read(path="smoke.md", ref=DEFAULT_BRANCH)` and confirm it now includes `branch: appended line`.

12. Validate merged history scope behavior:
   - Call `memory_history(path="smoke.md", scope="all", count=10)`.
   - Confirm each returned entry has a `scope` field and that repo entries are present.

13. Cleanup:
   - Call `memory_branch_delete(name=TEST_BRANCH, force=true, scope="repo")`.

## Final output format
Return:
1. A checklist table with `step`, `pass/fail`, `evidence`.
2. The exact `DEFAULT_BRANCH` used.
3. The short SHA + message list for `smoke.md` from repo history.
4. Any failures with likely root cause.

---

## Optional UI Check (manual)
After the agent passes:
1. Open Memory Manager UI.
2. Select `smoke.md`.
3. Open history view.
4. Confirm the diff preview shows:
   - colored/styled `+` additions
   - colored/styled `-` deletions
   - styled `@@` hunk headers
   - truncation hint like `… N more diff line(s)` for long diffs.
