# GoCode CLI MVP

GoCode is a local coding agent CLI written in Go. It follows the MVP PRD by combining:

- natural language task input
- an OpenAI-compatible chat model
- file and search tools inside a single workspace
- guarded command execution
- validation feedback and session summaries

## Features

- interactive CLI with `/plan`, `/diff`, `/files`, `/undo`, `/exit`
- workspace-scoped file operations and text search
- patch-first editing via `apply_patch`
- dangerous action confirmation for delete and overwrite flows
- command whitelist with captured output
- per-task session summary with files, changes, and command results

## Environment

Set the following environment variables before running the agent:

```bash
set OPENAI_API_KEY=your_key_here
set OPENAI_MODEL=gpt-4.1-mini
set OPENAI_BASE_URL=https://api.openai.com/v1
```

Optional settings:

```bash
set GOCODE_ALLOWED_COMMANDS=go,git,npm,pnpm,bun,node,python,pytest
set GOCODE_MAX_TURNS=10
```

## Run

```bash
go run ./cmd/gocode -workspace C:\path\to\repo
```

If `-workspace` is omitted, GoCode uses the current working directory.

## Notes

- all file operations are restricted to the selected workspace
- command execution is limited by an allowlist
- `/undo` reverts the last task's file mutations recorded in the current session
