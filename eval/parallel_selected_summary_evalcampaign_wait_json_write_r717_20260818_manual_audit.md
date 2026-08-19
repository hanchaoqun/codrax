# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T05:16:44Z
- sweep_start_ts: 20260818-221643
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260818-221644 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 143s | 34 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | Runner false negative: the answer says the policy ceiling cannot be bound to target running slices; the oracle omitted the Chinese equivalent `绑定`. State totals and bounded frequency conclusion are correct. B1138 production-positive: no zero-roster→no-wait/no-blocking expansion. New B1142/P1: model still labels scheduler-marked `io_wait=0` as generic “IO 等待为 0”, while the same prompt publishes a completion-closed S-state IO-wait lower bound of >=4 occurrences/>=4.384ms. These calibers must be named separately. Analyzer breadth swing is gone and trace_query 12→5/read 6→0, but one unrelated verbatim target-source quote retry remains. |
| 2 | github_issue_tokenizers_newline_run_multirepo_py | PASS | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260818-221644 | log_regex,write_apply,answer_regex,answer_contains | none | 672s | 25 | read=12,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Correct scoped write. First plan collapsed a single newline and its model-authored probe failed; controller did not sign green from the passing suite, replanned, preserved the five-newline test, and fixed the 2+-run guard. Second plan's single/even/odd/no-rule probes and `make check` pass; changed path is target_behavior covered, final verdict verified, worktree clean. No read/trace regression or answer substitution. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### real_trace_h4_supply_thermal_witness

- The runner failure is an oracle false negative. The principal answer says the observed CPU 4 policy ceiling cannot be bound to the target's running slices and therefore cannot prove target impact. Add only the semantic synonym `绑定`; retain the CPU 4/2.10GHz and unproven requirements.
- B1138 is production-positive. The final preserves Sleep=70.338ms and never expands the zero D/IO roster into “no wait” or “no blocking”. The preview carries the exact include/exclude formula.
- B1140 no longer exhibits the causal/bounded three-attempt swing. Analyzer takes two attempts only because its first target source quote adds artifact-derived `(17267)` that is absent from the current request. Exploration falls from 12 trace queries plus 6 reads to 5 queries and zero reads. The unique target-effect repair branch was not exercised because this model labeled the visible dimension `observed_value`; keep its production closure pending rather than claiming a direct positive.
- B1142/P1 is newly confirmed. The final says “D 状态和 IO 等待均为 0ms” from the scheduler-state account, while the typed completion bridge on the same page proves at least four completion-closed issuer waits totaling at least 4.384ms in S state. These are compatible measurements only if the first is explicitly named scheduler-marked D/io_wait and the second remains a separate completion-closed S-state IO-wait lower bound. A bare “IO wait=0” is false at the reader level.
- No full Trace causal projection is expected for this finite facts/effect request. The completion-closed IO relation is requested wait evidence, not permission to rank an off-chain background row as root cause.

### github_issue_tokenizers_newline_run_multirepo_py

- The first applied plan incorrectly collapsed a single newline. Its verification probe exited 1 while `make check` passed; controller correctly treated the model-authored comparator as non-authoritative for defect proof but did not finalize, and issued a second plan.
- The second plan adds the structural `i+1` guard, preserves the existing five-newline regression unchanged, and leaves ordinary BPE merging intact. Single newline, four newline, five newline, and missing-rule probes all pass; project `make check` also passes.
- Final report is `verified/all_batches_verified`, only `fastlex/tokenizer.py` changed, target behavior coverage is explicit, and worktree audit is clean. The 672s cost is real but reflects a correctly caught semantic boundary bug, not retry-contract conflict.

### Invariants

- No final-answer prose scan/rewrite, model conclusion substitution, or active-stream fixed-age degradation was used.
- Root-cause Trace requests still require typed on-chain evidence and keep causal projection/automatic supplement; finite facts remain finite.
- Write mode preserves isolated worktree, deterministic verification, replan after a real failed probe, and no automatic merge to main.
