# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T10:18:13Z
- sweep_start_ts: 20260818-031812
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | read_combo_loose_multi_question_units | PASS | eval/results/read_combo_loose_multi_question_units-20260818-031813 | answer_regex,answer_contains | none | 197s | 37 | read=9,repo_map=2,list=0,trace=0,source_lens=0 | midloop=5,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | 两个问题分节清楚，配置路径查找、KnownFields/UnknownKeys、Mermaid 四 outcome、降级 fence 和统计入口大体有源码支撑，真正 mechanism 请求也未被 B1066 提前关闭。但“配置如何加载和覆盖”只讲路径定位与 YAML decode，没有追到默认值→YAML→CLI 的实际应用/覆盖链；摘要又说 Rendered/FallbackRune/UnsupportedKind 均“保留原内容”，与正文“Rendered/FallbackRune 生成 ASCII”矛盾。typed member notes 的 support authority 已标 definition-site-only/executable-body-unproven，模型仍直接复述行为语义，系统只追加泛化矛盾附注，归入 B1064 的 member-local composite support 根因。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260818-031813 | answer_regex,answer_contains | none | 253s | 31 | read=4,repo_map=0,list=0,trace=0,source_lens=0 | midloop=5,inv=3/0,fin_reject=3,unavail=0,prune=0 | partial | B1066 生产覆盖：相对 r678 从 641s/45 Explorer/30 read/13 completion 降至 253s/12 Explorer/4 read/3 completion，且没有 mechanism semantic-descent helper 前沿。最终 Mermaid 合法、只画 `buildAnalysisIR→RunWith←Run` 两条已证边，没有伪造到 `gate.Run` 的有向路径，并列出内部检查；但开头仍写“从 buildAnalysisIR 到 gate.Run 所经过”，没有用一句面向用户的结论明确说明 no-directed-path，因此内容正确但表达 partial。首次成文拒绝已一次并列 endpoint coverage、standalone owner/identity 与 diagram anchor 修复，B1065 covered；后两次分别是模型 citation patch 错位和重复 replace block id。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Gap disposition

- `B1065-RELATIONVALIDATIONBATCHFEEDBACK1`: production-covered. The first QF rejection returned the independently computable endpoint-boundary, standalone relation ownership/identity, and diagram-anchor corrections together. Later rejects were new model patch mistakes, not hidden serial validator feedback.
- `B1066-CALLCHAINMECHANISMDESCENTFRONTIER1`: production-covered. QF wall time fell 60.5%, Explorer iterations 45→12, reads 30→4, and completion calls 13→3. The remaining completion retries resolve the real typed no-directed-path boundary (`buildAnalysisIR -> RunWith <- Run`) rather than recursively reading unrelated helpers.
- `B1064-PRINCIPALMEMBERCOMPOSITESUPPORT1`: broadened by the mechanism guard case. A model-authored member note can claim executable outcome semantics while its typed support authority is definition-site-only; the finalizer receives that limitation but lacks a member-local body-backed composite support set and lets the prose through with only a vague contradiction caveat. Treat this as the same generalized support-authority gap, not a Mermaid-outcome special case.
- JSON/patch teaching remains necessary: one QF retry duplicated the same replacement block id, but the mutation layer rejected it precisely and no old draft or empty answer was published.
- No runtime trace code changed or ran. Explicit-window Trace causal projection, typed-on-chain-only root causes, background-only adjacent evidence, auto-supplement, and the active-stream fixed-4ms no-degrade rule remain unchanged.
