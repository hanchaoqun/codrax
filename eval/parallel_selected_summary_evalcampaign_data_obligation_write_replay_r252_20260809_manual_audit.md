# Selected Eval Manual Audit Scaffold

- date: 2026-08-09T10:39:33Z
- sweep_start_ts: 20260809-033932
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_java_typo | PASS | eval/results/patch_java_typo-20260809-033933 | write_plan,write_patch_oracle | none | 52s | 21 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Plan-only result identifies `Main.greet` and the exact one-line `retrun` → `return` patch at `Main.java:16`; acceptance names `javac`/`java Main` without falsely claiming execution. No source apply or cross-mode mutation. |
| 1 | data_basic_sum_with_rules | PASS | eval/results/data_basic_sum_with_rules-20260809-033934 | log_regex,answer_regex | none | 292s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass-answer / fail-process | Final output is exactly `17`. B428 is production-effective: `compute_contributions` remains the published next action and no self-minted decision prerequisite recurs. Process still takes 10 data batches, 5 repair rounds and 3 failed action batches: action-level `input_paths` is absent despite exact artifacts being available; repeated failure fallback also detours through a same-source `normalize_entities` relation and emits 4 irrelevant resolutions. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- Answer correctness: 2/2 pass.
- Process correctness: Java pass; data fail.
- `EVAL-B428-ACTIONSELFPRECONDITION1`: production-effective. The old `action_outside_allowed_next_stage` self-conflict is absent.
- `EVAL-B430-ACTIONINPUTCARRIER1`: P1 confirmed. Typed capability requires action-local inputs, while the emitted tool schema required only `kind` and action scaffolds taught singular `input_path`; admission alone required plural `input_paths`, creating a repeated repair loop.
- `EVAL-B431-RELATIONLINEAGEOVERLAP1`: P1 confirmed. Deterministic fallback treated two artifacts with overlapping `orders.csv` lineage as an independent source/reference pair merely because each side also carried a different extra lineage root; it ran irrelevant entity normalization for a scalar sum.
- No raw user/model prose hard gate or system answer rewrite was observed. Trace surfaces were not exercised or changed; the on-chain-only root-cause authority remains an invariant for subsequent trace batches.
