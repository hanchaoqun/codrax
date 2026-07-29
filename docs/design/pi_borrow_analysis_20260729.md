# pi (earendil-works/pi) 借鉴分析与落地批次账本

- 日期：2026-07-29
- 对象：https://github.com/earendil-works/pi （Mario Zechner / earendil-works，TypeScript monorepo，调研时 v0.82.1，~125k LOC）
- 方法：全仓克隆 + 四路并行深读（agent 循环与 LLM 层 / coding-agent 工具与会话 / TUI 与交互 UX / 工程实践与生态），逐条与 codrax 现状对照。
- 本文档兼作落地账本：§6 批次排期表随每批落地更新状态。

## 1. pi 项目画像

四个核心包，层次自下而上：

| 包 | 职责 |
|----|------|
| `packages/ai` | 40+ provider 统一 LLM API：生成式模型目录（models.dev + 各家 /models 端点 → JSON + 编译期字面量类型）、compat 开关矩阵（30+ 布尔把 provider 方言下沉为模型元数据）、统一 thinking 等级映射、prompt cache 策略、重试/溢出分类、OAuth |
| `packages/agent` | 三层递进：`agentLoop`（无状态纯函数）→ `Agent`（有状态 + steering/follow-up 队列）→ `AgentHarness`（树状会话持久化 + 可返回值 hook 总线） |
| `packages/coding-agent` | 7 个内置工具（read/bash/edit/write/grep/find/ls）+ 四运行形态（interactive TUI / print / RPC / SDK）+ 扩展系统（26 种事件 hook）+ skills/prompt templates + compaction |
| `packages/tui` | 自研差分渲染 TUI：行数组组件模型、同步输出、kitty 键盘协议探测、粘贴折叠编辑器、终端图片 |

哲学：**"primitives, not features"** —— core 刻意极小（不内置 MCP/子 agent/plan mode/权限弹窗），一切能力下放给运行时加载的 TypeScript 扩展（jiti 免编译 + `/reload` 热加载）；agent 可给自己写扩展（系统 prompt 写死自身 docs/examples 路径 + 文档首行祈使句 + eval 固化验证该闭环）。

## 2. 哲学对比与总体裁定

codrax 与 pi 是两个极端：codrax 的价值在**确定性硬门禁、typed 证据链、可审计裁定账本**；pi 的价值在**极简可延展**。总体裁定：

- **不照搬哲学**：不引入"万物皆扩展"，不放松 L1-L8 红线与确定性风险门。
- **平移工程结晶**：pi 在长循环交互体验、会话工程、工具面容错、错误通道纪律上的机制与哲学无关，可直接借鉴。
- 借鉴优先级以"对 codrax 真实痛点的命中度"排序（§6），不按 pi 侧的实现精巧度排序。

## 3. 各维度详析（含 pi 侧文件锚点）

### 3.1 UX——交互输入侧

1. **两级消息排队（steering / follow-up）**：agent 流式运行时编辑器不锁。Enter 入 steering 队列（当前轮工具执行完即注入）、alt+enter 入 follow-up（整个 run 结束后注入）、alt+up 一键取回全部排队消息、esc 中止时队列原样还原进编辑器（`interactive-mode.ts:3728-4007`）。队列区渲染为 dim 单行 `Steering: ...` + 提示。
   - codrax 映射：read pipeline 一跑数分钟，REPL 期间无法补充线索；steering 注入点天然=阶段边界（analyze→explore 之间补锚点文件语义明确）；写模式 controller 循环同理。
