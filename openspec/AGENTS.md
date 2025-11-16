# OpenSpec 指南

本指南为使用 OpenSpec 进行规范驱动开发的 AI 编程助手提供指导。

## TL;DR 快速清单

- 搜索现有工作：`openspec spec list --long`，`openspec list` (仅在需要全文搜索时使用 `rg`)
- 决定范围：是新功能还是修改现有功能
- 选择一个唯一的 `change-id`：使用 kebab-case (短横线分隔命名法)，并以动词开头 (`add-`, `update-`, `remove-`, `refactor-`)
- 构建基本框架：`proposal.md`、`tasks.md`、`design.md` (仅在需要时)，以及每个受影响功能的增量规范
- 编写增量：使用 `## ADDED|MODIFIED|REMOVED|RENAMED Requirements`；每个需求至少包含一个 `#### Scenario:`
- 验证：`openspec validate [change-id] --strict` 并修复问题
- 请求批准：在提案获得批准之前，不要开始实施

## 三阶段工作流

### 阶段 1: 创建变更

在需要进行以下操作时创建提案：

- 添加特性或功能
- 进行重大变更 (API, schema)
- 改变架构或模式
- 优化性能 (会改变行为)
- 更新安全模式

触发词 (示例):

- "帮我创建一个变更提案"
- "帮我规划一个变更"
- "帮我创建一个提案"
- "我想创建一个规范提案"
- "我想创建一个规范"

模糊匹配指南:

- 包含以下词语之一: `proposal`, `change`, `spec`
- 并包含以下词语之一: `create`, `plan`, `make`, `start`, `help`

在以下情况下跳过提案：

- Bug 修复 (恢复预期行为)
- 拼写错误、格式化、注释
- 依赖更新 (非重大变更)
- 配置更改
- 为现有行为编写测试

**工作流**

1. 回顾 `openspec/project.md`、`openspec list` 和 `openspec list --specs` 来理解当前上下文。
2. 选择一个唯一的、以动词开头的 `change-id`，并在 `openspec/changes/<id>/` 下构建 `proposal.md`、`tasks.md`、可选的 `design.md` 和规范增量文件。
3. 使用 `## ADDED|MODIFIED|REMOVED Requirements` 起草规范增量，每个需求至少包含一个 `#### Scenario:`。
4. 运行 `openspec validate <id> --strict` 并在分享提案前解决所有问题。

### 阶段 2: 实施变更

将这些步骤作为 TODO 任务进行跟踪，并逐一完成。

1. **阅读 proposal.md** - 理解正在构建什么
2. **阅读 design.md** (如果存在) - 回顾技术决策
3. **阅读 tasks.md** - 获取实施清单
4. **按顺序实施任务** - 按顺序完成
5. **确认完成** - 在更新状态前，确保 `tasks.md` 中的每个项目都已完成
6. **更新清单** - 所有工作完成后，将每个任务标记为 `- [x]`，使其反映实际情况
7. **批准关卡** - 在提案被审查和批准之前，不要开始实施

### 阶段 3: 归档变更

部署后，创建一个单独的 PR 来：

- 将 `changes/[name]/` 移动到 `changes/archive/YYYY-MM-DD-[name]/`
- 如果功能有变，则更新 `specs/`
- 对于仅涉及工具的变更，使用 `openspec archive <change-id> --skip-specs --yes` (始终明确传递变更 ID)
- 运行 `openspec validate --strict` 确认归档的变更通过检查

## 在任何任务开始前

**上下文清单:**

- [ ] 阅读 `specs/[capability]/spec.md` 中的相关规范
- [ ] 检查 `changes/` 中待处理的变更以避免冲突
- [ ] 阅读 `openspec/project.md` 了解项目约定
- [ ] 运行 `openspec list` 查看活动的变更
- [ ] 运行 `openspec list --specs` 查看已有的功能

**创建规范前:**

- 始终检查功能是否已存在
- 优先修改现有规范，而不是创建重复的规范
- 使用 `openspec show [spec]` 回顾当前状态
- 如果请求不明确，在构建框架前提出 1-2 个澄清问题

