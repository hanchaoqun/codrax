# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T14:51:52Z
- sweep_start_ts: 20260822-075150
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260822-075152 | answer_regex,answer_contains | none | 285s | 30 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=6,inv=1/0,fin_reject=2,unavail=0,prune=0 | pass | 正确定位终端 `JsonPlugin`，完整说明 `run_pipeline -> resolve -> REGISTRY/cls() -> plugin.handle`、装饰器注册和 MRO；引用命中 `resolve` 的 `return cls()`。Mermaid 可渲染且主调用/返回关系正确，但注册中心、executor 与 MRO 主要留在正文/Note，图层关系覆盖仍可在后续异构用例继续观察。B1348 后无降级恢复，finalizer reject 由 9 降至 2。 |
| 2 | sr_cpp_virtual_chain | FAIL | eval/results/sr_cpp_virtual_chain-20260822-075152 | answer_regex,answer_contains | none | 341s | 31 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=11,unavail=0,prune=0 | fail | 正文主体识别 `Logger::log -> sink_->write -> ConsoleSink::write -> fputs/fputc` 和 `SinkRegistry::create(kind)`，但最终恢复稿把实际 `stderr` 写成 stdout，并把 `ConsoleSink` 的 base no-op flush 描述成强制落盘；不能判正确。B1348 的无动作死租约已消失，所有 relation failure 均有可执行分支，reject 由 20 降至 11；剩余失败来自孤点处置逐个披露及粗粒度重复错误提前停止。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case findings

- B1348 production-positive: 两案均未再出现 `target_carrier=unknown, allowed_actions=[]` 的死租约；Python 直接完成，C++ 的每个局部关系失败均有真实可执行 action。可选图的 exact `remove_block_ids` 能力在 schema 中可用，本轮模型未选择整图删除，不据此扩张系统所有权。
- 新 P1 B1350: C++ 在同一个原子关系修补中同时存在多个 `optional_orphan_cleanups`。旧执行器每次只返回首个缺失/不合格参与者，导致模型依次经历 CStd、Buffer、SinkBase；最后 typed 错误类仍只有固定的 tool+repair+fields，于是不同参与者的进展被算作同一错误并强制停止。
- B1350 最优边界：基于 live typed orphan roster、已选择的 edge edits、结构化端点与 typed anchors，一次返回完整 `missing`/`unexpected` 清单；系统不替模型选 `remove_if_isolated` 或 `retain_as_context`。重复错误门只消费该 typed 清单的闭合签名，不能读取错误摘要、请求、thinking、最终正文或 Mermaid message/label。
- B1349 保持开放：本轮 Python 图可渲染；C++ 降级恢复图也未复现 r863 的语法折叠，但恢复稿包含未通过证据门的关系且正文有事实错误。需要继续单独审计“恢复稿是否完整走 Mermaid compat/L7 fallback”，不能用本轮 C++ 具体图形做硬拟合。
