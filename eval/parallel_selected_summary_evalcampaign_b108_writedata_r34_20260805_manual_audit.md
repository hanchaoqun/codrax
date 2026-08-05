# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T12:50:29Z
- sweep_start_ts: 20260805-055027
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_memoclaw_text_search_multirepo_ts | FAIL | eval/results/github_issue_memoclaw_text_search_multirepo_ts-20260805-055029 | log_regex,write_apply,write_patch_oracle | none | 205s | 20 | read=5,repo_map=3,list=2,trace=0,source_lens=1 | midloop=2,inv=0/0,fin_reject=0,unavail=2,prune=0 | pass (patch) / fail (delivery) | Patch correctly changes GET `/v1/memories/search` to POST `/v1/search`, sends the required JSON body and preserves optional namespace; `make check` passes and unrelated files remain unchanged. Delivery is nevertheless typed `unverified:proof_weak`: the declared check is honestly classified `source_static`, but the hard behavior contracts never become proof-ledger obligations, so the controller has no typed item from which to schedule a target-behavior verification follow-up. |
| 2 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260805-055029 | log_regex,answer_regex | none | 411s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Intermediate answers repeatedly reach the expected `17,0,5`, but the contribution ledger incorrectly drops source GroupB=4 and inserts a synthetic target T2=0 before reconciliation. The typed reference-grounding guard correctly detects the wrong ledger domain, then issues a contradictory repair contract: it reports that contributions must be recomputed while allowing only `assemble_answer` and telling the model not to change contribution records. Eighteen rounds oscillate among reference keys; an earlier field-contract diagnostic also remains sticky after later successful progress. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Generic findings and disposition

- `EVAL-B108-WRITEPROOF1` (P0): proof classification and repair scheduling consume different typed domains. A hard required behavior contract plus zero `target_behavior` paths is sufficient to make the proof weak, but the proof ledger has no missing contract row unless a separate patch-review, impact-analysis, or verification-confidence producer happened to emit one. Fix by deriving a missing contract obligation directly from the normalized plan contract set; never promote `source_static` evidence.
- `EVAL-B108-DATAREF1` (P0): `reference_ledger_domain_mismatch` is a typed upstream-computation defect but the output graph and guard route it into the downstream `assemble_answer` lane. Split it from slot/cardinality presentation mismatch: domain mismatch must reopen compute contributions, replace the stale contribution generation, reconcile, then assemble.
- `EVAL-B108-DATASTALE1` (P1): derived field-contract issues scan the entire history and survive unrelated typed successful progress. Retire only issues older than the latest successful-progress boundary, while retaining issues derived from that latest artifact.
- Budget is not the primary fix. Increasing rounds while the allowed-action contract contradicts its own diagnosis only lengthens oscillation. Reconsider budget after the typed repair lane converges.
