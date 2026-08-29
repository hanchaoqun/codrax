# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T05:15:38Z
- sweep_start_ts: 20260828-221538
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_dateutil_relativedelta_float_symptom | PASS | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260828-221538 | write_apply,write_patch_oracle | none | 219s | 28 | read=6,repo_map=3,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 只修改 relativedelta.py；整数值 float 归一为 int、非整数 float 明确拒绝；精确探针与 4 个 unittest 全绿，测试文件未改，6 个 typed verifier 全通过。 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260828-221538 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 255s | 49 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 显式窗、四跳链、链上根因、双账户、业务线索与自动补齐均在；但内部 causal/frame 枚举再次进入正文，且 page_cache_churn 的计数当量被一处模型上下文错标成 ms，诱发“7.200ms/整体高负载”错误表述。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工审计结论

### github_issue_dateutil_relativedelta_float_symptom

人工 PASS。补丁只改变生产实现：整数值浮点 years/months 被规范化为整数，带小数部分的值稳定抛出 `ValueError`；没有修改测试或绕过验证。两个精确行为探针、`python3 -m unittest test_relativedelta.py -v` 的 4 个用例及其余 typed verifier 共 6 项全部通过。最终答案由模型正常产出，没有成文拒绝、salvage 或系统替写。

### trace_query_donghu_real_frame_multicausal

人工 partial，且不是 Trace 核心能力回退。最终答案保留了：

- 精确 `34579.472865..34579.587805` 用户窗与 5 次窗内、PID/线程约束查询；
- `ThreadPoolForeg-60555 -> NetworkService-60595 -> CookieMonsterCl-59843 -> com.baidu.tieba-59566` 四跳 typed 唤醒依赖链；
- 只从链上人口选主因，邻近/背景仅作补充；Cookie 23.994ms、Network 19.041ms、ThreadPool D/IO 10.433ms、目标算力供给 10.331ms 均保留；
- “实际时间占用/新修向”与“现规则可消除量”双账户，类校验/JIT 语义工作 0.285ms 也保留且未伪造可消除量；
- IO issue/complete、D/io_wait、代表窗、Trace 因果投影及系统自动补齐全部存在；没有固定 4ms/4m 活跃流降级，也没有 finalizer 硬拒或系统改写模型结论。

确认两个泛化 GAP：

1. `B1423-TRACEINTERNALENUMWORDING1` 再次出现，升级为 P1。模型正文直接复制 `causal_conclusion=unproven`、`frame_evidence_status=absent`、`storage latency corroborated` 及内部查询 view 名。r912 首见、r913 未复现后曾按模型波动观察；本轮重复证明模型前方多处高显著度 prompt 同时暴露内部键和值，单靠“不要复制”会反向启动复述。修复必须在 typed→模型上下文边界提供自然读者语义，内部原值继续只留在 IR、tool payload 和 validator；禁止扫描、拒绝或改写终稿。
2. `B1439-TRACECONTEXTNONWALLCLOCK1`，P1。最终系统投影正确显示“计数当量 7.200（非墙钟）”，但模型前方 `contextual_noncausal_rows` 把同一 `page_cache_churn` 行发成 `value=7.200; unit=ms`，形成同轮冲突，模型因此写成“页缓存抖动 7.200ms”。IO 压力 551.6 的 typed 口径本是跨单位活动指数、绝对高低未定义，模型仍写“整体高负载”。根修应复用 token registry 的 count/composite 分类，把模型上下文发布为“计数派生观测，非耗时”或“跨单位活动指数，非耗时且绝对高低未定义”，不得另造按 kind/prose 的硬门。

本轮 runner 的 PASS 只证明声明 oracle；人工 partial 由上述读者语言与量纲冲突决定，不得通过放宽 eval 或追加终稿关键词硬门改绿。

后续实现状态：B1423/B1439 已按上述方案落地。决策 handoff、final boundary 与 frame-edge hint 改为自然证据边界，raw causal/frame 原值由负 pin 禁止进入这些模型前方上下文；context row 直接复用 tracequery token registry 的 count/composite 分类，IO aggregate 的 evidence-quality/score-caliber 原值也改为自然标定。定向 agent 回归、完整 `go test ./... -count=1` 与 CGO release-tag `make` 全绿；等待下一次生产 Trace 回放确认模型正文不再复发。
