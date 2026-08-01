# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T01:59:41Z
- sweep_start_ts: 20260731-185939
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260731-185941 | log_regex,trace_attachment,answer_contains,principal_answer | perf_triage+trace_query | 194s | 40 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | FQ1 的确定性目标已覆盖：最终答案第一个可见块就是 system-owned frequency authority，先于模型摘要，完整发布 CPU0/CPU4 limit witness、ceiling presence、binding/impact 与 thermal/policy mechanism 边界；显式窗的完整 Trace 因果投影、主根因、链/唤醒读法和 58.320ms 可消除供给折算均在。整体人工仍判 fail：模型正文继续写“结论：受限”“CPU12 受热节流约束”，没有复述 typed disclaimer，且把窗尾 runnable 误写成计入 running。由于领先 authority 已明确覆盖冲突，本残余归为 model-prose variance，不增加答案关键词 hard gate。 |
| 2 | read_combo_config_absent_present_mix | PASS | eval/results/read_combo_config_absent_present_mix-20260731-185941 | answer_regex,answer_contains | none | 239s | 24 | read=4,repo_map=3,list=0,trace=0,source_lens=3 | midloop=5,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | 两键没有串值，真实键的 example=0 与 runtime.go:1035 锚点正确；但三层 absence 证明越权。唯一 accepted negative EvidenceItem 只验证 internal/config/runtime.go 文件；cmd 的 no-match 只停留在 typed grep result，YAML 仅读取了另一个键的正值行。completion 却用 member_set + `runtime.go:0`、正值 `codrax.yaml.example:1171`、裸 `cmd` 三个 support_ref 宣布三层均 absent，最终答案还声称 YAML 注释覆盖能证明 CLI 不存在。runner 的字面 oracle 未发现这个 target/scope 绑定缺口。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Findings

1. `EVAL-B15-FQ1` 可转 covered：生产 HTML 顺序已由真实 H4 验证，且
   `trace_query_final_projection_blocks=2` 固定显式窗因果能力无回退。模型正文
   仍可冲突，但系统 authority 已在最前面声明优先级；继续围绕“热节流”等词面
   增加拒绝器会违反本战役约束。
2. `EVAL-B15-NEG1`（P1）是新的泛化 GAP：typed negative proof 有 target 与
   scope，但 no-match `ToolPathDiscovery`、negative `EvidenceItem`、completion
   aggregate 和最终可见结论之间没有共享的 scope authority。一个局部文件
   negative 可以被 model-authored member_set 扩写成多文件/多层全局 absence。
3. `EVAL-B15-XR1`（P2）是既有长期 GAP 的新 witness：分析器把双 config-key
   exact lookup 分类成 source inventory，未发布 `exact_targets` /
   `exact_context_roles`；document-level exact_resolution 也无法同时表达
   target A absent、target B present。该项需要 per-target target_ref/role
   设计，不应以本 case 关键词做特判。
