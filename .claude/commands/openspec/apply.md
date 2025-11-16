---
name: OpenSpec: 应用
description: 实施一个已批准的 OpenSpec 变更，并保持任务同步。
category: OpenSpec
tags: [openspec, apply]
---
<!-- OPENSPEC:START -->
**基本原则**
- 优先采用直接、最小化的实现，仅在被要求或明确需要时才增加复杂性。
- 保持变更范围与所要求的结果紧密相关。
- 如果需要额外的 OpenSpec 约定或说明，请参阅 `openspec/AGENTS.md` (位于 `openspec/` 目录中 — 如果你看不到它，请运行 `ls openspec` 或 `openspec update`)。

**步骤**
将这些步骤作为 TODO 任务进行跟踪，并逐一完成。
1. 阅读 `changes/<id>/proposal.md`、`design.md` (如果存在) 和 `tasks.md` 以确认范围和验收标准。
2. 按顺序完成任务，保持编辑的最小化，并专注于所要求的变更。
3. 在更新状态之前确认完成 — 确保 `tasks.md` 中的每个项目都已完成。
4. 所有工作完成后更新清单，以便每个任务都标记为 `- [x]` 并反映实际情况。
5. 当需要额外上下文时，引用 `openspec list` 或 `openspec show <item>`。

**参考**
- 如果在实施过程中需要提案中的额外上下文，请使用 `openspec show <id> --json --deltas-only`。
<!-- OPENSPEC:END -->
