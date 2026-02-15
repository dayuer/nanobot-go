# 迁移状态追踪

> 上次更新: 2026-02-16
> upstream 版本: `v0.1.3.post7`

## 状态图例

| 图标 | 含义 |
|------|------|
| ⬜ | 未开始 |
| 🔴 | 测试已写，实现未完成 (TDD Red) |
| 🟢 | 测试通过 (TDD Green) |
| ♻️ | 重构完成 |
| ✅ | 已上线，通过契约测试 |

---

## Phase 1: 基础设施

| 模块 | Python 源文件 | Go 文件 | 状态 | 契约测试 | upstream 版本 |
|------|-------------|---------|------|---------|--------------|
| bus/events | `bus/events.py` | `internal/bus/events.go` | 🟢 | n/a | `v0.1.3.post7` |
| bus/queue | `bus/queue.py` | `internal/bus/queue.go` | 🟢 | n/a | `v0.1.3.post7` |
| config/schema | `config/schema.py` | `internal/config/doc.go` | 🟢 | n/a | `v0.1.3.post7` |
| config/loader | `config/loader.py` | `internal/config/loader.go` | 🟢 | n/a | `v0.1.3.post7` |
| session/manager | `session/manager.py` | `internal/session/doc.go` | 🟢 | n/a | `v0.1.3.post7` |
| utils/helpers | `utils/helpers.py` | `internal/utils/doc.go` | 🟢 | n/a | `v0.1.3.post7` |

## Phase 2: 工具系统

| 模块 | Python 源文件 | Go 文件 | 状态 | 契约测试 | upstream 版本 |
|------|-------------|---------|------|---------|--------------|
| tools/base | `agent/tools/base.py` | `internal/tools/base.go` | 🟢 | ✅ | `v0.1.3.post7` |
| tools/registry | `agent/tools/registry.py` | `internal/tools/registry.go` | 🟢 | n/a | `v0.1.3.post7` |
| tools/shell | `agent/tools/shell.py` | `internal/tools/shell.go` | 🟢 | ✅ | `v0.1.3.post7` |
| tools/filesystem | `agent/tools/filesystem.py` | `internal/tools/filesystem.go` | 🟢 | ✅ | `v0.1.3.post7` |
| tools/web | `agent/tools/web.py` | `internal/tools/web.go` | 🟢 | ✅ | `v0.1.3.post7` |
| tools/message | `agent/tools/message.py` | `internal/tools/message.go` | 🟢 | ✅ | `v0.1.3.post7` |
| tools/spawn | `agent/tools/spawn.py` | `internal/tools/message.go` | 🟢 | ✅ | `v0.1.3.post7` |
| tools/cron | `agent/tools/cron.py` | `internal/tools/message.go` | 🟢 | ✅ | `v0.1.3.post7` |
| tools/mcp | `agent/tools/mcp.py` | `internal/tools/mcp.go` | ⬜ | ⬜ | — |

## Phase 3: LLM Provider

| 模块 | Python 源文件 | Go 文件 | 状态 | 契约测试 | upstream 版本 |
|------|-------------|---------|------|---------|--------------|
| providers/base | `providers/base.py` | `internal/providers/base.go` | 🟢 | n/a | `v0.1.3.post7` |
| providers/registry | `providers/registry.py` | `internal/providers/registry.go` | 🟢 | ✅ | `v0.1.3.post7` |
| providers/provider | `providers/litellm_provider.py` | `internal/providers/provider.go` | 🟢 | ✅ | `v0.1.3.post7` |
| providers/transcription | `providers/transcription.py` | `internal/providers/transcription.go` | ⬜ | ⬜ | — |

## Phase 4: Agent 核心

| 模块 | Python 源文件 | Go 文件 | 状态 | 契约测试 | upstream 版本 |
|------|-------------|---------|------|---------|--------------|
| agent/memory | `agent/memory.py` | `internal/agent/memory.go` | 🟢 | ✅ | `v0.1.3.post7` |
| agent/skills | `agent/skills.py` | `internal/agent/skills.go` | 🟢 | ✅ | `v0.1.3.post7` |
| agent/context | `agent/context.py` | `internal/agent/context.go` | 🟢 | ✅ | `v0.1.3.post7` |
| agent/loop | `agent/loop.py` | `internal/agent/loop.go` | 🟢 | ✅ | `v0.1.3.post7` |
| agent/subagent | `agent/subagent.py` | `internal/agent/subagent.go` | 🟢 | ✅ | `v0.1.3.post7` |

## Phase 5: 频道集成

| 模块 | Python 源文件 | Go 文件 | 状态 | 契约测试 | upstream 版本 |
|------|-------------|---------|------|---------|--------------|
| channels/base | `channels/base.py` | `internal/channels/base.go` | 🟢 | ✅ | `v0.1.3.post7` |
| channels/manager | `channels/manager.py` | `internal/channels/manager.go` | ⬜ | ⬜ | — |
| channels/telegram | `channels/telegram.py` | `internal/channels/telegram.go` | ⬜ | ⬜ | — |
| channels/discord | `channels/discord.py` | `internal/channels/discord.go` | ⬜ | ⬜ | — |
| channels/slack | `channels/slack.py` | `internal/channels/slack.go` | ⬜ | ⬜ | — |
| channels/whatsapp | `channels/whatsapp.py` | `internal/channels/whatsapp.go` | ⬜ | ⬜ | — |
| channels/feishu | `channels/feishu.py` | `internal/channels/feishu.go` | ⬜ | ⬜ | — |
| channels/dingtalk | `channels/dingtalk.py` | `internal/channels/dingtalk.go` | ⬜ | ⬜ | — |
| channels/email | `channels/email.py` | `internal/channels/email.go` | ⬜ | ⬜ | — |
| channels/qq | `channels/qq.py` | `internal/channels/qq.go` | ⬜ | ⬜ | — |
| channels/mochat | `channels/mochat.py` | `internal/channels/mochat.go` | ⬜ | ⬜ | — |

## Phase 6: CLI + E2E

| 模块 | Python 源文件 | Go 文件 | 状态 | upstream 版本 |
|------|-------------|---------|------|--------------|
| CLI | `cli/commands.py` | `cmd/*.go` | ⬜ | — |
| cron/service | `cron/service.py` | `internal/cron/service.go` | ⬜ | — |
| heartbeat | `heartbeat/service.py` | `internal/heartbeat/service.go` | ⬜ | — |
| E2E 对比 | — | `e2e/comparison_test.go` | ⬜ | — |
