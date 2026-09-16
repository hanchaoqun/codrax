# r1078 人工审计：有界 Trace 语义成员与公开函数职责

## 执行与保全

- 2026-09-15 17:43:44 PDT（2026-09-16 00:43:44Z）启动，17:48:39 完成。快照 `aa52ef41eb0d`，提交后清洁二进制；开跑前远端与本地一致。
- 原两例各一次，`CAP=5 PARALLEL=2 TIMEOUT=1200`，均保原 15 步。没有第三例、重跑或修改 case/oracle/fixture/原答案。runner exit0 表示批次完成，不是两例全过。
- 243 例库存为 215 read/25 apply/3 plan。本对优先补长时间未覆盖的窗口成员清单与带职责解释的代码清单：H10 最近留存 r387/08-12；criterion 在留存结果/汇总中未找到对应 ID，不宣称历史从未运行。与前批 NAPI 写及流水线图互补。
- 输入保全收据：`.codrax/tmp/20260915-r1078-cases-before.sha`、`20260915-r1078-fixture-before.sha`。两例结束并核验输入未变后才开始 B1705/B1706 源码编辑。
- 机器汇总原样保留于同名前缀 `.md`：**1 PASS / 1 FAIL**。本文件独立记录人工正确性，不回填或覆盖机器判定。

| 用例 | 原机评 | 人工结果 | 耗时 / 最大上下文 | 成文过程 |
|---|---|---|---|---|
| real_trace_h10_spantop_member_subrows | FAIL | FAIL：遗漏已存在的两段语义 span，且等待口径混用 | 117s / 38% | 0 拒绝、1 patch，无 JSON 恢复 |
| read_combo_criterion_rich_functions | PASS | 核心正确性 PASS；过程效率 P2 | 294s / 32% | 1 拒绝、1 patch，无 JSON 恢复 |

本对没有 Mermaid，图形渲染验收 N/A；不借此销清全图关系/时序/逻辑矩阵。没有写模式 apply/verify，B1702/3/4 自然生产正证 N/A。

## 1. H10：底层有成员，但成文上下文未交接

结果目录 `eval/results/real_trace_h10_spantop_member_subrows-20260915-174344`；完整日志 `run-1.logs/codrax-20260915-174346-000-2226.log`。原答案 `.codrax/output/20260915-174539.159-2226.md`，同名 HTML 和 root-causes.json 保留。

### 原始事实与答案差异

1. 窗口 `13762.791708..13763.024898` 内，原 fixture 的 `5968–6113` 与 `12610–12663` 两段 JIT 分别 **1.781ms、0.607ms，合计 2.388ms**，发生在 `Jit thread pool-17284`。实际 attached_trace 有首行来源注释，运行时物理行号为 **5969–6114 / 12611–12664**；这一个行号偏移不是定位错误。
2. 目标线程是 `CompThread_0-2955`。`root_cause_rank` 完整结果的 `window_stats.trace_spans` 保留这两段，raw blob `9923dc2b` 的 553–554 行可复核；排名无语义根因行不等于窗口内没有语义 span。没有独立链证据时它们只能是窗口背景/额外排查线索，不能因在邻近线程发生而晋升主因。
3. 原答案遗漏三个数值和全部成员范围，反复断言窗口内不存在 JIT/类校验等 span，最后才限定未搜其他线程。这不是只因旧子行词面 oracle 失效：客户要求的实际成员和值确实未提供，人工亦 FAIL。
4. 目标调度状态为 running 74.915ms、runnable 1.576ms、sleep 118.586ms、D 36.757ms。12 条 blocked_reason 的 Σdelay=39.157ms 不是 D 墙钟分区，不能称全部 D 等待共 39.157ms、据此平均再换成 36.757ms。系统已有口径披露，模型仍混用，继续记解释质量债，不用正文扫描或系统代改结论补救。

### 上下文与过程

