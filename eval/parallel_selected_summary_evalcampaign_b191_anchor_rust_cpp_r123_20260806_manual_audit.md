# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T20:18:19Z
- sweep_start_ts: 20260806-131817
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_cpp_sink_impls | PASS | eval/results/sr_cpp_sink_impls-20260806-131819 | typed_inventory_rowset,answer_regex,answer_contains | none | 81s | 20 | read=4,repo_map=2,list=0,trace=0,source_lens=1 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 三个实现、定义文件、两级继承关系和逐行引用均正确，零 finalizer reject、principal coverage=0。模型把整个 `blocks[]` 二次编码成 JSON 字符串，系统按既有有界无损车道恢复且答案完整，构成 JSON salvage 的正向生产 witness；同时模型被旧教学诱导给 `contain` 边重复填写 schema 不允许的 `claim_form=definition_fact`，并因关系枚举缺席而用 containment 冒充 inheritance。两项系统合同 gap 记入 S23/S24，不否定本轮可见答案正确性。 |
| 1 | sr_rust_trait_impls | PASS | eval/results/sr_rust_trait_impls-20260806-131819 | answer_regex | none | 84s | 20 | read=2,repo_map=2,list=0,trace=0,source_lens=1 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 两个实现、`--fixed`/default 选择条件和 impl 位置 17/33 均正确；S22 后 `principal_support_member_coverage=0`，上一轮错误“证据较弱”系统 caveat 消失。最终逐项显式引用仍落在 struct 行 7/23，impl 行以可见 file:line 文字呈现；精确等价锚正臂由单元 pin 直接覆盖，本轮生产侧证明客户可见矛盾已消失但未单独证明模型一定选择 impl 引用。owner 补充只保留 `main`/`--fixed`，略显冗余但未改变结论。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
