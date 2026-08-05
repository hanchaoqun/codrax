# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T07:28:12Z
- sweep_start_ts: 20260805-002810
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260805-002812 | answer_regex,answer_contains | none | 236s | 23 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=9,inv=2/0,fin_reject=4,unavail=0,prune=0 | fail | B96-C 的 endpoint capsule 已在生产生效，并诚实显示 `endpoint_unresolved` 与 `buildAnalysisIR -> gate.RunWith` frontier；但 completion 在没有 `gate.Run` 存在性证据、也没有读取 `internal/analysis/gate/gate.go` 时接受 `no_directed_path` waiver。最终答案因此误称 `gate.Run` 可能不存在/仍需检索；源码实际是 `gate.Run -> RunWith` wrapper，和 `buildAnalysisIR -> RunWith` 构成并行汇合。runner 只钉名称/图形，形成假绿。另有 4 次 Finalizer reject，主因是短 operation alias 与 qualified callee 不一致，以及一次把 item id 当 block id；列为低于 admission 根因的 churn 观察项。 |
| 2 | qf_multi_member_set_count_caveat | PASS | eval/results/qf_multi_member_set_count_caveat-20260805-002812 | answer_regex,answer_contains | none | 558s | 44 | read=2,repo_map=15,list=0,trace=0,source_lens=15 | midloop=10,inv=7/0,fin_reject=0,unavail=0,prune=8 | fail | B96-A 的 source-class 分区已在 evidence 选择中生效：模型只发出 5 个 production function 证据；但 analyzer 的 bounded prescan 返回 operational `scopes=.`，未铸 `SourceInventoryRequestedPathScopes`。Explorer 因而被迫先做 repo-wide lens，随后目标包 89-row complete lens 不能消除 root candidate-budget debt，形成 15 次 lens、24 轮、558s。最终又把 51 个 test function 作为 principal function 发布，`函数 56` 与 production 动态 oracle 的 5 不符；正则被“5 个生产函数”旁文误命中，runner 再次假绿。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch judgment

- `EVAL-B97-CALLENDPOINTPROOF1`（P0）：`no_directed_path` 只有在请求 source/sink 均由当前源码 typed evidence 精确证明存在后才可 admission。call-edge node 可证明参与图的端点；不参与任何已收集 call edge 的叶端点还需 citable exact definition fact。端点缺证据时应生成定向 read/evidence repair，不能把“尚未查看”升级成 no-path。两端已证但无可达路径时仍保留既有 typed escape；不得由系统撰写结论。
- `EVAL-B97-REQUESTBOUNDARY1`（P0）：analyzer-stage selected-subgraph lens 必须保留 repo-root query coordinate。`path=<scope>` 与冗余同值 `scope=<scope>` 应规范为同一个 requested path，不得在 selected root 内叠成 operational `.` 后丢失请求身份，也不得生成 `<scope>/<scope>`。只有 analyzer tool-query provenance 与当前请求精确 path identity 的 typed join 可铸 hard boundary。
- `EVAL-B97-DIAGRAMALIAS1`（P1 观察）：短 operation alias 与 qualified endpoint 的修复抖动需要跨语言异构复现后再决定是否扩充共享 resolver；本轮最终自修复，不能据一例增设硬门。
- 修复顺序：先 CALLENDPOINTPROOF1，再 REQUESTBOUNDARY1；每批独立测试、文档、提交、推送，之后严格并行同两例复放。两批均不得读取 request/think/final prose作结论门，不影响 RootCauseTrace、显式时间窗、因果投影、自动补齐或模型结论所有权。
