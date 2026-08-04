# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T03:28:43Z
- sweep_start_ts: 20260803-202842
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap_fixture | PASS | eval/results/cangjie_repomap_fixture-20260803-202843 | dimension_substring,answer_contains | none | 95s | 19 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | pass | `source_inventory` 完整列出 1 个 extend、1 个 foreign func、3 个 public class；文件、行号和声明 package 均准确。一次成文拒绝把同名 `Cart` class/extend 行误判成主清单外条目，patch 后答案仍保留两条，属校验器 identity 观察项。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260803-202843 | answer_regex,answer_contains | none | 240s | 26 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=9,inv=4/0,fin_reject=1,unavail=0,prune=0 | fail | Analyzer 将 `AnalysisIR`（仅为 `buildAnalysisIR` 的词尾/派生上下文）误记为第三个用户实体；principal span 因而选成 `buildAnalysisIR -> AnalysisIR`，连续三次补读错误跨度。最终仍把 `gate.RunWith` 说成 `gate.Run` 的“实际/底层入口”，真实有向路径不存在。runner oracle 再次 false green。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
