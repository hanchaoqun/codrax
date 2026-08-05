# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T04:11:38Z
- sweep_start_ts: 20260804-211135
- total cases: 2
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_multi_member_set_count_caveat | PASS | eval/results/qf_multi_member_set_count_caveat-20260804-211138 | answer_regex,answer_contains | none | 405s | 42 | read=3,repo_map=16,list=0,trace=0,source_lens=16 | midloop=12,inv=8/0,fin_reject=0,unavail=0,prune=5 | pass | 最终 3 个 type、5 个 production function、30 个 Kind 常量及单一 const block 数量依据均与 checkout 一致，B92-A 的 selector/role 修复生产生效。但 Explorer 第 7 轮已得到完整目标包 roster，随后 completion authority 仍把先前全仓截断 observation 的 source-class/follow-up debt嫁接到本次有界答案，强制查询 cmd、fixtures、internal/skill、thirdparty corpus；16 次 lens、28 个 Explorer 轮和 405s 均属系统作用域血缘污染，登记 EVAL-B93-SCOPELINEAGE1。 |
| 1 | qf_sequence_analyzer_gate | FAIL | eval/results/qf_sequence_analyzer_gate-20260804-211138 | answer_regex,answer_contains | none | 542s | 31 | read=13,repo_map=2,list=0,trace=0,source_lens=0 | midloop=8,inv=8/1,fin_reject=6,unavail=0,prune=0 | fail | 源码只有两条并列真边：buildAnalysisIR -> gate.RunWith（analyzer.go:2666）与 gate.Run -> RunWith（gate.go:135），不存在 buildAnalysisIR -> gate.Run。模型连续三次提交 no_directed_path，但把该字段附在字符串化 aggregate_facts 的对象尾部；兼容解码器恢复 facts 却不恢复 waiver，于是 completion 反复要求同一 waiver。降级答案又把两条平行边写成“随后”，事实错误。B92-B 的 duplicate identity 诊断本次没有命中，因为没有两个 active alias 指向同一 exact typed endpoint，故其生产正证仍待下一轮。Finalizer 另有 gate.Run/Run 的 endpoint owner 展示身份抖动，登记 EVAL-B93-CALLIDENT2。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
