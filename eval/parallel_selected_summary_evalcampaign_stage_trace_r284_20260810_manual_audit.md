# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T01:20:33Z
- sweep_start_ts: 20260810-182031
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260810-182033 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 123s | 28 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B489 核心生产正证：typed completion 精确携带 waker_cpu=2、wakee_target_cpu=1、跨 CPU，最终稿不再声称 worker 与 app 同核；显式窗、自动补齐、class verification 4.600ms #1、runnable 0.800ms #2、actual/effective 双口径、因果投影与背景分层均保留。但 explorer 已明确 priority_inversion_candidate=false、无实测反转影响，finalizer 仍自行虚构“持有数据或锁”并写成反转候选；精确信息已足，记 B494/P2-watch 模型结论波动，不做 final-prose hard scan/系统改写。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260810-182033 | answer_regex,answer_contains,mermaid_edge_count | none | 219s | 32 | read=10,repo_map=3,list=0,trace=0,source_lens=0 | midloop=8,inv=3/0,fin_reject=5,unavail=0,prune=0 | fail | B488 partial：Analyzer 六个参与者均正确为 incident_required，Explorer 不再把字段声明当 initializer，并读取 applyStageOutput/appendStageOutputEvidenceToMutable；但把真实 call 发成语义化 writes→Mutable，emit_evidence 仅降为 text_reference，未给 parser-owned exact caller/callee 可复制修复形。completion 低增量收敛后 finalizer 无 carrier relation，5 次 patch 最终仍只保留 stage precedence、断开的 BusContext/Mutable，并因 typed Mutable 与 source type MutableState churn 出重复节点；正文继续声称完整共享流。确认 B490-CALLREPAIR1/P1 与 B493-FLOWNAV1/P1，B491-PARTALIAS1/P2。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- runner: 2/2 PASS；human: 0/2。
- B489 的 CPU topology transport 获直接生产正证；Trace 原有窗、补采、链上根因、双口径、投影与背景分层均隔离通过。B494 是 finalizer 在已获相反 typed 结论后的单次模型漂移，暂不以系统硬化接管。
- B488 从“角色误分+声明伪 initializer”推进到“正确角色+真实操作导航”，但尚未形成 carrier 图边。下一批优先处理通用证据修复链：B490 在 call authority 降级时发布 exact parser caller/callee 修复 tuple；B493 让 participant repair 在 typed 名称之外携带相关 grounded evidence 的 source aliases/operations，帮助找到 carrier 方法的 writer/reader 两侧。两者都只提供精确信息，不自动生成图边或答案。
- B491 记录为次级 churn：typed request identity `Mutable` 与源码 type `MutableState` 的 visible boundary recipe 可进一步减少重复节点/patch，但它不是空关系的根因，排在 B490/B493 之后。
