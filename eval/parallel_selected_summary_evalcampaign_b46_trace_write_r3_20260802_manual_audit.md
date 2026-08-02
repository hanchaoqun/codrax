# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T19:41:40Z
- sweep_start_ts: 20260802-124138
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_memoclaw_text_search_multirepo_py | PASS | eval/results/github_issue_memoclaw_text_search_multirepo_py-20260802-124140 | log_regex,write_apply,write_patch_oracle | none | 198s | 19 | read=9,repo_map=3,list=0,trace=0,source_lens=1 | midloop=2,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Patch only changes `memoclaw/client.py`, preserving sync/async signatures while replacing stale GET/query-string routing with POST `/v1/search` + JSON. Real `make check` ran from `declared_coverage_test_surface`, printed `python text search contract ok`, and the report now records `verification_status=passed` plus exact Python changed-path coverage. Meta-runner execution authority is closed without treating the declared roster alone as proof. |
| 1 | real_trace_h5_smr_multirow_disposition | FAIL | eval/results/real_trace_h5_smr_multirow_disposition-20260802-124140 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 284s | 40 | read=1,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | The runner failure is only the stale exact phrase `等待对象 dma_fence_default_w`; the symbol is present in the model-visible typed projection and final projection. Explicit window, four windowed queries, system supplement, two-axis occupancy/eliminable analysis, ranking, wakeup path and full projection remain. OCCUNIT1 is closed: page-cache count-equivalent is absent from the wall-clock occupancy table and retained in the caliber sidebar. The model no longer claims thermal throttling. However REL2 is not closed: despite `relation_authority=typed_pair_only`, the answer still upgrades missing carriers to “independent”, “mutually exclusive”, “physically non-overlapping”, and additive claims. Worse, the handoff itself labels every positive `DStateSplitMS/IOWaitSplitMS` as `embedded_components=already_inside_parent_row`; those fields are a row state breakdown and only establish a cross-row containment relation under the renderer's additional exact pair conjunction. The misleading system context contributed to the false CompThread running/D-state containment claim. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human verdict

- Runner: 1/2 PASS. Human: 1/2 PASS.
- `EVAL-B46-XMAKE2`: covered by a production replay witness.
- `EVAL-B46-OCCUNIT1`: covered by a production replay witness.
- `EVAL-B46-REMEDY1`: remains covered; the model correctly treats 58.320ms as compute-delivery headroom and does not promote a policy/rail ceiling to proven thermal throttling.
- `EVAL-B46-REL2`: reopened as a P1 context-authority defect, not merely model variance. The negative prompt instruction arrived, but a misleading system-minted pseudo relation and the absence of a concise exact pair roster left the model with contradictory context.
- `EVAL-B46-ORACLE1`: still eval debt only. Production must not be changed to force the phrase `等待对象`.

## Context sufficiency audit

The Trace context is abundant but not fully precise. Values, units, windows, ranking, wakeup evidence and remedy ceilings are sufficient; the relation layer is not. `DStateSplitMS/IOWaitSplitMS` describe a row's own state breakdown. They are not, on their own, proof that a separately rendered D/IO row is contained in that row. The post-finalize renderer uses extra exact pair predicates before emitting such a relation, while the pre-finalizer handoff skipped those predicates. The architectural fix is to remove that pseudo authority immediately and then expose only relation facts minted by the shared typed pair adjudicator. It must remain prompt-only and must not inspect or rewrite the model answer.
