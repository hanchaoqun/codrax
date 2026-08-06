# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T02:59:47Z
- sweep_start_ts: 20260805-195945
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260805-195947 | answer_regex,answer_contains | none | 107s | 25 | read=2,repo_map=2,list=0,trace=0,source_lens=1 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 四阶段、职责、产物、顺序和 Mermaid 全正确；首轮无图边拒绝，关闭 B156 两项生产回放。引用确定性修复却把模型正确的 stage_binding 席位重绑到只证明类型/枚举名的 analysis_ir/enums 行，属于系统把引用修弱的新通用 gap。 |
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260805-195947 | typed_inventory_rowset,dimension_substring,answer_contains | none | 142s | 21 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=1,prune=0 | pass | 12/12：2 extend、2 foreign func、8 public class，名称、文件、行、package 均精确；同名 Cart extension/class 与两个 native_add 均未串行。一次不可用 emit_evidence 是低优先级过程噪声。模型本轮未发 source_inventory_family，故字段 quarantine 由结构 pin 直接验收、此 case 只作 inventory 无回归。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case judgment

- `EVAL-B156-SUPPORTDETAILJOIN1` 与 `EVAL-B156-DIAGEDGECHOICE1` 已由 diagram 首轮零拒绝、完整职责答案关闭生产回放。
- `EVAL-B156-FIELDQUARDRIFT1` 未被模型直接触发 `source_inventory_family` 字段，但 Cangjie inventory 完整；字段接线仍以 full/patch/quarantine 反射 parity pin 为直接闭环证据。
- 新确认 `EVAL-B157-CITMONO1=P1 correctness`：两条 pre-emit citation normalizer 会覆盖已属于同席 typed 候选集的模型引用，且新引用证明力更弱。修复应只读 typed row/citation，不扫描答案 prose 作硬门。
- 新确认 `EVAL-B157-EVSPAN1=P1 context precision`：结构化对象的一条 evidence summary 同时消费 entry identity、responsibility 和 artifacts，但 scope=line 只锚 entry identity 行；应通过通用 JSON 教学引导 exact initializer 或 bounded line_range，不以自然语言相似度建立 validator。
