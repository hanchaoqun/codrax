# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T11:53:25Z
- sweep_start_ts: 20260808-045324
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260808-045325 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 156s | 44 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | S37bw 生产闭环：同一精确 VerifyClass occurrence 只剩链上 `E24(+3)` 一席，确定性优化表也仅一行；吸收的 E# 与 locator 仍在索引中。显式 114.940ms 窗、补采、唤醒链、根因排序、真实占时/规则可消双轴完整，主因只来自链上；邻近/背景未因数值或时间接近入榜。frame evidence absent 只限制具体丢帧因果。模型仍把 priority-inversion candidate 扩写为锁方向，typed 边界未证 holder/waiter；按模型服从波动留档，不增加 prose 扫描、答案替写或硬门。 |
| 2 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260808-045325 | log_regex,answer_regex | none | 237s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 最终错误 `17,4,5`，应按 targets 的 GroupA/GroupX/GroupC 输出 `17,0,5`。四条贡献与业务 reconcile 正确保留 GroupA=17/GroupB=4/GroupC=5；失败发生在末级 assemble：output_contract 省略 complete_reference，系统自动 scaffold 按 group_key 发出 present groups，随后 expected/actual 从同一错误答案互签 pass。output graph 虽有 typed candidate targets.csv#canonical_label，却因未声明仍显示 satisfied。B347 本轮没有 string→array 或 schema repair witness，不能据此宣称生产闭环。立案 B348：complete-reference 与 subset/present-only 必须成为显式 schema 决策，true 同时携 source reference path/key；候选 census 仍只软提示，系统不得扫描规则/用户文字猜范围或直接改答案。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
