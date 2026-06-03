# 通用命令行操作执行器设计

**状态**: 方案落盘, 分批实现。  
**日期**: 2026-06-03  
**目标**: 在独立 operation 路由下, 支持"自然语言需求 -> 命令计划 -> 风险策略 -> 澄清/审批 -> 执行/验证"的通用命令行操作能力。默认人工批准, 可配置低风险自动批准, 未知命令允许进入人工批准队列。不得影响现有源码分析、log/trace 分析、写代码、MCP 外部观测和 `!cmd` 显式命令通道。

---

## 1. 当前代码结论

### 1.1 已有 operation 路由

REPL 已有 typed `TurnPolicy` operation route, 位于 `internal/repl/turn_policy.go`:

- `route=operation` 独立于 `repo/hybrid/local/clarify`;
- `operation_kind/risk_level/side_effects/target_surface/requires_confirmation` 已经是结构化字段;
- `operation_route_enabled` 控制是否启用 operation 计划路径;
- `internal/operation/plan.go` 当前只做 side-effect-free 计划展示, 不执行命令;
- `internal/repl/repl.go` 的 `operationDispatch` 不进入源码分析 pipeline, 也不自动执行任何工具。

这说明第一版命令执行器应沿用 operation route, 不改 read/write pipeline 拓扑。

### 1.2 已有命令执行能力

当前有两条命令执行通道:

1. `!cmd` REPL 显式命令: `internal/repl/repl.go::handleShellBangCmd`
   - 用户直接输入命令, 无计划/审批;
   - stdout/stderr 真实流向 REPL;
   - 捕获前 32 KiB 进入 memory;
   - 适合"用户自己知道要跑什么"。

2. `exec_command` 工具: `internal/tool/builtin.go`
   - read-mode 工具, 带只读约束和证据/日志提示;
   - 主要给模型做代码/日志/trace 辅助调查;
   - 不适合作为通用 side-effecting operation 执行器。

因此新能力应复用底层 shell supervisor/输出截断思想, 但必须新增 operation 专用 plan/policy/approval/result 数据模型, 不能直接把 `exec_command` 放宽。

### 1.3 已有审批命令

REPL 已有 `/approve`、`/reject`、`/cancel`:

- `/approve` 当前用于 write-mode ChangePlan;
- `/reject` 当前用于拒绝 pending ChangePlan;
- `/cancel` 当前用于取消正在运行的 pipeline;
- `/operation` 暂无命令。

新能力必须让日常 UX 简单:

- pending command operation 存在时, `/approve` 审批该 operation;
- 没有 pending operation 时, `/approve` 继续走现有 write-plan 逻辑;
- `/reject` 同理优先拒绝 pending operation, 否则保留现有语义;
- `/cancel` 取消 pending/in-flight operation, 否则保留现有 pipeline cancel;
- `/operation` 仅作为高级查看入口, 不是日常必经命令。

### 1.4 已有澄清通道

`route=clarify` 已经能让系统在信息不足时向用户提问, 不进入 pipeline。命令执行器还需要一个 operation 内部状态 `needs_clarification`, 用于"已识别是 operation, 但缺目标路径/范围/确认策略/环境选择"的情况。

---

## 2. 设计目标

1. **通用**: 不靠固定白名单覆盖所有命令; 未知命令可以生成计划, 但默认需要人工批准。
2. **可控**: 自动批准默认关闭; 低风险自动批准仅覆盖确定只读查询和无覆盖目录创建。
3. **安全**: 高风险命令进入 hard deny 或强人工审批; destructive/network submit/external write 不自动执行。
4. **批量**: 支持多步命令计划, 尽量批量审批和批量执行, 每步有可见状态和输出摘要。
5. **可澄清**: 信息不足时不猜, 返回问题和建议选项, 状态为 `needs_clarification`。
6. **不扰动**: 不改源码分析、log/trace、写代码、MCP 外部观测的 hard gate 和证据合同。
7. **JSON 兼容**: 新增暴露给模型的字段必须进入现有 JSON 兜底修复链路, 支持字符串 bool/number、数组逗号串、尾逗号等常见小模型错误。

---

## 3. 非目标

