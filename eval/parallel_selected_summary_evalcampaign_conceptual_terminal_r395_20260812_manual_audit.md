# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T15:50:25Z
- sweep_start_ts: 20260812-085023
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | FAIL | eval/results/sr_java_call_chain-20260812-085025 | primary_answer | none | 152s | 24 | read=5,repo_map=4,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | B656 生产生效：Analyzer 直接选 `discover_terminal`；Explorer 完整读取五个关键文件；最终 prompt 明确载入 `selected_terminal_body_calls=parser_grounded` 与 `AuditLog.record -> System.out.println`，模型 thinking 也列出该第 6 跳。最终正文只说 `AuditLog.record`“记录审计/输出操作类型”，不再出现“完成审计落库”，但仍未明确说明 stdout 不等于持久化，因此严格语义 oracle 和人工仍判 fail。此时精确信息已经充分，剩余更接近模型消费波动；不可由系统硬改正文或扫描答案关键词强制结论。一次成文拒绝来自图中无依据 return/guard metadata，patch 后图被删除；记录为图保留率观察项。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260812-085025 | answer_regex,answer_contains | none | 215s | 30 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=9,inv=4/0,fin_reject=0,unavail=0,prune=0 | pass-with-gap | 正文和 sequenceDiagram 正确表达 `buildAnalysisIR -> gate.RunWith <- gate.Run`，runner 与核心语义人工通过；但 item `gate.Run` 原本正确引用 `gate.go:134`，backtick quote normalizer 被正文中的 `gate.RunWith` 吸引，改到 `analyzer.go:2722`。后续结构检查已精确给出正确候选 `gate.go:134/135`，却按 citation 软策略放行，用户最终看到错误引用。这是 B657 确定性系统 gap，不影响关系结论但必须修复。Explorer 四次 complete 拒绝继续计为 churn 观察。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
