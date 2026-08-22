# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T04:34:18Z
- sweep_start_ts: 20260821-213418
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-213418 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 201s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | uncertain | 显式 2.000..2.020s 用户窗、4 次 typed 查询、threadpool→network→cookie→app 唤醒链、11ms 链上 IO 第一席、三个互不相加的 1ms 调度/优先级候选、真实占时/规则可消双账户、链上业务下钻、背景隔离、自动补采和完整 Trace 因果投影均保留。唯一拒绝是模型把 summary-only `trace_causal_claim_caliber` 复制到 section，下一稿删除后通过；无固定时长降级。人工保留 uncertain：模型把 `fscache_page_wait_on_page_bit` 进一步解释成文件系统缓存页等待，并提出预取/回写/内存压力三种可能方向，虽然后文用“需下钻/不证明具体资源”限定，但同页机理口径仍略强于当前调用点证据。 |
| 1 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260821-213418 | answer_regex,answer_contains | none | 216s | 29 | read=11,repo_map=3,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=2,unavail=0,prune=0 | partial | B1325 生产目标通过：第一稿全部源码 item 直接提交 stable `evidence_ids`，没有手工引用下标拒绝。两个拒绝来自系统 fused block 自愈：模型在同一个 ordered_list+diagram 块已提交 6 条 edge_anchors，拆块器却把 anchors 全移到 diagram half，令仍声明 call_edge 的列表 half 自造空锚失败；第二轮 fused patch 再次被同一逻辑清空，第三轮只替换列表才通过。终稿调用链完整且图合法，但摘要把 tsconfig.base.json:8 的精确 `packages/core/src/index.ts` 缩写成 `packages/core/src`，人工因此不判 full pass。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case findings

1. `B1325-ITEMEVIDENCECARRIERCONFLICT1` 获生产正证：TypeScript 第一稿已经使用 stable `evidence_ids`，Rust/TypeScript 先手填 citation index 的重复修补没有复现；本轮所有引用绑定日志均是所选 ID 到 citation 的常规持久化，不是 evidence-ID adoption 拒绝。
2. 新 P1 `B1326-FUSEDEDGERETENTION1` 为确定性系统自冲突。fused block splitter 的旧字段分区把 `edge_anchors` 误当成 diagram-only；但同一个字段同时是 principal relation list/table 的结构化关系所有权。拆分后 diagram half 需要 anchors，声明 directed claim 的 visible half 也需要同一批 anchors。最优方案是按 typed block kind + principal role + directed claim form 克隆模型已经提交的 anchors 到两半；普通 prose/table 或非关系块仍只把 anchors 放到图半，不能从 items、label、Mermaid message 或请求原文推断关系。
3. Trace 核心能力无回归；一轮 `trace_causal_claim_caliber` 字段错位更像模型对投影 schema 的局部误用，先作为异构回放观察，不据单例增加正文关键词硬门或系统代写。
4. TypeScript 精确 alias literal 有一次模型缩写漂移。typed evidence 与引用表仍锚到 tsconfig.base.json:8，且 r839 同案曾准确输出 `index.ts`；当前先记模型事实遵循观察，不扫描或重写最终 prose。