1. 不把 read-mode `exec_command` 放宽成 side-effecting 执行器。
2. 不让 operation 结果进入 current-source citation/evidence gate。
3. 不用 Go 关键字匹配用户意图。
4. 不默认安装/卸载软件包、删除文件、覆盖文件、提交网络表单。
5. 不在第一版实现桌面点击、浏览器操作、PPT/文档/表格 executor; 这些后续挂到同一 OperationExecutor 接口。

---

## 4. 状态机

第一版操作计划统一使用以下状态:

```text
needs_clarification  缺关键信息, 需向用户提问
ready                计划完整, 等待审批或自动批准
blocked              策略拒绝、能力缺失或环境不满足
executed             已执行并完成验证/输出汇总
rejected             用户拒绝
cancelled            用户取消
failed               执行失败
```

状态转换:

```text
operation request
  -> needs_clarification | blocked | ready
ready
  -> auto-approved -> executed/failed
  -> /approve      -> executed/failed
  -> /reject       -> rejected
  -> /cancel       -> cancelled
```

---

## 5. 数据模型

### 5.1 CommandOperationPlan

```go
type CommandOperationPlan struct {
    ID              string
    RequestText     string
    Status          OperationStatus
    RiskLevel       string
    ApprovalMode    string // manual | auto_low_risk | denied
    WorkDir         string
    Steps           []CommandStep
    ClarifyingQs    []ClarifyingQuestion
    BlockReason     string
    CreatedAt       time.Time
}
```

### 5.2 CommandStep

```go
type CommandStep struct {
    ID            string
    Title         string
    Program       string
    Args          []string
    Shell         string // optional; shell form is manual-only by default
    WorkDir       string
    Env           []string
    TimeoutMS     int
    RiskLevel     string
    SideEffects   []string
    AutoApproval  string // eligible | manual | denied
    Reason        string
    VerifyHint    string
}
```

### 5.3 Result

```go
type CommandOperationResult struct {
    PlanID        string
    Status        OperationStatus
    StepResults   []CommandStepResult
    OutputPreview string
    PayloadRef    string
}
```

大输出不直接塞 REPL/memory/prompt, 只显示 preview 和 blob/PayloadRef。

---

## 6. 风险与审批策略

### 6.1 默认策略

```yaml
operation_command_enabled: false
operation_command_approval: manual
operation_command_unknown_program: manual
operation_command_auto_low_risk: false
operation_command_timeout_ms: 120000
operation_command_output_preview_bytes: 32768
```

含义:

- 第一版命令执行器独立 feature gate, 默认关闭或只计划不执行; 打开后默认仍人工审批。
- 未知命令不 hard deny, 但必须人工批准。
- 自动批准默认关闭。

### 6.2 低风险自动批准

仅当配置启用 `operation_command_auto_low_risk: true` 且所有 step 精确满足以下条件才自动执行:

- 只读查询: `pwd`, `ls`, `find` 非删除形态, `stat`, `du`, `df`, `which`, `--version`, `git status/log/show/diff`, `cat/head/tail/sed -n/grep/rg`;
- 无覆盖目录创建: `mkdir -p <new-or-existing-dir>`;
- 不含 shell 重定向、管道写文件、`&&` 链中写操作、网络提交、包安装、删除、移动/覆盖。

这些判断必须基于命令 AST/step 字段和策略枚举, 不是用户散文。

### 6.3 人工批准

以下进入人工批准:

- 未知 program;
- shell 形式命令;
- 文件写入、移动、复制、安装、卸载、下载、网络读取;
- 命令链中任何 step 非 low-risk auto eligible。

### 6.4 hard deny

以下默认 hard deny, 除非后续新增更强的二次确认模式:

- `rm -rf /`, 磁盘格式化、权限破坏、系统关键目录递归删除;
- 提交凭据/发送邮件/生产系统写入;
- 逃逸当前工作目录且带破坏性副作用;
- 明确绕过安全策略的命令。

---

## 7. REPL UX

### 7.1 日常命令

只要求用户记三个:

```text
/approve
/reject [reason]
/cancel
```

高级查看:

```text
/operation
/operation show
/operation history
/operation auto
```

### 7.2 计划展示

中文示例:

```text
• 操作计划 op-20260603-001 等待批准
  风险：medium · 审批：manual · 工作目录：/Users/han/opt/codrax
  1. 查询 Node 版本
     $ node --version
     只读，低风险
  2. 安装依赖
     $ npm install
     会写入本地依赖目录，需要批准

运行 `/approve` 执行，或 `/reject <原因>` 拒绝。
```

