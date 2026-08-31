# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T06:01:27Z
- sweep_start_ts: 20260830-230126
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260830-230127 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 232s | 43 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | B1473 的类别边界生效：模型不再把 NetworkService runnable 调度等待称为确定性优化工作；显式窗、链上排序、双账户、自动补齐和最终 Trace 因果投影仍完整。但模型主回答完全遗漏用户核心子问中的 `VerifyClass` 0.285ms 及其与目标的关系边界。typed finalizer 上下文和系统投影均已有 T7 span、直接 host→target 唤醒锚、完成/目标等待绑定未证及规则可消 0，故不是证据缺失或模型分类波动，而是请求 schema 缺少“运行时工作与目标关系”的独立 typed 可见维度。 |
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260830-230127 | typed_inventory_rowset,dimension_substring,answer_contains | none | 272s | 27 | read=0,repo_map=3,list=0,trace=0,source_lens=3 | midloop=4,inv=4/2,fin_reject=2,unavail=8,prune=0 | pass | B1472 获得生产正证：finalizer 上下文只发布一个 12 行 principal 集合，family counts 精确为 extend=2、foreign func=2、public class=8、coverage=12/12；可见答案完整保留 12 条声明、文件与 package，没有重复 native_add 或 14/14 污染。仍有 2 次表格 schema/row identity 修补，记为流程成本观察，不影响本轮答案正确性。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

1. Runner 2/2 PASS；人工结论为 Cangjie pass、Trace fail。结构 oracle 不能替代对用户核心子问题是否由模型回答的人工审计。
2. `B1472-INVENTORYAGGREGATEIDENTITY1` 获得生产正证：principal row、obligation、family count 与 coverage 已共同消费同一个 canonical typed declaration universe。
3. `B1473-TRACESEMANTICCLASSBOUNDARY1` 获得部分生产正证：错误类别命名消失，但 soft 类别教学不能确保核心关系子问进入最终回答。
4. 新确认 P1 `B1474-RUNTIMEWORKRELATIONDIMENSION1`：当前 requested-answer schema 只有 observed value、relation path 与 causal attribution，没有表达“某个测得的运行时 work/span/operation 是否及如何关联目标”的独立席位。修复应新增 analyzer-emitted `runtime_work_relation`，并以模型成文 principal block 的 hidden typed ownership receipt 验收遗漏；系统不得扫描可见文字、选择 verdict 或代写结论。
