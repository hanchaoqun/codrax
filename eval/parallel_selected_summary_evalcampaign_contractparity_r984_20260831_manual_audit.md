# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T17:58:37Z
- sweep_start_ts: 20260831-105836
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-105837 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 218s | 43 | read=1,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=1,prune=0 | pass | 显式 34579.490..34579.500s 主窗、目标四态、NetworkService 链上第一席、实际占时/规则可消双账户、VerifyClass 0.285ms 及“宿主直接唤醒已证、工作完成触发/目标等待未证”边界、自动补齐和最终 Trace 因果投影均完整；邻近/背景未进入主因。首轮 analyzer 被自身 raw-trace 教学诱导调用未发布的 read_file，浪费一轮，记 B1502。补充投影包含更宽上游查询窗的重复行但已明确跨窗且未改变主窗第一席，暂作压缩可读性观察，不加答案硬门。 |
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260831-105837 | typed_inventory_rowset,dimension_substring,answer_contains | none | 230s | 29 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=5/4,fin_reject=0,unavail=0,prune=0 | pass | 12 条精确声明、extend=2 / foreign func=2 / public class=8、符号、文件和 package 全部正确；零 finalizer reject，B1501 未引入行身份或列顺序回归。该次模型采用 member-first 表格，category-first 可执行正臂由结构测试钉住，不能把本次生产结果冒充对该具体布局的自然命中。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
