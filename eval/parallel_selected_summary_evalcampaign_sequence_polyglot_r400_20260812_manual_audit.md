# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T17:19:01Z
- sweep_start_ts: 20260812-101859
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260812-101901 | answer_regex | none | 125s | 24 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | 跨语言事实与 native/fallback 双路方向基本正确；首轮把 `blocks` 写成 JSON 字符串且只能恢复 3/4 block，系统正确 fail-closed，第二轮 native JSON 成功。确定性 GAP 是 citation normalizer 错摘 `pub fn tokenize_bytes (core-rs/src/lib.rs:10)` 的精确引用：标签已有唯一 file:line 且 citation 正好指向该行，最终却变成无引用并追加降级 caveat；Cargo.toml 仅出现在不可挂引用的 caveat，另记 presentation 债。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260812-101901 | answer_regex,answer_contains | none | 244s | 30 | read=5,repo_map=2,list=0,trace=0,source_lens=0 | midloop=10,inv=4/1,fin_reject=2,unavail=0,prune=0 | fail | Explorer 正确接受 `no_directed_path`：`buildAnalysisIR -> gate.RunWith`，而源码真实反向包装边为 `gate.Run -> gate.RunWith`。但 principal roster 未含 RunWith 时，已读 AST 反向边没有进入 finalizer，胶囊退化为 endpoint_unresolved；关系硬门正确删掉假边后，精确端点锚硬门又迫使模型把 definition-only `gate.Run` 塞进主路径，最终正文虚构 `RunWith` 间接到达 `Run`。这是 typed 边界传递/席位冲突，不是模型随机波动。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Generalized findings and disposition

1. `B662-NODIRECTEDPATHREVERSEPEER1` (P1-high): under an accepted typed
   `no_directed_path`, carry an already-read AST/Cangjie edge only when its
   caller is the exact requested sink and its callee is already reachable from
   the exact source through citable typed call edges. This preserves
   `source -> peer <- sink` in its real direction without depending on roster
   spelling or inventing `peer -> sink`. Teach the exact sink anchor as a
   separate supporting boundary seat, never a last principal-path hop.
2. `B663-EMBEDDEDLOCATIONCITEDETACH1` (P1): a unique structured `file:line`
   embedded in an item label/text and exactly aligned with the submitted
   citation is monotone identity authority. The detach layer must preserve it
   across all supported code/config suffixes; prose labels with no structural
   location and mismatched/ambiguous positions remain unprotected.
3. `B664-ACTIVESTREAMUPPERBUDGET1` (P0/red-line): the streaming HTTP adapter
   had already retired its absolute total-age timeout, but Analyzer terminal
   emit-only still wrapped the same request in a fixed 180s context deadline.
   That upper layer could cancel a byte-active stream and authorize stage
   retry/degradation. A typed streaming-liveness capability now suppresses only
   that evaluator wall budget; explicit parent cancellation/deadline,
   first-byte silence, mid-stream byte stall, transport failure, and decode
   failure remain authoritative. Mixed streaming/non-streaming fallback stacks
   retain the wall budget. Literal `4ms` and cumulative 4 minutes have no
   fallback authority.
4. No user/model/final prose is scanned; no answer or conclusion is authored by
   the system. Trace explicit-window causal projection, automatic supplement,
   on-chain-only principal-root authority, and adjacent/background separation
   are unchanged.