### 搜索指南

- 枚举规范: `openspec spec list --long` (或使用 `--json` 以便脚本处理)
- 枚举变更: `openspec list` (或 `openspec change list --json` - 已弃用但可用)
- 显示详情:
  - 规范: `openspec show <spec-id> --type spec` (使用 `--json` 进行过滤)
  - 变更: `openspec show <change-id> --json --deltas-only`
- 全文搜索 (使用 ripgrep): `rg -n "Requirement:|Scenario:" openspec/specs`

## 快速开始

### CLI 命令

```bash
# 基本命令
openspec list                  # 列出活动的变更
openspec list --specs          # 列出规范
openspec show [item]           # 显示变更或规范
openspec validate [item]       # 验证变更或规范
openspec archive <change-id> [--yes|-y]   # 部署后归档 (在非交互式运行时添加 --yes)

# 项目管理
openspec init [path]           # 初始化 OpenSpec
openspec update [path]         # 更新指南文件

# 交互模式
openspec show                  # 提示进行选择
openspec validate              # 批量验证模式

# 调试
openspec show [change] --json --deltas-only
openspec validate [change] --strict
```

### 命令标志

- `--json` - 机器可读的输出
- `--type change|spec` - 明确指定项目类型
- `--strict` - 全面验证
- `--no-interactive` - 禁用提示
- `--skip-specs` - 归档时不更新规范
- `--yes`/`-y` - 跳过确认提示 (非交互式归档)

## 目录结构

```
openspec/
├── project.md              # 项目约定
├── specs/                  # 当前的真实情况 - 已经构建了什么
│   └── [capability]/       # 单个专注的功能
│       ├── spec.md         # 需求和场景
│       └── design.md       # 技术模式
├── changes/                # 提案 - 应该改变什么
│   ├── [change-name]/
│   │   ├── proposal.md     # 为什么、什么、影响
│   │   ├── tasks.md        # 实施清单
│   │   ├── design.md       # 技术决策 (可选; 参见标准)
│   │   └── specs/          # 增量变更
│   │       └── [capability]/
│   │           └── spec.md # ADDED/MODIFIED/REMOVED
│   └── archive/            # 已完成的变更
```

## 创建变更提案

### 决策树

```
新请求?
├─ Bug 修复，恢复规范行为? → 直接修复
├─ 拼写/格式/注释? → 直接修复  
├─ 新特性/功能? → 创建提案
├─ 重大变更? → 创建提案
├─ 架构变更? → 创建提案
└─ 不清楚? → 创建提案 (更安全)
```

### 提案结构

1. **创建目录:** `changes/[change-id]/` (kebab-case, 动词开头, 唯一)

2. **编写 proposal.md:**

```markdown
# 变更: [变更的简要描述]

## 为什么
[1-2句话说明问题/机会]

## 变更内容
- [变更点列表]
- [用 **BREAKING** 标记重大变更]

## 影响
- 受影响的规范: [列出功能]
- 受影响的代码: [关键文件/系统]
```

3. **创建规范增量:** `specs/[capability]/spec.md`

```markdown
## ADDED Requirements (新增需求)
### Requirement: New Feature (新功能)
系统应当提供...

#### Scenario: Success case (成功场景)
- **WHEN** 用户执行操作
- **THEN** 预期的结果

## MODIFIED Requirements (修改的需求)
### Requirement: Existing Feature (现有功能)
[完整的修改后需求]

## REMOVED Requirements (移除的需求)
### Requirement: Old Feature (旧功能)
**Reason**: [移除原因]
**Migration**: [如何处理]
```

如果影响多个功能，则在 `changes/[change-id]/specs/<capability>/spec.md` 下为每个功能创建一个增量文件。

4. **创建 tasks.md:**

```markdown
## 1. 实施
- [ ] 1.1 创建数据库 schema
- [ ] 1.2 实现 API 端点
- [ ] 1.3 添加前端组件
- [ ] 1.4 编写测试
```

