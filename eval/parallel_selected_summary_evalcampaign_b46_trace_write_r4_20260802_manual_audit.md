# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T20:07:02Z
- sweep_start_ts: 20260802-130700
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_memoclaw_text_search_multirepo_py | PASS | eval/results/github_issue_memoclaw_text_search_multirepo_py-20260802-130702 | log_regex,write_apply,write_patch_oracle | none | 337s | 19 | read=9,repo_map=4,list=0,trace=0,source_lens=1 | midloop=2,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 仅改 `memoclaw/client.py`；sync/async 均改为 `POST /v1/search` + JSON body。真实 `make check` 执行成功并输出 `python text search contract ok`，ChangeReport=passed，Python changed path=covered。 |
| 1 | real_trace_h5_smr_multirow_disposition | FAIL | eval/results/real_trace_h5_smr_multirow_disposition-20260802-130702 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 392s | 42 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | runner 两项均为旧 oracle 债，但人工仍失败：模型跨 two-ruler 相加、把 row-local state split 写成跨行包含、把无 exact subtotal 的同修向席位相加。显式窗、补采、两维占用/可消、排序、唤醒链和投影均保留。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human verdict and context sufficiency

### Write

人工 PASS。补丁、真实项目 suite、changed-path coverage 与最终报告一致；没有以 probe
代替项目测试，也没有修改 API reference 或测试。9 次 read / 4 次 repo_map 和 337s 仍偏重，
记为后续跨 write case 的效率观察项，不影响本轮正确性。

### Trace

人工 FAIL，但 runner 的两个失败原因都不能直接代表生产缺陷：

- `×2` 只出现在“同一 peer 出现两次”的自然语言中；fixture 用禁止任意 `×2..×9`
  阻止旧合并计数词面，范围过宽；
- `dma_fence_default_w` 已在 typed projection、最终树和明细多次出现，固定前缀
  `等待对象` 不应作为模型自由表达的硬 oracle。

真实缺陷是关系结论不服从 typed authority：

1. rank #4 的 3.956ms 属于 self-wall-clock ruler，rank #10 的 1.648ms 属于
   wakeup-edge ruler；handoff 明确给出各自 subtotal=5.149/1.648ms、跨尺禁加，模型仍把
   3.956+1.648 写成目标 runnable=5.604ms；
2. `CompThread_0` 的 `d_state=3.598ms` 只是该 observation 的 row-local 状态拆解，
   handoff 明确 `cross_row_relation_authority=not_provided_by_state_breakdown`，模型仍称其
   被 priority-inversion 席位包含；
3. blocked_reason 只是不同口径记录，现有 typed 证据未证明其总和是目标 70.338ms sleep
   的严格子集，模型仍铸包含关系；
4. 模型把四个“锁与优先级”席位相加为 18.853ms；系统只对其中一个 exact
   mutually-exclusive pair 发布 12.115ms subtotal，没有授权四席总和。

最终成文上下文已经准确且足以支持正确回答；错误探索总结却在更早阶段形成，最终模型同时
看到了错误探索结论与正确 finalizer handoff。合同检查只耗时约 104ms，日志没有任何 reviewer
dispatch/skip 记录；配置代码也表明 semantic/self-consistency reviewer 默认关闭，因此不能把
`semantic_quality_dispatches=0` 误诊成 observation-only skip。

结论：这是 cross-stage typed relation authority 与 model-owned repair 的体系缺口。修复应让
共享关系载体在探索结论形成前可见，并让模型以结构化 relation claims 声明需要进入结论的
包含/重叠/可加判断，再由精确载体校验、触发模型重写；不得扫描原始题面/答案关键词，不得由
系统删除或替换模型正文。
