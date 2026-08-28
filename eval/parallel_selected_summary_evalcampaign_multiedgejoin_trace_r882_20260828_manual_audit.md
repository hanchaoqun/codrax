# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T11:17:05Z
- sweep_start_ts: 20260828-041704
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260828-041705 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 161s | 33 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 2.000..2.020s 主窗、四跳唤醒链、11.000ms 链上 IO 第一席、三个各 1.000ms runnable/优先级候选、实际占时/规则可消双账和完整 Trace 因果投影均保留；邻近/背景未升为主因。模型把是否“直接阻塞”额外系于业务请求关联，属于偏保守措辞，不改变 typed 链上根因。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260828-041705 | answer_regex,answer_contains,mermaid_edge_count,mermaid_incident_node_count | none | 611s | 53 | read=7,repo_map=2,list=0,trace=0,source_lens=0 | midloop=11,inv=6/0,fin_reject=6,unavail=0,prune=0 | partial | 最终图用同一 BuildAgentContext 技术节点接通 Extractor 与 BusContext 两个 typed 组件，证明 B1374 的完整两边路径可执行；但 participant-only lease 漏发安全技术节点 ID，模型连续猜错 3 轮后才用原始限定名，导致共 6 次拒绝、611s。正文另把共享 Mutable 指针误写成互不影响的独立副本，证据本身已给出 `Mutable: bus.Mutable`，先归模型解释漂移。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### B1374 production result

- Positive: the component-split provider published both exact argument-flow rows in one generation, and the model selected both rows in one atomic patch. The final visible graph therefore connects the stage spine and the BusContext/Mutable island through one shared technical handoff node. Multi-edge joins are no longer dropped merely because no single typed edge crosses the two current visible components.
- Negative: the same participant-only producer published `from_node_ids=[BusContext]` / `[Extractor]` but omitted the deterministic safe carrier for the shared qualified target `ctxbuilder.BuildAgentContext`. The executor accepts either the exact technical endpoint or a producer-listed carrier, yet the producer did not list the carrier already used by the ordinary relation-repair path.

### B1375 confirmed root cause

- Round 4 selected both live additions and used `ctxbuilderBuildAgentContext_860bba75bb1a60fb`; the executor rejected it because that alias was absent from the lease.
- Round 5 guessed unrelated business nodes and was correctly rejected.
- Round 6 made no edge progress because the contract still exposed no valid target node ID.
- Round 7 used the raw qualified identity `ctxbuilder.BuildAgentContext`; it passed, after which the compatibility renderer rewrote that unsafe display carrier to `codraxNode1`.
- This is not an atomic-group gap and not model variance. `diagramParticipantRepairAdditionDeltaJSON` skipped `bindDiagramRelationRepairCandidateTechnicalNodeIDs`, while its ordinary sibling candidate constructor called it. The fix is to make both constructors publish the same exact, syntax-safe endpoint carrier before adding participant-side aliases. No relation, label, layout, action, or conclusion is system-authored.

### Trace red-line audit

- Root-cause authority remains typed and on-chain only: `threadpool-400 iowait 11.000ms` is first; nearby sleeps and aggregate IO activity remain contextual.
- Scheduling/priority candidates remain separate 1.000ms seats and are not summed across overlapping directions.
- The explicit user window and deterministic supplementation remain active. No 4ms/4m/stream-age rule downgraded the answer.
