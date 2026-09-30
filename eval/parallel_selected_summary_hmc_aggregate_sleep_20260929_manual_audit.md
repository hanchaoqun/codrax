# HMC §199 进程聚合交接与完整睡眠统计：人工审计

- date: 2026-09-30T04:11:03Z
- sweep_start_ts: 20260929-211101
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

本批恰好2并行×1，runner正式exit0；机器2/2 PASS，完整人工0/2。逐份阅读primary、完整answer-transcript、工具调用与实际finalizer上下文，不以oracle命中替代成文验收。固定二进制为`627b714ba`，运行前SHA-256为`9cd21172a8368bd848f6fb3123291ef444dc0b421df47c6d6e336bcd05d3732c`；runner结束清理临时副本，后续构建不可冒充该快照。其后的`d5b4c30c7`纯数字主体兼容修复只有回归收据，不倒签本次live。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_process_profile | PASS | eval/results/trace_query_process_profile-20260929-211103 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 139s | 37 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 3成员、未知及8/7ms热点正确到场；单线程/进程记录数混用、caller越权和无打点误述保留 |
| 2 | trace_query_sleep_summary | PASS | eval/results/trace_query_sleep_summary-20260929-211103 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 279s | 40 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=5,inv=1/0,fin_reject=5,unavail=0,prune=0 | fail | 次数/均值/最长/唤醒者正确；源码关系门删去运行时唤醒箭头，最终图仅剩两条睡眠注释 |

## 1. 进程概览：已证交接修复与剩余问题

- 自然问题沿用`trace_query_process_profile.case`；实际`process_profile`调用未传PID，唯一typed目标补齐TID10成功。未读取源码，不把工具发现的成员变成用户诊断焦点。
- 日志2512行出现`已观测进程概览`，2515行完整成员数3，随后三条thread_account保留ui/worker/late、17/15ms运行、late20ms未知、两条Load8/7ms和submit_bio调用点。§198同分类组合下聚合被投影过滤的问题本次live不再出现。
- 最终正确列3成员，窗外outside-13未混入，late的状态用缺测/横线而非零，UI业务区间内运行5ms/等调度1ms/睡眠2ms也正确。完整窗口百分比85%/75%正确。
- 仍有内部矛盾：正文先说业务打点仅1条，又列worker一条，末尾写2条。实际最终消息的业务树经线程目标筛选只展示ui一条、profile有两条；这不是全量聚合丢失，但需要更明确的typed总体/展示范围接缝，不能把某个carrier行数当整个进程记录数。
- `submit_bio`＋scheduler IO标记只证明等待位置及类别，正文却断言等待块设备完成，表格重复。内核调用点不独立证明资源对象/完成事件，不能靠关键词否决正文修复。
- `late`存在I型ObservedOnly打点；空闭合同步业务热点不等于无任何打点。正文“其他线程无打点”及late“无打点”错误。profile没有每个原始marker种类的覆盖清单，下一片宜在生产者区分原始打点库存与可计量闭合区间，而不是要求用户解释这些口径。
- UI已列2ms可中断睡眠却另称“无可中断等待（D/IO=0）”，属状态词义错误。worker业务窗口未精确列出：profile仅给7ms及来源行，未提供区间端点；不能把此项归类成已给端点后的模型波动。以上保留01.3/04.5/16.4原ID，不能把本例整份答案销账。

## 2. 睡眠统计：数值能力通过，图关系未通过

新增自然问题只含附件、窗口、线程、所求统计及表/图，不向QUESTION灌入未知、右边界、因果等系统约束。确定性fixture在10.001起窗裁掉第一次S状态的1ms：S有2段，窗内总4ms、均2ms、最长3ms；IO有1段，3ms；loader-2/worker-3分别在10.004/10.006唤醒。IO尾段没有sched_wakeup，右边界10.012的switch不作为窗内唤醒。

