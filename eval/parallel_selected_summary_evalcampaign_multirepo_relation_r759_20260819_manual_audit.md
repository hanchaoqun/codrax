# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T06:22:11Z
- sweep_start_ts: 20260819-232209
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_tokenizers_newline_run_multirepo_py | PASS | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-232211 | log_regex,write_apply,answer_regex,answer_contains | none | 397s | 26 | read=5,repo_map=4,list=0,trace=0,source_lens=2 | midloop=0,inv=0/0,fin_reject=0,unavail=2,prune=0 | fail | B1218 路径修复获生产正证：计划/apply/test 均使用 `fastlex/...`，未再出现 `patch_path_missing`。但补丁把任何长度的换行 run（包括单个换行）都替换为 pair rank，违反计划与验收项“单换行不变”；probe 只测 5/2/4 个换行和无换行普通文本，项目测试也只含五换行与 `hi`，系统仍签 `all_verified`。 |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-232211 | answer_regex,answer_contains,mermaid_edge_count | none | 447s | 46 | read=14,repo_map=3,list=0,trace=0,source_lens=0 | midloop=12,inv=4/0,fin_reject=3,unavail=0,prune=2 | partial | 四阶段顺序与职责正确，但最终 Mermaid 中 BusContext/Mutable 与流水线断开，正文却继续声称全阶段经该载体流转。系统已有 BusContext 的两个 local-only typed 候选；第三次只剩 3 条失败 data_flow 边时却回退到 31,622 字节完整关系手册，模型删除失败边同时放弃局部真边。关系硬门正确，GAP 是局部修补上下文失控。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit findings

### 1. Multi-repo write: path identity fixed, verification claim still overstates coverage

- The emitted plan uses `fastlex/tokenizer.py` and `tests/test_tokenizer.py`; apply and verification both run inside the selected `bindings-py` repository. The previous deterministic `bindings-py/fastlex/...` versus `fastlex/...` split and `patch_path_missing` rejection are absent, so B1218 has a production-positive witness.
- The applied loop consumes every consecutive newline run and appends `newline_rank` unconditionally. Therefore one newline becomes the token for the `(10,10)` pair even though no pair exists.
- The model-authored plan summary and acceptance tests explicitly say a single newline remains unchanged. Verification nevertheless covers only five, two, and four newlines plus ordinary `hi`; neither the probe nor the two-test project suite exercises the singleton boundary.
- This is not a path or test-runner failure. `pytest` was unavailable, and the deterministic runner safely escalated to `unittest`, which executed two real tests. The remaining GAP is that `all_verified` has no typed mapping from each structured acceptance criterion to an executed proof. It must not be repaired by scanning plan prose, generated code, or test source keywords. A generalized solution needs schema-level acceptance IDs and verifier receipts, with uncovered criteria preventing the strong `all_verified` disposition or being disclosed as unverified.

### 2. Read relation view: candidate production works, patch repair context discards it

- Exploration reached 74 grounded evidence rows and found exact operations including `o.busCtx` argument flow and `Orchestrator.applyStageOutput -> o.busCtx.Mutable.SetTurnAArtifacts`. The finalizer prompt correctly classified Analyzer/Explorer/Extractor/Finalizer as request-scoped and Mutable/BusContext as local-only, and published bounded local candidates with `requested_relation_closure=unproven` and `retain_participant_boundary=true`.
- The first graph overclaimed seven assignment/data-flow edges; the hard relation authority correctly rejected them. On the next patch, only three unsupported `BC -> E/X/F` data-flow pairs remained.
- That local failure no longer carried the participant delta, so finalizer recovery fell back to the complete relation-boundary payload. The injected retry was 31,622 bytes. The model correctly removed the three unsupported pairs but also omitted the already-grounded local BusContext operations, producing a disconnected graph while prose continued to describe the shared carrier flow.
- The high-ROI repair is one producer-owned source-relation delta for every relation family: exact failed block/edge/identity/relation tuples, `preserve_unlisted_edges=true`, and only a globally bounded optional roster of local typed alternatives. Runtime Trace diagrams remain on their separate causal authority. The system must not select or write an edge.

## Runtime and red-line audit

- Both active streams completed normally at 397s and 447s. No 4ms, 4m, first-byte, stall, or accumulated-age fallback produced a degraded answer.
- This pair contains no runtime Trace query. The proposed source-diagram repair is explicitly excluded from `QFRootCauseTrace`; explicit-window causal projection and automatic supplementation remain unchanged.
- No hard gate reads raw user input, model reasoning, or final prose. No system answer/conclusion/diagram edge is authored or substituted.