### 7.3 澄清展示

```text
• 需要补充信息后才能生成命令计划
  1. 要移动哪个文件或目录？
  2. 目标目录是已有目录还是需要创建？

建议：给出源路径和目标路径，例如 `把 a.log 移到 logs/`。
```

---

## 8. 与现有场景隔离

| 场景 | 影响 |
| --- | --- |
| 源码分析 | 不变, 仍走 repo/hybrid pipeline |
| log/trace 分析 | 不变, trace_query/grep/read_file 仍在 runtime artifact lane |
| 写代码模式 | 不变, `/approve` 在无 pending operation 时仍审批 ChangePlan |
| MCP 外部观测 | 不变, observation MCP 不自动变成 operation executor |
| `!cmd` | 不变, 用户显式命令仍直接执行 |

---

## 9. 分批任务列表

### Batch 0: 方案与任务落盘

- [x] 深度探索现有 operation route、`!cmd`、`/approve`、配置链路。
- [x] 落本文档。
- [ ] 提交并推送设计文档。

### Batch 1: 数据模型、策略与计划状态

- [ ] 扩展 `internal/operation`:
  - [ ] `OperationStatus`
  - [ ] `CommandOperationPlan`
  - [ ] `CommandStep`
  - [ ] `ClarifyingQuestion`
  - [ ] `CommandPolicy`
- [ ] 增加策略判定:
  - [ ] low-risk readonly
  - [ ] manual unknown
  - [ ] hard deny
  - [ ] shell manual-only
- [ ] 更新本地化文案和 plan renderer。
- [ ] 单元测试覆盖 ready / needs_clarification / blocked。

### Batch 2: REPL pending operation store 与审批命令

- [ ] REPL 增加 `pendingOperation`。
- [ ] `operationDispatch` 生成 pending plan。
- [ ] `/approve` 优先消费 pending operation, 无 pending operation 时保留 write-plan 行为。
- [ ] `/reject` 优先拒绝 pending operation, 无 pending operation 时保留现有行为。
- [ ] `/cancel` 支持取消 pending/in-flight operation, 无 operation 时保留现有行为。
- [ ] `/operation show/history/auto` 最小实现。
- [ ] 测试看护 write-plan `/approve` 不回归。

### Batch 3: 命令执行器

- [ ] 新增 `internal/operation/command_executor.go`。
- [ ] 支持 Program+Args 执行。
- [ ] 支持 shell form, 但 manual-only。
- [ ] 工作目录锚定 REPL repo root, 可配置。
- [ ] timeout/cancel。
- [ ] stdout/stderr bounded preview + full blob。
- [ ] 多 step 批量执行, 任一步失败停止。
- [ ] 执行后记录 memory turn。

### Batch 4: 配置、教学与 JSON 兼容

- [ ] `codrax.yaml` 增加 operation command 配置项。
- [ ] `codrax.yaml.example`、用户手册 md/html 更新。
- [ ] 若新增模型可见 command-plan JSON 字段, 接入统一 JSON 修复。
- [ ] prompt/skill/hint 教学:
  - [ ] 信息不足时产出 clarification, 不猜;
  - [ ] 命令尽量用 Program+Args, shell 仅复杂场景;
  - [ ] 批量计划优先, 不逐条打断用户。

### Batch 5: 集成测试与评估

- [ ] 只读查询计划 + 手动批准执行。
- [ ] 低风险自动批准关闭时仍等待 `/approve`。
- [ ] 启用低风险自动批准后 `pwd/ls/which --version` 可自动执行。
- [ ] `mkdir -p` 无覆盖自动批准。
- [ ] `rm -rf` hard deny。
- [ ] 未知 program manual。
- [ ] 缺路径 needs_clarification。
- [ ] `!cmd` 行为不变。
- [ ] 写模式 `/approve` 行为不变。
- [ ] 源码/log/trace/MCP 分析不进入 operation executor。

---

## 10. 商用交付准则

1. 每批提交前跑 focused tests; 最后一批跑相关 package tests。
2. 所有副作用命令必须可取消、可见、可审计。
3. 所有大输出必须截断展示并保留 blob/PayloadRef。
4. 默认配置不能让模型自动执行未知命令。
5. 不因 operation 失败影响 read/write pipeline 的稳定性。
