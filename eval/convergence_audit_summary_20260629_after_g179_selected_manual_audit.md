# Selected Eval Manual Audit Scaffold

- date: 2026-06-29T11:19:22Z
- sweep_start_ts: 20260629-191922
- total cases: 6
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | arkts_repomap | PASS | eval/results/arkts_repomap-20260629-191922 | typed_inventory_rowset,answer_contains | none | 98s | 18 | read=6,repo_map=1,list=1,trace=0,source_lens=1 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Final answer lists 4 `@Entry` components and 2 `@Builder` fragments with file:line citations. It correctly excludes `EntryAbility` because the source comment says it has no class decorator. No finalizer reject; source_inventory is used as one bounded navigation pass, then read_file verifies evidence. |
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260629-191922 | typed_inventory_rowset,dimension_substring,answer_contains | none | 121s | 20 | read=4,repo_map=3,list=0,trace=0,source_lens=3 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Machine PASS was false-positive before audit: final visible answer said `public class 8` but listed only 7 rows and dropped `Cart @ eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj:14`. Root cause was section-insensitive eval row matching plus production duplicate-label citation repair using the `extend Cart` row. Fixed by section-scoped eval row oracle and scoped source-inventory citation repair; rerun `eval/results/cangjie_repomap-20260629-193450` PASS and human-pass, with residual efficiency flags only (`wall=192s`, `midloop=6`, `finalizer_rejects=2`). |
| 3 | qf_relation_subagent_registry | PASS | eval/results/qf_relation_subagent_registry-20260629-192100 | answer_regex,answer_contains | none | 118s | 26 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Final answer resolves the default registered SubAgent member set to the single `explorer` implementation and cites registry/name evidence. `repo_map=2` is appropriate for navigation; no source_inventory loop or finalizer reject. Some generated supplement text repeats broader command/root anchors, but it does not change the principal conclusion. |
| 4 | trace_query_openharmony_bytrace_thread | PASS | eval/results/trace_query_openharmony_bytrace_thread-20260629-192124 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 171s | 27 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Trace-first behavior is correct: uses perf_triage and 5 trace_query calls, no repo_map distraction. Final answer explains the ACCS0/Binder wakeup relation, time unit semantics, and OpenHarmony priority interpretation. Mermaid source repair was applied once and is presentation-only. |
| 5 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260629-192259 | trace_attachment,answer_regex | perf_triage | 153s | 40 | read=7,repo_map=0,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Final answer correctly separates runtime artifact observations from current-source parsing evidence, explains HiTrace B/E span parsing, seconds-to-ms conversion, 16.67ms frame budget, and evidence boundary. Residual watch: context reached 40% with 4 midloop injections, so this remains an efficiency/noise case, not correctness fail. |
| 6 | patch_go_typo | PASS | eval/results/patch_go_typo-20260629-192415 | write_apply,write_patch_oracle,answer_contains | none | 118s | 16 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Write Auto Pilot produced a one-file patch changing `retrun` to `return`, applied it in the isolated worktree, and verified with `go test -json ./...`. The scratch repo file remains original by design; the summary exposes the post-apply worktree content and applied diff. Planning still needed 7 iterations because verification-probe constraints for `main` packages are strict; track under write-mode planning efficiency, not correctness. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
