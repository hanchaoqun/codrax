# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T10:06:02Z
- sweep_start_ts: 20260731-030602
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_log_current_code_boundary | FAIL | eval/results/read_combo_log_current_code_boundary-20260731-030602 | log_attachment,answer_regex | log_triage | 87s | 16 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | route 输出 `current_source=optional`，尽管 reason 和 analyzer thinking 都识别“需要结合当前源码”；analyzer 又漏发 `current_source_explanation_profile`。最终只用日志解释 timeout，明确声明未读源码，未满足 mixed 请求。 |
| 2 | logtri_oversized | PASS | eval/results/logtri_oversized-20260731-030602 | log_attachment | log_triage | 296s | 35 | read=12,repo_map=1,list=2,trace=0,source_lens=0 | midloop=6,inv=4/0,fin_reject=0,unavail=0,prune=0 | pass_with_efficiency_gap | 主结论已恢复为 runtime stack top `main.crashy`、caller `main.main`，checkout mismatch 只作映射 caveat；但 analyzer 把 artifact phrase 塞入 `source_scope_profile(all)`，旧 RM authority 绕过 route optional 重开 required source，产生 12 reads、4 completion、无关 `sealedTraceStreamerDBOutputs.Size`。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论

- runner 1/2 PASS，人工同为 mixed case FAIL、oversized pass-with-efficiency-gap。
- S3 的 producer 和 wire 接线生效：两例路由日志都明确显示 `needs_repo=true current_source=optional source=artifact`。第二例的 runtime 主事实不再被 checkout mismatch 擦除，说明 runtime authority 方向正确。
- S3 尚未闭环：`RequestModel.CurrentSourceLaneDecision()` 仍可通过 `external_observation_policy=allow` 或 `source_scope_profile` 独立把 lane 升回 required，绕开新 route enum。第二例就是该旧旁路的真实 witness。
- `source_scope_profile` 的语义是 repo path role（production/test/docs/all），但 analyzer 用 artifact quote “这个大日志里的 panic”铸出 `all`；当前 `sourceScopeHasCurrentRequestAnchor()` 只检查 quote 非空，无法区分 repo scope 与 artifact scope。
- 最优修向：route optional 是 artifact-only 的初始上限；只有独立、精确的 current-source proof（专用 `current_source_explanation_profile` exact quote、resolved current files、exact code/path target、required current-code dimension）才能重新升级。generic source-scope quote、policy allow/default、diagnostic prose flag不能单独越权。legacy unspecified/route required 保持旧行为。
- mixed case 同时漏发 route required 和 dedicated analyzer profile，暂判 typed producer 波动。增强两个 schema prompt 的正交规则和专用 carrier 要求，但不根据 reason、用户关键词或答案正文修补；修复旁路后再成对复放判断稳定性。
