# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T03:39:48Z
- sweep_start_ts: 20260828-203948
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-203948 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 186s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式 2.000..2.020s 窗、四跳唤醒链、11ms 链上 IO、三席 1ms 调度候选、实际占时/规则可消双轴与完整 Trace 因果投影均保留；无 4ms/4m 降级、无成文拒绝。模型仍把 blocked_reason 调用点扩写成确定的 fscache 页面缓存 IO 完成路径，同时又承认等待对象/后端/直接阻塞关系未证，属 B1269/B1271 既有软教学/模型遵循问题。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260828-203948 | answer_regex,answer_contains,mermaid_edge_count,typed_diagram_participant_coverage | none | 457s | 43 | read=19,repo_map=5,list=0,trace=0,source_lens=1 | midloop=18,inv=7/0,fin_reject=7,unavail=1,prune=0 | partial | B1434 获生产正证：新增技术端点均有模型写的业务可见声明，图语法合法。最终图保留四阶段先后和部分上下文关系，但 Mutable/BusContext 到阶段的数据流仍偏薄，并留下一个无关系 bus.Mutable 节点。7 次成文拒绝中 3 次来自端点是否已声明的状态合同未进入动态 schema：新节点漏填名称、既有节点多填名称、暂存底稿已新增节点后重放同名名称均被拒。确认 B1435。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual Findings

### B1434 production witness

- The final read diagram declares `ctxbuilderBuildAgentContext_860bba75bb1a60fb["BuildAgentContext"]` and
  `cloneMultiRepoFocusDecision["cloneMultiRepoFocusDecision"]` before using them as edge endpoints.
- The technical IDs remain hidden carriers while their reader-facing names are model-authored. The executor did not infer a name, relation, direction, or conclusion.
- Mermaid remained parseable and the typed relation gate still rejected unsupported edges before the final accepted document.

### B1435-ENDPOINTLABELSTATECONTRACT1 (confirmed P1)

- The exact endpoint-name obligation depends on the current immutable patch base: a new endpoint requires a visible label, while an already explicit endpoint does not.
- The existing generic optional fields did not publish that state. A prior accepted repair can turn an endpoint from implicit to explicit, so replaying the same correction against the staged base encountered the opposite executor rule.
- Three of seven finalizer rejects were attributable to this hidden/contradictory state: missing `to_node_visible_label` for a new endpoint, redundant naming of an existing endpoint, and replaying the same label after an earlier staged patch had declared it.
- General fix: derive exact explicit endpoint IDs and labels from parsed Mermaid declarations on the live immutable base; encode the new-vs-existing condition in the branch JSON schema; require model names only for new endpoints; accept an exact existing-label replay idempotently; reject conflicting relabel attempts. Do not read request prose, answer prose, relation messages, or inferred technical identities.

### Trace authority audit

- Root-cause eligibility remains typed on-chain only. Adjacent and background rows stayed support-only.
- The final answer preserved both the raw occupancy axis and the rule-eliminable axis.
- The remaining `fscache_page_wait_on_page_bit` over-interpretation is not grounds for a prose scanner, answer rewrite, or hard keyword gate. It remains a soft evidence-caliber teaching item pending heterogeneous replay.
