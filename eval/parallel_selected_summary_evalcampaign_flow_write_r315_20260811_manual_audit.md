# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T13:55:09Z
- sweep_start_ts: 20260811-065508
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_c_typo | PASS | eval/results/patch_c_typo-20260811-065509 | write_apply,write_patch_oracle,answer_contains | none | 109s | 22 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确单行 `retrun buf;`→`return buf;` 落入隔离 worktree；`make test` 通过，`main.c` changed-path coverage=covered、caliber=project_runner、final verdict=verified，累计验证域未清空。模型曾把 Python probe 绑定到 C 路径，typed verifier 正确判 `verification_probe_language_target_mismatch` 并继续使用项目 Makefile；这是被安全吸收的计划波动，本轮不新立高优先级 gap。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260811-065509 | answer_regex,answer_contains,mermaid_edge_count | none | 233s | 37 | read=7,repo_map=3,list=0,trace=0,source_lens=0 | midloop=6,inv=3/0,fin_reject=1,unavail=0,prune=0 | fail | B531 继续生效，六个 participant 未被系统删除；但本轮 Explorer 只读 `orchestrator.go` 头部，从未读取 1685–1718 的真实 BusContext/Mutable 初始化点，因此 B532 没有生产触发机会。flow completion 已内部算出 bounded files/keywords，却在同轮 Summary 只告诉模型“Mutable/BusContext 缺 operation”；模型自行猜 `dispatchStage` grep，未命中 writer/reader，随后被既有 recovered-row closure 窗拉回重复定义，最终以两个 unproven 孤点出厂。确认 B534/FLOWREPAIRVIS1：两条 flow repair 车道必须同轮显示同一 soft navigation plan；它只指路，不作 edge authority。233s 模型原答，无四分钟降级。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
