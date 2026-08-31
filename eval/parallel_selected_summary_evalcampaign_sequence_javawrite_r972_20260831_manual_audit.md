# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T13:03:45Z
- sweep_start_ts: 20260831-060344
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_java_typo | PASS | eval/results/patch_java_typo-20260831-060345 | write_plan,write_patch_oracle | none | 49s | 27 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确单行 `retrun -> return` 计划，保持 `pending_approval`，没有修改源码；无固定耗时/轮次降级。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260831-060345 | answer_regex,answer_contains | none | 457s | 37 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=15,inv=4/0,fin_reject=10,unavail=0,prune=0 | partial | 可见答案、两条端点边、时序图和 22 项关键函数清单正确，但成文发生 10 次拒绝。主因是 endpoint ownership 仍以易漂移 `citation_ref` 池位置为权威，append/replace/remap 后反复指向旧行；最终 exact `evidence_ids` 才通过。随后清单携带关系证据 claim_uses 被原子 member_set 候选误排除，模型调用 `add_facet_id` 后被 `field_not_published`，系统保留上一份可用草稿。见 B1489。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

- B1488 was not naturally exercised: the first draft did not create the standalone endpoint-edge list; it instead required a new principal carrier plus independent diagram boundary/label repair. No regression was observed, but production closure still needs a direct zero-anchor replay.
- B1489-CITATIONIDENTITY1 (P0 control-plane): endpoint-boundary ownership checked and taught mutable `citation_ref` pool positions even though every exact current-source edge already had a stable accepted evidence id. Patch citation normalization/append changed pool positions, so otherwise identical relation rows repeatedly failed with 1892/1895 instead of 2724/135. The generalized fix is stable `items[].evidence_ids` first, citation resolution only as compatibility fallback, and never instruct pool-index arithmetic for current-source ownership.
- B1489-MEMBERFACETPROVENANCE1 (P1): the unique requested key-function roster had `enumeration_item`, exact item evidence and no edge anchors/principal-path facet, but its evidence-backed `call_edge` claim_uses caused the atomic member_set projector to reject the carrier. A relation evidence row can prove a visible member without making the list a path carrier. Admit this shape only when every directed claim has a non-empty evidence id already owned by one list item; missing/out-of-list evidence, actual anchors, principal-path blocks, source-inventory rows and ambiguous rosters remain fail-closed.
- Final visible answer remained useful and no fixed elapsed/round/context degradation or empty-answer fallback occurred. Trace projection/auto-supplement paths were not changed.
