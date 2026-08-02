# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T11:49:10Z
- sweep_start_ts: 20260802-044910
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_text_filter_count | FAIL | eval/results/data_text_filter_count-20260802-044910 | log_regex,answer_regex | none | 68s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Router 在 data/operation 间长时间摇摆后选 operation。命令读取和计数均正确，但 operation finalizer 输出任务完成、解释、列表、代码块，违反“只输出一个数字”，也没有 data terminal。根因是路由把“需要 cat/grep”当目标，而不是把“按本地材料规则计算派生标量”识别成 data；登记 DATAROUTEOBJ1。 |
| 2 | trace_query_binder_ipc_peer | PASS | eval/results/trace_query_binder_ipc_peer-20260802-044910 | trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 113s | 29 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | analyzer 第二次发射稳定为 bounded_fact_set；模型 ipc_graph+wakeup_chain 两调用，三个事实及方向均正确。pre-finalize heavy debt 不再出现，system supplement 明确 `families_present` 零执行，无根因排序/可消除量/因果投影。残余：系统校验附注仍追加与三个 IPC 字段无关的 client 五态，登记 PRINCIPALREL1。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch verdict

- Runner: 1/2 PASS；human: 1/2 PASS。
- Binder 的 full-report 扩面已关闭；剩余是较窄的 target-state principal-value 相关性。
- Data 是路由目标语义波动，不是 data workflow 回退。采用 soft classifier guidance：按任务
  objective 而非 incidental file command 分类；不新增用户文本关键词 hard gate。
