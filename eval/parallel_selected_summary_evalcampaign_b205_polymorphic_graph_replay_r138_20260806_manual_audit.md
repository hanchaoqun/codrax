# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T00:34:05Z
- sweep_start_ts: 20260806-173403
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260806-173405 | answer_regex,answer_contains | none | 97s | 20 | read=2,repo_map=2,list=0,trace=0,source_lens=1 | midloop=6,inv=2/0,fin_reject=2,unavail=0,prune=0 | fail | 主结论、import-time registration、`resolve -> cls()` 与 callback 文字链正确，但仍漏 `TimestampMixin -> ValidationMixin -> BasePlugin` 的真实 cooperative MRO。首稿把 `blocks[]` JSON-encoded 为字符串，系统无损恢复成功；两次拒绝来自模型把 callback 锚成 `run_pipeline -> executor` 而 typed 事实是 `loop.run_in_executor -> plugin.handle`，最终删图。 |
| 1 | sr_cpp_virtual_chain | FAIL | eval/results/sr_cpp_virtual_chain-20260806-173405 | answer_regex,answer_contains | none | 476s | 36 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=7,unavail=0,prune=0 | fail | 7 次成文拒绝后走 degraded recovery，仍保住 stderr、factory、injection、virtual dispatch 等有用文字并明确告知降级；但 Mermaid 节点定义中的 `sink_->write<br/>` 被解析器凭空读成 `sink_write -> write<br/`，repair 每轮都在追不存在的边，context 膨胀到约 71k tokens。图层同时缺 virtual-dispatch 一等关系与逐边 typed composition。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

- `MERMLABEL1`（P0）：flowchart edge parser 在 node declaration/quoted label 内找 `->`，把 C/C++/Rust/ArkTS 等代码身份里的箭头和 `<br/>` 展示字节当图边；这是系统自铸假边，直接造成 7 次确定性拒绝与 476s 退化。
- `QUALPRES1` 的代码修复有效，但本轮 C++ 没有形成与 r137 相同的 `make_sink -> SinkRegistry.create` typed call witness，因此不能用该边做生产闭环；`Logger.log -> sink_->write` 不再出现 dot/`::` 形拒绝。仍需修完假边后再回放。
- `JSONCARRIER1`（观察项）：Python 首稿把 `blocks` 作为 JSON-encoded string。系统现有 lossless repair 成功保留全部块，最终答案无需因这个载体错误降级；projected schema 明确为 array，canonical teaching 也已且仅出现一次“native array / never quote”，未发现系统的 required/type/example 矛盾。本轮属于模型波动，先保留指标而不增加第二套 prose schema。
- `CALLBACKEND1`（P1）：callback 的 typed 方向是 receiving API → callable，模型却反复画 caller → receiving API；当前教学解释语义但没有提供本轮事实对应的可复制 anchor，增加了关系重建心智。
- `COOPPATH1`、`POLYCOMPOSE1` 继续确认；degraded lane 已按用户裁定同时展示最后可用结构稿、模型最后原文和明确降级说明，没有“模型回答消失”。

## Batch decision

1. S36d 先修 Mermaid parser：只在 node shape、quoted label、pipe label 之外识别 flowchart operator；不根据 label 词义推断 relation。
2. S36b/S36c 合并设计 typed cooperative/polymorphic relation composition 与可复制 edge-anchor hint，减少模型在 callback/return/assignment/type relation 间手工重建 JSON；不得替模型下结论。
3. 修完后继续原两个异构 case，仍严格并行 2；若 JSON string carrier 再现但 lossless recovery 稳定，不把模型随机波动升级为硬门。
