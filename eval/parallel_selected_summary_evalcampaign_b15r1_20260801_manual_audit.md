# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T01:46:13Z
- sweep_start_ts: 20260731-184612
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260731-184613 | log_regex,trace_attachment,answer_contains,principal_answer | perf_triage+trace_query | 117s | 38 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 四态/157.248ms/CPU4 2.10GHz direct limit row/完整因果投影均在；但模型把“实际 558MHz 低于 ceiling”直接推成明确 thermal throttle，并说不是 policy 硬限制，违反 finalizer 已收到的 typed binding boundary。正确 frequency authority 只在文末 Caveats，未形成领先权威面。 |
| 1 | real_trace_c2_dstate_iowait | PASS | eval/results/real_trace_c2_dstate_iowait-20260731-184613 | log_regex,trace_attachment,answer_regex,answer_contains,principal_answer | perf_triage+trace_query | 174s | 35 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | REL1 真实收账：complete roster/窄报告继续正确，答案和日志均不再出现 self-pair 跨工件关系块。残余：算术 advisory 把 0.635ms/144.557ms=0.44% 错配成 144.557ms/0.44%，生成错误系统校验附注，立 AR1。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Findings

### C2：REL1 收账，发现算术 advisory 操作数误绑

- system lead 仍发布 3 次、0.635ms 和三条完整 occurrence，完整因果投影保持
  缺席。
- `runtime_artifact_pair_relation` / “跨工件关系边界” / 同路径 `↔` 在答案和
  日志中均缺席，证明 canonical path identity 在真实 replay 生效。
- 模型正文正确写“0.635ms 占 144.557ms 约 0.44%”。系统 arithmetic
  advisory 却发布“正文 144.557ms / 0.440%，重算为 100%”，说明 prose
  relation extractor 在同句三个数值中把 denominator 选成 numerator。这是
  用户可见的错误系统附注，但未参与 hard gate。

### H4：频率权限到达 prompt/footer，但领先主结论仍失守

- 模型正文准确发布 running=157.248ms、runnable=5.604ms、
  sleep=70.338ms、D-state=0，以及 CPU4 direct policy ceiling=2.10GHz；
  `132.041` 未出现，完整 Trace 因果投影和补采仍在。
- finalizer prompt 明确包含：
  `policy_limit_status=present`、
  `binding_caliber=limit_row_proves_ceiling_presence`、
  `binding_impact_requires_separate_overlap_or_supply_evidence`，并逐字说明
  actual/residency 低于 ceiling 不能证明没有 policy limit，也不能单独证明
  binding 性能影响。
- 模型仍写“558MHz 远低于 2.10GHz，因此明确 thermal throttle，且不是 policy
  硬上限约束”。低于 ceiling 本身不能区分 workload demand、policy、thermal
  或其他治理原因；这条成因与排他结论均越权。
- 相同 typed authority 已由 `materializeRuntimeTraceFrequencyAuthorityCaveat`
  生成，但载体是 `doc.Caveats`，只在约千行因果报告后的系统附注出现。用户先
  看到错误 principal，正确权限没有形成 lead authority。
