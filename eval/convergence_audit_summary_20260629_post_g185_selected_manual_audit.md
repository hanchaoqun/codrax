# Selected Eval Manual Audit Scaffold

- date: 2026-06-29T11:59:24Z
- sweep_start_ts: 20260629-195924
- total cases: 6
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260629-195925 | typed_inventory_rowset,dimension_substring,answer_contains | none | 143s | 19 | read=0,repo_map=4,list=1,trace=0,source_lens=4 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Correct requested rows for extend/foreign_func/public_class, including `demo.stringext`, `demo.ffi`, and `demo.greeter`. Residual soft noise: broad-budget caveat/status wording still suggests verification instability after exact row-set closure. |
| 2 | arkts_repomap | PASS | eval/results/arkts_repomap-20260629-195925 | typed_inventory_rowset,answer_contains | none | 166s | 26 | read=6,repo_map=0,list=2,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | Final answer satisfies the ArkTS page/builder inventory, but the path is architecturally weak: no `repo_map`/`source_inventory`, broad `list_files`/`grep`/`read_file` did the work. Track as source-inventory navigation cutover gap, not a correctness failure. |
| 3 | qf_relation_subagent_registry | PASS | eval/results/qf_relation_subagent_registry-20260629-200149 | answer_regex,answer_contains | none | 103s | 27 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Correctly identifies `explorer` as the registered subagent. Tool path is healthy (`repo_map` first-hop present). Residual UX noise: final answer has redundant supplemental sections. |
| 4 | read_combo_trace_current_source_explanation | FAIL | eval/results/read_combo_trace_current_source_explanation-20260629-200211 | trace_attachment,answer_regex | perf_triage+trace_query | 173s | 35 | read=5,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Substantive explanation mentions current source mechanisms (`parse.go`, `flavor.go`, `query.go`) but emitted zero citations, so current-source evidence was lost at finalization. Root gap: runtime+current-source aggregate producer boundary lets source mechanism facts fall into runtime-only aggregate lane. |
| 6 | patch_cpp_typo | PASS | eval/results/patch_cpp_typo-20260629-200504 | write_plan,write_patch_oracle | none | 82s | 16 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Plan targets the expected C++ typo. One local `emit_change_plan` retry repaired an `old_text` mismatch; not a functional failure. |
| 5 | read_combo_log_current_source_explanation | PASS | eval/results/read_combo_log_current_source_explanation-20260629-200332 | log_attachment,answer_regex | log_triage | 215s | 27 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Correct mixed log/current-source answer with current-source anchors. Residual noise: mermaid repair and two completion lanes fired, worth tracking under form/UX churn rather than correctness. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
