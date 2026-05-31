# MCP 集成 + Plan 导出 —— P1+P3 设计文档

> ⚠ **本文档的实现框架（§9 / §15）已被 [`mcp_integration_v2.md`](mcp_integration_v2.md) 取代。** v2 基于对当前代码的 file:line 级深度探索：发现 MCP 消费侧链路（executeTool→ObservationLedger→answer 契约）已全部 wired，仅 producer 是 stub，最优方案因此从"为 MCP 输出新建三路径"翻转为"补完 producer，复用已 wired 的 typed observation lane"。本文档的威胁模型 §3、配置预算 §11、决策记录 §14 仍可参考；§2/§15 的 file:line 已 stale。

**状态**：设计审查。**未开始实现。**
**目标 PR 范围**：`P1 = MCP 协议完整化（tools + prompts + resources + yaml 加载 + 启动注册 + 资源限制）` + `P3 = plan_output_hook`
**场景**：客户有内部可观测性工具链（k8s / monitoring / audit ledger），希望 codrax 能**自主**调用它们抓运行时证据参与根因分析，最后把 ChangePlan 导出到 PR 机器人 / 工单 / 审批系统

---

## 0. 目录

- [1. 目标 / 非目标](#1-目标--非目标)
- [2. 现状审计](#2-现状审计)
- [3. 威胁模型](#3-威胁模型)
- [4. 架构总览](#4-架构总览)
- [5. P1.a — MCP stdio 协议完整化](#5-p1a--mcp-stdio-协议完整化)
- [6. P1.b — yaml 加载 + 启动注册 + 生命周期](#6-p1b--yaml-加载--启动注册--生命周期)
- [7. P1.c — prompts 通道](#7-p1c--prompts-通道)
- [8. P1.d — resources 通道](#8-p1d--resources-通道)
- [9. P1.e — 大日志与大输出的消化路径](#9-p1e--大日志与大输出的消化路径)
- [10. P3 — plan 导出 hook](#10-p3--plan-导出-hook)
- [11. 资源预算与失败隔离](#11-资源预算与失败隔离)
- [12. 测试计划](#12-测试计划)
- [13. 开工顺序](#13-开工顺序)
- [14. 决策记录 / 未决项](#14-决策记录--未决项)

---

## 1. 目标 / 非目标

### 目标

1. **MCP 协议三个标准通道全部实现**：`tools` / `prompts` / `resources`（JSON-RPC over stdio）
2. **yaml 声明 MCP server**，启动时拉起，`Run()` 结束优雅关闭
3. **内置 agent 自动消费 MCP 资源**：
   - tools 加入工具 schema（现有机制 ✓，补协议即可）
   - prompts 以 "External Guidance" 段注入对应 stage 的 system prompt
   - resources 以 `read_file` 风格的按需读取工具（名字空间 `mcp-resource://<server>/<resource-id>`）
4. **MCP 工具产出的大日志自动流入 log_triage pipeline** —— 客户的 kube logs / auditd / app log 任何格式都能被 LLM-based triager 结构化，不拉爆 context
5. **ChangePlan 导出到外部系统** via `plan_output_hook` shell 命令 + plan.json 走 stdin

### 非目标（本 PR 明确不做）

- **外部 skill overlay（原 P2）** —— MCP prompts 已经覆盖"服务端供应领域策略"这一需求，不需要 codrax 再自造 skill overlay 格式
- **MCP HTTP/SSE transport** —— 本次只做 stdio；HTTP 留待以后（协议层抽象已经留了 `types.TransportType` 枚举）
- **MCP sampling**（让 MCP server 反向请求 LLM 调用） —— 规范里有，但当前无用户场景
- **动态加载 / 热重载 MCP server** —— 启动时加载一次，运行中不变
- **MCP server 身份鉴权 / 签名** —— 信任边界是"操作员部署了这份 yaml"，与 providers.yaml 同等信任级别

---

## 2. 现状审计

### 2.1 MCP 代码骨架（`internal/mcp/`）

```
internal/mcp/
├── mcp.go      Registry / Server interface / ToolSchema 定义
└── stdio.go    StdioServer（start/close 已实现；tools/list 和 tools/call 是 TODO）
```

- `Server` 接口（mcp.go:18-25）：`Name / Transport / ListTools / CallTool / Close` 五个方法
- `Registry`（mcp.go:28-91）：线程安全的 map；有 `Register / Get / List / ListAllTools / CallTool / Close`
- `StdioServer.Start()` 完整（stdio.go:39-68）：`exec.Cmd`、stdin/stdout pipe、1MB 扫描 buffer、started flag
- `StdioServer.ListTools()` 和 `StdioServer.CallTool()` **都是 TODO 桩**（stdio.go:90, 109） —— JSON-RPC 协议完全没实现，当前行为是"返 nil / 返'not yet implemented'"
- `Server` 接口**没有** `ListPrompts / GetPrompt / ListResources / ReadResource`

### 2.2 agent 侧消费链（`internal/agent/agent.go`）

- `BaseAgent.buildToolSchemas`（agent.go:1395-1445）把 `tool.Registry.ToolSuggestions` 过滤结果 + `mcpServers.ListAllTools()` 合并为 LLM 工具 schema；第 1422 行 **无条件追加所有 MCP 工具**（不经过 skill allowlist）
- `CallMCPTool` / `MCPCall` 在哪里？grep 过，现在没有；LLM 发起 MCP 工具调用的代码路径**未实现** —— 因为 agent.go 的 `executeTool()` 只分发给 `tool.Registry`，遇到 MCP 工具名会直接 "tool not found"

### 2.3 yaml / cmd 现状

- `codrax.yaml` 无 `mcp_servers` 键
- `cmd/root.go:1345` 调 `mcpRegistry := mcp.NewRegistry()` 后**从不注册任何 server**
- `cmd/root.go:1346` 日志 `"registered %d MCP servers"` 永远打印 0

### 2.4 log_triage 现状

- 前置 stage（`internal/orchestrator/topology.go`），条件触发：`BusContext.AttachedLog != ""` 才跑
- `AttachedLog` 来源目前**只有 CLI**：`--log <file>` / `--log-text <string>` / REPL `/log` 命令
- 已实现：`log-triager` agent + `log-triage-skill` + `log-segmentation-skill`（两步 fallback）+ `logtriage.ValidateBundle` 系统验证 + `logtriage.MergeBundles` 合并多个 partial bundle
- 产物：`BusContext.Mutable.LogTriage()` 的 `*LogBundle`，下游 analyzer / explorer / extractor / finalizer 都能读
- 关键能力：**能处理大日志（32 KB+ 自动两步 fallback）、多种语言格式（Meta.lang）、多栈（Errors[]）** —— 为 MCP 日志路由提供了现成基础

### 2.5 关键缺陷清单

| 缺陷 | 文件:行 | 影响 |
|---|---|---|
| C1 | `stdio.go:90` `ListTools` 返 nil | MCP server 声明了工具，codrax 永远看不到 |
| C2 | `stdio.go:109` `CallTool` 返错误 | LLM 想调 MCP 工具也调不通 |
| C3 | `mcp.go:18-25` Server 接口无 prompts/resources | 服务端领域策略无法注入 |
| C4 | `cmd/root.go:1345` 无 yaml 加载 | 运营无法声明 server |
| C5 | agent 的 `executeTool` 对 MCP 工具名返 "tool not found" | 就算前面都修了，运行时仍调不到 |
| C6 | 无 MCP 调用超时 / 字节上限 / 生命周期保护 | 恶意 / bug server 可以把 codrax 拉爆 |
| C7 | MCP 工具产出大日志时走普通 blob preview 路径 | LLM 看到"头 24K + 尾 4K"，实际根因可能在中间；大日志的结构化机会（log_triage）被浪费 |
| C8 | ChangePlan 产出后只落盘，无对接外部系统通道 | 用户手抄或自写脚本 |

---

## 3. 威胁模型

| # | 威胁 | 严重度 | 现有防御 | 需要新防御 |
|---|---|---|---|---|
| T1 | MCP server 崩溃 / 退出 / hang 阻塞 agent 主循环 | HIGH | 无 | call 级超时 + 生命周期看门狗 |
| T2 | MCP server 返回巨大响应（GB 级日志 dump） | HIGH | 部分 —— blob offload 到 32KB 阈值 | MCP 专用单次响应上限 + 基于预算的截断 + 日志类走 log_triage 路径 |
| T3 | MCP 工具 prompt injection（"忽略之前的指令"） | MEDIUM | 无 | 工具响应进 prompt 时打 `[mcp: <server>.<tool>]` 外套，glossary lint 对 MCP description 做一次扫描 |
| T4 | 两个 MCP server 声明同名工具 | MEDIUM | 无（map 覆盖） | 加载时 namespace 强制 `<server>.<tool>` 避免冲突 |
| T5 | MCP server 声明写语义工具但 codrax 当读工具调用 | HIGH | 无 | 工具 schema 入列时通过 yaml 的 `write_capable: true` 显式标记；读模式 run 不注入 write-capable MCP 工具 |
| T6 | MCP prompts 过长撑爆 context | MEDIUM | 无 | 注入总字节 cap + 按 stage 筛 + 超限截断并日志告警 |
| T7 | MCP server 进程未清理（孤儿 / zombie） | MEDIUM | Close 调用了 Kill + Wait | 进程组管理 + 启动失败回滚 + SIGTERM grace period |
| T8 | plan_output_hook 被恶意设为危险命令 | LOW-MEDIUM | 无 | hook 是操作员在 yaml 自己声明的；信任边界与 providers.yaml 等同；但 codrax 要做 stderr 抑制防止泄漏 plan 内容到奇怪地方 |
| T9 | MCP resource URL 指向本地敏感文件（`file:///etc/shadow`） | HIGH | 无 | resources/read 只接受 server 自己回声的 URL；codrax 不主动构造；每次读也走 blob 上限 |
| T10 | MCP server 返回恶意 JSON 破坏 agent 状态 | MEDIUM | 无 | 严格 schema decode + DisallowUnknownFields |

**两个红线（本 PR 引入 + 守护）**：

- **L-MCP-1**：MCP 工具输出**绝不**进 `ground.BuildContext` / `GroundItem` / `GroundCitation` —— MCP 是外部环境，不是 repo 事实源，不该成 citation pool 候选。结构性测试 go/ast 扫 `internal/mcp/` 确保不 import `internal/tool/ground`
- **L-MCP-2**：MCP resource URL 解析**绝不**允许 `file://` / `/proc` / `/sys` / `/dev` 等本机路径；只接受 server 声明的 opaque ID，实际读取由 server 实现 —— codrax 不承担 URL resolve 责任

---

## 4. 架构总览

```
                  ┌────────────────────────────────────────┐
                  │       codrax.yaml                      │
                  │  mcp_servers:                          │
                  │    - name: obs                         │
                  │      command: mycorp-obs-mcp           │
                  │      tool_metadata:                    │
                  │        fetch_pod_logs: {kind: log_source}│
                  │  plan_output_hook: ./hooks/to_github.sh│
                  └───────────────┬────────────────────────┘
                                  ↓ P1.b 启动加载
      cmd/root.go → mcp.LoadServersFromConfig()
                  ↓
      mcpRegistry.Register(stdioServer) + stdioServer.Start()
                  ↓
      启动时 server.Initialize() → tools/list + prompts/list + resources/list
                  ↓
      每次 agent dispatch：
      ┌───────────────────────────────────────────────┐
      │ BuildAgentContext + BuildPromptContext        │
      │   ├─ 内置 system/user sections（不变）          │
      │   └─ + MCP prompts 相关条目 → "External       │
      │       Guidance" 段（P1.c）                    │
      │ buildToolSchemas                              │
      │   ├─ 内置工具（ToolSuggestions 过滤）            │
      │   ├─ MCP 工具（全量，带 <server>.<tool> 前缀）   │
      │   └─ MCP resources 暴露为 mcp_read_resource     │
      │        伪工具（P1.d）                          │
      └──────────────────┬────────────────────────────┘
                          ↓
      LLM 调 tool（可能是 obs.fetch_pod_logs）
                          ↓
      agent/agent.go::executeTool 识别 `<server>.<tool>` 前缀
                          ↓
      mcpRegistry.CallTool(server, tool, params)
                          ↓
      stdioServer.CallTool → JSON-RPC tools/call → 子进程
                          ↓
      响应返回 → 三条路径（P1.e 大输出消化）：
        (a) 日志类（log_source）→ 喂 log_triager → Mutable.LogTriage
        (b) 普通大输出 → blob offload（走 head+tail 预览）
        (c) 小输出 → 直接进 ToolResult.Summary
                          ↓
      (write mode) plan 阶段后 → plan_output_hook 执行（P3）
```

---

## 5. P1.a — MCP stdio 协议完整化

### 5.1 JSON-RPC 消息格式

MCP 1.0 使用 [JSON-RPC 2.0](https://spec.modelcontextprotocol.io/)，stdio 传输下一行一个 JSON 消息：

```jsonc
// 请求
{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}
// 响应
{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"...","description":"...","inputSchema":{...}}]}}
// 通知（无 id）
{"jsonrpc":"2.0","method":"notifications/initialized"}
// 错误
{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"invalid params"}}
```

### 5.2 生命周期

```
codrax 启动 → StdioServer.Start() exec 子进程
           ↓
         发送 initialize 请求（包含 codrax client 能力声明）
           ↓
         接收 initialize 响应（server 能力 + server 版本）
           ↓
         发送 notifications/initialized（确认）
           ↓
         [server 就绪]
           ↓
         按需发送 tools/list / prompts/list / resources/list（启动时各一次）
           ↓
         运行中：每次 LLM 调工具 → tools/call
                每次需要 prompt → prompts/get
                每次需要 resource → resources/read
           ↓
codrax 退出 → Close() 优雅 SIGTERM（3s grace）→ SIGKILL fallback
```

### 5.3 `Server` 接口扩展

```go
// internal/mcp/mcp.go
type Server interface {
    // 既有
    Name() string
    Transport() types.TransportType
    ListTools() []ToolSchema
    CallTool(name string, params json.RawMessage) (types.MCPResponse, error)
    Close() error

    // 新增
    Initialize() error                                                // 握手
    ListPrompts() []PromptSchema                                      // P1.c
    GetPrompt(name string, args map[string]string) (PromptResult, error) // P1.c
    ListResources() []ResourceSchema                                  // P1.d
    ReadResource(uri string) (ResourceContents, error)                // P1.d

    // 健康
    IsHealthy() bool                                                  // 看门狗调用
}
```

### 5.4 并发 / 阻塞保护

当前 stdio 是**行-based 同步** JSON-RPC。codrax 每次调一个 tool 要：

- 拿到 server 级 mutex
- 写入请求行
- 扫描回单行（阻塞等待）
- 释放 mutex

**超时**：每次 call 带 `context.WithTimeout(parent, serverConfig.TimeoutMs)`。超时 → 放弃该 call + 给 LLM 返 `MCPResponse{Success: false, Summary: "call timeout after 10s"}` + 不 kill 进程（进程或许还能恢复响应后续请求；但超时的 id 结果被丢弃）

**响应字节上限**：scanner buffer 当前 1 MB（stdio.go:60）；改成可配置 `mcp_max_response_bytes`（默认 4 MB），超出 scanner error → server 标记 unhealthy，下次 call 直接返错误不发请求

**进程看门狗**（可选 defer 到 P1-stretch）：每 30s 跑一次 `IsHealthy()` —— 发 `ping` 或复用 `tools/list` heartbeat；连续 3 次失败自动 Close + 启动 replacement。**本 PR 不做**，简单 fail-fast 即可

---

## 6. P1.b — yaml 加载 + 启动注册 + 生命周期

### 6.1 yaml schema

```yaml
# codrax.yaml
mcp_servers:
  - name: obs                          # 必须唯一；在工具 schema 里以 "<name>.<tool>" 出现
    enabled: true                      # 默认 true；false 时加载但不 start，便于调试
    transport: stdio                   # 目前只支持 stdio
    command: mycorp-obs-mcp            # 子进程可执行文件
    args: ["--project", "prod"]        # 子进程参数（可选）
    env:                               # 额外环境变量（可选；基础环境来自 os.Environ）
      OBS_ENDPOINT: "https://obs.internal"
    timeout_ms: 10000                  # 单次 call 超时（默认 10s）
    startup_timeout_ms: 3000           # 启动 + initialize 握手超时（默认 3s）
    max_response_bytes: 4194304        # 单次响应字节上限（默认 4 MB）
    tool_metadata:                     # 工具级元数据，覆盖默认行为
      fetch_pod_logs:
        kind: log_source               # 触发 P1.e 日志路由
        log_source_prefix: "/var/log/pods/"  # 传给 logtriage 的 source-prefix strip
      query_metrics:
        kind: data_fetch               # 默认路径：走 blob offload
      apply_config:
        write_capable: true            # 标记为写工具 → 受 mcp_write_enabled 约束（决策 #5）

# 全局 MCP 预算
mcp_max_prompt_bytes_per_stage: 8192   # External Guidance 段字节上限
mcp_max_servers: 8                     # 同时运行的 MCP server 数上限

# 决策 #5 —— 写模式 MCP 工具 opt-in 开关
# 即使 tool_metadata 标 write_capable: true，默认也 NOT 暴露给任何 agent
# （读模式 hide；写模式 hide）。操作员想让 MCP 写工具参与 apply 阶段，必须
# 显式把此键翻 true。
mcp_write_enabled: false
```

### 6.2 cmd/root.go 加载流程

```go
// 位置：现在 mcpRegistry := mcp.NewRegistry() 之后
if rs != nil && len(rs.MCPServers) > 0 {
    if len(rs.MCPServers) > getMCPMaxServers(rs) {
        return fmt.Errorf("mcp_servers count %d exceeds mcp_max_servers cap %d", ...)
    }
    for _, cfg := range rs.MCPServers {
        srv, err := mcp.NewStdioServerFromConfig(cfg)
        if err != nil {
            return fmt.Errorf("mcp server %q: %w", cfg.Name, err)
        }
        if !cfg.Enabled {
            logging.Info("[mcp] %s: disabled by yaml, skipping", cfg.Name)
            continue
        }
        if err := srv.Start(); err != nil {
            return fmt.Errorf("mcp server %q start: %w", cfg.Name, err)
        }
        if err := srv.Initialize(); err != nil {
            _ = srv.Close()
            return fmt.Errorf("mcp server %q initialize: %w", cfg.Name, err)
        }
        mcpRegistry.Register(srv)
        logging.Info("[mcp] %s: ready (tools=%d prompts=%d resources=%d)",
            cfg.Name, len(srv.ListTools()), len(srv.ListPrompts()), len(srv.ListResources()))
    }
}

// 在 main 或 signal handler 退出链尾部：
defer mcpRegistry.Close()
```

### 6.3 失败模式

- **命令不存在**：Start 失败 → `cmd/root.go` 返 error → 启动失败 loudly
- **initialize 超时**：Close + 启动失败
- **任一 server 启动失败**：整个 codrax 启动失败，不部分生效
- **运行中 server hang / 崩溃**：下次调用返 `{Success: false, Summary: "server <name> unreachable"}`；不 kill codrax；不重启 server（简单）

---

## 7. P1.c — prompts 通道

### 7.1 目的

让 MCP server 对客户端**注入领域化调用策略** —— 代替"外部 skill overlay"。

### 7.2 选择策略（决策 #1 —— 选 always-inject）

**决策**：所有连接的 MCP server 的所有 prompts，**无条件**注入到每个 dispatch 的 system prompt（受总字节 cap 约束）。

**理由**（相比 auto/stage+keyword 过滤）：
- **可扩展性**：server 作者不需要知道 codrax 的 stage 分类法、不需要学习 `[[codrax:stages=...]]` 元数据语法
- **跨工具复用**：MCP server 的投入可以同时给 Claude Desktop / Zed / codrax 用，无 codrax-specific 耦合
- **LLM 判断力够用**：现代 LLM 看完 prompt name + description 能判断是否相关；让系统做"为你过滤"的聪明事反而可能筛掉边界 case
- **实现简单**：不用在 codrax 里维护 stage↔keyword↔prompt 的匹配矩阵
- **如果 server 作者想精细化**：就写**多个粒度更细的 prompt**（如 `diagnose-kube-in-analyze` 和 `diagnose-kube-in-explore`），通过命名 + description 让 LLM 自筛

**注入顺序确定性**：server name 字母序 → 每 server 内 prompt name 字母序。超总字节 cap 时截断尾部（按字母序先加载的优先保留），日志 WARN 列出被丢弃的 prompt 名。

### 7.3 PromptSchema

```go
// internal/mcp/mcp.go（追加）
type PromptSchema struct {
    Name        string           `json:"name"`
    Description string           `json:"description"`
    Arguments   []PromptArgument `json:"arguments,omitempty"`
}

type PromptArgument struct {
    Name        string `json:"name"`
    Description string `json:"description"`
    Required    bool   `json:"required,omitempty"`
}

type PromptResult struct {
    Description string          `json:"description"`
    Messages    []PromptMessage `json:"messages"`
}

type PromptMessage struct {
    Role    string `json:"role"`    // "user" / "assistant"（MCP 规范不支持 system）
    Content string `json:"content"`
}
```

### 7.4 注入机制

在 `internal/context/builder.go::BuildPromptContext` 新增一段 `External Guidance`：

```go
// 位置：在 "Output Format" 之后、"Prohibitions" 之前
// 签名（新）：
//   func buildExternalGuidanceSection(mcpReg *mcp.Registry, maxBytes int) (string, []string)
// 返回 (content, droppedPromptNames)
if ac.MCPRegistry != nil {
    section, dropped := buildExternalGuidanceSection(
        ac.MCPRegistry,
        ac.MCPMaxPromptBytesPerStage, // 从 codrax.yaml 读
    )
    if section != "" {
        pc.SystemSections = append(pc.SystemSections, types.PromptSection{
            Title:   "External Guidance (from MCP servers)",
            Content: section,
        })
    }
    if len(dropped) > 0 {
        logging.Warning("[mcp] external guidance truncated; dropped prompts: %v", dropped)
    }
}
```

**渲染格式**：

```markdown
## External Guidance (from MCP servers)

> External prompt from `audit.compliance-annotation`:
>
> [prompt content — full body from prompts/get]

---

> External prompt from `obs.diagnose-production-issue`:
>
> [prompt content]
```

`>` blockquote prefix + `[mcp: <server>.<name>]` 外套保证 LLM 知道这段是"外部系统的建议"而非 codrax 自家指令（T3 缓解）。

### 7.5 客户写 prompt 的范式

```python
# MCP server Python 示例
@mcp_server.prompt(name="diagnose-production-issue")
async def diagnose_prod():
    """Investigate a crashing Kubernetes pod and propose a fix plan.

    Applicable when: user mentions a pod name / OOM / CrashLoopBackOff /
    container restart / production incident.
    """
    return [
        PromptMessage(role="user", content="""
When the user mentions a crashing pod, investigate in this order:

1. Call `obs.describe_pod(name=<pod_name>)` for last-known state + events
2. Call `obs.fetch_logs(pod=<pod_name>, since="1h")` — this is log_source-kind,
   the LogBundle will be auto-triaged for downstream stages. Do NOT re-parse
   the raw log text yourself.
3. Call `obs.query_metrics(pod=<pod_name>, metric="memory")` to confirm
   memory pressure vs. other causes.
4. Grep the repo for code paths named in stack traces.
5. Propose JVM heap tuning / resource limit / leak fix in the ChangePlan.
""")
    ]
```

server 作者的**相关性提示**写在 prompt 的 description 首行（LLM 自筛依据）。codrax 不解析任何 `[[codrax:...]]` 元数据。

---

## 8. P1.d — resources 通道

### 8.1 目的

让 MCP server 暴露**数据快照** —— 当前 k8s context / 最新 grafana dashboard JSON / 当前 sprint ticket 列表 …… LLM 可以按需拉取。

### 8.2 接入形态

resources 是 "URI-addressable snapshots"：server 声明 `resource://<server>/<id>` 形式的 URI 列表 + mime-type + description。

codrax 把它暴露为一个**伪工具 `mcp_read_resource`**：

```jsonc
{
  "name": "mcp_read_resource",
  "description": "Read a resource snapshot from an MCP server. Resources are described in the 'External Resources' section of your system prompt.",
  "parameters": {
    "type": "object",
    "properties": {
      "uri": {"type": "string", "description": "The resource URI (e.g. 'resource://obs/current-k8s-context')"}
    },
    "required": ["uri"]
  }
}
```

可用 resources 列表在 system prompt 的 **`External Resources`** 段渲染：

```markdown
## External Resources (MCP)

| URI | Description | MIME type |
|---|---|---|
| `resource://obs/current-k8s-context` | Current kubeconfig context name, cluster, namespace | application/json |
| `resource://audit/open-incidents` | Open incidents in PagerDuty (JSON) | application/json |
```

LLM 要读时调 `mcp_read_resource({uri: "resource://obs/current-k8s-context"})` → codrax 路由到对应 server 的 `resources/read`。

### 8.3 安全（L-MCP-2 + 决策 #2 —— 信任 server 作者）

- **codrax 不解析 URI scheme**；server 自己声明的 URI 原样转回去让 server 自己 resolve
- **codrax 拒绝非 `resource://` 前缀的 URI**；防止 LLM 被诱导传 `file:///etc/passwd`
- **每次读取走 blob offload**（同工具输出，避免大 resource 拉爆）
- **`mcp_read_resource` 的可见性**：只在 `mcpRegistry.ListAllResources() != nil` 时才暴露给 LLM；无 resources 的部署不会看到这个伪工具
- **无 ACL / allowlist**：server 作者声明的所有 resources 对 LLM 全部可见 —— 信任边界是"你在 codrax.yaml 里声明了这个 MCP server"。想精细控制就在 server 侧做，不在 codrax 侧

### 8.4 是否本 PR 做

**建议做**。成本低（协议加两个 method + 一个伪工具），价值高（LLM 能查 k8s 当前 context 再判断日志路径等）。若 scope 超预算可延到下一个 PR。

---

## 9. P1.e — 大日志与大输出的消化路径

### 9.1 问题

客户典型日志 source 的量级：

| source | 典型单次响应大小 | 单条结构 |
|---|---|---|
| kubectl logs pod --since=1h | **10 MB ~ 500 MB** | 一行一条 JSON / 纯文本混合 |
| journalctl --since=... | 10 ~ 100 MB | 半结构化 |
| datadog logs query | **可达 1 GB**（按 query 窗口） | JSON-lines |
| sentry issue detail | 100 KB ~ 5 MB | 单个大 JSON object |
| auditd 原始输出 | 10 ~ 50 MB | 每行 key=value |

如果按现有 blob 机制（`blob_max_inline_bytes: 32768`）：LLM 只看到头 24K + 尾 4K。**关键根因极可能在中间的 stack frame / 特定错误消息**，LLM 完全看不到 → 直接失败。

### 9.2 三条消化路径

**路径 1 —— 日志类工具走 log_triage pipeline（主路径，P1 核心）**

yaml 标记 `tool_metadata.<tool>.kind: log_source` 后，agent/agent.go 的 `executeTool` 在收到 MCP 响应后：

```go
if toolMeta.Kind == "log_source" {
    // 把响应原文作为 AttachedLog 加进 log_triage 处理
    bundle, err := logTriagerAgent.TriageInline(ctx.Mutable, response.Content, toolMeta.LogSourcePrefix)
    if err != nil {
        // 降级：作为普通 blob 输出
        return fallbackBlobPath(response)
    }
    // 合并到现有 Mutable.LogTriage （可能已有 --log 传入的 bundle）
    if existing := ctx.Mutable.LogTriage(); existing != nil {
        merged := logtriage.MergeBundles(existing, bundle)
        ctx.Mutable.SetLogTriage(merged)
    } else {
        ctx.Mutable.SetLogTriage(bundle)
    }
    // LLM 看到的 Summary 是结构化摘要，不是原始日志头尾
    return types.ToolResult{
        ToolName: fmt.Sprintf("%s.%s", server, tool),
        Summary: renderLogBundleSummary(bundle),  // e.g. "[log_bundle: lang=java signals=[oom,panic] errors=3 residue=12KB]"
        Success: true,
    }
}
```

**好处**：
- LLM 拿到的是**结构化 bundle**：`Meta{lang:java, signals:[oom]}` + `Errors[0]{type:OutOfMemoryError, frame:App.main()}` + 关键 residue 片段 + Layer 4 的 `ResolvedFiles`
- 512 MB 原始日志 → 几 KB 结构化摘要，LLM 上下文友好
- 现有的两步 fallback（大日志 > 32 KB 自动 segment）**免费复用**
- 同一个 run 里 `--log` 传入的 bundle + MCP 拉回的 bundle 自动 merge（`logtriage.MergeBundles` 已实现）

**路径 2 —— 非日志大输出走 blob offload**

kind 不是 `log_source` 的大输出（metrics JSON dump、ticket 列表等）走现有 blob 路径，head+tail 预览 + blob path 让 LLM 按需 `read_file`。

**路径 3 —— 小输出直接进 Summary**

响应 ≤ `blob_max_inline_bytes` → 照常进 `ToolResult.Summary`，不改动。

### 9.3 log_triage 增量能力

- 现有 log_triage 是**一次性 CLI 前置**（Run 开始前 `AttachedLog` 非空才跑）
- 新需求：**运行中动态触发**，每次 LLM 调 `log_source` MCP 工具都要现场跑一次
- 改动：`log_triager` agent 暴露 `TriageInline(mut, rawText, sourcePrefix) (*LogBundle, error)` 方法，**不依赖** preStage 机制
- 内部路径：直接一次 `emit_log_triage` LLM 调用（复用 `log-triage-skill`）；size > `log_triage_two_step_bytes` 走 `emit_log_segmentation` fallback —— 这些逻辑现在全部依赖 `BusContext.AttachedLog`，改成参数传入即可

### 9.4 资源边界

- **单次 MCP 日志响应**：硬上限 `mcp_log_source_max_bytes`（默认 64 MB）；超限 → 截断 + 日志 WARN + 继续 triage（bundle 会有 `Residue.OvercapTruncated: true` 标记）
- **单 Run MCP 日志累积**：所有 log_source 调用加总不超过 `mcp_log_source_total_bytes_per_run`（默认 256 MB）；超限 → 后续 log_source 调用直接返"quota exhausted"错误
- **LogBundle 合并后总大小**：借用现有 log_triage 的结构化 cap（`Errors` 数、`Residue` 字节）—— 已经有兜底

### 9.5 降级语义

- log_triager 失败（LLM 出错 / 超时）→ **降级为 blob 路径**（路径 2），LLM 拿到的是原始头+尾；log 里 WARN
- log_source 工具调用失败 → `Success: false` 的 ToolResult，LLM 可换 tool 重试
- 配了 log_source 但 response 看起来不像日志（JSON blob）→ triager 会返 coverage 极低的 bundle，**按 coverage 阈值**决定是结构化展示还是走 blob 原文

---

## 10. P3 — plan 导出 hook

### 10.1 配置

```yaml
# codrax.yaml
plan_output_hook:
  command: ./hooks/plan_to_github_pr.sh     # 必填
  args: ["--repo", "mycorp/svc", "--label", "auto-plan"]
  timeout_ms: 30000                          # 默认 30s
  trigger:                                   # 哪些 PlanStatus 触发
    - pending_approval                       # 默认：emit_change_plan 刚写 + 落盘后
    - applied                                # 可选：apply 成功后再送一次
  on_failure: warn                           # warn（默认，不影响主流程）/ fail（fail-loud）
  env:                                       # 额外环境变量
    GITHUB_TOKEN_FILE: /run/secrets/gh-token
```

### 10.2 运行时

- 在 `cmd/root.go::writePlanFile` 成功后（plan.json 已落盘）调 `runPlanOutputHook(hookCfg, plan)`
- hook 以 **plan.json 完整内容** stdin 喂入
- hook stdout 被 codrax 吃掉（避免干扰 REPL）；stderr 记 warning
- hook exit code：0 → OK；非 0 + `on_failure: warn` → WARN 继续；非 0 + `on_failure: fail` → Run 返 error
- hook 超时 → SIGTERM + 3s grace → SIGKILL；timeout 事件按 on_failure 处理

### 10.3 触发点（可选触发多次）

- `pending_approval`：emit_change_plan 后立即（单 shot 或 REPL `/mode plan` 后）
- `applied`：`PlanStatus` 翻成 `applied` 时（apply + verify 都成功）
- `verify_failed`：apply 成功但 verify 挂了时，如果客户想收"部分成功"通知

默认**只**触发 `pending_approval`。其他需要 yaml 显式声明。

### 10.4 hook 脚本范式

```bash
#!/usr/bin/env bash
# ./hooks/plan_to_github_pr.sh
set -e
PLAN_JSON=$(cat)  # 从 stdin 读 plan.json

# 解析 plan ID + summary + target files + rationales
PLAN_ID=$(echo "$PLAN_JSON" | jq -r '.id')
SUMMARY=$(echo "$PLAN_JSON" | jq -r '.summary')
FILES=$(echo "$PLAN_JSON" | jq -r '.target_paths | join(", ")')

# 调 GitHub API
curl -X POST \
  -H "Authorization: token $(cat $GITHUB_TOKEN_FILE)" \
  -H "Accept: application/vnd.github.v3+json" \
  https://api.github.com/repos/$1/issues \
  -d "$(jq -n --arg title "$SUMMARY" --arg body "Files: $FILES\nPlan ID: $PLAN_ID" \
      '{title: $title, body: $body, labels: ["auto-plan"]}')"
```

---

## 11. 资源预算与失败隔离

### 11.1 配置汇总

| 键 | 默认值 | 作用 |
|---|---|---|
| `mcp_max_servers` | 8 | 同时 running server 数上限 |
| `mcp_max_prompt_bytes_per_stage` | 8192 | External Guidance 段字节 cap |
| `mcp_log_source_max_bytes` | 67108864 (64 MB) | 单次 log_source 响应字节上限 |
| `mcp_log_source_total_bytes_per_run` | 268435456 (256 MB) | 整个 Run 的 log_source 累积 |
| 每 server：`timeout_ms` | 10000 | 单次 call 超时 |
| 每 server：`startup_timeout_ms` | 3000 | 启动 + initialize 握手 |
| 每 server：`max_response_bytes` | 4194304 (4 MB) | 单次响应硬上限（非 log_source） |
| `plan_output_hook.timeout_ms` | 30000 | hook 执行超时 |

### 11.2 失败隔离策略

| 失败类型 | 处理 |
|---|---|
| server 启动失败 | 整个 codrax 启动失败 (fail-loud) |
| server 运行中单次 call 超时 | 返 `{Success: false}` + 不 kill server（可能下次还能响应） |
| server 运行中返回畸形 JSON | 返 error + 标记 unhealthy，下次 call 直接拒 |
| server 进程崩溃 | Close + 本 Run 后续 call 全部返错误；不重启 |
| log_triager 解析大日志失败 | 降级 blob 路径 |
| plan_output_hook 失败 + on_failure=warn | WARN + 继续 |
| plan_output_hook 失败 + on_failure=fail | Run 返 error（影响 CI / 脚本退出码） |

### 11.3 日志 / 诊断

- 启动：`[mcp] <name>: ready (tools=N prompts=N resources=N)` INFO
- 每次 call：`[mcp] <name>.<tool> called (args_bytes=X, latency_ms=Y, response_bytes=Z)` DEBUG
- 每次失败：`[mcp] <name>.<tool> failed: <reason>` WARN
- hook 执行：`[plan_hook] triggered (status=<s>, exit=<code>, duration_ms=<d>)` INFO

**决策 #4 —— env 不做额外屏蔽**：yaml 里写的 `env` map 就地传给子进程，codrax 的启动日志**不**打印 env 内容（连 key 也不打）。用户把 secret 通过 `TOKEN_FILE=/run/secrets/x` 方式挂进文件系统而非写明文是操作员的职责，codrax 不替他兜底。启动日志只打印 server name / command / args（args 按原样；如果 args 里有敏感信息是用户自己的选择）。

---

## 12. 测试计划

### 12.1 内部单元 / 结构性

| 测试 | 覆盖 |
|---|---|
| `TestStdioServer_JSONRPCRoundTrip` | 启动 fake echo server，发 tools/list → 收响应 |
| `TestStdioServer_InitializeHandshake` | 跑完整握手 + 超时分支 |
| `TestStdioServer_TimeoutReturnsFailure` | mock 一个慢 server，超时返 Success:false 不 kill |
| `TestStdioServer_LargeResponseExceedsCap` | mock 4 MB+ 响应；ListTools 返错误 |
| `TestMCPRegistry_LoadFromYAML_DupNameRejected` | 两个同 name server → reject |
| `TestMCPRegistry_LoadFromYAML_AllOrNothing` | 多 server 有一个启动失败 → 全部回滚 |
| `TestMCPPromptInjection_AutoStageMatch` | stages=explore 的 prompt 只在 explore stage 注入 |
| `TestMCPPromptInjection_KeywordMatch` | keywords 匹配才注入 |
| `TestMCPPromptInjection_BytesCapEnforced` | 合计 > cap → 截断 + WARN |
| `TestMCPReadResource_PseudoToolOnlyWhenResourcesExist` | 无 resource 时伪工具不暴露 |
| `TestMCPReadResource_RejectsNonResourceScheme` | `file:///etc/passwd` → reject |
| `TestMCPWriteCapableHiddenInReadMode` | yaml 标 write_capable → 读模式 build schema 不含此工具 |
| `TestMCPLogSourceRoutesToTriage` | kind=log_source 工具响应 → Mutable.LogTriage 非空 |
| `TestMCPLogSourceMergesWithAttachedLog` | `--log` + MCP log_source 两处 → MergeBundles |
| `TestMCPLogSource_QuotaExhausted` | 累积字节超上限 → 后续 call 返错误 |
| `TestPlanOutputHook_ExecutesOnApprovalAndAppliedStatuses` | 按 trigger 列表触发 |
| `TestPlanOutputHook_TimeoutKillsProcess` | 长跑 hook 到超时 |
| `TestPlanOutputHook_OnFailureWarnVsFail` | 两种 on_failure 分支 |
| `TestMCPNoGroundingImport` | go/ast 扫描 internal/mcp/ 不 import internal/tool/ground（L-MCP-1） |

### 12.2 端到端 smoke（新 `eval/cases`）

**Case 1 —— MCP log fetch + triage**
- fixture: 起一个本地 fake MCP server（Python）暴露 `fetch_pod_logs` 返回一段内嵌 Java OOM 栈
- eval case: 问 "pod-abc 最近 1h 有 crash 吗，分析一下"
- 期望：analyzer 调 MCP `fetch_pod_logs` → log_triage → bundle.signals 含 `oom` → finalizer 点出 OutOfMemoryError

**Case 2 —— plan 导出 hook**
- fixture: 现有 patch_typo_go + 一个 hook 脚本把 plan.json 写到临时文件
- eval case: `--mode=apply --auto-apply ...`
- 期望：apply 后临时文件存在，内容是 plan.json

### 12.3 压测 / 边界

- 512 MB 日志输入压力测试 → log_triage 两步 fallback + 累积上限 truncate
- 100 个 MCP 工具同时在 tool schema 中 → LLM schema 大小 / 解析延迟
- 并发 MCP call（如果 BaseAgent 单轮多 tool_calls）→ 观察 mutex 串行化

---

## 13. 开工顺序

**阶段 A — 基础协议** (independent commit)

1. `internal/types/providers.go` / `internal/config/runtime.go` 加 `MCPServers []MCPServerConfig` + 全局 MCP caps
2. `internal/mcp/stdio.go` 补完 `Initialize / ListTools / CallTool`（JSON-RPC 同步实现 + 超时 + 字节上限）
3. `internal/mcp/stdio.go` 补 `ListPrompts / GetPrompt` + Server 接口扩展
4. 单元测试（`TestStdioServer_*`）
5. `internal/mcp/stdio.go` 补 `ListResources / ReadResource`
6. 测试补完

**阶段 B — yaml 加载 + 启动生命周期** (commit)

7. `cmd/root.go` 加 `loadMCPServers(rs)` 启动注册 + defer Close
8. 测试 `TestMCPRegistry_LoadFromYAML_*`

**阶段 C — agent 消费 MCP** (commit)

9. `agent/agent.go::executeTool` 识别 `<server>.<tool>` 前缀并路由到 mcpRegistry.CallTool
10. `agent/agent.go::buildToolSchemas` 写模式 / 读模式分流；write_capable MCP 工具按 mode 暴露
11. `context/builder.go` 加 "External Guidance" 段 + prompt 选择逻辑
12. `context/builder.go` 加 "External Resources" 段 + mcp_read_resource 伪工具注册

**阶段 D — 大日志路由** (commit)

13. `internal/agent/log_triager.go` 暴露 `TriageInline` 方法
14. `agent/agent.go::executeTool` 对 log_source MCP 工具接响应 → 调 TriageInline → 更新 Mutable.LogTriage
15. 加 quota counter + 超限降级逻辑
16. 测试 `TestMCPLogSource*`

**阶段 E — plan 导出 hook** (commit)

17. `internal/config/runtime.go` 加 `PlanOutputHook` 配置
18. `internal/types/change_plan.go` 相邻：加 `runPlanOutputHook(cfg, plan)` helper
19. 触发点：`cmd/root.go::writePlanFile` 之后 + `orchestrator::persistPlanStatus` 里条件触发
20. 测试 `TestPlanOutputHook_*`

**阶段 F — 文档 + eval** (commit)

21. `codrax.yaml.example` 加 mcp + hook 注释示例
22. `docs/user_guide.md` 新章节 "MCP 集成" + "plan 导出"
23. `docs/architecture.md` §3.4/§3.5 扩到 MCP 完整三通道 + §4.5 log_triage 支持动态输入
24. `eval/cases/mcp_log_triage.case` + `eval/cases/plan_hook.case` 两个端到端 smoke
25. `eval/fixtures/fake_mcp_server/` Python 测试 server

每阶段独立 commit + 全仓测试绿才进下一阶段。Total 估 ~20 commits，但每个都小。

---

## 14. 决策记录 / 未决项

### 已决

| 决定 | 选项 | 选中 | 理由 |
|---|---|---|---|
| 外部能力第一载体 | MCP tools / 自造 skill 外挂 | **MCP tools** | 协议标准化；多工具生态（Claude Desktop / Zed 都用） |
| 领域策略注入渠道 | 自造 skill overlay / MCP prompts | **MCP prompts** | 避免发明专有格式；用户 MCP server 投入跨工具复用 |
| resources 本 PR 做不做 | 做 / 延 | **做**（低成本高回报） | 让 LLM 能查 k8s context / grafana 快照；协议本就要求支持 |
| 大日志消化 | blob 硬截断 / log_triage 动态路由 | **log_triage 动态路由** | 现有两步 fallback 已经能消化 GB 级日志；复用胜过重造 |
| plan 导出机制 | REST API / webhook / shell hook | **shell hook** | 最大灵活性；跨平台；客户写脚本成本低 |
| 导出触发时机 | 总是 / 按 status | **按 status（默认 pending_approval）** | 不同下游系统订阅不同状态；yaml 声明 |
| 失败处理策略 | 默认 warn / 默认 fail | **默认 warn** | hook 是 best-effort 通知机制，不该阻塞主流水线；但可 opt-in fail |

### 原未决项 —— 决策已锁定

| # | 议题 | 选项 | **定稿** | 理由 |
|---|---|---|---|---|
| 1 | MCP prompts 选择策略 | `auto`(stage+keyword 双过滤) / `always`(全注入) | **`always` 全注入 + 总字节 cap** | 扩展性最好：server 作者不绑 codrax stage 分类法，MCP server 投入跨 Claude Desktop / Zed 可复用；LLM 读 prompt name+description 自判断相关性足够；实现最简 |
| 2 | `mcp_read_resource` ACL | 加 allowlist / 全信任 | **全信任 server 作者** | 信任边界：yaml 里声明了 server = 信任；想精细控制放在 server 侧做 |
| 3 | MCP server 进程生命周期 | per-Run 重启 / 单次 Start 跑到 codrax 退出 | **单次 Start** | cost 低且合理；per-Run 重启会拖慢每次 REPL 提问；污染隔离通过 codrax 侧的 Mutable 全新实例已经保证 |
| 4 | env 屏蔽策略 | mask secret-like keys / 不额外处理 | **启动日志不打 env（不打 key 不打 value）** | 用户用 `TOKEN_FILE` 挂文件而非 env 明文是操作员职责；codrax 不替他兜底复杂的 secret 识别 |
| 5 | write_capable MCP 工具在写模式可见性 | 直接暴露 / 显式 opt-in | **`mcp_write_enabled: false` 默认关** | 安全优先：标记 write_capable 的 MCP 工具默认不出现在任何 agent 的 tool schema；操作员显式翻开关才生效 |

### 非决 —— 给下一 PR 的 scope

- MCP sampling（反向 LLM 调用）
- MCP HTTP/SSE transport
- MCP server 热重载 / SIGHUP 配置重读
- MCP server 熔断器 / 退避重连
- 多模型 MCP prompts 分发（给不同 agent 的 LLM 不同 prompts）
- ChangePlan 反向收回（外部系统 push 修正）

---

## 15. 代码面详细设计（file:line 级）

> 所有决策已锁定（§14）。本节把每个阶段的新增 / 修改点落到具体文件 + 函数签名。下一 session 开工直接按此对码。

### 15.1 新增文件

| 文件 | 内容 |
|---|---|
| `internal/mcp/jsonrpc.go` | JSON-RPC 2.0 编解码 helper：`request(id, method, params)` / `parseResponse(line)` / `parseNotification(line)`；错误类型 `RPCError{Code, Message, Data}` |
| `internal/mcp/initialize.go` | `(s *StdioServer) Initialize() error` —— 握手流程：发 `initialize` 请求、读响应、发 `notifications/initialized`，超时控制 |
| `internal/mcp/prompts.go` | `PromptSchema` / `PromptArgument` / `PromptResult` / `PromptMessage` 类型（§7.3）+ `StdioServer.ListPrompts()` / `.GetPrompt()` |
| `internal/mcp/resources.go` | `ResourceSchema` / `ResourceContents` 类型 + `StdioServer.ListResources()` / `.ReadResource()`（§8） |
| `internal/mcp/config.go` | `MCPServerConfig` / `MCPToolMetadata` yaml 结构 + `NewStdioServerFromConfig(cfg) (*StdioServer, error)` |
| `internal/mcp/loader.go` | `LoadServers(reg *Registry, cfgs []MCPServerConfig, maxServers int) error`：for-loop 创建 / Start / Initialize / Register，任一失败 → Close 已注册的 + 返 error |
| `internal/mcp/render.go` | `RenderLogBundleSummary(b *types.LogBundle) string` / `RenderExternalGuidanceSection(reg *Registry, maxBytes int) (body string, dropped []string)` / `RenderExternalResourcesSection(reg *Registry) string` |
| `internal/mcp/tools_pseudo.go` | 注册 `mcp_read_resource` 伪工具的 `tool.Tool` 实现（Execute 内部查 `BusContext.Mutable.MCPRegistryHandle()`）|
| `internal/mcp/stdio_test.go` | 启动 fake echo server（临时脚本 via `exec.Command("bash", "-c", "...")`），覆盖 RPC 握手 / timeout / 超限 |
| `internal/mcp/loader_test.go` | yaml 加载 / dup name / 启动失败回滚 |
| `internal/mcp/render_test.go` | prompt 截断 + dropped 列表 + resource 渲染 |
| `internal/tool/plan_hook.go` | `type PlanHookConfig` + `func RunPlanOutputHook(cfg *PlanHookConfig, planJSON []byte, statusTrigger string) error` —— exec 子进程、stdin 喂 JSON、超时 SIGTERM+3s+SIGKILL |
| `internal/tool/plan_hook_test.go` | 超时 / exit 非零 + warn / exit 非零 + fail |

### 15.2 修改既有文件（新加字段 / 方法）

**`internal/mcp/mcp.go`**

```go
// Server 接口追加方法（§5.3）：
type Server interface {
    // 既有 Name / Transport / ListTools / CallTool / Close 保持
    Initialize() error
    ListPrompts() []PromptSchema
    GetPrompt(name string, args map[string]string) (PromptResult, error)
    ListResources() []ResourceSchema
    ReadResource(uri string) (ResourceContents, error)
    IsHealthy() bool
}

// Registry 追加 helpers：
func (r *Registry) ListAllPrompts() []ServerPromptPair { ... }
func (r *Registry) ListAllResources() []ServerResourcePair { ... }
func (r *Registry) GetPrompt(server, name string, args map[string]string) (PromptResult, error)
func (r *Registry) ReadResource(server, uri string) (ResourceContents, error)
```

**`internal/mcp/stdio.go`**

```go
// 补完 ListTools / CallTool（§5.1 JSON-RPC）+ Initialize + Health；新字段：
type StdioServer struct {
    // 既有
    name, command string
    args          []string
    mu            sync.Mutex
    cmd           *exec.Cmd
    stdin         io.WriteCloser
    scanner       *bufio.Scanner
    started       bool

    // 新增
    env              []string
    timeoutMs        int
    startupTimeoutMs int
    maxResponseBytes int
    toolMeta         map[string]MCPToolMetadata // name → kind / log_source_prefix / write_capable
    promptsCache     []PromptSchema  // Initialize 后填充
    resourcesCache   []ResourceSchema
    rpcCounter       atomic.Int64    // 递增 request id
    healthy          atomic.Bool
}

// 补 CallTool 的 JSON-RPC 流：
func (s *StdioServer) CallTool(name string, params json.RawMessage) (types.MCPResponse, error) {
    // 1. acquire mu
    // 2. build request id
    // 3. send {"jsonrpc":"2.0","id":N,"method":"tools/call","params":{"name":...,"arguments":...}}
    // 4. read one response line with context.WithTimeout(s.timeoutMs ms)
    // 5. parse; if error code != 0 → return MCPResponse{Success:false}
    // 6. if result.content size > maxResponseBytes → return MCPResponse{Success:false, Summary:"response exceeded cap"}
    // 7. return MCPResponse{Success:true, Summary/Content:...}
}
```

**`internal/types/providers.go`**（无改，复用）

**`internal/config/runtime.go`**

```go
type RuntimeSettings struct {
    // 既有字段保持

    // 新增（P1）：
    MCPServers                  []MCPServerConfig `yaml:"mcp_servers"`
    MCPMaxServers               *int              `yaml:"mcp_max_servers"`                 // 默认 8
    MCPMaxPromptBytesPerStage   *int              `yaml:"mcp_max_prompt_bytes_per_stage"`  // 默认 8192
    MCPWriteEnabled             *bool             `yaml:"mcp_write_enabled"`               // 默认 false
    MCPLogSourceMaxBytes        *int              `yaml:"mcp_log_source_max_bytes"`        // 默认 64 MB
    MCPLogSourceTotalBytesRun   *int              `yaml:"mcp_log_source_total_bytes_per_run"` // 默认 256 MB

    // 新增（P3）：
    PlanOutputHook *PlanOutputHookConfig `yaml:"plan_output_hook"`
}

type MCPServerConfig struct {
    Name              string                  `yaml:"name"`
    Enabled           *bool                   `yaml:"enabled"`            // 默认 true
    Transport         string                  `yaml:"transport"`          // 必须 "stdio"
    Command           string                  `yaml:"command"`
    Args              []string                `yaml:"args"`
    Env               map[string]string       `yaml:"env"`
    TimeoutMs         *int                    `yaml:"timeout_ms"`          // 默认 10000
    StartupTimeoutMs  *int                    `yaml:"startup_timeout_ms"`  // 默认 3000
    MaxResponseBytes  *int                    `yaml:"max_response_bytes"`  // 默认 4 MB
    ToolMetadata      map[string]ToolMetaYAML `yaml:"tool_metadata"`
}

type ToolMetaYAML struct {
    Kind             string `yaml:"kind"`                // "" / "log_source" / "data_fetch"
    LogSourcePrefix  string `yaml:"log_source_prefix"`
    WriteCapable     bool   `yaml:"write_capable"`
}

type PlanOutputHookConfig struct {
    Command   string            `yaml:"command"`    // 必填
    Args      []string          `yaml:"args"`
    Env       map[string]string `yaml:"env"`
    TimeoutMs *int              `yaml:"timeout_ms"` // 默认 30000
    Trigger   []string          `yaml:"trigger"`    // 默认 ["pending_approval"]
    OnFailure string            `yaml:"on_failure"` // "warn"（默认）/ "fail"
}
```

**`internal/types/context.go`**

```go
// BusContext 不加字段，但 AgentContext 需要：
type AgentContext struct {
    // 既有字段保持

    // 新增：
    MCPRegistry                *mcp.Registry // 窄视图：builder 要读 prompts/resources
    MCPMaxPromptBytesPerStage  int
    MCPWriteEnabled            bool
}
```

**`internal/context/builder.go`**

```go
// 既有 BuildAgentContext 签名不变，但内部填充新字段（若 bus.MCPHandle != nil）：
ac.MCPRegistry = bus.MCPHandle
ac.MCPMaxPromptBytesPerStage = bus.MCPMaxPromptBytesPerStage
ac.MCPWriteEnabled = bus.MCPWriteEnabled

// 新增函数：
func buildExternalGuidanceSection(ac *types.AgentContext) (string, []string)
func buildExternalResourcesSection(ac *types.AgentContext) string

// BuildPromptContext 内部（§7.4 位置）：
if ac.MCPRegistry != nil && !ac.Stage.IsWrite() {
    // 读模式：总是注入 prompts
    body, dropped := buildExternalGuidanceSection(ac)
    if body != "" {
        pc.SystemSections = append(pc.SystemSections, types.PromptSection{
            Title:   "External Guidance (from MCP servers)",
            Content: body,
        })
    }
    if len(dropped) > 0 {
        logging.Warning("[mcp] guidance truncated; dropped: %v", dropped)
    }

    if resSection := buildExternalResourcesSection(ac); resSection != "" {
        pc.SystemSections = append(pc.SystemSections, types.PromptSection{
            Title:   "External Resources (MCP)",
            Content: resSection,
        })
    }
}
// 写模式：§4.5.1 的 IsWrite 门同样对外部 MCP prompts 关闭
// —— 写模式的 prompt 是硬契约（apply 机械、verify 机械），外部 server
// 改不了它的语义；避免 prompt-injection 进写路径
```

**`internal/agent/agent.go`**

```go
// buildToolSchemas（§1395-1445）：
// 在现有 "append all MCP tools" 循环（1422）上加过滤：
for _, pair := range mcpRegistry.ListAllToolsWithServer() {
    toolName := pair.Server + "." + pair.Tool.Name
    meta := pair.Meta // MCPToolMetadata from StdioServer

    if meta.WriteCapable && !ac.MCPWriteEnabled {
        // 决策 #5：write_capable 默认不暴露，需 mcp_write_enabled: true
        continue
    }
    if meta.WriteCapable && !ac.Stage.IsWrite() {
        // 读模式永不暴露 write_capable（双层防）
        continue
    }
    schemas = append(schemas, llm.ToolSchema{
        Name:        toolName,
        Description: pair.Tool.Description,
        Parameters:  pair.Tool.Parameters,
    })
}
// 外部 skill（future）走 §4.5.1 的严格 allowlist 路径；本 PR 不实现

// executeTool（查找位置后）：
if strings.Contains(toolName, ".") && mcpRegistry != nil {
    // 是 <server>.<tool> 形式
    parts := strings.SplitN(toolName, ".", 2)
    if _, err := mcpRegistry.Get(parts[0]); err == nil {
        // 路由到 MCP
        resp, err := mcpRegistry.CallTool(parts[0], parts[1], params)
        return b.translateMCPResponse(resp, err, parts[0], parts[1])
    }
}
// 否则走原 tool.Registry 路径

// translateMCPResponse —— 新增：
func (b *BaseAgent) translateMCPResponse(resp types.MCPResponse, err error, server, tool string) (types.ToolResult, error) {
    if err != nil {
        return types.ToolResult{
            ToolName: server + "." + tool,
            Success:  false,
            Summary:  fmt.Sprintf("mcp: %s.%s failed: %v", server, tool, err),
        }, nil // 不 bubble error；让 LLM 决定是否重试
    }

    // 查 toolMeta.Kind = log_source → 路由到 TriageInline
    meta := b.mcpRegistry.GetToolMeta(server, tool)
    if meta.Kind == "log_source" {
        return b.routeMCPLogSource(resp, server, tool, meta)
    }

    // 普通：大输出走 blob offload（复用现有 tool.OffloadToBlob）
    summary := resp.Summary
    rawRef := ""
    if len(resp.Content) > blob.MaxInlineBytes() {
        rawRef, summary = blob.Offload(b.workDir, fmt.Sprintf("%s-%s", server, tool), resp.Content)
    }
    return types.ToolResult{
        ToolName: server + "." + tool,
        Summary:  summary,
        RawRef:   rawRef,
        Success:  resp.Success,
    }, nil
}

// routeMCPLogSource —— 新增：
func (b *BaseAgent) routeMCPLogSource(resp types.MCPResponse, server, tool string, meta mcp.MCPToolMetadata) (types.ToolResult, error) {
    // 预算检查
    bs := b.logSourceBudgetState // *logSourceBudget on BaseAgent
    if !bs.Reserve(len(resp.Content), b.cfg.MCPLogSourceMaxBytes, b.cfg.MCPLogSourceTotalBytesPerRun) {
        return types.ToolResult{
            ToolName: server + "." + tool,
            Success:  false,
            Summary:  "mcp log_source quota exhausted for this run",
        }, nil
    }

    // 调 TriageInline
    bundle, err := b.logTriagerHandle.TriageInline(b.busCtx.Mutable, resp.Content, meta.LogSourcePrefix)
    if err != nil {
        // 降级：blob 路径
        logging.Warning("[mcp] triage inline failed for %s.%s: %v; falling back to blob", server, tool, err)
        return b.translateMCPResponseFallback(resp, server, tool)
    }
    // 合并 / 写入
    if existing := b.busCtx.Mutable.LogTriage(); existing != nil {
        b.busCtx.Mutable.SetLogTriage(logtriage.MergeBundles(existing, bundle))
    } else {
        b.busCtx.Mutable.SetLogTriage(bundle)
    }
    return types.ToolResult{
        ToolName: server + "." + tool,
        Success:  true,
        Summary:  mcp.RenderLogBundleSummary(bundle),
    }, nil
}
```

**`internal/agent/log_triager.go`**

```go
// 新增公开方法 TriageInline：
// 现有 Execute() 走 preStage 路径，依赖 bus.AttachedLog。
// TriageInline 拆分核心逻辑让运行中可重用。
//
// 签名：
func (e *logTriagerEvaluator) TriageInline(
    mut *types.MutableState,
    rawText string,
    sourcePrefix string,
) (*types.LogBundle, error) {
    // 1. 两步判断（size > log_triage_two_step_bytes → segmentation）
    // 2. 单步：组一个 fake AgentContext，skill = log-triage-skill，发 emit_log_triage LLM 请求
    //    反解响应 → LogBundle
    // 3. 两步：log-segmentation-skill → emit_log_segmentation → 逐段 emit_log_triage → MergeBundles
    // 4. ValidateBundle（路径校验 + Layer 4）
    // 5. 返 *LogBundle
}
```

**`cmd/root.go`**

```go
// 位置：现在 mcpRegistry := mcp.NewRegistry() 那一行之后（§1345）
if rs != nil && len(rs.MCPServers) > 0 {
    maxServers := getOrDefault(rs.MCPMaxServers, 8)
    if len(rs.MCPServers) > maxServers {
        return fmt.Errorf("mcp_servers count %d exceeds mcp_max_servers %d", len(rs.MCPServers), maxServers)
    }
    if err := mcp.LoadServers(mcpRegistry, rs.MCPServers, maxServers); err != nil {
        return fmt.Errorf("load mcp servers: %w", err)
    }
}
defer mcpRegistry.Close()

// 位置：writePlanFile() 成功之后（§1567 附近）
if rs != nil && rs.PlanOutputHook != nil {
    if err := tool.RunPlanOutputHook(rs.PlanOutputHook, planJSON, "pending_approval"); err != nil {
        logging.Warning("[plan_hook] error: %v", err)
    }
}

// 位置：orchestrator persistPlanStatus 内部（非 cmd/root.go），在 applied / verify_failed 触发时同理调 RunPlanOutputHook
```

### 15.3 类型依赖图

```
types.MCPResponse   →  （既有，扩）
types.LogBundle     →  （既有）

mcp.ToolSchema           （既有，扩）
mcp.PromptSchema         （新）
mcp.PromptResult         （新）
mcp.ResourceSchema       （新）
mcp.ResourceContents     （新）
mcp.MCPToolMetadata      （新）
mcp.MCPServerConfig      （新）

config.RuntimeSettings   扩字段：MCPServers / MCPMaxServers / ...
config.PlanOutputHookConfig  新

types.AgentContext  扩字段：MCPRegistry / MCPMaxPromptBytesPerStage / MCPWriteEnabled

agent.BaseAgent deps 扩：mcpRegistry / logTriagerHandle / logSourceBudgetState
```

### 15.4 测试新增（汇总表）

| 文件 | 测试函数 |
|---|---|
| `mcp/stdio_test.go` | TestStdioServer_JSONRPCRoundTrip / _InitializeHandshake / _TimeoutReturnsFailure / _LargeResponseExceedsCap |
| `mcp/loader_test.go` | TestLoadServers_YAMLRoundTrip / _DupNameRejected / _AllOrNothingOnStartupFailure |
| `mcp/render_test.go` | TestRenderExternalGuidance_AlphaOrder / _BytesCapTruncatesTail / _DroppedListCorrect / TestRenderExternalResources_OnlyWhenNonEmpty |
| `mcp/prompts_test.go` | TestPromptsListGet_JSONRPCWire |
| `mcp/resources_test.go` | TestResources_RejectNonResourceScheme |
| `agent/agent_test.go` | TestMCPWriteCapable_HiddenWhenWriteEnabledFalse / TestMCPWriteCapable_HiddenInReadModeRegardless / TestMCPCall_SummaryFormat |
| `agent/log_triage_test.go` | TestTriageInline_BasicBundle / _MergesWithExisting / _TwoStepFallbackLargeLog |
| `agent/mcp_log_source_test.go` | TestMCPLogSource_RoutesToTriage / _QuotaExhausted / _TriagerFailureFallsBackToBlob |
| `tool/plan_hook_test.go` | TestPlanHook_StdinCarriesJSON / _TimeoutKillsProcess / _OnFailureWarn / _OnFailureFail |
| `mcp/red_lines_test.go` | TestMCP_NoGroundingImport (go/ast L-MCP-1) |
| `mcp/red_lines_test.go` | TestMCPResource_RejectsFileScheme (L-MCP-2) |
| eval | `eval/fixtures/fake_mcp_server/` + `eval/cases/mcp_log_triage.case` + `eval/cases/plan_hook.case` |

### 15.5 开工前 checklist（复查）

- [ ] 确认 `internal/tool/blob` 有 `OffloadToBlob(workDir, name, content)` 函数 —— `routeMCPLogSource` 降级路径依赖它，要先 grep 确认是否存在 / 签名匹配
- [ ] 确认 `logtriage.MergeBundles(a, b *LogBundle) *LogBundle` 存在且对 nil / 空 bundle 稳健
- [ ] 确认 `MutableState` 有 `SetLogTriage(b *LogBundle)` setter —— 若无要加
- [ ] 确认 `BaseAgent.Deps` 能塞入 `logTriagerHandle` 和 `logSourceBudgetState` —— 避免循环依赖（log_triager 现在是 agent 实例，引用要注意）
- [ ] `mcp_read_resource` 伪工具注册时机：在 `tool.Registry` 构建后（cmd/root.go 对应位置）插入，仅当 `mcpRegistry.ListAllResources() != nil`

---

**Ready for review。** 决策锁定，代码面展开完成，下一 session 按 §13 阶段 A 开工。
