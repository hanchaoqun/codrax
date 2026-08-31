# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T11:29:36Z
- sweep_start_ts: 20260831-042935
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-042936 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 205s | 47 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 首稿直接接受。显式 10ms 窗、目标四态、NetworkService→CookieMonster→目标两跳唤醒链、链上根因排序、实际占时/规则可消双账户、业务线索及邻近/背景隔离、系统自动补采和最终 Trace 因果投影完整。VerifyClass 0.285ms 被准确表述为链上宿主线程的确定性工作线索、不是直接阻塞；系统明细继续发布有效归因 0.000ms 和 completion→target-wait 未证边界。无固定时限、活动流年龄、轮次或上下文比例降级。 |
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260831-042936 | typed_inventory_rowset,dimension_substring,answer_contains | none | 256s | 28 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=2,inv=3/0,fin_reject=1,unavail=6,prune=0 | pass | B1484/B1485 生产闭环：最终答案精确枚举 12 条（extend=2、foreign func=2、public class=8），每条符号、文件、package 与引用正确；Animal/Service 的 sealed/abstract 细节保留，但 canonical family 正确为 public class。没有内部 `source inventory principal rows` bucket 泄漏、重复 roster、requested-dimension 误报、旧稿恢复或 degraded answer。唯一 reject 是模型首轮 section 缺 schema 必填 `text`；系统恢复 blocks 并给精确错误，第二轮即接受，属于普通 JSON/schema 波动而非合同互斥。三个 explorer 共 6 次尝试当前 schema 不提供的 grep，记为低优先级过程噪声，不以原文扫描或新硬门处理。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
