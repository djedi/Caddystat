# Complete Next Task

Read the task list from `TASKS.md` and complete the next uncompleted task.

## Instructions

1. **Read the task list** from `TASKS.md` in the project root

2. **Select task(s) to complete**:

   - Choose the next unchecked task (`- [ ]`) following the "Suggested Order of Attack" priority
   - If there are related tasks that logically belong together, complete them as a group
   - Quick Wins should generally be tackled first unless there's a blocking dependency

3. **Implement the task**:

   - Write clean, idiomatic Go code that follows existing patterns in the codebase
   - Follow the project's architecture (see `CLAUDE.md` for details)
   - Keep changes minimal and focused - don't over-engineer

4. **Add tests**:

   - Write unit tests for any new functionality
   - Place tests in `*_test.go` files alongside the code
   - Use table-driven tests where appropriate
   - Ensure tests pass: `go test ./...`

5. **Verify the implementation**:

   - Run `go build ./...` to ensure it compiles
   - Run `go test ./...` to ensure tests pass
   - If frontend changes, verify with `cd web && npm run build`

6. **Update TASKS.md**:

   - Mark completed tasks with `[x]`
   - Update the Progress Tracking table with new counts and percentages
   - If you discovered new tasks during implementation, add them to the appropriate section

7. **Update Documentation**:

   - If relevant, update README.md
   - If relevant, update CLAUDE.md

8. **Provide a summary**:
   - List which task(s) were completed
   - Describe what was implemented
   - Note any new files created or modified
   - Mention any issues encountered or follow-up tasks identified

## Example Output

```
## Completed Tasks

- [x] Add `GET /health` endpoint (returns DB connectivity status)
- [x] Add `--version` flag to CLI

## Summary

Implemented health check endpoint at `/health` that returns:
- HTTP 200 with `{"status": "ok", "db": "connected"}` when healthy
- HTTP 503 with `{"status": "error", "db": "disconnected"}` when DB is unreachable

Also added `--version` flag that prints the version and exits.

### Files Modified
- `internal/server/server.go` - Added `/health` handler
- `cmd/caddystat/main.go` - Added version flag parsing
- `internal/server/server_test.go` - Added health endpoint tests

### Tests Added
- `TestHealthEndpoint_Healthy`
- `TestHealthEndpoint_DBError`

All tests passing: `go test ./...` ✓
```