- 5 次 trace_query，0 次源码 read/repo_map/list；最大上下文 76708/200000。analyze/explore/finalizer 各一次、extract 0；finalizer 两迭代、0 拒绝、1 patch，semantic reviewer 0。
- log545 的 typed profile 为 `bounded_fact_set`，事实维度含 count_or_duration/recorded_reason/occurrence_time；具体窗口/目标已知。现行完整因果报告权限因此不激活。不能仅凭用户 prose 或有时间窗就强开全量因果投影。
- 确认新 **B1706/P1**：`renderAnswerDocTraceDecisionHandoff` 用完整报告发布权限提前返回，连无需因果授权的语义成员事实也一起省略。初始成文 prompt（log1618–1842）缺两成员名称/耗时/范围，而完整 observation ledger 已有。`answer_document_mutation_runtime.go` 的系统输出表亦受完整报告权限约束；本次只修模型上下文，不启用系统表代写。
- 这不是单纯模型波动或 JSON 畸形。模型早期查询范围选择偏窄、未正确消费 rank caveat 也有责任；但事实供给与完整报告权限耦合是可独立修复的系统问题。
- 默认旁路文件正常生成：`schema_version=2,root_causes=[],status=unavailable,reason_code=trace_root_cause_contract_not_active`。不是 `valid_model_root_cause_selection_unavailable`，不是文件遗失。新事实交接不得改变这个权限/选择状态。
- 末尾三条非目标唤醒事实对照未当作主因，但相关性较低，记 P2 展示观察；本片不扩大为新的答案重写机制。

### 根修范围

新增与完整因果报告独立的、有界事实 prompt：只消费当前成功原生查询、精确本附件/窗口/查询目标的 typed semantic span；成员物理来源可寻址，按数量/字节预算明确截断。保名称、耗时与成员范围，关系未证时明示背景，不生成排名、可消除量、修向或模型根因选择。完整 Trace 因果投影及自动补齐旧路径保持不动；公共查询→dispatch→context→handoff 先红后绿后才交付。此处记录修复前生产结果，不将后续单测冒充修复后 live 正证。

## 2. criterion：五个公开函数与职责均正确

结果目录 `eval/results/read_combo_criterion_rich_functions-20260915-174344`；完整日志 `run-1.logs/codrax-20260915-174346-000-2216.log`（含 NUL，审计使用文本模式）。原答案 `.codrax/output/20260915-174836.859-2216.md` 及 HTML 保留。

1. 最终恰好列五个公开生产函数：`Eval(eval.go:15)`、`EvalAll(:36)`、`SetExternalArtifactFloor(:1126)`、`IsRegistered(grammar.go:110)`、`RegisteredKinds(:116)`。未混入类型、变量、Kind 常量或私有函数；五个定义引用准确。
2. `Eval` 对未知 Kind 返回 UnknownKind=true；`EvalAll` 对未知或不满足均令 allOK=false 并收 failed，不声称自身 panic。配置兼容入口已无评估控制作用、注册列表无序的职责亦与源码一致。模型提及调用方可 panic 不等于 EvalAll 实现会 panic。原 `ErrUnknownKind` 注释有陈旧表述，答案未重复该错。
3. 工具 11 read/3 repo_map/3 source inventory lens；最大上下文 63061/200000。3 次 explorer dispatch/19 迭代、10 次 midloop、5 次完成调用。机指标 investigation reject=0，但日志1214、1250确有两次 completion DOWNGRADED：先缺每成员支持，再缺职责维度 grounded 支持。不得用单个零指标说全程零收束修补。
4. 两次补充后职责证据可用。临近收束仍探索 private helpers 等非交付成员，记录 P2 成本观察；尚无第二个确定性自相矛盾合同见证，不加关键词硬门限制模型探索。
5. 成文首稿只有表，因既有 required summary 缺失被拒一次；模型 patch 只加 summary，原表保留。自动引用处理只修精确已验证清单成员引用，没有改职责文字。summary 置于表后只是 soft advisory，不再拒绝；没有整稿替换或系统重写结论。
6. 本例返回证据来自不同坐标，不是 B1694 同坐标异声明合并的验收正证；B1694 保持独立 OPEN。

## 3. 后续顺位与保护边界

- B1705：把图的结构“需要一条边”和语义“需要某种关系”分开；布局本身不得铸造 call/guard/contain 最低数。已有 typed 实际边真实性门、required 空图与未证出口保留。公共复现/修复与本对 live 无因果混称。
- B1706：上述缺失成员上下文 P1，先补可复现供给通道，不强行开启全量报告。
- B1694：同坐标异语义 claim 合并 P1 继续排后继独立修复；模型单位/等待解释、图矩阵与原生写验证债不因本次测试被销账。
- 代码默认 600s 首响应 / 300s 实际字节静默 / 600s 非流式不变；心跳/推理/工具字节继续续活。没有“4ms 或旧4分钟未出现正文就降级”的新策略。eval 的显式 1200s 外层上限是另一边界，本对均未到上限。
