# nanobot-go

Ultra-Lightweight Personal AI Assistant — Go edition.

This is a TDD-driven incremental rewrite of [nanobot](https://github.com/HKUDS/nanobot) from Python to Go, using the **Strangler Fig** pattern for smooth migration.

## Project Status

See [docs/migration_status.md](docs/migration_status.md) for per-module migration progress.

## Quick Start

```bash
# Build
make build

# Run tests
make test

# Run contract tests only
make test-contract
```

## Architecture

See [docs/implementation_plan.md](docs/implementation_plan.md) for the full TDD migration plan.

## Directory Structure

```
├── cmd/              # CLI commands (cobra)
├── internal/
│   ├── agent/        # Core agent loop, context, memory, skills
│   ├── bus/          # Async message bus (Go channels)
│   ├── channels/     # Chat platform integrations
│   ├── config/       # Configuration schema & loader
│   ├── providers/    # LLM provider interface & implementations
│   ├── session/      # Conversation session management
│   ├── tools/        # Agent tools (shell, fs, web, mcp...)
│   ├── cron/         # Scheduled tasks
│   ├── heartbeat/    # Health checks
│   └── utils/        # Shared helpers
├── contracts/        # Shared test fixtures
├── upstream/         # Git submodule → Python nanobot
└── docs/             # Documentation & migration plans
```
