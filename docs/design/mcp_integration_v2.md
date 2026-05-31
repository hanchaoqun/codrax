# MCP 集成 v2 —— 最优方案设计（基于代码深度探索重写）

**状态**：设计审查 + 部分落地（取代 `mcp_integration.md` 的 §9 / §15 实现框架；威胁模型 §3、配置预算 §11 仍可复用）。2026-05-31 已落地 Phase 1 的 stdio producer / `mcp_servers` 加载 / 命名空间 / explorer-family gate，以及 Phase 2 的枚举资源读取和外部 guidance 元数据。后续仍可扩展 plan hook / log_source 特化。
**修订动机**：v1 写于 953-commit 漂移之前。本次对当前代码做了 file:line 级深度探索，发现 **MCP 的消费侧链路已经全部接通并经测试**，只是因 producer 是 stub 而休眠。最优方案因此从"为 MCP 输出新建三条路径"翻转为"**补完 producer，让已建好的 typed observation lane 自动接管**"。这同时把两条硬约束变成设计的天然结果：
- **泛化通用**：任何 MCP server 的任何工具输出，统一变成一种 typed 外部观测（`origin = MCPResource`），与 k8s / 日志 / 任何领域解耦。
- **不影响稳定性**：下游零新增代码路径；所有改动 gate 在"registry 非空"；**无 MCP 配置时行为字节等价于今天**。

---

## 0. 目录

