# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T16:34:16Z
- sweep_start_ts: 20260828-093416
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | log_path_question_multi_runtime_files | PASS | eval/results/log_path_question_multi_runtime_files-20260828-093416 | answer_regex,answer_contains | none | 157s | 26 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=2,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | B1386 is production-positive: the first analyzer payload used a combined, non-verbatim exclusion quote and was precisely rejected; the second copied the exact user phrase `不分析代码`, preserving `external_observation_policy=exclude`, and no repository source was read. A separate existing repair then changed the incompatible `root_cause + bounded_fact_set` pair to `causal_diagnosis`; this was not a B1386 loop or a contradictory answer contract. B1387 reached both explorer and finalizer, but the answer still promoted observations into unsupported mechanism: `Get(0x0, …)` proves the receiver value at that frame, not that `ServeHTTP` supplied it or that the method accessed a field; `context deadline exceeded` at `Fetch` does not prove response waiting, internal throwing, or caller/upstream policy. Repeated failure after the shared soft directive confirms a deeper typed observation-vs-inference authority gap (B1388); do not repair it by scanning or rewriting final prose. |
| 2 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260828-093416 | answer_regex,answer_contains | none | 192s | 28 | read=12,repo_map=1,list=0,trace=0,source_lens=1 | midloop=9,inv=4/1,fin_reject=0,unavail=0,prune=0 | partial | The default 50, YAML overlay, explicit CLI override, and final `SetMaxSteps` injection are correct. The parsing-mechanism answer is false: the repository has no Viper dependency; `LoadRuntimeSettings` reads bytes and uses `gopkg.in/yaml.v3` via `yaml.NewDecoder(...).KnownFields(true)` before the caller overlays the resulting pointer field. Explorer never read that loader even though “解析机制” was a typed requested dimension, yet completion was accepted and the finalizer filled the missing mechanism with a familiar library. The lone visible citation also cannot support the multi-hop claims, and `codrax.yaml.example（cmd/root.go:485）` misattributes the example file to `cmd/root.go`. Record B1389 as requested-mechanism evidence-closure failure; the fix must use typed dimension/provider evidence, not the literal word Viper or final-answer text. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
