# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T23:49:47Z
- sweep_start_ts: 20260807-164946
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260807-164947 | log_regex,answer_regex | none | 34s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | PASS | `instructions.md` 以 typed `planner_distilled` 使用，`users.json` 以 `script_consumed` 使用；一次执行、零 repair，最终严格为 `{"ids":["u1","u3"]}`，没有解释文字或降级披露污染 JSON-only 合同。 |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260807-164947 | answer_regex,answer_contains | none | 192s | 22 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=5,inv=4/0,fin_reject=1,unavail=0,prune=0 | PARTIAL | Analyzer 两次 emit：首次 exact/discover 冲突被精确拒绝，第二次仍漏发 `completeness_obligation`。Explorer 已读 line30/37/38，并发出 line38 conditional/call 折叠形；未发 line30 guard，且没有按教学拆出 line37 guard + line38 call。完成阶段又因成员 `sink_->write()` 与证据 anchor `write` 的 receiver-operator 表面不等价连续降级，最终被迫把成员缩成 `write`。答案只在导语提到 error 额外 flush，未披露入口早退条件，也未把 `level >= kError` 与 flush 分行引用。唯一 finalizer reject 是首稿可选图含 7 条无 typed call authority 的边，修复后保留两条已证子图，属于正确 fail-closed。另有边界：仓内没有 `make_sink(...) -> Logger(...)` wiring callsite，最终正文仍把两段 API 叙成已发生的连续注入，应该保持条件式。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B324-CALLCONTROLCOMPLETENESS1`: process-positive / production-partial。教学在 prompt 中存在，但 analyzer 没有发射 typed completeness，所以下游的完整 mechanism coverage 软清单没有启用；不能把 runner PASS 当作闭环。
- `EVAL-B325-COMPLETENESSSIGNALOPTIONAL1=P1`: exhaustive/complete intent 仍是 optional typed 字段，模型可在理解“完整路径”的同时漏掉载体。最优方向是让 analyzer 必须显式回答一个 typed completeness decision（false 也要发），而不是下游扫描用户原文或答案关键词。
- `EVAL-B326-RECEIVEROPERATORIDENTITY1=P1`: 通用 symbol-tail 归一化识别 `.`, `::`，但不识别 C/C++/PHP 风格 `->`；正确成员表面 `sink_->write()` 无法绑定同位置的 typed call anchor `write`，制造 3 次无价值 completion 修复并损失可读身份。应在共享 code-surface normalizer 中补 receiver operator，不在 Logger/C++ fixture 处特判。
- `EVAL-B327-CONDITIONALWIRINGDISCLOSURE1=P2`: factory 选择和 constructor initializer 分别已证，但两者之间没有 wiring callsite；typed runtime-selection 状态已是 conditional，模型仍把它们叙成连续事实。后续应增强 conditional selection 的软披露材料，不用答案 prose 硬门，也不由系统改写结论。
- JSON 教学/执行本批无新 GAP：单源 typed plan、严格输出、零 repair 均为生产正证。
