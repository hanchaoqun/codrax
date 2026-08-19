# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T20:51:40Z
- sweep_start_ts: 20260819-135139
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-135140 | log_regex,write_apply,answer_regex,answer_contains | none | 668s | 26 | read=8,repo_map=3,list=0,trace=0,source_lens=2 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | First patch failed the concrete newline assertion; typed failure handoff correctly restored the checkpoint and produced a real second patch. The second probe, `make check`, changed-path coverage, and active slice all passed, but the cumulative ledger still changed the batch to `behavior_contract_observation_missing`. The retained probe imported the unique changed Python module but its two non-path symbol refs did not resolve, and verify-failure rebasing retired the probe's four old contract refs while minting three new fallback IDs. No pre-finish proof-followup was scheduled; terminal enforcement merely downgraded the delivery. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-135140 | answer_regex,answer_contains,mermaid_edge_count | none | 1194s | 53 | read=50,repo_map=4,list=0,trace=0,source_lens=0 | midloop=26,inv=23/0,fin_reject=6,unavail=1,prune=1 | partial | B1183 production-positive: the final no-arrow group correctly makes `BusContext` own `Mutable`; the old reversed group did not survive. The requested state-carrier interactions still disappeared. Two stage-precedence candidates consumed the bounded extractor roster, hiding the exact `extractorEvaluator.BuildInitialInstruction -> ctx.Mutable.TurnAArtifacts` dual-participant relation. The model tried the business edge, was rejected as a nonincident retarget, and ultimately kept only three stage arrows plus a BusContext/Mutable unproven note. Six finalizer rejects and 1194s show typed candidate diversity/dual-endpoint carry remains incomplete. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusions

- `B1183-DIAGRAMNOARROWOWNERSHIPDIRECTION1` is production-positive. The final Mermaid has `subgraph BusContext` with nested `Mutable`; the validator did not author or rewrite that grouping.
- `B1185-REPLANPROBEGENERATIONAUTHORITY1` is a confirmed write-mode typed-authority gap. A unique imported target is lost whenever the planner supplies only unresolved symbol-shaped refs, and a verify-failure contract rebase can leave a retained probe referring exclusively to retired IDs. A passed probe/project suite/path receipt therefore cannot resolve the new generation.
- `B1185-PREFINISHPROOFFOLLOWUPORDER1` is a separate confirmed scheduler seam. The slice path marks the active batch complete before finish, but the earlier failed-generation handoff remains visible and suppresses the already-implemented proof-followup even after the latest typed verify passes. Terminal enforcement is too late and can only downgrade.
- `B1186-DIAGRAMCANDIDATEDIVERSITY1` is a confirmed generalized diagram gap. A per-participant limit of two is exhausted by stage-precedence rows, so a distinct exact operation connecting two requested participants is withheld. Candidate bounding needs deterministic relation-family diversity, not a larger unbounded prompt.
- No Trace path was exercised or modified. Explicit windows, causal projection, auto-supplement, typed on-chain-only root causes, raw occupancy/business clues, and rule-priced eliminable impact remain protected. Active semantic bytes continued far beyond 4ms; neither case used fixed-age degradation.
