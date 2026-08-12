# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T20:00:35Z
- sweep_start_ts: 20260812-130033
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_python_typo | PASS | eval/results/patch_python_typo-20260812-130035 | write_plan,write_patch_oracle | none | 57s | 23 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | ChangePlan is correctly limited to `main.py:20`, changes only `retrun` to `return`, retains the surrounding function, and carries import plus return-type probes. No speculative file or unrelated edit entered the plan. |
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260812-130035 | answer_regex | none | 202s | 23 | read=3,repo_map=1,list=0,trace=0,source_lens=1 | midloop=7,inv=1/0,fin_reject=3,unavail=0,prune=0 | fail | B667 is production-positive: Rust line 42 is now a typed `py.tokenize_bytes -> tokenize_bytes` call and remains in the answer. The real boundary still fails earlier: the model submitted the PyO3 module definition as a sparse registration instead of the actual line-47 `m.add_function(wrap_pyfunction!(...))` binding; schema feedback used irrelevant factory-only wording and the optional-diagram completion lane let this typed debt disappear. Finalizer then rejected three invented Native→Rust call arrows and accepted a two-component diagram. First answer JSON also quoted `blocks`; tolerant recovery preserved it, so there was no blank answer, but the avoidable retries confirm B669. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch verdict

- Runner: `2/2 PASS`; human: `1/2 PASS`.
- The write-mode case is clean and confirms that this relation work has not
  disturbed a basic plan-only lane.
- The polyglot case is not a generic model fluctuation. The model found the
  exact binding expression at `core-rs/src/lib.rs:47`, but the system did not
  keep the failed typed registration row actionable. Existing strict call
  validation then correctly refused to turn a registration boundary into an
  invocation. The remaining break is therefore evidence transport and diagram
  authoring input, not a reason to weaken the call-edge gate.
- The first Finalizer payload encoded `blocks` as a string. Local JSON recovery
  successfully recovered useful content and explicitly recorded one recovery
  event; the answer was not replaced by system-authored prose. The three later
  rejects came from the missing typed binding relationship, not malformed JSON.

## GAP and repair batch

### B669 — registered boundary continuity (`P1`, implemented)

Root cause has two general layers:

1. `emit_evidence` taught every missing registration endpoint using a
   factory-specific guard/return sentence. For a registry call or native export
   binding that advice is wrong cognitive load: the repair must point back to
   the exact binding expression and its slot/target endpoints.
2. A schema-invalid registration row blocked completion only when a required
   diagram contract happened to be active. A cross-component call-chain could
   therefore ignore the model's own load-bearing typed binding attempt when the
   diagram was optional, even though doing so necessarily stranded otherwise
   proved invocation segments.

Implemented general repair:

- registration JSON teaching now uses one compact source-shape example:
  `registry.add(wrapper(target))` maps to `registration/call`, registry slot,
  and bound target. Factory selection remains a separate conditional+return
  form rather than the universal error message;
- a schema-invalid registration explicitly submitted in a typed
  cross-component `QFCallChain` remains an action-required completion debt even
  without a required diagram. This reads only request schema plus the submitted
  evidence kind; Runtime Trace is excluded and no prose is scanned;
- when an exact registered-export join already exists, the same typed relation
  capsule now gives a sequence diagram a `Note over export,callable` carrier.
  It never adds an arrow or edge anchor, never calls registration an invocation,
  and leaves visible business wording plus the conclusion to the model;
- endpoint resolution is unique-or-none. Ambiguous aliases emit no Note. The
  implementation reuses the existing language-neutral registration claim, so
  it applies to PyO3, JNI/FFI/native modules, plugin registries, generated RPC
  bindings, and future languages without framework-name matching.

Status: `B669-REGISTEREDBRIDGEDIAGRAMCONTINUITY1=implemented/targeted+core-pass/pending-production-replay`.

### B664 live-stream red-line recheck

r403 again exercised the production streaming adapters. Analyzer terminal
requests logged `llm_request_budget skipped ... ownership=stream_first_byte_and_byte_stall_watchdogs`.
There is no `4ms`, four-second, or cumulative-age degradation authority: a
byte-producing stream remains live regardless of total duration. Only explicit
caller cancellation/deadline, no-first-byte timeout, byte-stall timeout,
transport failure, or decode failure may terminate/recover. The 202-second
case finished with `runtime=none`; its churn was typed answer validation, not
stream-age fallback.
