# Selected Eval Manual Audit Scaffold

- date: 2026-09-08T14:48:21Z
- sweep_start_ts: 20260908-074819
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_d4_demand_vs_supply | FAIL | eval/results/real_trace_d4_demand_vs_supply-20260908-074821 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 152s | 44 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 精确目标账/投影/补齐均在；模型误加重叠收益、混淆修向席与状态总量，过强排除供给。机器旧措辞 oracle 漏匹配另记，不改原判定。 |
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260908-074821 | answer_regex,answer_contains | none | 284s | 44 | read=7,repo_map=1,list=0,trace=0,source_lens=0 | midloop=12,inv=3/0,fin_reject=4,unavail=0,prune=0 | partial | 核心注册/实例化/异步回调/MRO 正确；把 concrete handle 说成 abstract，装饰器细节由模型 patch 删除。另查真调用误判为文档的系统问题。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 冻结与独立复核

使用 main@1518f5abf652 的 clean build，构建时间 2026-09-08T14:47:53Z；两路固定同一二进制快照，未修改 case、oracle、历史答案。runner 152/284s 与内部 wall 150/282s 是不同计时边界，不混写。此前全仓 `go test ./... -count=1` 86 包通过。

### Python：答案可用但不以机器 PASS 销账

最终 `.codrax/output/20260908-075303.894-52865.md`。独立读取冻结 fixture 的 runner/registry/plugins/base：导入 plugins 执行 `@register("json")`，内部 bind 将类存入 REGISTRY 并返回原类；resolve 再取类并执行 cls()。runner 通过 `run_in_executor(None, plugin.handle, payload)` 回调，MRO 为 TimestampMixin → ValidationMixin → BasePlugin。不是直接 await 同步 handle；无默认 kind/fallback，content_type 不在这条执行路径。

正文保留上述核心，但 BasePlugin.handle 是具体实现，不是抽象方法；abstract 属于 content_type。装饰器详细段被模型自己的第 3 轮 patch（log4007 remove_block_ids）删去，摘要仍保留其注册作用；系统没有代删正文。没有请求强制 Mermaid，最终有序列表/关系行不是图渲染失败。

四次成文拒绝逐条核对：log3871 缺端点身份；3965 同块同时 atomic relation edit 与 replace（3912 已明确禁止）；4017 手填 citation_ref 而非已发布 evidence_ids；4144 最后一次 advisory 修补把 summary 块误改为 ordered_list。第 4 轮已接受（4063–4075），最终保留这份已接受答案；第 5 轮失败没有污染已发布版，不是活跃流超时降级。前述四处没有确认“同一合法声明同时必带必拒”的新冲突，不增硬门。

另有独立系统问题 B1628：log1162–1165 将 runner.py:15 的真实 `plugin = resolve(kind)` 判为 comment/doc/example，前一行恰是 docstring 结束。根读公共 comment scanner 的两臂 maxWalk 下界错误，实际只回看上一行。实际原fixture ParseFiles→BuildGraph→ReadFile→EmitEvidence 首红重现同ID、illustrative_only和Do NOT repair；公共分类7格也证Python闭合后误判及Python/C/Lua多行正文漏判，见 `.codrax/tmp/20260908-b1628-runner-comment-red.log`。需多语言/稀疏读边界安全修复，不能把这一错误归给模型。

B1627 本轮没有实际生成启发式 resolution join，故不能称候选反升级获得生产覆盖；只记录完整 return/context 与精确关系可用。原回归覆盖仍有效。

### Trace：证据齐全，模型结论仍越界

最终 `.codrax/output/20260908-075051.396-52864.md` 及同名 HTML、root-causes.json。独立按原始 sched_switch 复算窗口 34579.472865..34579.587805（114.940ms），目标 59566：Running26.946、Runnable3.636、S84.358、D0ms；D/IO 的零只限目标调度状态/调度标记，不能声明所有层级 I/O 已排除。五 CPU 运行分别 6.588/11.487/8.130/.245/.496ms，总计26.946ms。普通 window_stats/root_cause_rank 的模型上下文 log947/965 已提前给出完整目标账，B1625 获真实生产正证。

CPU0–2 的频点来自同簇 CPU3 推导；不是直接频率采样，也不是硬件/策略天花板证明。供给反事实为26.946−16.615＝10.331ms。同窗全局 idle221.695 cpu·ms，不能由全局余量排除目标及依赖线程的供给问题。

人工失败见 MD17–37：把重叠的 Cookie23.994 与 Network19.041ms 加成约43ms；以单个调度修向席.021ms替代目标全部Runnable3.636ms；每次 wake 被解释为等待上游工作完成；过强声称提高频率不会帮助推进。原始 emit 已含这些句子，系统没有重写。正式上下文 log768、1623、2435、2443、2449–2457 已提供正供给席、不可相加、方向席非状态总量及语义完成未证说明。记录为模型证据使用错误，一次运行不足以证明纯随机，但也不能新立“系统没给证据”的工单；禁止为了命中期望答案改 prose 或增加关键词硬门。

自动补齐 frame bundle/事件普查 824ms，正式 HTML 因果投影和可消除量概览各一份。优先级反转、目标供给10.331ms、上游D/IO、类校验.285ms以及业务线索均在；邻近/背景保持支持席。Binder 库存1段.524ms、35段未关联的边界实际进入模型。

默认根因侧车合法且 available，两项模型选择的时间窗、候选限定、精确数值正确；其中43ms等说明也忠实来自模型，不是系统计算出的总收益。已知 B1623 的“最高线程11.487ms”仍是单CPU桶误作线程总量，作为新 witness 归原工单，不重复立案。机器 FAIL 原因是旧否定措辞 regex 不识别“而非 CPU 算力供给不足”；保留原结果，不能因此把人工错误签绿。

## 下一小批

先处理已有 B1623 的分层聚合和 B1628 的源码注释范围判定。后续仍按两例并发轮换 Cangjie/ArkTS 读与原生 Python 写；不为本轮模型错误重复同题追绿。B1624b/c、B1626 多请求窗、B1622 等开放项保持原状态。无 4ms/旧4m 活跃流年龄降级；真正停滞、用户取消、显式任务上限保留。
