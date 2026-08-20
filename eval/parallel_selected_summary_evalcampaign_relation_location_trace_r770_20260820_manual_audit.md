# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T12:02:27Z
- sweep_start_ts: 20260820-050225
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-050227 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 180s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 窗、threadpool→network→cookie→app 唤醒链、11.000ms 链上 iowait 首因、三席各 1.000ms 调度延迟、实际占时/规则可消双轴及 Trace 因果投影均保留。目标 sleep 只作症状，IO 综合评分只作背景；4 次 typed trace_query 后一次成文通过，无流式降级。 |
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260820-050227 | answer_regex,answer_contains | none | 221s | 32 | read=8,repo_map=2,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=2,unavail=0,prune=0 | pass | 终稿列出全部 12 个 production implementer，每行有真实 file:line；图保留 12 条 model-authored implements 边，无抽象集合 boundary 或 raw relation enum。两次拒绝分别是旧 citation index→evidence_id 迁移和 patch block-id 大小写笔误，第三次 patch 只补 member_set facet。首稿其实已经有所有路径，日志却未出现逐成员 location 教学/修补，证明 B1235 输出正证但确定性兜底仍误接 advisory aggregate，需改接 shared exact provider。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case judgment

- Runner: 2/2 PASS; human: 2/2 pass.
- Active-stream protection: both lanes remained active and completed normally; no 4ms/4m/first-byte/stall/total-age downgrade.
- Trace regression guard: explicit-window causal projection, deterministic supplement, chain-only root cause, actual-occupancy/rule-eliminable axes, and background demotion all survived.
- New wiring finding: B1235 is not production-closed at `d40338de6`. The Analyzer's presentation-only location receipt survived, and the exact typed provider exposed all 12 paths, but `answerDocumentRelationMemberSetRequestsVisibleLocations` required an evidence-authorized aggregate. The production aggregate was explicitly `fact_authority=advisory_model_inference` and `principal_contract=not_authorized`, so the hard display fallback never activated. The correct answer came from the model's first draft, not from the deterministic coverage contract.
