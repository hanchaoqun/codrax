# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T14:57:25Z
- sweep_start_ts: 20260816-075724
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260816-075725 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 115s | 34 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 核心事实与频率权限正确：running=157.248ms，runnable=5.604ms，sleep=70.338ms，D/IO=0；CPU4 的 2.10GHz policy 与本核 558/640MHz 分列，CPU12 无 policy，目标受限保持 unproven。B910 未再触发源码补证，B911 未再用窗长把 96.081/157.248=61.1% 误判为 41.203%。FAIL 是 oracle `[Dd]...state` 不接受正文 `D-State` 的大写 S。模型把精确 67.433% 写成 67.5%，系统仅 advisory 披露未改正文，记模型算术波动；bounded effect 不需要完整因果投影。另有未请求 blocked_reason 背景块，记 typed breadth 展示债。 |
| 1 | read_combo_answer_document_tools | PASS | eval/results/read_combo_answer_document_tools-20260816-075725 | answer_regex,answer_contains | none | 397s | 36 | read=8,repo_map=2,list=0,trace=0,source_lens=0 | midloop=8,inv=6/0,fin_reject=2,unavail=0,prune=0 | partial | B908-N2b 生产正证：两个独立 Name() return 不再冒充请求关系，completion 明确要求 typed relation component；最终图只保留三条真实 patch 局部 call，并把两个工具作为可见 unproven boundary，系统未造边。仍未回答可由源码/协议进一步证明的工具关系，且关系补证三次连续导航 patch 自身局部点，Explorer=21、completion=6、397s；确认 B912：缺少面向“连接当前参与者分量”的 relation-focused typed navigation，现有任意 incident unread-site 排序放大重试。两次 Finalizer reject 均正确（首稿九条无锚边；第二稿 boundary 不可见）。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