5. **在需要时创建 design.md:**
如果满足以下任一条件，则创建 `design.md`；否则省略它：

- 跨领域的变更 (多个服务/模块) 或新的架构模式
- 新的外部依赖或显著的数据模型变更
- 安全性、性能或迁移的复杂性
- 在编码前需要通过技术决策来消除的模糊性

最小化的 `design.md` 骨架:

```markdown
## 背景
[背景, 限制, 相关方]

## 目标 / 非目标
- 目标: [...]
- 非目标: [...]

## 决策
- 决策: [内容和原因]
- 考虑过的替代方案: [选项 + 基本原理]

## 风险 / 权衡
- [风险] → 缓解措施

## 迁移计划
[步骤, 回滚方案]

## 开放问题
- [...]
```

## 规范文件格式

### 关键: 场景格式化

**正确** (使用 #### 标题):

```markdown
#### Scenario: 用户登录成功
- **WHEN** 提供了有效的凭据
- **THEN** 返回 JWT 令牌
```

**错误** (不要使用项目符号或粗体):

```markdown
- **Scenario: 用户登录**  ❌
**Scenario**: 用户登录     ❌
### Scenario: 用户登录      ❌
```

每个需求必须至少有一个场景。

### 需求措辞

- 对规范性需求使用 SHALL/MUST (除非有意设为非规范性，否则避免使用 should/may)

### 增量操作

- `## ADDED Requirements` - 新功能
- `## MODIFIED Requirements` - 行为变更
- `## REMOVED Requirements` - 废弃的功能
- `## RENAMED Requirements` - 名称变更

标题匹配时会使用 `trim(header)` - 空白符会被忽略。

#### 何时使用 ADDED vs MODIFIED

- ADDED: 引入一个新的功能或子功能，可以作为一个独立的需求存在。当变更是正交的（例如，添加“斜杠命令配置”）而不是改变现有需求的语义时，优先使用 ADDED。
- MODIFIED: 改变现有需求的行为、范围或验收标准。始终粘贴完整、更新后的需求内容（标题 + 所有场景）。归档工具会用你提供的内容替换整个需求；部分增量会导致之前的细节丢失。
- RENAMED: 仅当名称改变时使用。如果也改变了行为，使用 RENAMED (名称) 加上 MODIFIED (内容) 并引用新名称。

常见陷阱: 使用 MODIFIED 添加新的关注点但未包含原有文本。这会在归档时导致细节丢失。如果你没有明确改变现有需求，应在 ADDED 下添加一个新需求。

正确编写 MODIFIED 需求：

1) 在 `openspec/specs/<capability>/spec.md` 中找到现有需求。
2) 复制整个需求块 (从 `### Requirement: ...` 到其所有场景)。
3) 将其粘贴到 `## MODIFIED Requirements` 下，并编辑以反映新行为。
4) 确保标题文本完全匹配 (忽略空白)，并保留至少一个 `#### Scenario:`。

RENAMED 示例:

```markdown
## RENAMED Requirements
- FROM: `### Requirement: Login`
- TO: `### Requirement: User Authentication`
```

## 故障排查

### 常见错误

**"变更必须至少有一个增量"**

- 检查 `changes/[name]/specs/` 是否存在且包含 .md 文件
- 验证文件是否包含操作前缀 (## ADDED Requirements)

**"需求必须至少有一个场景"**

- 检查场景是否使用 `#### Scenario:` 格式 (4个井号)
- 不要为场景标题使用项目符号或粗体

**场景解析静默失败**

- 需要精确的格式: `#### Scenario: 名称`
- 使用以下命令调试: `openspec show [change] --json --deltas-only`

### 验证技巧

```bash
# 始终使用严格模式进行全面检查
openspec validate [change] --strict

# 调试增量解析
openspec show [change] --json | jq '.deltas'

# 检查特定需求
openspec show [spec] --json -r 1
```

## 最佳实践脚本

