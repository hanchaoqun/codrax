# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T02:16:54Z
- sweep_start_ts: 20260805-191653
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_go_typo | PASS | eval/results/patch_go_typo-20260805-191654 | write_apply,write_patch_oracle,answer_contains | none | 107s | 20 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Exact one-line `retrun→return` structured patch and `go test ./...` both passed. Analyzer completed in one attempt, closing the previous field-value carrier churn. Planner still spent two avoidable retries: it represented the invariant “go.mod/go.sum unchanged” as an empty `go.mod` patch, then copied the changed implementation into a standalone probe before switching to a valid same-package probe. Hard validators correctly rejected both; generic mutation-only and project-runner-first soft teaching is required. |
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260805-191654 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 158s | 30 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=3/2,fin_reject=0,unavail=0,prune=0 | fail | Explicit `5.000..5.007` window, target scoping, auto-supplement, causal projection, rank board, wakeup path, and both actual-occupancy/existing-rule-eliminable axes were preserved. The model-authored lead nevertheless states a definite dropped-frame cause and later says the dependency “caused app-100 to be blocked”, while typed authority and the same answer's caveat say `causal_conclusion=unproven`, `frame_evidence_status=absent`. The system did not replace the model answer; this is a model-owned causal-caliber contradiction despite precise prompt context. Two investigation-completion retries also came from unnecessary model-authored relation claims and a missing typed runtime-artifact origin. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case judgment

- JSON/schema teaching: neither case lost its final answer and neither finalizer emitted malformed JSON. The Trace completion retries were valid typed-schema rejections, not lossy JSON recovery. The system should reduce optional carrier choices, but must not recover a disputed causal conclusion by mining prose.
- Trace authority: system-generated blocks remained separately marked and only disclosed typed measurements/coverage; they did not rewrite the model-authored conclusion. The user-visible contradiction is therefore real and cannot be called a projection/materialization pass merely because the runner oracle passed.
- Next structural work: add a model-authored typed causal-caliber carrier for the principal Trace synthesis and compare that carrier with typed Trace authority. The carrier must not be inferred from answer wording and the system must not author or replace the conclusion. A one-witness soft prompt tweak alone is insufficient evidence for a new prose hard gate.
