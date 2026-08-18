# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T04:21:47Z
- sweep_start_ts: 20260817-212143
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260817-212147 | answer_regex | none | 192s | 28 | read=2,repo_map=2,list=1,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=3,unavail=0,prune=0 | partial | B1041-2 production closed: analyzer retained source=`FastTokenizer.tokenize` in discover_terminal mode and final context used it. The final answer still has two evidence-use defects: it says `_tokenize_slow` is absent although repo-map/pre-read showed its definition at lines 24–36, and the visible diagram leaves the typed `_fastlex.tokenize_bytes -> py.tokenize_bytes` registered-export handoff as a note rather than a relation edge. Three finalizer rejects show relation authoring remains too costly. |
| 2 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260817-212147 | write_apply,answer_regex | none | 195s | 25 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass-with-unverified-boundary | Implementation and falsy-value regressions are correct and `make check` passed. The final unverified verdict is honest because this fixture supplies source-static verification and the host has no Node runtime. The deterministic terminal renderer still leaked `production_verification_source_static_only` to the reader. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch finding

- `B1041-CALLCHAINSINGLESOURCEWIRE1=production-closed/r663`: the exact current-request method identity survived enum normalization and provenance. There was no second demotion to authority-free `discover_path`; finalizer received `grounded source endpoint: FastTokenizer.tokenize` and `destination mode: discover_terminal`.
- `B1042-WRITETERMINALREASONLANGUAGE1=P1/confirmed`: the write terminal card renders raw workflow `ReasonCode` values even though the structured plan/report already preserves them. The reader needs the reason's meaning, not the transport token. General fix: localize typed reason codes at this deterministic rendering seam, keep raw codes only in structured artifacts, and use a generic reader-facing fallback for future unknown codes.
- `B1043-REQUESTEDSUBTOPICEVIDENCECLOSURE1=P1/confirmed`: the analyzer created an explicit fallback-behavior subtopic and the pre-read showed `FastTokenizer._tokenize_slow` at lines 24–36, but the evidence emission carried only its call site and the final answer falsely said the implementation was outside the workspace. The completion gate needs a language-neutral requested-subtopic evidence obligation, not a Python method-name special case.
- `B1044-VISIBLENONCALLHANDOFF1=P1/confirmed`: the principal carrier selected both sides of a typed registered-export handoff. The contract eventually required hidden `relation_kind=register` metadata but allowed the visible diagram to represent that bridge only as a note. A relation requested by the user is still visually missing. The generalized repair must treat typed non-call handoffs as first-class visible edges without relabeling them as calls or drawing them on the model's behalf.
- No malformed-JSON recovery, blank answer, active-stream fixed-4ms degradation, Trace mutation, or system-authored conclusion occurred in this sweep.