```bash
# 1) 探索当前状态
openspec spec list --long
openspec list
# 可选的全文搜索:
# rg -n "Requirement:|Scenario:" openspec/specs
# rg -n "^#|Requirement:" openspec/changes

# 2) 选择变更ID并构建框架
CHANGE=add-two-factor-auth
mkdir -p openspec/changes/$CHANGE/{specs/auth}
printf "## Why\n...\n\n## What Changes\n- ...\n\n## Impact\n- ...\n" > openspec/changes/$CHANGE/proposal.md
printf "## 1. Implementation\n- [ ] 1.1 ...\n" > openspec/changes/$CHANGE/tasks.md

# 3) 添加增量 (示例)
cat > openspec/changes/$CHANGE/specs/auth/spec.md << 'EOF'
## ADDED Requirements
### Requirement: Two-Factor Authentication
Users MUST provide a second factor during login.

#### Scenario: OTP required
- **WHEN** valid credentials are provided
- **THEN** an OTP challenge is required
EOF

# 4) 验证
openspec validate $CHANGE --strict
```

## 多功能示例

```
openspec/changes/add-2fa-notify/
├── proposal.md
├── tasks.md
└── specs/
    ├── auth/
    │   └── spec.md   # ADDED: Two-Factor Authentication
    └── notifications/
        └── spec.md   # ADDED: OTP email notification
```

auth/spec.md

```markdown
## ADDED Requirements
### Requirement: Two-Factor Authentication
...
```

notifications/spec.md

```markdown
## ADDED Requirements
### Requirement: OTP Email Notification
...
```

## 最佳实践

### 简单优先

- 默认新增代码少于100行
- 优先使用单文件实现，直到证明不够用为止
- 没有明确理由时避免使用框架
- 选择枯燥但经过验证的模式

### 复杂性触发条件

仅在以下情况下增加复杂性：

- 性能数据显示当前解决方案太慢
- 具体的规模需求 (大于1000用户, 大于100MB数据)
- 多个经过验证的用例需要抽象

### 清晰的引用

- 使用 `file.ts:42` 格式表示代码位置
- 引用规范为 `specs/auth/spec.md`
- 链接相关的变更和 PR

### 功能命名

- 使用动词-名词: `user-auth`, `payment-capture`
- 每个功能只负责单一目的
- 10分钟可理解性原则
- 如果描述需要 "和" (AND)，则进行拆分

### 变更ID命名

- 使用 kebab-case，简短且具描述性: `add-two-factor-auth`
- 优先使用动词前缀: `add-`, `update-`, `remove-`, `refactor-`
- 确保唯一性；如果已被占用，则附加 `-2`, `-3`, 等。

## 工具选择指南

| 任务 | 工具 | 原因 |
|------|------|-----|
| 按模式查找文件 | Glob | 快速模式匹配 |
| 搜索代码内容 | Grep | 优化的正则表达式搜索 |
| 读取特定文件 | Read | 直接文件访问 |
| 探索未知范围 | Task | 多步调查 |

## 错误恢复

### 变更冲突

1. 运行 `openspec list` 查看活动的变更
2. 检查重叠的规范
3. 与变更负责人协调
4. 考虑合并提案

### 验证失败

1. 使用 `--strict` 标志运行
2. 检查 JSON 输出获取详情
3. 验证规范文件格式
4. 确保场景格式正确

### 缺少上下文

1. 首先阅读 project.md
2. 检查相关的规范
3. 回顾最近的归档
4. 请求澄清

## 快速参考

### 阶段指标

- `changes/` - 提议中，尚未构建
- `specs/` - 已构建并部署
- `archive/` - 已完成的变更

### 文件用途

- `proposal.md` - 为什么和做什么
- `tasks.md` - 实施步骤
- `design.md` - 技术决策
- `spec.md` - 需求和行为

### CLI 要点

```bash
openspec list              # 有哪些正在进行的工作?
openspec show [item]       # 查看详情
openspec validate --strict # 它是否正确?
openspec archive <change-id> [--yes|-y]  # 标记完成 (使用 --yes 实现自动化)
```

记住：规范是真理。变更是提案。保持它们同步。
