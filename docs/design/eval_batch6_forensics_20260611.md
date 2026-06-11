# Eval 批 6 — 多代理深挖取证(11 项 confirmed gap,5/6 PASS 掩盖大量系统问题)

批 6:`data_join_entity_reconcile` / `patch_java_typo` / `patch_c_typo` / `mr_focus_single` / `qf_diagram_pipeline` PASS,`operation_system_inventory` FAIL。27-agent 工作流(逐案扫描 → FAIL 根因 → 逐 claim 对抗校验)后 11 项 claim confirmed、3 项 refuted(修正后缩窄为更小的真 gap)。工具使用审计:patch_c/qf_diagram 主动用了 repo_map(qf_diagram 3 次 repo_map 全部 load-bearing);data/operation lane 正确地不需要管线工具。

## A. 立即修(本批落地)

### A1. SPEC_STALE:`[cmd/operation]` → `[cmd/route]`(operation FAIL 根因)
`c4a9eae8`(2026-06-05)把 single-shot turn-policy 日志 tag 从 `[cmd/operation]` 统一改名 `[cmd/route]`,但 `operation_system_inventory.case:8` 与 `operation_web_manual_summary.case:7` 仍硬编码旧 tag。run 本身路由/执行全对(4 命令全部 exit 0,答案正确)。**修**:两 spec 改新 tag。

### A2. 日志 tag 契约无系绳(A1 的泛化)
eval spec 的 `EXPECT_LOG_MATCHES_REGEX` 自由字符串无任何机制绑定发射点(无共享常量/契约测试),改名 6 天不可见。**修**:契约测试——提取所有 case spec 中 `\[tag\]` 形态的日志 tag,断言每个都存在于 Go 源码;任何 rename 在 `go test` 即红。

### A3. harness plan 形态误报
`run.sh` 统计 plan.json 中所有 `"kind":` / `"path":` 出现次数当 changes 数,嵌套 `edits[].kind`、`write_analysis_ir.request.task.kind`、`approval.reasons[].path` 全被算入——每个 patch 型 plan 的 summary 都虚高。**修**:只数顶层 `changes[]`。

### A4. string-carrier strict-decode 遥测盲区
`remapCannotUnmarshalStringIntoNativeJSON` 无任何日志标记(只有 misplaced-field 变体在 `strict_decode_remap.go:94` 打日志),`run.sh:525` 的计数器只抓 misplaced-field 形态——该类 emit 摩擦系统性漏计。**修**:remap 加 `logging.Info` 标记 + run.sh 计数器补一行。

## B. 模型面正确性(本批落地)

### B1. `[]struct` 元素形态误诊为 string-carrier(qf_diagram,confirmed + 复现)
`required_files:["a.go","b.go"]`(原生字符串数组,目标 `[]emitRequiredFileParam`)→ Go decode 错误引用元素 struct 类型(带包点)→ 分类器误判 `jsonStringCarrierObject` → 双重错误指引:指责模型 quote-wrap(从未发生)+ 让它发"native object"(正解是对象数组)。泛化:所有 emit 工具的所有 `[]struct` 字段 + 平行 `tool_param_json_string_carrier` ToolRepair 通道。本轮模型靠 schema 自愈,字面照做 hint 的模型必再失败。**修**:remap 前解析原始 params——字段值已是原生 JSON 数组时,string-carrier 诊断证伪,改发"该字段是对象数组,每项 {path,confidence,...}"指引。

### B2. analyzer-prescan files_only 静默改写无模型面解释(qf_diagram)
`normalizeAnalyzerPrescanGrepCompat` 改写 files_only→true 只打系统侧 WARN,模型看到零信息结果('1 matching files' = 自己 include= 的文件)自费一轮 prescan 自诊;且 prompt 声明此类调用"会被拒",实际静默成功——结果与模型契约矛盾。**修**:改写时往 ToolResult 附一行模型面说明(分类步只允许 files_only=true;行级证据后续阶段采集;文件放 required_files)。

### B3. operation lane evaluator=complete 后仍跑续航 planner 轮
`commandOperationStatusFromEvaluation` 把 EvalComplete 映射为 ""(default)→ 短路不触发;`OperationEvaluation.IsTerminal()` 已存在但 internal/repl 零引用。每 run 浪费一整次 LLM(23.5KB prompt,~5.4s)+ 重复 env probe。**修**:complete(高置信)→ 短路直达 final report。

### B4. data 终态 journal 刷掉 typed override 标记(refuted→缩窄后的真 gap)
完成门 normalize 把 evaluator 的 repair_node 改写为 complete 时**保留了**原话 + `original_status=repair_node` typed 标记(`evaluation.go:135-153`,审计 JSON 深处可见),但 `BuildWorkflowJournalDecision`(journal.go:252-256)把顶层终态决策刷成 `{status:complete, reason_code:complete}`,终态日志行 reason=""——override 对用户/日志不可见。**修**:顶层决策与日志行携带 `original_status`。

