# Representative Eval Gap Tracking

Date: 2026-06-04

## Scope

This ledger records findings from the representative eval sweep under:

`eval/results/representative-20260604-145414`

The sweep ran 12 CLI cases in batches of 4 plus focused operation E2E Go tests.
The purpose of this document is to separate real product gaps from external
provider failures, then design generalized fixes that do not rely on prose
keyword matching or one-off case patches.

## Findings

### F1. Multi-repo focus can miss exact anchors

- Case: `mr_focus_single`
- Symptom: no explicit user focus; selector chose `repo-tools-py` because
  `unrelated_constant` looked Python-like, while the exact definition lives in
  `repo-stub-rust/src/lib.rs`.
- User-visible result: bounded absence answer from the wrong active set.
- Logs: selector prompt explicitly prohibited reading files, so the model had
  only repo names, primary languages, and file counts.

Root cause:

The current no-focus multi-repo selector asks a model to choose sub-repos from
compact topology alone. For exact symbol/config/file-anchor lookups, topology is
insufficient: language naming conventions are noisy and can select the wrong
sub-repo before the normal analyzer/explorer has a chance to search. The later
active-set gate then correctly refuses out-of-set paths, so the wrong initial
scope becomes a false negative.

Generalized solution:

Add a deterministic, bounded exact-anchor pre-scan before the model selector.
It must:

- extract only code-like literal tokens from the current request;
- search those exact literals across sub-repo source-like files using bounded
  I/O;
- route to every sub-repo with exact hits when the hit set fits the active cap;
- otherwise fall back to the existing typed selector/fallback flow;
- mark the source as typed `exact_prescan`, not as a model recommendation.

This is not intent keyword matching: it never classifies the request by prose.
It only uses exact literal occurrences as routing evidence. It is also not final
answer evidence; explorer still has to read/grep the selected files.

### F2. MCP typed-line eval is correct but inefficient

- Case: `mcp_typed_line`
- Symptom: PASS, but metrics show repeated MCP calls plus extra source reads.
- Observed metrics: `mcp_tool_calls=3`, `tool_mcp_read_resource=4`,
  `tool_read_file=6`, `explorer_iters=18`.

Root cause:

The system correctly allows external observations to combine with source code by
default, but MCP-only fixture questions can still drift into source exploration
when the external observation is already enough. The default mixed-analysis rule
is correct; the gap is efficiency and stopping confidence, not correctness.

Generalized solution:

Add future eval/telemetry gates that flag repeated identical external-observation
tool calls and unnecessary source reads for external-only questions. Any hard
route must remain typed, e.g. based on an explicit external-observation-only
policy or a typed classifier signal, never on user prose keywords.

### F3. Several failed cases were blocked by external provider balance

- Cases: `patch_go_typo`, `patch_python_typo`, `arkts_repomap`,
  `hitrace_jank`, `read_combo_log_current_code_dimensions`.
- Symptom: eval verdict `FAIL/no_result`, logs contain LLM API
  `402 insufficient_balance_error`.

Root cause:

The eval harness currently records these as normal failures. That is useful for
rollup accounting, but it hides that the product did not get a usable model
response and therefore the feature was not actually evaluated.

Generalized solution:

Introduce an eval-side external-blocked classification such as
`BLOCKED_PROVIDER` for recognized provider/runtime outages. Keep it out of
product logic. This makes eval summaries actionable without masking product
regressions.

### F4. Trace representative cases passed

- Cases: `trace_query_donghu_mixed_platform`,
  `trace_query_frame_timeline_flow`.
- Status: PASS.

No immediate product fix is required. Continue covering these cases in regular
sweeps because they exercise Harmony/Donghu priority semantics and frame-flow
views.

## Delivery Plan

### Batch 1: Ledger + multi-repo exact-anchor pre-scan

- [x] Record this eval gap ledger.
- [x] Add `exact_prescan` as typed multi-repo focus source.
- [x] Implement bounded exact-anchor token extraction and per-sub-repo scan.
- [x] Apply exact-prescan routing before model selector only when:
  - read mode;
  - multi-repo enabled;
  - no user-pinned focus;
  - hit sub-repo count is non-zero and within cap.
- [x] Add tests for:
  - exact symbol hit chooses the owning sub-repo;
  - no exact hit falls back to model selector/fallback;
  - hit count greater than cap does not silently trim;
  - single-repo and explicit user focus bypass the pre-scan.
- [x] Run focused tests and representative `mr_focus_single`.
- [x] Commit and push this batch.

Validation note: focused Go tests passed for the new routing path. The
representative `mr_focus_single` run reached the corrected route
(`exact focus pre-scan selected ... repo-stub-rust`) but the downstream LLM
provider returned `402 insufficient_balance_error`, so the eval verdict was not
a valid product correctness signal. Batch 2 records that provider-blocked state
explicitly in the eval harness.

### Batch 2: Eval provider-blocked classification

- [x] Add eval summary classification for provider-balance/API outages.
- [x] Preserve raw FAIL/exit data while surfacing `BLOCKED_PROVIDER` in summary.
- [x] Add shell tests for verdict/summary classification.
- [ ] Commit and push this batch.

Validation note: `bash eval/runner_lib_test.sh` passed. A focused
`mr_focus_single` run against the current provider outage now reports
`BLOCKED_PROVIDER insufficient_balance` instead of a normal product `FAIL`.

### Batch 3: MCP efficiency guardrails

- [ ] Add telemetry/audit thresholds for repeated same MCP resource calls.
- [ ] Add advisory-only prompt guidance for external-observation sufficient
  closure, gated by typed external observation policy.
- [ ] Extend eval metrics so PASS-with-inefficiency is visible without changing
  correctness verdicts.
- [ ] Commit and push this batch.

## Red Lines

- Do not parse model prose or user prose keywords into hard routing decisions.
- Do not weaken active-set gates; wrong or inactive paths must still be refused.
- Do not affect single-repo or `multi-repo=false` behavior.
- Do not make MCP/log/trace external observations suppress source exploration
  unless a typed policy says source is excluded or already irrelevant.
- Do not classify provider outages inside product answer logic; keep that in eval
  reporting only.
