# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T18:05:25Z
- sweep_start_ts: 20260812-110524
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260812-110525 | answer_regex | none | 115s | 24 | read=1,repo_map=2,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B663 获生产正证：`pub fn tokenize_bytes` 的唯一 `core-rs/src/lib.rs:10` 引用未再被 detach，且零成文拒绝。人工仍失败：Explorer 已读 line 42 的 `py::tokenize_bytes -> super::tokenize_bytes`，但同名 wrapper/core 在 principal member 匹配中形成歧义，typed handoff 漏掉真边；最终列表靠相邻顺序暗示“完整链”。正文还把已读的 `_tokenize_slow` 函数体说成“超出当前代码范围”。登记 B665/B666，runner regex 未覆盖关系闭包与范围披露。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260812-110525 | answer_regex,answer_contains | none | 303s | 30 | read=5,repo_map=0,list=0,trace=0,source_lens=0 | midloop=9,inv=6/0,fin_reject=0,unavail=0,prune=0 | pass | B662 获生产正证：Finalizer 首稿直接消费 `parallel_convergence`，正文和 Mermaid 均保持 `buildAnalysisIR -> gate.RunWith <- gate.Run`，没有反向伪边；`gate.Run` 以 definition-only 独立边界块出现，没有被塞进 principal directed list。零成文拒绝、零 patch。Explorer 仍有 6 次 completion 拒绝和 23 个中循环，属于已有收敛 churn 观察，不影响本轮关系真值。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual conclusion

- Runner `2/2 PASS`；human `1/2 PASS`。B662、B663 获生产闭环；不能用
  polyglot 的字符串 oracle 掩盖 B665/B666。
- B665 是 typed transport gap，不是 Finalizer 波动：Rust AST/解析器已经有
  `py::tokenize_bytes -> super::tokenize_bytes @ core-rs/src/lib.rs:42`，但两个
  principal member 的尾名都为 `tokenize_bytes`，旧 handoff 只做名称唯一性判断，
  没消费 member/support_ref 的精确声明位置，因此 fail-closed 丢边。
- B665 的泛化修复以“符号身份 + 精确声明位置”消歧同名角色。仅当 aggregate
  member 的 positional support ref 唯一命中 parser-owned Symbol.File/Symbol.Line
  时才携带已读 AST/Cangjie parser call；缺位置、同位置冲突、regex relation 均无权限。
  这覆盖 Rust/PyO3、JNI、C/C++ facade、ArkTS/Cangjie 等同名桥接，不读取请求、模型
  或终稿 prose，不选择图边，不生成结论。
- B666 独立保持开放：`_tokenize_slow` 的调用点已 typed，但其已读定义/body 没进
  evidence handoff，模型因而作出“实现细节超出当前代码范围”的错误披露。后续应从
  typed requested entity / exact parser symbol / read closure 构造有界 supporting
  definition authority；不能仅因正文提到 fallback 就扫描文本或全量注入所有定义。
- B664 生产接线出现明确日志：真实 streaming adapter 的 analyzer terminal
  emit-only 固定预算被标记 `skipped ... ownership=stream_first_byte_and_byte_stall_watchdogs`。
  本轮没有持续超过 180 秒的单次 stream，因此长活流语义仍由专项 pin 证明；累计年龄
  与字面 4ms 均无降级权限。
- Trace 显式时间窗、因果投影、系统补齐、链上-only 主因以及实际占用/规则可消双轴
  均未改；本批没有系统替写答案或基于 prose 的硬门。
