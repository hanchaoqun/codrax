# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T12:22:53Z
- sweep_start_ts: 20260814-052252
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | s8a | FAIL | eval/results/s8a-20260814-052254 | answer_regex,answer_contains | none | 299s | 32 | read=6,repo_map=0,list=0,trace=0,source_lens=0 | midloop=11,inv=3/0,fin_reject=2,unavail=0,prune=0 | partial | B792 production-positive：typed block 的 source/sink existence 已分别为 definition_and_call_edge/call_edge，不再与两条 capsule 边冲突；最终也正确保留 `buildAnalysisIR -> RunWith <- gate.Run`。第一次拒绝只是 candidate_role JSON 类型错误，第二次正确隔离 principal facet 与抽象中间边。残余是探索/选材只列出早期 reconcile/triage 等 17 项，漏 normalizer/compiler/hdp/binder 三个核心阶段，属于证据覆盖/模型选材波动观察，未发现新的矛盾硬合同。 |
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260814-052253 | answer_regex,answer_contains | none | 322s | 30 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=12,inv=3/0,fin_reject=1,unavail=0,prune=0 | fail | Analyzer 的 required sequence diagram 已用 current-request quote 固定 incident participants=`buildAnalysisIR`,`gate.Run`，call profile 却发 `source=buildAnalysisIR,sink="",mode=discover`，且遗漏 exact_targets；系统未交叉校验两个 typed 载体，故后续没有 `gate.Run -> RunWith@135` 边界，Finalizer 把 RunWith 当成题设 gate.Run 终点。关系门仅纠正一条错误 assignment/reply，无法恢复从未进入 IR 的用户终点。确认 B793 typed IR 自冲突，非终稿关键词问题。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
