# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T16:52:32Z
- sweep_start_ts: 20260802-095231
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260802-095232 | log_regex,answer_regex | none | 52s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | data=1,repair=1,consumed=2 | pass | 最终精确输出 `{"ids":["u1","u3"]}`；首 plan 漏调度 instructions.md，被 typed `required_material_scheduling` 在执行前拒绝并正确修复。真实 gap 是终态 context 同时给出 workflow `json_only` 与 result `freeform`，见 EVAL-B44-DATACONTRACT1。 |
| 2 | operation_web_manual_summary | PASS | eval/results/operation_web_manual_summary-20260802-095232 | log_regex,answer_regex | none | 121s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | command_rounds=4,repair=1 | partial | 安装、配置、读写/trace/data 场景主体可用；但把真实 `/repos focus` 写成 `/focus`，并加入材料未支持的“最近 3 轮”。每页正文只给 final/evaluator 前 4000 rune，广域手册总结上下文不足；旧 metric 又把网页里的 Trace 标题误计为 9 个投影块。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. 数据案的材料调度 guard 工作正常，问题不在 repair，而在 terminal custom-transform
   为执行方便使用 freeform sub-plan 后把该内部契约泄露成 `result.output_contract`。工作流和
   output projection graph 仍按 `json_only` 验收，因此答案碰巧正确，但模型必须自行解释两个
   相反的 typed face。
2. 操作案确实下载了三个完整 HTML payload；系统也做了正文抽取。然而每个 material prompt
   excerpt 二次限制为 4000 rune，`source_truncated=false` 只描述 256 KiB 读取层，并不描述
   4000-rune prompt 层。模型没有获得后半手册，却被 evaluator 标为 complete，最终出现命令名
   失真与无依据记忆轮数。
3. `trace_query_final_projection_blocks=9` 是 eval 量具污染：旧实现统计任意“Trace 因果投影”
   子串，连模型进度、HTML title、文档名都算。此 operation 没有 trace attachment、trace_query
   或系统因果投影。
4. 本批没有触碰 Trace 查询/投影/补齐权限。B43 的显式窗正例继续作为因果投影、根因排序、
   唤醒链和双轴可消除量的不变量证据。
