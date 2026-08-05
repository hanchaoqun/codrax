# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T09:40:20Z
- sweep_start_ts: 20260805-024018
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260805-024020 | answer_regex,answer_contains | none | 228s | 30 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=8,inv=2/1,fin_reject=4,unavail=0,prune=0 | fail | Analyzer omitted the required ordered `call_chain_endpoints`; runtime accepted the missing field, so endpoint reachability/capsule stayed inactive. Explorer emitted an unordered 13-member set and finalizer reversed `gate.Run -> RunWith` in summary/list/caveat. Diagram hard checks removed the false call edge after four patches, but the false prose conclusion survived. |
| 2 | qf_multi_member_set_count_caveat | FAIL | eval/results/qf_multi_member_set_count_caveat-20260805-024020 | answer_regex,answer_contains | none | 241s | 19 | read=0,repo_map=3,list=0,trace=0,source_lens=3 | midloop=3,inv=8/3,fin_reject=1,unavail=1,prune=0 | fail | B99 context hygiene is effective (`iota` never appears) and the typed result is 3 types / 5 functions / 30 constants. The final answer nevertheless omits every constant name: an authored Markdown summary table hid 30 citation-sidecar `items[]`, while coverage treated those invisible items as visible rows. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusions

### B99 replay status

- `EVAL-B99-INVENTORYCONTEXTHYGIENE1` receives production close: the analyzer/tool context contains no `iota`, and no later evidence or answer reintroduced it.
- `EVAL-B99-PATCHCITATIONDRIFT1` remains replay-pending. The sequence run never reached the earlier stable-ID insertion shape because ordered endpoint authority was absent upstream; this run is not a valid positive or negative exercise of the citation-delta repair.

### New P0: `EVAL-B100-VISIBLECARRIER1`

The inventory draft carried a complete Markdown table in `block.text` and also supplied 30 member-labelled `items[]` as citation sidecars. The renderer deliberately returns after the authored Markdown table, so those `items[]` never appear to the user. The member coverage validator nevertheless counted them and accepted a document that says “30 constants” without listing any constant name. This is validation/render surface drift, not a model arithmetic error.

Generic repair: renderer and validators share one block-level visibility predicate. When a Markdown table is canonical, hidden items may attach citations only to members already visible on the table's first-column identity axis; they cannot mint absent rows. Structured tables without authored Markdown continue to render and validate their items normally.

### New P0: `EVAL-B100-ENDPOINTADMISSION1`

The source call-chain analyzer payload set `question_kind=call_chain` and `predicate_axis=call` but omitted `call_chain_endpoints`. Although the JSON schema declares the object required, strict Go decoding accepted the missing field. Downstream correctly refused to infer direction from `entities`, leaving `CallChainOrderedEndpointHints` empty; exact source-to-sink completion, `no_directed_path` admission, endpoint evidence capsule, and finalizer direction guidance all stood down. The model then guessed the reverse wrapper relation in prose. Diagram validation removed the unsupported arrow but could not rewrite model-owned prose, so four retries still ended in a wrong answer.

Generic repair: every non-scalar source-code call-chain analysis must provide a normalized, request-validated ordered endpoint profile. Missing or invalid endpoints trigger an analyzer repair before exploration. Runtime-artifact call chains and typed scalar/role-locate compatibility lanes remain on their existing authorities. This gate reads only typed classification/profile fields; it does not scan request or model/final prose.

### Invariants

Neither fix changes RootCauseTrace, explicit-window selection, causal projection, automatic trace supplementation, double-axis performance diagnosis, or answer conclusion ownership. The system validates carriers and supplies evidence direction; it does not author the final conclusion.
