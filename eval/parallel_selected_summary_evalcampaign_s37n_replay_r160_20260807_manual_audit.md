# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T11:01:20Z
- sweep_start_ts: 20260807-040119
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260807-040121 | answer_regex,answer_contains | none | 87s | 21 | read=2,repo_map=2,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | `JsonPlugin`、`run_pipeline -> resolve -> REGISTRY -> cls() -> handle` 与 import-time `@register` 职责均正确；一次成文、无系统改写。该案没有 diagram，因此没有覆盖 sequence 消息参数污染 endpoint identity 的开放守护。 |
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260807-040121 | answer_regex,answer_contains | none | 359s | 32 | read=5,repo_map=1,list=0,trace=0,source_lens=1 | midloop=13,inv=5/0,fin_reject=3,unavail=0,prune=0 | fail | S37n 生效：首个 `discover + gate.Run` emit 被 fail-loud，下一轮改为 exact，最终诚实披露 no-directed-path。但探索把同函数内 14 个直接调用当作完整链，漏掉 `analyzerSymbolResolver -> analyzerGraphForNormalize`；最终又把真实 `gate.Run -> RunWith` 包装方向说反。首稿重复发射两个顶层 `replace_blocks` 键，兼容层融合为 fused list+diagram，保真拆分新增第二图，导致 3 次拒绝后模型删除原图；渲染回收旧图虽保住可见图，重试心智与答案准确性均失败。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Gap disposition

- `EVAL-B258-DISCOVERSINKWIRE1`: production-closed. Analyzer 第 2 轮明确收到 typed shape 拒绝并改发 `sink_mode=exact`，精确 endpoint reachability 与 `no_directed_path` 恢复。
- `EVAL-B260-DUPARRAYKEY1` (P0/P1): confirmed. 合法 JSON 允许重复 object key，而 Go 默认解码 last-key-wins；本例兼容过程把两个 `replace_blocks[]` 批次融合成复合块，后续保真拆分制造第二张 diagram。最优方案是 schema 证明为 array 且所有 occurrence 都是 native array 时按出现顺序拼接；冲突 scalar/object 不猜。
- `EVAL-B261-REVERSEEDGEHANDOFF1` (P1): confirmed. `principal_span_waiver=no_directed_path` 已精确，但 finalizer handoff 只有 `Run` / `RunWith` 定义身份，没有一条 `Run -> RunWith @ gate.go:135` typed relation；模型把方向说反。应在 no-directed-path 调查中要求把已观察到的 reverse/parallel direct edge 独立发证，不能由系统根据定义邻接代写关系或结论。
- `EVAL-B262-TRANSITIVEHOPDEPTH1` (P1): confirmed. 用户要求关键中间函数时，调查只列 `buildAnalysisIR` 的直接 callees，未下钻 `analyzerSymbolResolver -> analyzerGraphForNormalize`。应以 typed call graph/path frontier 软引导至少一层关键 helper；不按当前函数名加硬编码。
- `EVAL-B263-PATCHOPTEACH1` (P1): partial/confirmed. Patch 四操作教学正确，但 malformed duplicate-key 恢复后的实际 merged ids 没有反馈给模型，模型无法解释“为什么有两张图”。先修载体使该症状消失；若仍需重试，后续考虑在 typed repair metadata 中返回 offending block ids，而不是让模型猜。
- `sequence-display-parameter-identity`: open; r160 Python 无 diagram，qf 消息无参数，因此没有获得生产证伪/闭环。
- `all-language-flowchart-relation-anchor`: open; r160 只有 sequenceDiagram，未覆盖无标签 flowchart edge。
