# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T17:25:21Z
- sweep_start_ts: 20260808-102518
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260808-102521 | answer_regex,answer_contains | none | 483s | 36 | read=8,repo_map=2,list=0,trace=0,source_lens=1 | midloop=10,inv=2/0,fin_reject=6,unavail=0,prune=0 | fail | 四个 stage 与职责正文基本正确，但模型为绕过关系校验最终删除了 Mermaid 中全部箭头，只剩节点和 subgraph；这不是“流程图”。runner 仅检查 fence/stage 词而误签 PASS。authority 仍把无关 `hitraceconv -> repl test` repo-wide flow 标为 `typed_flow_paths_present`，证明无 support-plan 兼容臂仍过宽。 |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260808-102521 | answer_regex,answer_contains | none | 665s | 44 | read=17,repo_map=4,list=0,trace=0,source_lens=0 | midloop=10,inv=3/0,fin_reject=9,unavail=0,prune=0 | fail | 新 operation-carrier 门正确触发并促使 Explorer 读到/发出 `Orchestrator.runAnalyzePhase -> Orchestrator.dispatchStage -> Orchestrator.applyStageOutput -> Mutable.SetTurnAArtifacts` 精确调用点；但 Finalizer 使用短名节点后，validator 未将其唯一解析到 typed FQ endpoint，连续 9 次 `call_edge_unproven`，最终仅降级恢复上一稿。恢复稿保留了有用正文与图且没有泄漏 repair thinking，但 runner 按 degraded 跳过检查。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- Runner: 1/2 PASS；human: 0/2。`qf_diagram_pipeline` 是明确 false-positive：Mermaid 有 fence 和节点但零边，未满足“流程图”语义。
- `EVAL-B366-MECHANISMOWNERCONTEXT1` 的第一层修复获得生产 witness：definition-only completion 被挡住，Explorer 确实补读了 operation sites。新的断点位于 evidence → diagram validator 的 typed endpoint identity 交接，而不是模型没有查代码。
- 新立 `EVAL-B372-FLOWENDPOINTIDENTITY1=P0/REDLINE`：同一 citable call row 已正规化为 FQ endpoint，短名 Mermaid endpoint 在全证据池唯一时仍被硬拒。应以 typed call rows 做 fail-closed 唯一解析，存在同尾多 owner 时继续拒绝；不得放宽成任意 suffix/prefix/prose 匹配。
- `EVAL-B368-FLOWCONTEXTRELEVANCE1` 仍是 partial：已有 support-plan 时的 evidence-ID/双端绑定收窄正确，但本轮两案没有 support plan，兼容臂继续让任意 production repo flow 获得 ordered authority。新子项 `EVAL-B373-FLOWAUTHNOSUPPORT1=P0/HIGH` 要求无 support-plan 时也必须绑定本轮 citable operation evidence；无绑定则 `unproven`，repo-wide flow 只留背景。
- 新立 `EVAL-B374-DIAGRAMORACLEEDGES1=P1`：显式关系图 eval 的 declared oracle 至少需要 typed diagram block/可解析 edge count，不能仅用 Mermaid fence 与词项存在性。该修复属于 eval runner/oracle，不应反向成为生产答案关键词硬门。
- B369 本轮未按旧形复现：最终输出虽有系统 typed 核对附录，但没有再次附“系统保留内容 / 第一稿答案（校验前参考）”。附录仍偏长，作为信息密度观察项，不与 endpoint P0 混批。
- 两案均无 Trace 查询。上述 source-flow 修复继续显式排除 `QFRootCauseTrace`；Trace 显式窗、自动补采、因果投影、根因排序、唤醒链、窗内可消除量及双维耗时结论不变。Trace 主因只允许 typed on-chain 席，邻近/背景只能作额外排查方向。