2. **粘贴折叠为原子标记**：>10 行或 >1000 字符的 bracketed paste 显示为 `[paste #1 +123 lines]`，标记是原子字素（光标/删除/词导航当单字符），删除后重编号，提交时还原（`editor.ts:32-88, 1156-1315`）。codrax 的 `/log` 粘贴（`/end` 结束）与 `/paste` 是显式命令式，pi 这套零命令自动识别。
3. **@ 文件模糊引用**：`@` 触发 fd 全仓模糊搜（文件名精确 100/前缀 80/含 50/全路径 30，目录 +10），带引号路径处理、目录补全不加空格方便下钻（`autocomplete.ts`）。codrax 映射：@ 选中文件 seed 进 analyzer RequiredFiles，把日志附件锚点能力泛化到任意文件。
4. **prompt 模板**：`.pi/prompts/*.md` 文件名即 `/命令`，frontmatter 带 `description`/`argument-hint`，参数替换 bash 风格 `$1`/`$@`/`${@:-default}`/`${@:2}`（`prompt-templates.ts`，单条正则实现）。codrax 映射：`.codrax/prompts/` 让用户把高频审计话术固化成斜杠命令。
5. 其它：`!cmd` bash 直通且编辑器边框实时变色提示 bash 模式、`!!` 不入上下文；ctrl+g 外部编辑器；`\`+Enter 换行兜底（不支持 Shift+Enter 的终端）；上/下键首末行双语义（有历史翻历史，否则跳行首尾）。

### 3.2 UX——输出/状态侧

1. **重试可视化**：`Retrying (2/5) in 7s... (esc to cancel)` 每秒改写倒计时；重试期间临时借走 esc 绑定给 abortRetry、结束归还（语义栈式）；只有最终失败才报错，中途成功零噪音（`status-indicator.ts:42-72`、`interactive-mode.ts:3070-3149`）。
   - codrax 映射：直接回应死等会话事故史（客户卡 14m35s；W1 流式断路/W2 心跳已落地，缺的是用户面"正在退避、还剩几秒、esc 可放弃"）。
2. **上下文溢出车道**：LLM 报溢出 → 剔除错误消息 → 自动压缩 → 自动重试该轮；**只允许一次恢复**，二次失败给可执行建议（"换大窗口模型"）（`agent-session.ts:1979-2010`）。
3. **Footer 实时账**：`↑in ↓out R缓存读 W缓存写 命中率% $成本 上下文%/窗口`，>70% 警告色、>90% 错误色；宽度不足先隐藏 provider 前缀再截断（`footer.ts:148-160`）。
4. **工具输出折叠纪律**：默认折叠，各工具自报折叠上限（read 10 行/grep 15/bash 尾 5），按视觉行（含折行）取尾部，提示语嵌实时 keybinding 渲染的键名；ctrl+o 全局展开；启动头折叠成一行帮助（`visual-truncate.ts` + 各工具 renderResult）。
5. **esc/ctrl+c 分层语义**：esc 依次=关补全→中止流式（还原队列）→中止 bash→退 bash 模式→空编辑器 500ms 双击开会话树；ctrl+c 第一次清空、500ms 内二次退出；ctrl+d 仅空编辑器退出；双击 500ms 全局统一。
6. **状态行原地改写**：新状态紧跟上一条状态行则覆写不追加；清除按 kind 匹配防竞态误清（`interactive-mode.ts:3190-3213`）。
7. 低于阈值不打扰：缓存未命中提示仅当 ≥20k tokens 或 ≥$0.1 才显示。

### 3.3 会话管理

1. **树状会话**：单 JSONL 文件内 entry 带 8 位 id/parentId + leaf 指针，分支不开新文件；`/tree` 跳任意 user 消息时把消息文本回填编辑器、叶移到其 parent（=编辑重发）；切走的分支自动生成 branch summary（`session-manager.ts`、`agent-harness.ts:791-882`）。11 种 entry 类型；`custom`（扩展状态，不入上下文）与 `custom_message`（入上下文）分离。
2. **compaction 五细节**（`compaction.ts` + `docs/compaction.md`）：
   - 切点绝不落在 toolResult 上；单轮超预算 split-turn 双摘要合并；
   - compaction entry 自带 `retainedTail` 自包含 checkpoint，上下文重建到此截断；
   - 待摘要内容序列化成 `[User]:/[Assistant]:` 标签文本防模型当对话续写，tool result 截 2000 字符；
   - 摘要请求用全新 session id + `cacheRetention:"none"`（辅助调用不污染主对话 prompt cache）；
   - 混合 token 计量：以最后一条 provider 真实 usage 为锚，只对其后增量做字符估算。
   - 结构化摘要模板（Goal/Constraints/Progress/Key Decisions/Next Steps + read-files/modified-files 跨压缩累积）+ 增量更新 prompt（带旧摘要、In Progress 迁 Done）。
3. **会话导出/导入复现闭环**：CI 用 pi 分析自己的 issue → 会话导出 gist → 评论给一行 `pi "/ir <gistId>"` → 任何人一键导入 CI 会话本地继续追问（`.github/workflows/issue-analysis.yml`、`.pi/extensions/import-repro.ts`）。会话选择器搜索支持 `re:` 正则与引号短语 token。

### 3.4 工具层设计（LLM 面）

1. **截断提示即下一步指令**：read 截断附 `[Showing lines A-B of N. Use offset=B+1 to continue.]`；grep 附 `Use limit=200 for more`；bash 超限自动落盘回 `Full output: <tmpfile>`；单行超 50KB 给可执行 `sed -n 'Np' | head -c` 命令（`tools/read.ts:293-303` 等）。原则：模型永远知道怎么拿剩下的部分。
2. **双上限（行+字节）+ 永不半行**：`DEFAULT_MAX_LINES=2000`、`DEFAULT_MAX_BYTES=50KB` 先命中者胜，统一 `truncate.ts` 被 7 工具复用；`TruncationResult` 携带 `truncatedBy/totalLines/totalBytes/...` 供 UI 与模型两端复用；bash 用 `truncateTail`（日志错误在尾部）。
3. **OutputAccumulator**：滚动尾部缓冲（maxBytes*2）+ 增量 UTF-8 decoder，内存有界；超限落盘 temp file 流式写。
4. **`prepareArguments` 畸形参数容错层**：已知模型坏习惯（edits 传 JSON 字符串、旧字段平铺）进 schema 校验前修正（`edit.ts:94-118`）。
5. **`stopReason=length` 整批工具拒执行**：截断参数可能能解析但语义残缺（`agent-loop.ts:381-406`）——fail-loud 精确信号。
6. **工具自带 prompt 片段**：`promptSnippet`/`promptGuidelines`，系统 prompt 随激活工具集自动拼装去重（`system-prompt.ts:82-113`）。
7. 杂项：macOS 截图路径 4 变体回退（U+202F/NFD/U+2019/组合）；文件级 realpath 串行队列（abort 不在事件回调里 reject 防提前释放锁）；edit 的 BOM/CRLF 规范化三明治 + 模糊匹配只覆盖变更行 + overlap 显式检测。

### 3.5 LLM 层

1. **重试三分类**：不可重试（配额/账单，速败）/ 可重试（429/5xx/网络，指数退避）/ 溢出（不重试，改走压缩续跑）。每条重试正则带出处注释与 issue 号（`utils/retry.ts`、`utils/overflow.ts`）。
2. **传输/策略两层退避分离**：传输层贴 SDK 语义（`x-should-retry`/`retry-after`），`maxRetryDelayMs=60s`，服务端要求更久时**上抛给上层做可视化/可取消**而非闷头 sleep。
3. **错误只走数据通道**：`StreamFn` 契约禁止 reject，失败编码成 `stopReason:"error"` 最终消息并补齐完整事件序列（message_end→turn_end→agent_end）；abort 归一化 `stopReason:"aborted"`；`lazyStream` 把 setup 失败也转成流内事件——消费端永不处理事件缺口。
4. **重试双账**：重发前把 error assistant 消息从内存上下文摘除、会话历史保留（`agent-session.ts:2698-2701`）——上下文干净 + 审计可回放并存。
5. **`agent_end` vs `agent_settled`**：前者=一次底层 run 结束（可能还有重试/压缩/队列续跑，带 `willRetry`），后者=真正安静。嵌入方只能信 settled。
6. **save point 写入模型**：运行期配置变更入 `pendingSessionWrites`，只在 turn_end flush——会话文件永远停在一致轮次边界。
7. **并行工具双序**：prepare 严格顺序（权限询问顺序稳定）、execute 并发、`tool_execution_end` 按完成序（UI 实时）、tool-result 消息按源序（transcript 确定）。
8. **配置值解析**：`!cmd`（请求时执行 shell 取密钥，可接 keychain/vault，故意不缓存并写明理由）、`$VAR`/`${VAR}` 插值、`$$`/`$!` 转义（`resolve-config-value.ts`）。
9. thinking 统一等级枚举 + 每模型 levelMap（null=不支持）+ 就近降级 clamp；compat 开关矩阵是"一份 API 实现服务 N 个伪兼容端点"的关键。

### 3.6 运行形态

四形态共享同一 `AgentSession`。RPC 契约三纪律（`docs/rpc.md`）：只按 LF 分帧（点名 Node readline 会错切 U+2028）；response 只表达"已接受"，接受后失败走事件流、不对同一 id 发第二个 response；`partialResult` 是累计值非增量。SDK 层 `AgentSessionRuntime` 拥有会话替换（newSession/switch/fork），契约明说替换后必须重新 subscribe。

### 3.7 写码工作流与写安全（扩展生态深读；对 codrax 写模式的专项对照）

pi core 无写模式概念（永远可写，安全靠容器化），但其扩展生态把"写工作流的确定性控制"拆成了一组可单独借鉴的机制。逐个读了 `examples/extensions/` 下全部写相关扩展后的机制速查：

| # | 机制 | 出处（pi） | 对 codrax 写模式的价值 |
|---|------|-----------|------------------------|
| 1 | **无 UI 即 fail-closed**：危险动作需确认但无确认通道时默认拒绝 | `permission-gate.ts:20-22`、`dirty-repo-guard.ts:28-31` | Auto Pilot 无人值守（非 TTY/一次性 CLI）时"高风险 + 无审批通道"的默认行为必须是拒绝——需自查 codrax 现状是否处处如此 |
| 2 | **黑名单 ∧ 白名单双判 + 默认拒绝**：34 条破坏性正则 + 51 条行首锚定只读白名单，两判都过才放行 | `plan-mode/utils.ts:97-101` | 比单一黑名单可靠；白名单行首锚定防管道绕过；可对照 codrax exec_command 语义验证（§29.213）查缺 |
| 3 | **`git stash create` 零副作用快照**：每轮打悬空 commit ref（不动工作区/不入 stash 栈），回退时 `stash apply` | `git-checkpoint.ts:21-27` | worktree 内每个 DAG 动作前打点，verify 失败/replan 时精确回退到动作边界，比整树重建轻 |
| 4 | **工具面收紧 + 精确还原**：plan 阶段剔除 edit/write，退出时还原"进入前的工具集"而非硬编码默认集 | `plan-mode/index.ts:104-114` | codrax L6 已按 skill 分工具面（planner 无 exec_command）；"精确还原"语义可对照 |
| 5 | **阶段上下文注入 + 退出后清洗**：阶段指令用 display:false 消息注入，退出后从 context 过滤陈旧阶段指令 | `plan-mode/index.ts:177-247` | 防阶段指令残留污染后续动作；对照 codrax 控制器多阶段 prompt 组装 |
| 6 | **三段式审批：执行 / 保持 / 人工修订**：高风险暂停不止 yes/no，第三条 Refine 打开编辑器收人工修订直接接回 replan | `plan-mode/index.ts:301-336` | codrax `/approve` `/reject` 是二元的；**"修订后重规划"通路是真实 gap**——用户对计划不满时目前只能 reject 重来 |
| 7 | **冲突/失败结构化回灌**：merge 冲突扫成 `file:startLine-endLine (ours a-b, theirs c-d)` 清单作为 followUp 喂回 agent 自行解决 | `git-merge-and-resolve.ts:62-113` | verify 失败/apply 冲突时给 replan 的输入应是结构化定位清单而非裸错误文本 |
| 8 | **同名覆盖内置工具 + pluggable operations**：read/bash/edit/write 的执行后端可整体替换（沙箱/VM/SSH），比 hook 拦截彻底 | `sandbox/index.ts:214-227`、`gondolin/index.ts` | codrax worktree 已是执行域隔离；operations 层启示是"工具实现与执行后端解耦"的架构方向 |
| 9 | **审计日志行格式**：`[ts] ALLOWED\|BLOCKED: <absPath> (reason)` + 文件级串行队列写入 | `tool-override.ts:47-60` | 风险门每次放行/拒绝落审计行，可直接作为写模式决策日志的展示格式 |
| 10 | **进程级子 agent + 逐角色限权 + chain 失败即停**：scout(只读)/planner(只读)/worker(全能)/reviewer(只读+bash)，`--tools` 逐角色限定 | `subagent/index.ts:305-477` | codrax write 侧 agent 族（planner/coder/verifier/critics）已同构；chain 失败即停并报告哪一步的语义可对照 |
| 11 | **repo 内配置默认不信任**：项目内 agent/规则文件默认不加载，显式确认才用（提示注入面） | `subagent/index.ts:505-530` | codrax 若引入项目级 prompts/skills（PIB-7）必须继承此原则 |
| 12 | **提示层与执行层双保险**："Assume tool permissions are not perfectly enforceable" 写进只读角色 prompt | `subagent/agents/reviewer.md:10-11` | 风险门是主防线、prompt 是补充——与 codrax 精确/嘈声信号原则同构 |
| 13 | **超时确认兜底**：确认框带倒计时，超时默认取消 | `timed-confirm.ts:14-22` | 无人值守下高风险暂停的超时默认动作（默认拒绝）范式 |
| 14 | **edit 三重防错**：oldText 唯一性（报出现次数）/ 区间非重叠 / 非空操作，全部对原始文件匹配、逆序应用 | `edit-diff.ts:268-351` | apply 动作的确定性前置校验；失败信息可直接驱动 replan——对照 codrax coder 的编辑通道防错完备度 |
| 15 | **状态存会话 entry 而非外部文件**：plan/todo 状态随会话分叉自动对应历史点 | `todo.ts` 头注释、`plan-mode` appendEntry | codrax durable workflow 状态已持久化；"随分叉正确"语义在引入会话 fork（PIB-4/会话树）时需对照 |
| 16 | **写收尾硬约束**：禁 `git add .`/`-A`、只提交本会话文件、非 main 分支停下来问、多 issue 绑定停下来问 | `.pi/prompts/wr.md` | 可直接作为 codrax apply/commit 节点的确定性规则与 CLAUDE.md 制度化条目 |
| 17 | **只读审查零副作用**：review 不 checkout 分支，只用 `gh pr diff` + `git show <ref>:<path>` | `.pi/prompts/pr.md` | verify/review 节点的零工作区副作用范式 |

反面参考（不照抄）：① `protected-paths.ts` 用未 resolve 的原始路径做 `includes` 匹配，可被相对路径/软链绕过——路径判定必须 realpath 前缀；② 示例 `auto-commit-on-exit.ts` 用 `git add -A`，与 pi 自己 `wr.md` 的硬约束相矛盾（演示件）。
另注：`tool_call` hook 的 `event.input` 可原地 mutate 但**不做 re-validation**（`docs/extensions.md:751-790`）——凡引入"门内改参"能力必须自带二次校验。

### 3.8 工程实践

1. **REPL 渲染可测试性**：`@xterm/headless` 做 `VirtualTerminal` 记录全部转义序列，断言差分渲染/重绘行为（`tui/test/virtual-terminal.ts`）；render 行宽超限直接抛错 + dump 崩溃日志（用崩溃换永不错位）。
2. **测试隔离**：`test.sh` 用 `env -i` 空环境白名单注入（无 API key = LLM 测试自动 skip）；`PI_OFFLINE=1` 默认断网、需网络显式 `allowNetwork()`；faux provider 代替 record/replay；回归测试 `<issue号>-<slug>.test.ts` 收 `regressions/`。
3. **发布事务**：draft release → npm 发布 → 成功才 publish，任一环节失败自动删草稿；文档内链发布时改写成 tag 冻结链接；Actions 全 pin commit SHA；依赖精确 pin + lockfile 提交门 + install-script allowlist（失效条目也报错）；`min-release-age=2` 避开当日新发布依赖。
4. **AGENTS.md git 纪律**：前提"同 cwd 并发多 agent 会话"，只准 `git add <显式路径>`，明令禁止 `git reset --hard`/`git checkout .`/`git clean -fd`/`git stash`/`git add -A`。（codrax memory 中的散条教训值得同样制度化进 CLAUDE.md。）
5. **文档自解释**：docs 全量打进二进制，系统 prompt 写死自身 docs/examples 路径，文档首行祈使句；离线可"问 agent 自己"。
6. eval 体系：与单测共用 vitest 基座；provider+model 必须成对显式给出（绝不隐式换模型）；"agent 写扩展→reload→用扩展"固化为可回归行为测试；eval 不进 CI（真实付费模型，人工触发）。

### 3.9 工作流 / DAG 调度对照（读模式 pipeline · 写模式 DAG vs pi 单循环+扩展拼装）

范式差异先说清：**pi 没有 pipeline 也没有 DAG 引擎**。它的调度=单 agent 双层循环（`agent-loop.ts`：外层只管 follow-up 续跑，内层管 assistant→工具→steering 注入），一切"工作流"都是扩展在这个循环上拼出来的（plan-mode 两阶段状态机、subagent 的 scout→planner→worker→reviewer 链）。codrax 相反：读=代码即拓扑的固定 4 阶段，写=controller 驱动的 typed 动作 DAG + 持久化恢复。**裁定：范式不动摇**——codrax 的确定性拓扑/typed 边/风险门/durable run 整体领先 pi 一个层级（pi 连 verify 失败回灌都要靠 `git-merge-and-resolve` 扩展手搓散文清单）。可借的是循环内的**调度语义细节**：

| # | pi 机制 | codrax 对照与裁定 |
|---|---------|-------------------|
| 1 | **轮次边界原子生效**：运行中 setModel/setTools/appendMessage 全部入 `pendingSessionWrites` 队列，只在 `turn_end` 统一 flush 并发 `save_point` 事件——会话文件永远停在一致轮次边界（`agent-harness.ts:512-556`） | codrax 配置变更目前是 run 级静态；若未来支持运行中变更（如 REPL 中途切模型），必须按此范式在阶段边界原子生效。记为设计约束，非本期开发 |
| 2 | **steering 注入点**＝当前轮工具全部执行完、下一次 LLM 请求前；follow-up 注入点＝agent 本会停止时；`agentLoopContinue` 显式拒绝从 assistant 消息续跑 | PIB-2 的注入点设计直接输入：codrax 读管线的等价点=阶段边界（analyze→explore 间收锚点最有语义）；写模式=controller 决策点之间 |
| 3 | **并行工具双序**：prepare 严格顺序（审批顺序稳定）→ 执行并发 → `tool_execution_end` 按完成序（UI 实时）→ tool-result 消息按源序（transcript 确定）（`agent-loop.ts:489-554`） | codrax explore fan-out 已有 ParallelGroupID/UnitID 事件面；"UI 按完成序、账本按源序"的解耦原则值得对照自查一遍 |
| 4 | **整批终止语义**：工具批提前停止要求**每个**结果都 `terminate:true`；`stopReason=length` 时整批工具拒绝执行 | 后者已排 PIB-3（截断参数可解析但语义残缺=精确信号硬门）；前者与 codrax 完成门权属模型一致 |
| 5 | **`agent_end` vs `agent_settled` 两级完成信号**：前者=一次底层 run 结束（带 `willRetry`，后面可能还有重试/压缩/队列续跑），后者=真正安静；嵌入方只能信 settled | codrax auto-resume 的写工作流状态机若做事件外露（RPC/JSON 模式），这个区分应写进第一版契约 |
| 6 | **子 agent 链**：`chain` 模式 `{previous}` 占位符串联，失败即停并报告**哪一步**；每 task 模型可见输出 cap 50KB、全量留 `details` | codrax 写 DAG typed 边+ProgressLedger 已覆盖"哪一步失败"；输出治理与 explore 收口 typed 载体等价。无需借 |
| 7 | **阶段状态随会话正确分叉**：plan-mode 进度 `[DONE:n]` 从会话 entry 重放重建、todo 状态存 tool-result details——fork 到历史点状态自动对应 | codrax durable run 状态在独立 JSON、不随会话分叉；引入会话 fork（PIB-4 之后）时这是必须补的一致性语义 |
| 8 | **工作流即 prompt SOP**：pi 自己的发布/审查/收尾流程全是 `.pi/prompts/*.md` 硬约束清单（wr/pr/is/cl/sa），零引擎代码 | 轻量多步流程不必上 DAG——PIB-7 的 `.codrax/prompts/` 模板命令天然可承载这类 SOP（审计话术、回访清单） |

### 3.10 prompt 语料写法模式（原文级调研）

pi 的 prompt 面貌：基础系统 prompt 仅 ~18 行骨架（一句角色定位、工具一行清单、动态 guidelines、自身文档路径、cwd），**无任何 NEVER/MUST 红线段**——硬约束全部下沉到工具自述、AGENTS.md 与 SOP prompt。从全部语料（系统 prompt / 7 工具自述 / compaction 三套摘要 prompt / 4 角色 prompt / plan-mode 注入 / wr·pr·is 三 SOP / skill / AGENTS.md）提炼的可迁移模式，按对 codrax 的适配度排序：

1. **"停下来问"写成可判定谓词**：`If it is tied to multiple issues, stop and ask which one to use.` / `Check the current git branch. If it is not main, stop and ask.` —— 全部是 `If <可机检条件>, stop and ask <具体问什么>`，不是态度词。与 codrax 精确信号原则同构，可直接用于 write 控制器与 SOP prompt。
2. **否定式必须配替代路径**：`Never modify models.generated.ts directly; update generate-models.ts instead, then regenerate.` 光禁止会让模型卡死或绕路——对照 codrax 各 skill prompt 的禁令是否都带出路。
3. **禁令附因果理由（规则升级为世界模型）**：`Never run the full vitest suite directly: it includes e2e tests that activate when env vars are present.`；git 纪律段先讲"同 cwd 可能并发多会话、越界操作会踩别人"再列规则。
4. **声明下游读者，倒逼自足输出**：compaction 摘要 prompt 开头 `another LLM will use to continue the work`；scout 角色 `Your output will be passed to an agent who has NOT seen the files you explored.`；planner 收尾 `The worker agent will execute it verbatim.` —— codrax Turn A→B Summary handoff 与各 agent 收口 prompt 可对照补这句。
5. **输出模板即 schema + 空段落逃生口**：`Use this EXACT format:` 后给带 `[占位符即写作指令]` 的骨架；每个可空段预置 `(none)` / `Omit the section entirely` / `say so under Bad` 三种空值策略——堵死"为填格式而编造"（与拒渲绝不造数同精神，prompt 侧版本）。
6. **压缩与保真配对成句**：`Keep each section concise. Preserve exact file paths, function names, and error messages.` 一句给上界+下界，防"简洁"被理解成可丢标识符。
7. **防越权双保险 + 明示白名单不可靠**：planner 工具白名单之外正文再写 `You must NOT make any changes`；reviewer 更进一步 `Assume tool permissions are not perfectly enforceable; keep all bash usage strictly read-only.` —— 与 codrax L6"guardrail 非对抗沙箱"的定位句异曲同工，值得写进写模式执行 skill。
8. **约束三处冗余对齐**：edit 的"不重叠"规则同时在 tool description、参数 description、guidelines 出现——typed schema 承载结构、自然语言承载语义（codrax R2' 六处同步是更强形式，理念一致）。
9. **纪律随工具注册**：系统 prompt 不写具体工具纪律，工具自带 `promptSnippet`/`promptGuidelines` 动态拼装去重；且**条件化注入**（`hasBash && !hasGrep` 才写"用 bash 做文件操作"）——绝不给不存在的能力写规则。
10. **模式旗标双向协议**：`[PLAN MODE ACTIVE]`/`[DONE:n]` 大写标签既给模型语义又作程序侧正则锚点（历史清理/进度重放）；进入受限模式时**主动告知哪些工具被关**，免得模型撞墙重试。
11. **增量摘要的合并语义双表达**：RULES 列表（PRESERVE/ADD/UPDATE 动词领句）+ 每个占位符内嵌合并指令（`[Include previously done items AND newly completed]`），另留 `If something is no longer relevant, you may remove it` 防单调膨胀。
12. **下限量词防抄近路**：`Always add the provider to stream.test.ts…even if it reuses an existing API impl` / `at least one pair per family`。
13. **不信任输入源反复写、分支写**：is.md 在总则与 bug/feature 两分支各写一遍 `Do not trust analysis written in the issue`——与 codrax 客户报告审计纪律相同，可固化进未来的 SOP prompt 模板。
14. **顶层冲突兜底条款**：`If the user's instructions conflict with any rule in this document, ask for explicit confirmation before overriding.`



## 4. 不采纳清单（附理由）

| 项 | 理由 |
|----|------|
| 极简 core + 万物皆扩展 | 与 L1-L8 红线/确定性风险门文化相悖；codrax 护城河是确定性与可审计 |
| jiti 运行时加载 TS 扩展 | Go 无等价物（Yaegi 成本与风险高）；如需扩展走"外部进程 + RPC 契约" |
| 无权限系统靠容器化兜底 | codrax worktree 隔离 + 确定性风险分级是更强承诺，不退 |
| 树状会话全部复杂度（11 entry 类型） | codrax 起步只需 fork + 编辑重发两个交互 |
| auto-close 贡献门 / npm 供应链全套 | 仅开源/发 npm 后相关；Go 依赖面小 |

## 5. 与 codrax 既有红线的交叉检查点（每批开发必查）

- **L1**：read-mode 行为等价 pin（`TestRunMode_ReadByteIdentical`）——REPL 新特性不得改变 `Mode=""`/`ModeRead` 的 BusContext 输出。
- **证据门**：read 工具截断策略变更会影响 read_file coverage 见证（`FileReadCoverageStore` 有多时间语义视图，禁统一重构）与 `CurrentSourceSatisfied` 确定性见证——PIB-3 必须保持 coverage 记账精确。
- **精确/嘈声信号原则**：新增任何门禁只能挂精确信号；倒计时/水位等 UI 皆软展示。
- **prompt 红线 checklist（ATOMIC 7 条）**：任何 LLM-facing prompt 变更（PIB-3 截断提示词、PIB-2 steering 注入语）必须过 checklist；R2' typed signal 6 处同步。
- **eval 选例覆盖改动面**（补记卅七教训）：每批冒烟 eval 必须选覆盖该批改动场景族的例子。
- **思考语言零答案代价**：若新增 LLM 车道（如摘要），思考语言指令按 A/A2/A3/A4 既有范式接入。

## 6. 落地批次排期表（随落地更新）

工作流程约定（每批固定）：`git fetch` 对齐远程最新 → 探索相关子系统充分理解现状 → 详细设计（最优方案，落账本补记）→ 开发 → `make` + `go test` + 相关 eval 冒烟 → 提交 → rebase（冲突响亮停）→ push + 双零检查。

| 批次 | 范围 | 涉及 codrax 面 | 状态 |
|------|------|----------------|------|
| PIB-1 | 重试/溢出用户面：REPL 重试倒计时 + esc 取消 + 退避可视化 + 最终失败才报错；非 TTY 车道日志化 | LLM 客户端重试分层、REPL 状态卡、W1/W2 既有断路/心跳的 UX 面 | **已落地**（2026-07-29，见 §7 补记一） |
| PIB-W | 写模式借鉴批（用户 2026-07-29 点名写模式 gap，优先级提升）。**2026-07-29 现状探索后收窄**（全文见 §7 补记二）：六候选中三项撤销（verify 结构化回灌=VerifyFailureHandoff 已完备；无 UI fail-closed=已正确返回错误不挂起；动作检查点=apply commit 快照+best-known-good 热回退+slice typed checkpoint 三层已在役）。保留四件：**W-1 三段式审批**（主件：`/reject` 现在把 batch 打成 Blocked 死路，拒绝意见零回流——新增修订意见 typed 载体→batch 回 ReadyToPlan→replan 消费，对标 pi plan-mode Refine 臂）；**W-2 审批决策 append-only 账本**（现 `plan.Approval` 单槽覆盖，跨 replan 只留最后一次；`/workflow show` 不展示 `Reasons[]`）；W-3 apply 中途失败（partially_applied）的检查点保全（`/approve --retry` 现开全新 worktree 丢前次状态）；W-4 raw diff 拒绝散文 typed 化（structured-edit 侧已有 reason code 双臂，raw 侧是纯散文）。 | write controller、审批流、REPL 写模式卡 | **W-1+W-2 已落地**（2026-07-29，见 §7 补记二）；W-3/W-4 候补 |
| PIB-2 | 两级消息排队：pipeline 运行期不锁输入；steering（阶段边界注入）/ follow-up（run 结束注入）/ esc 还原队列 | REPL 输入循环、orchestrator 阶段边界、写模式 controller | 待启动 |
| PIB-3 | 工具面截断纪律：双上限统一截断 + 永不半行 + 截断提示即下一步指令 + 输出截断时工具调用拒执行硬门 | read/grep 等工具实现、coverage 记账、agent 工具循环 | **已落地**（2026-07-29，见 §7 补记三） |
| PIB-4 | 会话导出/导入复现闭环：诊断会话/工件一键导出 bundle + 一键导入还原 | blob session、REPL /history、报告投影 | 待启动 |
| PIB-5 | REPL 水位与输入增强：footer token/cost/上下文水位 + 粘贴自动折叠 + @ 文件引用 seed RequiredFiles | REPL footer/输入组件、usage 计量、analyzer RequiredFiles | 待启动 |
| PIB-6 | 渲染层几何 pin：teatest/VT 级差分渲染断言，BARGRID-1 教训泛化为渲染红线 | REPL 渲染面、测试基建 | 待启动 |
| PIB-7 | `.codrax/prompts/` 模板命令 + providers.yaml `!cmd`/`$VAR` 值解析 | REPL 斜杠命令注册、internal/config | **已落地**（2026-07-29，见 §7 补记四） |

排序依据：PIB-1 直接封堵死等会话事故的用户面缺口（最高性价比、改动面小，先行）；PIB-W 因用户点名写模式 gap 提升为第二批（§3.7 十七条机制 + 两条反面参考为输入，批内先探索现状再裁定收窄）；PIB-2 交互质变但改动面最大；PIB-3 纯 LLM 面正确性收益、独立可并行；PIB-4/5 日用增强；PIB-6/7 基建与顺手件。批内如探索发现现状已覆盖（或与红线冲突），允许收窄/撤销并在本表记录裁定。

PIB-1 前置探索结论（2026-07-29，全文见批次补记）：codrax 重试分层 L1(adapter HTTP,默认 6 次,退避分类含 Retry-After/quota/full-jitter)→L2/L3(fallback)→L4(orchestrator transient,预算 3)→L5(force finalize,3) 完备；W1 四层流式看门狗 + 退化断路、W2 心跳(2 的幂限频)已在役。**用户面 gap 三处**：① `render.Event` 无最大次数/截止时刻字段，TTY dock 重试行是静态字符串不倒计时（唯一带 `N/M` 分母的信息只进日志不进 UI）；② 运行期无键盘监听，取消粒度=整 Run 的 Ctrl+C（dock row3 已有提示；pi 式 esc-只取消重试等待 与既有取消架构不符，裁定不做）；③ usage 每响应已捕获但全仓零消费（无累计/无成本换算），归 PIB-5 基础。

## 7. 批次落地记录（补记区）

（每批落地后在此追加：探索结论 / 方案裁定 / 提交哈希 / 验证结果。）

### 补记一：PIB-1 重试用户可见面（2026-07-29 落地）

**方案裁定（对照 pi status-indicator 逐项）**：
1. **倒计时**：复用 dock 既有 `deadline` 字段 + 100ms 动画 ticker，新增 compose 时变换 `retryingActivityWithCountdown`（与 `lightRouteActivityWithCountdown` 同款纯函数范式，每 tick 从 deadline 推导剩余秒）。TTY row1 词形：退避中 `正在重新请求模型（第 2/5 次重试 · 还剩 7s）`；倒计时归零后翻转为诚实的 `已重发，等待响应`（不冻结在"还剩 1s"、不造进度）。无 deadline 的遗留构造路径保持旧静态词形。
2. **分母（第 N/M 次）**：`llm.ChatOptions.OnRetry` 签名扩维加 `maxRetries`（=RetryMaxAttempts-1，与 `[llm] retry N/M` 日志分母同源）；`render.Event` 加 `RetryMaxAttempts`（0=未知→渲染省略分母，禁止 `N/0`）；贯通 agent 发射点与 REPL 直调 `chainRetryCallback` 两条链。durable scrollback 行与非 TTY 镜像行同步携带 `第 2/5 次`，共用 `retryAttemptLabel` 单源。
3. **esc 取消重试**：**裁定不做**——codrax 取消粒度=整 Run 的 CancelToken（Ctrl+C 已贯通、退避 sleep 全部 ctx 可取消、dock row3 常驻取消提示）；pi 式"只取消重试等待"与取消架构不符，收益不抵通路复杂度。
4. **成功零噪音 vs 每次重试留痕**：**保留 codrax 每次重试的 durable 行**（审计文化：非 TTY/事后审计需要完整重试痕迹），偏离 pi 的"只报最终失败"，账本明示为有意偏离。
5. **范围**：L1 adapter 面（主导车道）；L4/L5 orchestrator 重试沿用既有 NoticeRetry 词形不动。

**触点**：`internal/llm/llm.go`（OnRetry 契约）、`openai.go`（发布预算）、`internal/agent/agent.go`（事件扩维）、`internal/repl/direct_llm_trace_adapter.go`（链路扩维）、`internal/render/event.go` / `dock_state.go` / `renderer_dock.go`（词形+倒计时+两车道标签）。

**验证**：`make` 构建绿；`go test ./...` 83 包 exit=0 零 FAIL；新增 `internal/render/retry_countdown_test.go` 五组 pin（变换语义/三词形双语/事件接线/非 TTY 分母+禁 `N/0` 负臂/compose 端到端含归零翻转）；`stream_wait_matrix_test.go` 加分母断言；既有子串 pin（`重新请求模型`/no-jargon/静态词形/双镜像唯一性）全存活未改。

### 补记五：PIB-5 REPL 水位与输入增强（2026-07-29，分件落地）

**5a usage 水位（已落地）**：usage 自捕获以来全仓零消费——本件使显示车道成为首个消费者。`render.Event` 加 `UsageInputTokens/UsageOutputTokens`（EventAgentResponse 载荷，display lane only 注释钉死）；BaseAgent 与 REPL 直调两个发射点接 `resp.Usage`；renderer 累计 run 总量（EventObjectiveStarted 清零），dock row2 新增 `↑12.3k ↓500 tok` 段；上下文段按水位变色（≥70% 黄、≥90% 红、窗口未知永不告警——精确整数阈值，`contextTokensStyleFor`）。成本换算（$）不做：无价格表，避免造数。pin 三组。
**5b 粘贴自动折叠（收窄不做）**：REPL 输入是 per-prompt bubbletea program，bracketed-paste 事件与原子标记字素需重造输入层；已有 `/log`（粘贴到 `/end`）与 `/paste` 显式车道覆盖主场景。裁定：效益不抵改造成本，不做。
**5c @文件引用（已落地，见本补记后段）**：探索结论——日志锚点是唯一确定性必读通路，`MentionedEntities` 半成品依赖 analyzer LLM 复述（真实 eval 失败在案）。落地：REPL 从请求文本抽 `@路径`（存在性校验）→ `BusContext.UserPinnedFiles`（AttachedLog 同款可选接口范式）→ analyzer RequiredFiles 最高 tier + 合成 `RequiredFileHint{Confidence:1.0}` + `RequiredFileHintCurrentSourceCoverageApplies` 增加 user-pinned 分支（强制读闸门的关键决策，探索报告 A.3）→ 既有 `runForcedReads` 原语在 explorer LLM 看到任何东西之前真正读完。L1 安全（pin 只比较六个量、新字段恒 nil 路径不变）。@ 自动补全收窄不做（补全面板双实现 + 每击键三次重算的性能陷阱，账本记录）。

### 补记四：PIB-7 模板命令 + 配置密钥解析（2026-07-29 落地）

**A. `.codrax/prompts/*.md` 模板命令**（`internal/repl/prompt_templates.go`）：文件名即 `/命令`（`^[a-z0-9][a-z0-9_-]*$`），frontmatter 只认 `description:`；bash 风格参数子集 `$1..$9`（引号感知分词）/`$@`/`$ARGUMENTS`。**三道安全设计**（承 §3.7 件11"repo 内配置默认不信任"）：① 静态注册表恒胜——模板只在 `NormalizeREPLCommandAlias` 未命中后才查询，loader 拒绝装载与内置动词同名的模板；② 展开结果**只能是普通分析请求**——恒剥前导 `/`，repo 内容永远够不到 slash 分发器（`/approve`/`/merge` 不可被模板触达）；③ 展开必打可见披露行（模板名+来源文件+字符数），内容不可无痕注入。接线：`New()` 一次性加载 `<runtimeAnchor>/prompts/`；`Loop()` 在注册表 miss 后查询。与 `TestHandleSlashDispatchMatchesRegistry` 守卫天然共存（守卫只查静态 case 字面量）。
**B. providers.yaml 密钥引用**（`internal/config/secret_value.go`）：`api_key` 支持 `!cmd`（sh -c 执行取 stdout，可接 keychain/vault）/`$VAR`/`${VAR}`（全串引用形）/`$$` 转义；在 `LoadProviders` 内、**per-agent 继承之前**解析（每个声明点只执行一次，merge 非空判定不会把未解析 `$VAR` 字面量误判为已设置）；声明了引用但解析不到=配置错误 fail-loud（点名 scope），空值仍走 mergeEnv 兜底。**有意偏离 pi 已落账**：pi 每请求重执行 shell（轮转 token 场景），codrax 载入时一次性解析（API key 轮转罕见，不值得热路径开子进程）。
**验证**：全仓 83 包零 FAIL；pin 三组（模板 loader 校验+遮蔽守卫+frontmatter、bash 子集展开+缺参空展开、slash 前缀中和+未知名 miss 负臂）+ config 两组（十形态值解析表、load 集成含继承前解析与 fail-loud 点名 scope）。

### 补记三：PIB-3 工具面截断纪律（2026-07-29 落地）

**探索结论（要点）**：codrax 数值上限已有中央 width governor（`internal/tool/width/`，逐值 pin）、read_file/grep 的类型化续读指令（Refinement + PreferredParams）已比 pi 完整、coverage 记账全部基于 typed 载体（banner 首行两种形状是唯一 load-bearing 文本，不可动）。四个真 gap：① 传输层 `blob.go` 预览 head/tail 是纯字节裸切（可产半行/半 rune；`types.CutPrefixRuneSafe` 家族存在但未接通）；② 三条 preview 提示教模型用 legacy `offset`，而模型可见 schema 只有 `line_offset`（prompt_hygiene_test 明令禁止 offset 出现在模型面）；③ grep 无单行长度上限（500KB minified 行整条进 LLM 面）；④ **`resp.StopReason` 全仓零消费**——finish_reason=length 带工具调用整批照常执行，`tryCompleteTruncatedJSON` 甚至补全被截 JSON 后放行（正是 pi 拒执行论证的场景）。

**落地四件**：
1. **传输层永不半行/半 rune**：`blob.go` 新增 `cutHeadAtBoundary`/`cutTailAtBoundary`（优先行边界，单巨行回退 rune-safe；修掉 `CutPrefixRuneSafe` 快路径旁通的真 bug——裸切片长度=预算时原样返回，测试先红抓出，窗口加宽 4 字节使退避生效），`buildHeadPreview`/`buildPreview` 全接，行数从实际保留切片重算。
2. **提示词参数名对齐**：全部 `offset=`/`use offset/limit` → `line_offset=`/`use line_offset/limit`。
3. **grep 单行上限**：width governor 新增 `DefaultGrepMaxLineBytes=512`（pin 测试同步）+ `capGrepLineForInline` 在 `writeCappedGrepSection` 单点生效（rune-safe + 恢复指令标记；typed 载体与 full_raw_saved 不受影响）。
4. **length 整批拒执行硬门**：`AgentContext.LLMOutputTruncatedBatch` 精确信号（每轮重 stamp：`StopReason=="length" && len(ToolCalls)>0`）；`executeTool` 在 JSON 修复**之前**短路，返回 typed 拒执行结果（repair code `llm_output_truncated`，Summary 声明未执行+原因+恢复动作"少量重发"）。`degenerate_repetition` 车道刻意不动（客户已验证的 soft-stop 车道，账本留问）。

**验证**：全仓 83 包零 FAIL（含全部 banner 消费者/prompt hygiene/refinement 矩阵既有 pin）；新 pin 四组（头尾边界+rune 安全多预算扫描、line_offset 词面+负臂、grep 行帽+恢复标记、拒执行门+flag=false 走 malformed 车道负臂）。**收窄记录**：read_file clamp 三漏口的锐利面（半行/半 rune）已被传输层修复覆盖，banner 记账口径小改不做；grep MaxMatches 不加（条目阈值+压缩 governor 已治理）。

### 补记二：PIB-W 现状探索结论与收窄裁定（2026-07-29）

对照 §3.7 六候选逐项核验 codrax 写模式现状（探索 agent 全文要点）：

1. **审批通路——gap 真实（W-1 主件）**。用户动词只有 `/approve`、`/reject`（`internal/repl/input.go:371,385`；typed next-action 枚举同样只有两条）。`/reject` 链路：`planStore.Settle(Rejected)` → 丢 worktree → `markWriteWorkflowBatchRejected` 置 batch `Blocked`；`plan.RejectionReason` **全仓无消费者**，`resumableWriteWorkflowBatchID` 显式跳过 Blocked → 单 batch run 死路，用户对计划不满只能整个重来。无 revise 动词、无对标 `VerifyFailureHandoff` 的用户修订意见 typed 载体。改造落点：`internal/types/write_workflow_next_action.go`、`internal/repl/input.go`+`repl.go` 派发、`internal/repl/handle_workflow.go:367`（置回可规划态+写 carrier）、`internal/orchestrator/write_controller_scheduler.go:281-292`（replan 入口消费）。
2. **动作级检查点——verify 路径 gap 不存在**：已有三层（`applyPostHook` 的 apply commit 快照+主仓 `refs/codrax/applied/<id>` 锚定；`clearForReplan` 的 best-known-good 热回退+增量重放；slice 级 `WriteWorkflowCheckpoint`/`Restore` typed 检查点，`verify_failed_replan` 触发 `ResetHard`）。pi 的 `git stash create` 方案**不采纳**（codrax 的 commit+ref 形态更强）。残留：**apply 中途失败（partially_applied）无自动回退**，`/approve --retry` 开全新 worktree 丢前次状态（W-3，候补）。
3. **verify 失败回灌——gap 不存在，撤销**：`types.VerifyFailureHandoff` 从 `ChangeReport` typed 投影（FailingTests/BuildErrors/FailureSignals 带 `file:line`+Expected/Actual、RepairSourceSnapshots 带行区间、diff/surface 工件 ref、900 字符 rune-safe 截断+BlobRef 分页），比 pi `git-merge-and-resolve` 的散文清单完备一个量级。
4. **无 UI fail-closed——gap 基本不存在，撤销**：非 TTY 高风险=置 `PendingApproval`+持久化+guidance+**带错误返回**（不挂起不降级）；critical 直接 deny；`write_approval_policy` 对 high 一律 manual、无 `--yes` 旋钮。pi timed-confirm 超时兜底**不适用**（codrax 根本不等待）。小残留：脚本化 REPL `/approve` 走 `readInputLines` 阻塞 stdin（记录不动）。
5. **审计面——部分 gap 真实（W-2）**：`WriteApprovalRecord` typed 盖章+防篡改指纹已有，但 `plan.Approval` **单槽覆盖**（跨 slice/replan 只留最后一次），`/workflow show` 不打 `Reasons[]`，final audit 只带三字段——"为什么判高风险"事后不可考。
6. **apply 编辑防错——大半不存在，残留 W-4**：structured-edit 侧唯一性/重叠/no-op/全文唯一才重定位/typed diagnostic（reason code+RetryInstruction）已比 pi edit 工具硬；raw unified diff 拒绝走 `composeApplyRejection` 纯散文（无 reason code）。

**批内裁定**：本批做 W-1+W-2（同属审批车道）；W-3/W-4 列候补，视 W-1 落地后价值再排。

**W-1 详细设计（定稿）**：
- 新 typed 载体 `types.WriteWorkflowRevision{PlanID, Feedback, RequestedAt, ConsumedBy}`，batch 新增 append-only `Revisions []`；pending 判定=最后一条 `ConsumedBy==""`（精确信号）。
- REPL 新动词 `/revise <修订意见>`（意见必填，空则打用法；与 `/reject` 同套 pending-plan 冷恢复路径）：`planStore.Settle(id, Rejected, "revise: "+feedback)` 留审计；**不丢 worktree**（前序 batch 已应用工作在里面——这正是 /reject 是终局而 /revise 不是的原因）；batch 置 `ReadyToPlan`（复用 approve_failed 已验证的可重规划态，auto-resume 天然接手）+ 追加 Revisions + ProgressLedger reason `revision_requested`。
- 下一步动作卡：`WriteWorkflowNextActionReviseBatch` 加入 needs_approval 的 secondary；渲染提示升级为 `/approve · /revise <修订意见> · /reject <reason>`。
- 调度器消费：`runControllerPlanBatch` 的 planner 指令注入 pending revision（操作者原话作 data 引用，标明被否 plan id）；新 plan 铸出后回填 `ConsumedBy`，防陈旧意见跨 batch 复灌。
- 对标 pi plan-mode 三段式审批的 Refine 臂（§3.7 件6）；prompt 面遵守 §3.10 模式 4（声明下游读者）与模式 5（引用而非改写用户意见）。
- W-2（append-only 审批账本 + `/workflow show` 展示 `Reasons[]`）如本窗上下文不足则单独成批，设计已锚定：run 级 `ApprovalRecords []WriteApprovalRecord` 在 stage_hooks 与 repl approve 两个盖章点追加。

**W-2 落地记录（2026-07-29）**：run 级 append-only 审批决策账本 `WriteWorkflowRun.ApprovalRecords []WriteApprovalRecord`（值拷贝防事后改史）+ `types.AppendWriteWorkflowApprovalRecord` nil-safe 助手；两个盖章点全接（orchestrator `stampWriteApprovalRecord` 追加后立即 persist；REPL `/approve` 追加后 best-effort Save 失败仅告警不阻批）；`/workflow show` 审批块补 `Reasons[]`（cap 4+溢出卷叠行，[level] code — detail 形）与账本计数披露行（双语）。验证：全仓 83 包零 FAIL；pin 三组（approve 落账/值拷贝防泄漏/渲染含卷叠与超帽负臂）。PIB-W 主批（W-1+W-2）收口；W-3/W-4 留候补。

**W-1 落地记录（2026-07-29）**：按定稿设计全量落地。触点：`internal/types/write_workflow_run.go`（`WriteWorkflowRevision` + batch `Revisions[]` + `WriteWorkflowBatchPendingRevision` 精确信号访问器）、`write_workflow_next_action.go`（`revise_batch` 动作，needs_approval secondary 首位）、`internal/types/conversation.go`（`/revise`+`\revise` 别名双注册——由 `TestHandleSlashDispatchMatchesRegistry` 结构守卫抓出，否则 dispatch case 是死代码）、`internal/repl/input.go`+`repl.go`（动词注册/派发/`handleReviseCmd`：意见必填、同套冷恢复、Settle 留审计、**不丢 worktree**、置 ReadyToPlan 后立即同步 resume）、`messages.go`（四条双语消息 + `isWriteModeCommand` 归类——由帮助分区守卫抓出）、`handle_workflow.go`（`markWriteWorkflowBatchRevised` + 带 mutate 回调的 update 变体 + 卡片提示三臂化）、`write_controller_scheduler.go`（`appendOperatorRevisionHint` 纯函数注入 planner hint（%q 引用原话+声明下游读者）+ `pendingActiveBatchRevision` + `updateWorkflowRunBatchPlan` 铸新 plan 时盖 `ConsumedBy` 防陈旧复灌）。验证：`make` 绿、全仓 83 包零 FAIL；新 pin 七组（repl 三：意见必填负臂/全链路含 **ledger 顺序证明 revision_requested 后有 resumed（ReadyToPlan 可恢复非死路的判决性证据）**/卡片三臂；orchestrator 三：hint 引用+命名+nil 零字节、活跃 batch 限定+消费态过滤、ConsumedBy 盖章+历史保留；两结构守卫红→绿）。