- 预分析首次仍按未裁剪起点算S均2.5ms/最长4ms；后续同窗`thread_timeline`、`wakeup_chain`产出正确统计，正文数值和唤醒者全部纠正。预分析不是最终事实权威，但它带来的矛盾上下文和额外模型负担留01.3/16.4。
- 逐段复核实际finalizer消息发现第二处系统缺口：原始wakeup payload有正确`state_statistics`，但最终消息的所求事实索引仅列旧D/IO等待记录，没有新的完整睡眠统计摘要。实际分类为状态＋等待＋次数、带显式窗；旧公开测试没有显式窗，不能证明这个组合。扩大为实际分类×有无direct_waker×timeline后wakeup查询序列后先复现FAIL；`9daf89eb8`补同一typed事实索引登记后公开测试PASS。只影响软排序/展示，不扩因果或旁邻权限；这不是本次live已用修复版的证明，原人工FAIL保留。
- Analyzer有4次emit尝试：把附件发现的loader/worker当用户明确参与者/lookup而被原句身份验证拒绝，最终去除。硬边界正确，schema教学与自动补齐角色区分仍需降噪，不放宽原句权威。
- Finalizer首稿包含两条真实唤醒关系及状态自箭头。日志2596附近报`missing_call_anchor`，后续模型补`call`又被`call_edge_unproven`拒绝，最终删除唤醒边和孤立参与者，图仅留target的两条S注释，IO/唤醒/具体时点均未表达。共7次成文工具调用（首次emit＋6次patch），指标记5次reject；这不是美观问题或Mermaid渲染库限制。
- 源码审计：`diagramCallEdgeEvidenceMismatchesWithRequestModel`只对QFRootCauseTrace整体退出；本次generic+flow进入源码关系所有权。已存在的report-local runtime通路处理frame temporal及business contain，未覆盖普通有限事实问法的wakeup事件关系。正确改进是生产者来源/同窗端点/关系类型驱动的局部所有权与修复候选，覆盖多图、多来源和重复实例；不是把有trace附件的整份报告全局豁免，更不能改为根因类型以绕门。
- 完整transcript的状态附注保留D/IO区分及3ms，系统事实对照有loader拓扑；附录不修复正文残缺图。窗口裁剪统计不代表物理完整等待时长，最终没有明显说明首段从窗外开始，宜由同一typed统计scope表达，不能再把约束塞回用户问题。

图关系来源接缝为已证系统P1，挂16.4/12.5/04.5，优先于已充分供证的局部措辞。不得用只有一个线程的空关系图通过机器格式检查就称图能力验收完成。稳定验收父项仍5；本批两个完整答案均保留FAIL，不追第三例求绿。

## 3. 原始证据指纹

以下路径均相对仓库，原件未覆盖。机器汇总与人工判定独立；开发回归版本见主账本§199。

| 原件 | SHA-256 |
| --- | --- |
| trace_query_process_profile-20260929-211103/run-1.primary.md | `090e42d48e8bd014c726c5d231b580e284dde1450c77b3fdc720585df05fca95` |
| 同目录run-1.answer-transcript.md | `0ab8bb049f740c2063af1e6b9323aca3a60930dbe1dea848c3235fa278e02185` |
| 同目录run-1.logs/codrax-20260929-211106-000-35984.log | `11ace496958924af98a7667bc86c914d3f5764e46f202ff006bc87102086dc71` |
| trace_query_sleep_summary-20260929-211103/run-1.primary.md | `fec5dfa147ffd5732ca91f32920805955285332febf7a8841e749b72035fb4c2` |
| 同目录run-1.answer-transcript.md | `2272dd07f8704858ee8a622a08924510f607689f1f5cd0a3668e861305317f11` |
| 同目录run-1.logs/codrax-20260929-211106-000-35983.log | `2e7ba8441e92fec2011c784103745029673a2b7400d5837fd47ebf4f9f336ffb` |

完整根目录为`eval/results/`。两例LLM日志仍使用首响应600秒/静默300秒/非流式600秒；本批没有活跃流固定时长降级，也没有写模式验收收据。
