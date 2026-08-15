# Selected Eval Manual Audit Scaffold

- date: 2026-08-15T16:57:18Z
- sweep_start_ts: 20260815-095717
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_python_typo | PASS | eval/results/patch_python_typo-20260815-095718 | write_plan,write_patch_oracle | none | 69s | 24 | read=1,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Plan-only lane correctly selected one `patch` operation for `main.py`, preserving the requested one-line `retrun` → `return` scope. No apply or runtime verification was requested or falsely claimed. |
| 1 | qf_relation_subagent_registry | PASS | eval/results/qf_relation_subagent_registry-20260815-095718 | answer_regex,answer_contains | none | 112s | 27 | read=4,repo_map=2,list=0,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | The principal fact is correct (`1`, member `explorer`) and the model used the real registration/identity chain, but the accepted member set still arrived as `fact_authority=advisory_model_inference`; the final prompt said no authorized relation set was active and publication appended a weak-evidence caveat. A second independent failure attached an unrelated `internal/hitraceconv/...:807` citation to the patched count block. Runner regexes do not cover either defect. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. B844 was not production-closed by the r516 patch. The deterministic
   `bridge_literal + bridge_literal_terminal` pair is built in
   `Explorer.ParseOutput`, after `emit_investigation_complete` had already
   classified the accepted aggregate fact. The exact provider therefore existed
   but never ran against the post-explore pair before Turn-A/finalizer handoff.
2. The real terminal companion also shares its semantic StableEvidenceID with
   the ordinary `concrete_values` return row. The coverage collector used the
   coarser ID as a first-wins key, so even a post-explore refresh would be order
   dependent unless the typed amendment survives and the purpose-built terminal
   wins companion lookup deterministically.
3. The final count block was appended through `emit_answer_document_patch` with
   `citation_ref=0`, then rendered against an unrelated hitrace conversion line.
   This is not a model factual error: the visible scalar stayed `1`, but patch
   citation identity was rebound to an unrelated pool row. Track separately as
   B846; B844's authority fix may avoid this exact patch path but does not prove
   the generic citation remap safe.
