# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T19:44:20Z
- sweep_start_ts: 20260828-124419
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260828-124420 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 156s | 43 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 114.940ms 主窗、四跳唤醒链、链上优先级反转/D-IO/算力/调度供给、实际占时与规则可消双账户、VerifyClass 业务线索、邻近/背景隔离及完整 Trace 因果投影均在；帧证据缺失按席位披露为 unproven，未摘除已证链上加冕。无 4ms/4m/活动流降级。 |
| 1 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260828-124420 | answer_regex,answer_contains | none | 281s | 31 | read=17,repo_map=3,list=0,trace=0,source_lens=2 | midloop=7,inv=7/1,fin_reject=0,unavail=0,prune=0 | fail | 默认值和 default→YAML→CLI 顺序正确，但 Explorer 未读取 `LoadRuntimeSettings` 的 `yaml.NewDecoder(...).KnownFields(true)/Decode` 实现。完成门先拒绝字段定义，模型随后按系统另一条教学发射 `evidence_kind=mechanism + anchor_kind=definition`，同一字段定义却被当作 operation 放行，暴露 B1401 合同/确权漏洞。最终另有错误的系统“枚举标签核对”称已在 `cmd/root.go:653` 引用的 `flagMaxSteps` 无声明，以及无具体成对声明的泛化不一致脚注。B1400 的可选 analyzer file binding 本轮未自然发射，记 no-regression 而非 production-positive。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Focused findings

1. **B1401 — definition/mechanism authority inversion (P1):** `EvidenceCarriesExplanationOperation` currently returns true from `EvidenceKind` before it checks `AnchorKind`. A grounded field/type definition can therefore be relabelled `mechanism` and close a seat whose rejection text explicitly requires a producer/consumer/call/return/assignment/initializer/condition/branch operation. This is not a `pipeline_max_steps` special case: it affects every required `function_or_purpose` / `branch_behavior` dimension.
2. **B1401 teaching conflict:** the generic ROLE-DESCRIPTION instruction recommends `evidence_kind=mechanism, anchor_kind=definition` for descriptive role prose. That row is valid context, but must be stated as non-operation ownership; otherwise the model is taught the exact carrier that bypasses the completion contract.
3. **B1402 — false enumeration-label supplement (P1):** the accepted table cites `flagMaxSteps` at `cmd/root.go:653`, but a noisy declaration oracle still creates a system supplement saying the label cannot be matched. A noisy oracle miss may stay operator telemetry/soft repair guidance; it must not produce a user-visible doubt notice when the same accepted document already carries a grounded exact-token source line.
4. **B1403 — generic contradiction without actionable pair (P2):** the answer publishes “答案前后某些表述存在不完全一致” without naming summary/body claims. The current materializer has a specific paired-claim path, so this generic text is non-actionable repair telemetry and should not be written into an otherwise accepted answer.
5. **B1396 remains open:** config finalizer received 116 evidence rows and cited 2. The excessive same-file/context enrichment contributes model load and oracle noise, but will be fixed as a separate typed context-selection batch rather than by token/time cutoffs.