- [1. 核心发现：消费侧已 wired，只缺 producer](#1-核心发现消费侧已-wired只缺-producer)
- [2. 稳定性不变量（新红线 L-MCP-3）](#2-稳定性不变量新红线-l-mcp-3)
- [3. 泛化原则：typed observation 是唯一通用载体](#3-泛化原则typed-observation-是唯一通用载体)
- [4. 架构总览（v2）](#4-架构总览v2)
- [5. Phase 1 — Producer + 生命周期（真正的工作量）](#5-phase-1--producer--生命周期真正的工作量)
- [6. Phase 2 — prompts + resources 通道](#6-phase-2--prompts--resources-通道)
- [7. Phase 3（可选/可延）— log_source 运行时 triage](#7-phase-3可选可延--log_source-运行时-triage)
- [8. Phase 4 — plan 导出 hook](#8-phase-4--plan-导出-hook)
- [9. 红线汇总](#9-红线汇总)
- [10. 配置 schema（指针约定）](#10-配置-schema指针约定)
- [11. 测试计划](#11-测试计划)
- [12. 开工顺序 + 风险表](#12-开工顺序--风险表)
- [13. 对 v1 的更正记录](#13-对-v1-的更正记录)

---

## 1. 核心发现：消费侧已 wired，只缺 producer

深度探索把整条 MCP 数据流逐段 ground 到 file:line。**除 producer 外全部已实现并有测试**：

| 环节 | 位置 | 状态 |
|---|---|---|
| `BaseAgent.executeTool` 返回 `(*ToolResult, *MCPResponse)` 双通道 | `internal/agent/agent.go:3193` | ✅ |
| executeTool 的 MCP 分发分支：**遍历 `MCPServers.List()` → 精确匹配 `t.Name` → `server.CallTool`** | `agent.go:3326-3338` | ✅（注意：已是"精确成员遍历"，**不是** v1 担心的 `strings.Contains(name,".")`） |
| `allMCPResponses` 累积（并行 + 串行两路）→ 写入 `StageOutput.MCPResponses` | `agent.go:1685 / 2329 / 2400 / 2515 / 1994` | ✅ |
| orchestrator 把 `output.MCPResponses` merge 进 `busCtx.MCPResponses`；并有 `MCPCallCount` 遥测 | `orchestrator.go:7768 / 2204` | ✅ |
| `BuildAgentContext` 复制到 `AgentContext.MCPResponses` + 渲染 `RelevantMCPNotes` 进 prompt | `internal/context/builder.go:176-177`；`extractMCPNotes:2382` | ✅ |
| `ObservationLedgerInput.MCPResponses` → `compileMCPResponseObservations` → `ObservationRecord{Origin: MCPResource}` | `internal/types/observation_ledger.go:290 / 334 / 1379-1413` | ✅ |
| MCP origin 的 grounding policy（principal→repairable / supporting→soft） | `internal/types/answer_claim_binding.go:135` | ✅ |
| answer 契约识别 MCP origin（principal enum 编译、pre-emit 校验） | `internal/tool/answer_document_pre_emit_check.go:3222`；`answer_document_principal_enum_compile.go:1224` | ✅ |
| **L-MCP-1**（MCP 永不进 `internal/tool/ground` citation pool）结构性测试 | `internal/types/observation_ledger_test.go:672-674` | ✅ 已强制 |
| `MCPResponse` 富字段（`PayloadRef/RowSetRef/PageRef/ResourceURI/MIMEType/JSONPointer/Selector/Row/LineStart/LineEnd`）—— 为精确 answer-binding 设计 | `internal/types/context.go:4742-4759` | ✅ 已存在 |
| **producer**：`StdioServer.ListTools()` / `CallTool()` / `Initialize()` / yaml 加载 / 启动注册 / 优雅关闭 | `internal/mcp/stdio.go:82,97`；`cmd/root.go:2906` | ❌ **全部 stub / 缺失** |

**结论**：MCP 集成 ≈ **"完成 producer + 生命周期，typed lane 自动承载其余"**。v1 设计的 §9（log_triage / blob / Summary 三路由）和 §15（`translateMCPResponse` 把 MCP 结果塞进 `ToolResult.Summary`）**是多余且有害的**——它绕过了已建好的 typed observation lane，把"非代码外部事实"降级成无 provenance 的散文摘要。v2 不走那条路。

> **v1 伪代码的硬错误**：`MCPResponse` 类型**没有 `Content` 字段**（`context.go:4742` 实证），而 v1 §9/§15 反复 `resp.Content` —— 第一步就编译不过。v2 通过填充已有的 `Summary` + `PayloadRef`（blob 引用）解决，不引入新字段（除非 Phase 3 需要）。

---

## 2. 稳定性不变量（新红线 L-MCP-3）

**L-MCP-3（本 PR 引入 + 结构性守护）：当 `mcp_servers` 未配置（registry 为空）时，read / write 两条 pipeline 的 prompt、tool schema、dispatch、ledger 全部字节等价于引入本 PR 之前。**

这条之所以"天然成立 + 容易守护"，是因为现有代码的每个 MCP 触点都已经 gate 在非空判断上：

- `buildToolSchemas`：`if b.deps.MCPServers != nil && !observationOnly... { for ... ListAllTools() }`（`agent.go:2717-2726`）。空 registry → `ListAllTools()` 返回 `nil` → 循环零次 → schema 不变。
- `executeTool`：`for _, serverName := range b.deps.MCPServers.List()`（`agent.go:3326`）。空 registry → 循环零次 → 落到原有 `unknownToolResult` 之前的本地工具路径不变。
- `builder.go:176`：`extractMCPNotes(bus.MCPResponses)`，空 slice → `RelevantMCPNotes` 为空 → section 不渲染。
- `compileMCPResponseObservations`：空 slice → 零 record。

**守护方式**：新增结构性测试 `TestMCP_DisabledIsByteIdentical` —— 用空 `mcp_servers` 跑一遍 `buildToolSchemas` / `BuildPromptContext`，断言输出与"不存在 MCP registry"分支逐字节相同。这把 L-MCP-3 变成 CI 级（实为 `go test` 级，本仓无 CI）不变量。

**推论（这是 v2 最强的稳定性论据）**：补完 producer **不新增任何下游代码路径**——它只是让早已存在、已被单测覆盖（`observation_ledger_test.go:1492-1557` 已有两个 MCP→ledger 集成测试）的休眠路径**第一次拿到真实数据**。风险面被压缩到 `internal/mcp/` 一个包 + `cmd/root.go` 的加载块 + `config/runtime.go` 的字段，几乎不触碰 agent / orchestrator / context 主干。

---

## 3. 泛化原则：typed observation 是唯一通用载体

v1 的隐性过拟合：把"客户有 k8s 可观测性工具链"写进了核心数据流（`kind: log_source` 触发 log_triage 是 §9 的"主路径，P1 核心"）。这违反 `feedback_generalization_over_project_success` + `feedback_no_overfitted_solutions`。

v2 的泛化立场：

1. **核心载体与领域无关**。无论 MCP server 是 k8s 日志、Jira 工单、Grafana 快照、内部知识库还是任意自研工具，其 `tools/call` 结果一律变成 `MCPResponse` → `ObservationRecord{Origin: MCPResource}`。下游 answer 契约已经知道怎么 bind / 校验这种 origin（§1 表末三行）。**这就是"任何 MCP server 都能用"的通用能力，不需要任何领域 hint。**
2. **大输出用已有 blob 机制**，不为日志特判。`CallTool` 内若响应 > `blob.MaxInlineBytes`（`blob.go:44`，默认 32KB），调用现成的 `StoreBlob(ctx, "mcp-<server>", payload)`（`blob.go:99`）拿到 `(summary, ref)`，把 `ref` 放进 `MCPResponse.PayloadRef`。LLM 经 `read_file` 按需翻页——与今天 grep / read_file 的大输出处理同一条路。GB 级响应天然受 `max_response_bytes` 上限保护，无需 log 专用通道。
3. **log_source → log_triage 降级为可选特化（Phase 3）**，且默认关闭。它是"结构化日志"这一垂直场景的增量价值，不是 MCP 通用能力的前提。把它从核心移到可选，既保住泛化，又把最 invasive 的改动（运行中起 LLM 子调用、quota 计数）隔离到可独立 ship / 可不 ship 的阶段。

---

## 4. 架构总览（v2）

```
codrax.yaml :: mcp_servers[]  ──(Phase1 启动加载)──►  cmd/root.go (2906 之后)
                                                          │
                              mcp.LoadServers(reg, cfgs)  │  all-or-nothing
                                                          ▼
                          StdioServer.Start() → Initialize()(握手+版本协商+声明 client caps)
                                                          │
                                  tools/list · prompts/list · resources/list（各一次，缓存）
                                                          │
                              注册进 mcpRegistry（已在 deps，agent.go:2962）
                                                          │
   ┌──────────────────────────────────────────────────────────────────────────┐
   │  每次 agent dispatch（全部既有代码，零改动）：                                │
   │   buildToolSchemas: ListAllTools() → schema（Phase1 加 <server>__<tool>）    │
   │   BuildPromptContext: + External Guidance / External Resources（Phase2 新增）│
   └───────────────────────────────┬──────────────────────────────────────────┘
                                    ▼
   LLM 调 obs__fetch_logs  → executeTool(3326) 精确解析 server/tool → CallTool
                                    ▼
   StdioServer.CallTool: JSON-RPC tools/call（reader-goroutine + id-demux）
       大输出 → StoreBlob → PayloadRef  ；小输出 → Summary
                                    ▼
   返回 *MCPResponse ──► allMCPResponses(既有) ──► StageOutput ──► BusContext(7768)
                                    ▼
   既有 typed lane：BuildAgentContext(176) → RelevantMCPNotes(prompt)
                    ObservationLedgerInput(290) → compileMCPResponseObservations(1379)
                    → ObservationRecord{Origin:MCPResource} → answer 契约 binding/校验
                                    ▼
   (写模式) plan 落盘后 → plan_output_hook（Phase4）
```

**v2 与 v1 的结构差异**：v1 图里 producer 之后有"三条消化路径（log_triage / blob / Summary）"分叉；v2 只有"填 `MCPResponse` 字段 → 既有 lane"一条主路，blob 是 `MCPResponse.PayloadRef` 的来源而非旁路，log_triage 退到 Phase 3 可选。

---

## 5. Phase 1 — Producer + 生命周期（真正的工作量）

### 5.1 JSON-RPC 框架（新文件 `internal/mcp/jsonrpc.go`）

依赖约束：**本仓仅 `gopkg.in/yaml.v3`，无第三方 JSON-RPC 库**（CLAUDE.md §Dependencies）。手写编解码，与 v1 一致：

```go
type rpcRequest struct {
    JSONRPC string          `json:"jsonrpc"` // 恒 "2.0"
    ID      int64           `json:"id"`
    Method  string          `json:"method"`
    Params  json.RawMessage `json:"params,omitempty"`
}
type rpcResponse struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      *int64          `json:"id"`      // notification 无 id → nil
    Result  json.RawMessage `json:"result,omitempty"`
    Error   *rpcError       `json:"error,omitempty"`
}
type rpcError struct{ Code int; Message string; Data json.RawMessage }
```

用 `encoding/json` 严格解码（`Decoder.DisallowUnknownFields` 仅用于我方已知子结构，对 server 的 `result` 用 `RawMessage` 宽松接，避免 server 多发字段就崩——这是 T10 的正确平衡点）。

### 5.2 并发模型：reader-goroutine + id-demux（**修正 v1 的协议正确性缺陷**）

v1 §5.4 是"持 server 级 mutex → 写一行 → scanner 读下一行 = 我的响应"。**这是错的**，三个连锁问题（与 v1 §5.4 自身的"超时不 kill"决策直接冲突）：

1. MCP 规范允许 server 主动发 `notifications/*`（progress / message / log）。"读下一行"会把 notification 当 response 解析。
2. v1 决定"超时放弃该 call 但不 kill 进程"。单行同步读下，迟到的响应留在 pipe，**下一次 call 读到的是上次的迟到响应 → 全部错位**。
3. `explorer 并行 fan-out` 已 SHIPPED（`pipeline_max_parallelism`，memory 速查）。多个并行 agent 可能并发调同一 MCP server，server 级 mutex 把它们串行化。

**正确模型（mirror 仓内既有 idiom）**：
- **进程启动**：`Setpgid` + reader goroutine，照搬 `internal/tool/exec_supervisor_unix.go:25-58`（`cmd.SysProcAttr.Setpgid=true`；`go func(){ waitErr <- cmd.Wait() }()`；ctx 取消时 `syscall.Kill(-pgid, SIGKILL)`）。
- **stdout 读取**：一个常驻 reader goroutine 用 `bufio.Scanner`（buffer 1MB，已在 `stdio.go:60`；可配 `max_response_bytes`），逐行解析。照搬 `internal/llm/openai.go:770-909` 的 SSE 单 reader + `atomic.Int64` 进度看门狗模式。
- **分发**：`pending map[int64]chan rpcResponse`（加锁）。reader 读到带 `id` 的行 → 投递到对应 channel；`id == nil`（notification）→ 投递到 notification sink（默认 DEBUG 日志，progress 可选转 emitter）。
- **CallTool**：分配递增 `id`（`atomic.Int64`），注册 pending channel，写请求行，`select { case resp := <-ch: ...; case <-ctx.Done(): delete(pending,id); return timeout }`。超时只是注销 channel——迟到响应被 reader 读到时发现无 pending → 丢弃。**这样"超时不 kill"才正确，且并发 call 天然支持（写请求那一下用细粒度 `writeMu`，读完全并发）。**

这是 Phase 1 的技术核心，也是 v1 最大的协议正确性返工点。

### 5.3 Initialize（新文件 `internal/mcp/initialize.go`）

握手必须做版本协商 + 显式声明 client capabilities（v1 漏了）：

```jsonc
// → 发
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{
  "protocolVersion":"2025-03-26",
  "capabilities":{ "roots":{"listChanged":false} },   // 显式 NOT 声明 "sampling"
  "clientInfo":{"name":"codrax","version":"<build>"}
}}
// ← 收 server capabilities + protocolVersion；做版本兼容（不一致→WARN，按交集能力工作）
// → 发 {"jsonrpc":"2.0","method":"notifications/initialized"}
```

- **显式不声明 `sampling`**：把 v1 的"非目标：MCP sampling"从被动变主动防御——合规 server 不会尝试反向 LLM 调用（堵死一类信任面）。
- `startup_timeout_ms`（默认 3s）控制整个握手；超时 → `Close()` + 启动失败 loudly（fail-loud，与仓库 fail-loud 风格一致）。

### 5.4 ListTools / CallTool（补完 `stdio.go` 桩）

- `ListTools()`：握手后发一次 `tools/list`，缓存进 `toolsCache`（`Initialize` 时填充，运行中不变——对应 v1 决策 #3"单次 Start"）。`ListTools()` 返缓存，**不再每次发 RPC**（修正现状的隐含 round-trip）。
- `CallTool(name, params)`：
  1. 发 `tools/call`，`params = {"name":name,"arguments":params}`；
  2. `select` 等响应（`timeout_ms`，默认 10s）；
  3. server 返 `isError` 或 RPC error → `MCPResponse{Success:false, Summary:<msg>}`；
  4. 提取 `result.content`（MCP 的 content 是 `[{type,text|data}]` 数组）拼成 payload；
  5. `len(payload) > blob.MaxInlineBytes` → `StoreBlob(ctx,"mcp-"+server, payload)` → `Summary=preview, PayloadRef=ref`；否则 `Summary=payload`；
  6. 填 `MCPResponse{ServerName, Method:"tools/call:"+name, Success:true, Timestamp, Summary, PayloadRef, MIMEType（若 content 带）}`。
  - **注意**：`ctx *types.BusContext` 要能传到 `CallTool` 以便 `StoreBlob` 拿 `WorkDir`。现 `Server.CallTool(name, params)` 签名无 ctx。两个选项：(a) 给 `CallTool` 加 `ctx` 参数（改接口 + `mcp.Registry.CallTool` + `executeTool` 调用点 3330，三处同步）；(b) `StdioServer` 持有 workDir，由 orchestrator 在 Run 起始 setter 注入。**选 (a)**——更显式，且 executeTool 本就持有 `ctx`，传递自然；接口改动被结构性测试覆盖。

### 5.5 namespacing 与碰撞（精确信号，合 `feedback_precise_signals_for_hard_gates`）

现状：`buildToolSchemas` 用扁平 `ts.Name`（`agent.go:2722`）；dispatch 按 `t.Name == tc.Name` 跨 server 遍历首个命中（`agent.go:3326-3334`）；本地工具先于 MCP 查（`Tools.Get` 在 3272，MCP 在 3325）。风险：两个 server 同名工具、或 MCP 工具与 builtin 同名（builtin 会 shadow）。

因为此路径目前休眠（无 server 注册 → 无流量），**可以安全地一次性引入 namespacing，无既有行为要保**：

- **schema 端**：`buildToolSchemas` 把 MCP 工具命名为 `<server>__<tool>`（双下划线）。**为何不用 `.` / `/`**：部分 LLM provider 限制工具名 `^[a-zA-Z0-9_-]+$`，`.`/`/` 会被拒（泛化风险）；单 `_` 与 builtin 的 `emit_log_triage` 等歧义；`__` 在所有 provider 合法且无 builtin 使用（探索确认无 builtin 含 `.`/`/`，`_` 为普通分隔）。
- **dispatch 端**：新增 `mcpRegistry.ResolveNamespaced(name) (server, tool string, ok bool)` —— **精确成员校验**：对每个注册 server 检查 `name == server+"__"+t` 且该 server 的 `ListTools()` 含 `t`。executeTool 在本地 `Tools.Get` 失败后调它（替换现 3326 的裸遍历）。这是精确信号做硬路由，不靠 `strings.Contains`。
- **加载端碰撞守护**：`LoadServers` 拒绝重名 server（现 `Registry.Register` 是 map 覆盖，无声丢失——加显式 dup-name error）。

`MCPResponse.Method` 设为 `"tools/call:"+rawTool`，`extractMCPNotes`（`builder.go:2382` 渲染 `[OK] server.Method: Summary`）与 ledger 的 `Producer`/`ClaimKey` 都拿到稳定 provenance。

### 5.6 yaml 加载 + 启动注册 + 生命周期

- **新文件 `internal/mcp/loader.go`**：`LoadServers(reg *Registry, cfgs []MCPServerConfig, maxServers int) error`，for 循环 `NewStdioServerFromConfig` → `Start` → `Initialize` → `Register`；**all-or-nothing**：任一失败 → `Close` 已注册的 + 返 error（启动 loudly 失败，对齐 v1 §6.3）。
- **启动点**：`cmd/root.go:2906`（`mcpRegistry := mcp.NewRegistry()` 现位置）之后插入加载块。`rs.MCPServers` 经标准 `yaml.Unmarshal`（`config/runtime.go:1342`）自动解析，新字段零额外解析代码。
- **优雅关闭（修正 v1 的 defer 落点 bug）**：探索发现 `mcpRegistry` 在 `initApp()`（`PersistentPreRunE`）创建，**生命周期横跨整个进程**（单发 CLI 一次 Run；REPL 多次 Run；`app.orch` 持有它，`cmd/root.go:2962/3197`）。因此 **v1 的 `defer mcpRegistry.Close()` 放在 rootRun/runREPL 都错**（REPL 会在首次 Run 后过早关闭，或单发漏 signal 路径）。**正确落点**：mirror 唯一的顶层信号处理 `worktree.InstallSignalHandler()`（`cmd/root.go:1883`）——新增 `mcp.InstallShutdown(mcpRegistry)`，在 SIGINT/SIGTERM 与正常退出两路都 `Close()`。这覆盖单发、REPL、信号三种退出。
- **进程崩溃韧性（轻量补 v1 决策 #3 的体验缺陷）**：v1"崩了不重启"对长 REPL 是整 session 失能。改为 **lazy-restart-on-next-call**：`CallTool` 前若 `healthy==false`（reader goroutine 检测到 EOF/管道断 → 置 false），尝试一次 `Start()+Initialize()` 重连；连续失败则该 server 本 session 标记 dead 返 `Success:false`。成本低、不引入看门狗 goroutine、体验显著改善。

### 5.7 env 最小化（补 v1 决策 #4 的另一维度）

v1 决策 #4 只管"启动日志不打 env"。但"把 codrax 全部 `os.Environ`（含 provider API key、`CODRAX_SETTINGS`）传给第三方 MCP 子进程"是 secret 泄漏面。v2：**默认最小环境**（`PATH` + yaml 显式 `env`），`inherit_env: true`（默认 false）才全继承。这是泛化的安全默认，不替操作员兜底但也不默认泄漏。

---

## 6. Phase 2 — prompts + resources 通道

独立 commit，gate 在 registry 非空，不影响 Phase 1 与无 MCP 路径。

**2026-05-31 落地说明**：当前已实现 `resources/list` / `resources/read` / `prompts/list`。`prompts/list` 只作为 capped 外部 guidance 元数据暴露；`prompts/get` 暂未执行，避免在没有明确 server 场景前引入额外 prompt-injection 面。

**Typed line support**：`tools/call` 与 `resources/read` 结果若包含显式 `codrax.mcp.observation.v1` envelope 或 `application/vnd.codrax.observation+json` MIME，producer 会解析 `line_start` / `line_end` 等 typed 坐标并放入 `MCPResponse.Observations`。普通文本/普通 JSON 不解析坐标；所有 typed 坐标仍走 `mcp_resource` 外部观测 lane，不进入当前源码 citation。

### 6.1 prompts → External Guidance section

- **接口扩展**（`internal/mcp/mcp.go` 的 `Server`）：`ListPrompts() []PromptSchema` / `GetPrompt(name, args) (PromptResult, error)`（类型同 v1 §7.3）。
- **注入点**（探索给出安全位）：`BuildPromptContext` 在 `SectionProhibitions` 之后追加新 system section；标题用 **SST 常量**（`internal/context/section_titles.go` 加 `SectionExternalGuidance = "External Guidance (MCP)"` 并入 `canonicalSystemSectionOrder`，否则违反 SST 红线 `feedback_prompt_redline_checklist`）。
- **三道安全（合红线）**：
  1. **stage 门**：仅 read 非 finalize 注入（`!ac.Stage.IsWrite()` 已在 `builder.go:132`；额外 `ac.Stage != StageFinalize`）。finalize 是答案塑形阶段，外部 prompt 不得污染（对齐 `feedback_no_internal_info_in_llm_prompts` 精神 + 写模式硬契约）。
  2. **TypedDenials 消毒**：注入前过 `sanitiseSectionForLLM`（`builder.go:2919`）——外部 prompt 文本若含被否定 token 一并消毒（T3 prompt-injection 缓解）。
  3. **外套 + 字节 cap**：每条以 `> [mcp: <server>.<name>]` blockquote 包裹（标记"外部建议，非 codrax 指令"）；合计 > `mcp_max_prompt_bytes_per_stage`（默认 8192）按 server 声明序截断 + WARN 列出 dropped。
- **选择策略**：沿用 v1 决策 #1（always-inject + LLM 自筛 by name/description），但**注入范围收窄到 analyze+explore**（非每个 dispatch），省 token 且不碰 finalize。

### 6.2 resources → enum 校验的伪工具（**修正 v1 的 L-MCP-2 红线手段**）

v1 §8.3 + L-MCP-2 用 `resource://` **前缀白名单**硬拒非该前缀 URI。**这是 noisy signal 做硬门**（违 `feedback_precise_signals_for_hard_gates`）：MCP 规范的 resource URI 是 server 自定义 scheme，合法 server 常用 `file://`(其沙箱内)、`https://`、自研 scheme——前缀白名单会**误杀合法 server**。

**v2 精确手段**：`mcp_read_resource` 伪工具只接受 server 在 `resources/list` **回声过的 URI 集合**（精确 enum membership）。LLM 只能从该 enum 选；codrax **永不主动构造 / resolve URI**（L-MCP-2 的真意图）。这既满足"不读本机敏感路径"，又不误杀，且是精确信号做硬门。

- **伪工具实现**：新建 `internal/mcp/tools_pseudo.go`，实现 `tool.Tool` 接口（`Name/Description/Parameters/Execute/IsWrite=false/Confidence=0`），`Execute` 校验 `uri ∈ registry.ListAllResources()` 后路由 `server.ReadResource(uri)`，大输出走 `StoreBlob`。
- **注册 + 可见性**：仅当 `registry.ListAllResources() != nil` 时 `Register` 进 `tool.Registry` 并加入相关 skill 的 `ToolSuggestions`（探索确认这是 LLM 看到工具的标准路径，`agent.go:2676`）。无 resources 的部署完全看不到此伪工具（L-MCP-3 一致）。
- **section**：`SectionExternalResources` 常量 + 表格渲染（URI / description / mime），入 `canonicalUserSectionOrder`（探索建议位：`SectionRawToolOutputs` 与 `SectionKnownFacts` 之间）。

---

## 7. Phase 3（可选/可延）— log_source 运行时 triage

**定位变更**：v1 把它当 "P1 核心主路径"；v2 降为**可选垂直增量**，默认关闭，可独立 ship 或暂不 ship。理由：泛化（§3）+ 这是最 invasive 的部分（运行中起 LLM 子调用 + quota 计数 + 触碰 log_triager 内部）。**默认路径已由 Phase 1 的 blob offload（`PayloadRef`）覆盖**——大日志不会拉爆 context（LLM 看 preview + 按需 read_file），只是不做"结构化 LogBundle"那一步增值。

若做，feasibility 已探明（`TriageInline` 可行）：

- `log_triager` 现 `Execute` 依赖 `ctx.AttachedLog` / `ctx.Mutable` / skill registry / LLM（`log_triager.go:295-339`）。抽出 `TriageInline(rawText, sourcePrefix, repoRoot, deps, settings) (*LogBundle, error)`：构造最小 `AgentContext{AttachedLog:rawText, Mutable:临时}` → 跑既有 agent → 读回 `mutable.LogTriage()`。两步 fallback（`TwoStepBytes=32KB`，`log_triager.go:309`）与 `MergeBundles`（`validate.go:217`，nil 安全）免费复用。
- 触发：`tool_metadata.<tool>.kind: log_source`。executeTool 收到该 kind 的 MCP 响应后调 `TriageInline`，结果 merge 进 `Mutable.LogTriage()`（`SetLogTriage` 存在，`context.go:1334`），与 `--log` 传入的 bundle 自动合并。
- 资源边界沿用 v1 §9.4（`mcp_log_source_max_bytes` 64MB / `..._total_bytes_per_run` 256MB / quota 耗尽降级 blob）。
- 降级：triager 失败 → 退回 Phase 1 的 blob 路径（即"什么都不做特殊处理"），**保证 Phase 3 失败不影响 Phase 1 基线**。

**建议**：Phase 1+2+4 先 ship 拿到通用 MCP 能力；Phase 3 作为后续 PR，避免运行时 LLM 子调用的复杂度拖累主线稳定性。

---

## 8. Phase 4 — plan 导出 hook

独立、低风险，与 MCP 解耦。沿用 v1 §10 设计，更正落点 file:line：

- 新文件 `internal/tool/plan_hook.go`：`RunPlanOutputHook(cfg *PlanOutputHookConfig, planJSON []byte, statusTrigger string) error`，exec 子进程、stdin 喂 plan.json、`SupervisedRun`（`exec_supervisor.go`，复用 SIGTERM+grace+SIGKILL，不重造）+ `timeout_ms`（默认 30s）。
- 触发点（探索给出当前 file:line）：
  - `pending_approval`：`cmd/root.go::writePlanFile`（`1124-1149`，`os.WriteFile` 在 `1145`）成功后。
  - `applied` / `verify_failed`：`orchestrator::persistPlanStatus`（`orchestrator.go:3038-3070`）内，对应 `write_scheduler.go:143/277/279` 的状态转换。
- `on_failure: warn`（默认，不阻塞主流程）/ `fail`（Run 返 error，影响 CI/脚本退出码）。stdout 吞掉防干扰 REPL，stderr 记 WARN。

---

## 9. 红线汇总

| 红线 | 内容 | 守护 |
|---|---|---|
| **L-MCP-1**（既有，已强制） | MCP 输出永不进 `internal/tool/ground` citation pool；只走 ObservationLedger（origin=MCPResource） | `observation_ledger_test.go:672-674` go/ast 扫描 |
| **L-MCP-2**（v2 更正手段） | codrax 永不构造/resolve resource URI；`mcp_read_resource` 只接受 `resources/list` 回声的**精确 enum** —— 非前缀白名单 | `TestMCPResource_OnlyEchoedURIsAccepted` |
| **L-MCP-3**（v2 新增） | 无 `mcp_servers` 配置时，schema/prompt/dispatch/ledger 字节等价于本 PR 前 | `TestMCP_DisabledIsByteIdentical` |
| **L-MCP-4**（v2 新增） | initialize 显式不声明 `sampling` capability；无反向 LLM 调用面 | `TestMCPInitialize_NoSamplingCapability` |
| **精确信号红线**（既有架构红线） | dispatch 路由 = registry 精确成员（`ResolveNamespaced`）；resource gate = enum membership —— 均非字符串 contains/前缀 | 单测 |
| **SST 红线**（既有） | External Guidance/Resources 标题用 section_titles 常量并入 canonical order | 既有 prompt snapshot 测试 |
| **泛化红线**（既有） | 核心载体领域无关；log_source 为可选特化（Phase 3 默认关） | 设计审查 + Phase 划分 |

---

## 10. 配置 schema（指针约定）

探索确认：`RuntimeSettings` 全字段 `*T`（`config/runtime.go:34+`），deref 用 inline nil-check（无集中 helper，`cmd/root.go:1429-1437` 模式）。新字段照此：

```go
// internal/config/runtime.go（追加，全部指针）
MCPServers                []MCPServerConfig     `yaml:"mcp_servers"`
MCPMaxServers             *int                  `yaml:"mcp_max_servers"`                  // 默认 8
MCPMaxPromptBytesPerStage *int                  `yaml:"mcp_max_prompt_bytes_per_stage"`   // 默认 8192
// Phase 3（可选）：
MCPLogSourceMaxBytes      *int                  `yaml:"mcp_log_source_max_bytes"`         // 默认 64MB
MCPLogSourceTotalBytesRun *int                  `yaml:"mcp_log_source_total_bytes_per_run"`// 默认 256MB
// Phase 4：
PlanOutputHook            *PlanOutputHookConfig `yaml:"plan_output_hook"`

type MCPServerConfig struct {
    Name             string                  `yaml:"name"`
    Enabled          *bool                   `yaml:"enabled"`            // 默认 true
    Transport        string                  `yaml:"transport"`          // 仅 "stdio"
    Command          string                  `yaml:"command"`
    Args             []string                `yaml:"args"`
    Env              map[string]string       `yaml:"env"`
    InheritEnv       *bool                   `yaml:"inherit_env"`        // 默认 false（§5.7）
    TimeoutMs        *int                    `yaml:"timeout_ms"`         // 默认 10000
    StartupTimeoutMs *int                    `yaml:"startup_timeout_ms"` // 默认 3000
    MaxResponseBytes *int                    `yaml:"max_response_bytes"` // 默认 4MB
    ToolMetadata     map[string]ToolMetaYAML `yaml:"tool_metadata"`      // Phase 3 才读 kind
}
```

> 注：v1 的 `mcp_write_enabled` + `write_capable` 暂不纳入（无 stdio MCP 写工具真实场景，且写模式 prompt 是硬契约）。若未来需要，再按 6-spot typed-signal 同步红线（`feedback_typed_signal_six_spot_sync`）单独加。这避免引入未被任何场景驱动的 gate（YAGNI + 泛化）。

`TransportType` 枚举已含 `stdio/sse/http`（`enums.go:175-181`），HTTP/SSE 留待以后，协议层抽象已就位。

---

## 11. 测试计划

**单元 / 结构性**（`internal/mcp/`）：
- `TestJSONRPC_RoundTrip`：fake echo server，`tools/list` → 解析。
- `TestStdioServer_ReaderDemux_OutOfOrderNotification`：server 在 response 前插 notification，断言 response 仍正确路由（§5.2 核心）。
- `TestStdioServer_TimeoutReturnsFailure_NoDesync`：慢 server，call A 超时后 call B 不读到 A 的迟到响应（§5.2 错位修复）。
- `TestStdioServer_ConcurrentCalls`：并发 N call 经 id-demux 各得其所。
- `TestInitialize_VersionNegotiation` + `TestMCPInitialize_NoSamplingCapability`（L-MCP-4）。
- `TestStdioServer_LargeResponseToBlob`：> MaxInlineBytes → `PayloadRef` 非空。
- `TestLoadServers_DupNameRejected` / `_AllOrNothing` / `_LazyRestartOnNextCall`。
- `TestResolveNamespaced_PreciseMembership`（精确路由）。
- `TestMCPResource_OnlyEchoedURIsAccepted`（L-MCP-2）。
- **`TestMCP_DisabledIsByteIdentical`（L-MCP-3，最重要的稳定性守护）**。
- `TestMCP_NoGroundingImport`（L-MCP-1，go/ast 扫 `internal/mcp/` 不 import `internal/tool/ground`）。

**集成 / e2e smoke**（`eval/`）：
- `eval/fixtures/fake_mcp_server/`（Python，暴露 `echo_tool` + 一个含 Java OOM 栈的 `fetch_logs`）。
- Case：问"调 obs 工具看看" → 断言 `MCPResponse` 进 ledger（`ObservationRecord{Origin:MCPResource}` 出现）→ answer 引用之。**验证既有 lane 真的接通**。
- Phase 4：`eval/cases/plan_hook.case` —— apply 后 hook 把 plan.json 写临时文件。

**回归基线**：跑全量 eval（memory 速查：`make && PARALLEL=2 TIMEOUT=1200 bash eval/parallel_all.sh`）确认无 MCP 配置时 0 回归（L-MCP-3 的端到端佐证）。

---

## 12. 开工顺序 + 风险表

| 阶段 | 内容 | 风险 | 可独立 ship |
|---|---|---|---|
| **1a** | `jsonrpc.go` + reader-goroutine/id-demux + `Initialize`（版本协商 + 无 sampling） | 中（协议正确性，靠 §11 三个 demux/timeout 测试守） | 否（1 的前置） |
| **1b** | `ListTools`/`CallTool` 补完 + blob offload + `CallTool` 加 ctx 参数（三处同步） | 低 | 否 |
| **1c** | `config.go`/`loader.go` + `cmd/root.go` 加载块 + `mcp.InstallShutdown` + lazy-restart + namespacing（schema+dispatch+dup 守护） | 低（全 gate 在非空；L-MCP-3 守护） | **是** ← 此点已有"通用 MCP 工具"能力 |
| **2** | prompts(External Guidance, SST+消毒+stage 门) + resources(enum 伪工具) | 低 | 是 |
| **3**（可选） | log_source `TriageInline` + quota | 中（运行时 LLM 子调用） | 是（默认关，可不 ship） |
| **4** | plan_output_hook（复用 SupervisedRun） | 低 | 是 |
| **F** | 文档（`docs/architecture.md` §3.4/§3.5）+ `codrax.yaml.example` + eval fixtures | 低 | 是 |

每阶段独立 commit + `go test ./...` 绿 + 关键阶段跑 eval 全量验 0 回归才进下一步（`feedback_eval_pass_is_not_green` / `feedback_no_eval_bar_relaxation`）。**核心工作量集中在 1a（并发模型），其余多为接线 + gate。**

---

## 13. 对 v1 的更正记录

| v1 处 | v1 说法 | v2 更正 | 依据 |
|---|---|---|---|
| §2.2 / §9 / §15 | MCP 输出走 ToolResult.Summary / blob / log_triage **三条新路径** | 消费侧已 wired（ledger lane）；只填 `MCPResponse` 字段，无新下游 | `agent.go:1685/2515/3326`、`observation_ledger.go:334`、`builder.go:176` |
| §9.2 / §15.2 伪代码 | 用 `resp.Content` | `MCPResponse` 无 `Content` 字段；用 `Summary`+`PayloadRef` | `context.go:4742-4759` |
| §5.4 | server 级 mutex + 单行同步读 | reader-goroutine + id-demux（否则 notification/超时/并发三处错） | `exec_supervisor_unix.go:25`、`openai.go:770` |
| §5.2 | 发 initialize（未提版本/caps） | 加 protocolVersion 协商 + 显式不声明 sampling（L-MCP-4） | MCP 规范 |
| §8.3 / L-MCP-2 | `resource://` 前缀白名单硬拒 | 改 `resources/list` 回声 enum 精确 membership（前缀会误杀合法 server） | `feedback_precise_signals_for_hard_gates` |
| §6.2 | `defer mcpRegistry.Close()` | registry 横跨进程；defer 在 rootRun/runREPL 都错；用信号处理 + 正常退出双路 | `cmd/root.go:1883/2906/2962` |
| §9 定位 | log_source = "P1 核心主路径" | 降为 Phase 3 可选特化（泛化 + 隔离 invasive 改动） | `feedback_generalization_over_project_success` |
| §6.1 | env 基础 = 全 `os.Environ` | 默认最小环境，`inherit_env` opt-in（secret 泄漏面） | 安全默认 |
| §6.1 决策 #5 | `mcp_write_enabled` + `write_capable` | 暂不纳入（无场景驱动 + 写模式硬契约）；未来按 6-spot 同步红线再加 | YAGNI + `feedback_typed_signal_six_spot_sync` |
| §2/§15 行号 | cmd/root.go:1345 / agent.go:1395 等 | 全 stale；本文已重 ground（2906 / 2667 / 3326 …） | 探索 |

---

**Ready for review。** 核心立场：MCP 集成的最优解是"**补完 producer，复用已 wired 的 typed observation lane**"——这让泛化通用与零稳定性影响成为设计的自然结果，而非额外约束。下一 session 按 §12 阶段 1a 开工。