## C. 多仓定向(本批落地)

### C1. parent-graph 兼容回退无视路由 active set(mr_focus_single,confirmed)
`buildAnalyzerRepoOverview` 请求 parent graph → `multigraph_facade.go:203-207` 回退 `pickPrimarySubRepo`(:396-426)只看 REPL `/repos focus` pin 或 largest-by-FileCount,**看不到本问题的路由 active set**(routing fold 只写 ExactPrescanSlugs/PendingSubRepos,从不写 FocusSlugs)→ 对 Rust 问题扫描+注入 Go 子仓 overview,污染 LRU(终态 active=2 vs 路由 active=1)。**修**:回退前优先消费路由决策的 active set。

### C2. 双 prompt 节 active-set 矛盾(C1 下游,confirmed 独立 gap)
'Multi-Repo Active Set' 节读路由决策(greet-go 将被拒),'Multi-repo overview' 路由注读 multigraph LRU(被 C1 污染,只报 tools-py inactive)——模型若信 overview 注会以为 greet-go 可查而工具门会拒。**修**:overview 注改读路由决策(单一事实源)。

### C3. findings_validator 多仓路径误报(C1 verify 中发现的兄弟 bug)
`validator.go:44` pathRegex 前缀锚定(src/|lib/...)把 `repo-stub-rust/src/lib.rs` 截成 `src/lib.rs`,再对 parent root os.Stat → 必失败 → 正确路径被标 unverified。**修**:多仓下先对(parent root + 完整 RootRel 路径)stat,正则不截断子仓前缀。

### C4. CGEC I1 错图校验(C1 下游)— C1 修后 SearchGraph 即正确图,无需独立修;回归验证覆盖。

## D. data lane 答案投影(本批落地)

### D1. 系统合成 `complete_reference=true` 把正确单实体答案改坏(confirmed,HIGH)
round-10 答案 "30"(reconcile=pass)正确;系统 continue 转换在 round-11 合成 `assemble_answer` + `complete_reference=true`,零填充 reference 全键域 → 终答 "30,0",问题明确只问一个实体一个数。两次系统转换间隔 ~100ms 无 LLM 参与。§1.5 违规:结构完备性动作硬套在单实体范围问题上。泛化:任何引用表 >1 键的单组聚合问题都会被附加零填充组。**修**:系统转换合成 answer-projection 时,精确 typed 信号(输出契约单值形态)下不设 `complete_reference`。

## E. 已立项/记录,不本批修

- **E1. data lane 无算术派生路径** — **已修(本 session 追加)**:derive_fields 增加 multiply/divide/add/subtract(product/sum_fields 别名),big.Rat 精确十进制(与贡献账本同引擎),操作数缺失/非数值/除零 → 空(落 Default,镜像 parse_number 语义);校验复用 concat 的 source_fields 字段契约;planner skill 文档同步(明示"不要发明预计算字段或硬编码逐行常数")。**残留子项**:idempotency 毒化粒度收窄(blocked plan 全 action key 被毒化)仍归专项。
- **E2. analyzer 复杂度 Rule 6 退化升级**(confirmed):typo 对(retrun/return)+文件+函数被数成 2+ entities → mechanism 问题硬升 complex,无视模型 typed `is_cross_component=false` + 0.98 置信 simple——§1.5/§1.6 双重张力;且写模式 planner 预算读该读侧信号(soft cap 6→10)。**专项**:Rule 6 需 §1.6 typed escape 设计(analyzer 核心,需谨慎)。
- **E3. write_controller 确定性生命周期转换烧 LLM 轮**:单批 micro plan 4 次 emit_write_workflow_decision 有 3 次完全由 typed state 决定。**专项**(G 类优化)。
- **E4. operation lane 零 op_* 计数器**;**E5. data planner prompt 12x 膨胀**(27.6KB→323KB/17 calls);**E6. 'Retry Directive' 首发 prompt 误标**(low);**E7. auto_low_risk 标签硬编码**(low);**E8. diagram_edges soft violation 无取证轨迹**(low);**E9. 并行 explorer 取消挽救路径继承已取消 context 必败**(low)。

## Refuted(记录防重查)
- 终态"静默"覆盖 evaluator(→缩窄为 B4 surfacing gap);idempotency 键"按边不按内容"(实际全内容键,真因是 blocked plan 全 key 毒化,归 E1);prompt 重放"无界"(实际 bounded 但 12x 增长,归 E5)。
