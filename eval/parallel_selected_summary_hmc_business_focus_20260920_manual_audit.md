# HMC accepted business focus：固定双例人工审计

- date: 2026-09-20T14:57:11Z
- sweep_start_ts: 20260920-075709
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_business_focus_20260920

固定实现358f47f4c011，原生构建2026-09-20T14:56:29Z；2并行×1，每例1800秒，不追加第三例、不改oracle。机器1/2，人工0/2。本轮只验业务旧FAIL与明确窗口回归，不替写模式、B2–B6或79项父任务背书。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/hmc_business_focus_20260920/trace_query_wakeup_causal_io_chain-20260920-075711 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 187s | 45 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 明确20ms窗、11ms链上IO及真实链保持；列表正文丢失，另有模型口径错误 |
| 1 | trace_query_business_marker_io_chain | FAIL | eval/results/hmc_business_focus_20260920/trace_query_business_marker_io_chain-20260920-075711 | log_regex,typed_trace_projection_count,trace_attachment,primary_answer | perf_triage+trace_query | 367s | 42 | read=0,repo_map=0,list=0,trace=7,source_lens=0 | midloop=3,inv=1/0,fin_reject=2,unavail=0,prune=0 | FAIL | 正文缺35ms请求；仍用53ms探索窗；未定位/提交实例，新焦点路径未命中；背景排序量冒充实测 |

## 业务例

报告：`.codrax/output/20260920-080316.351-7088.md`；日志为结果目录内`run-1.logs/codrax-20260920-075716-000-7088.log`。

7次查询为3次事件搜索、两线程时间线、唤醒链及根因榜，没有span_window/span_locate、window_stats。只做0.999–1.052探索查询，没有原生实例引用可供选择；一次completion无business_span_ref，补齐skip reason=no_typed_target。不能说系统丢弃已提交引用，也不能由定向测试51/52→50ms通过替本例销账。正文知道业务标记50ms，投影却仍53ms；31ms S态完成闭合阻塞和1ms调度等待保留，但请求35ms未在正文回答。

系统口径gap：原生请求47ms，rank内部按探索窗53×35%=18.55ms压低背景排序量；发布层只对调度状态纠偏，IO资源行漏过。18.55被共享finalizer handoff明说为measured，系统图/表亦显示18.550ms，模型随后照抄。不能归咎模型波动。需保留内部Score/排序/背景资格，恢复有原生凭证的测量口径；不能统一拿累计、区间宽度或描述猜测替代。

analyzer实际6次emit、5次拒绝，涉及artifact锚冒充用户显式目标及typed分析合同不一致，不以摘要ana=1称零重试。finalizer两次拒绝：blocks JSON字符串可恢复但缺限定字段，随后运行时观测ID误当源码evidence ID；没有证明同一字段同时必带必拒。原生35/31/47均在上下文，未到容量上限。mandatory schema2旁路与Trace投影存在，本轮没有首响应或中途静默超时，没有活跃流因暂时无成文而降级。

## 明确窗口例

报告：`.codrax/output/20260920-080016.087-7078.md`；原始附件：`.codrax/blob/20260920-075716-000-7078/attached_trace-5d309add.txt`。

四次查询均保2.000–2.020/TID100，未纳入2.020020窗外Running；链400→300→200→100、D/IO 11ms（2.003–2.014）及占用/累计/有效归因分账通过。Harmony较大数较高优先级，目标52与依赖20支持低优先级候选，未证反转/锁的限制保留。旧补齐families_present，不是选择新焦点的正例。

人工FAIL依据：

1. 报告21–24、30–33只剩编号。原模型section.items携带cells字符串，渲染不消费，run-1.out同样丢失；系统结构恢复缺口，不应补造语义。
2. 报告28“所有唤醒均跨CPU”不符：附件第8行irq-2 CPU004→threadpool target_cpu004为同核。正确原始证据存在，不新增散文扫描门。
3. 报告41及root-causes第51行把1ms说成下游wakee runnable；实际是依赖线程自身wake-to-run，app本窗runnable为0。
4. 报告49“缓存页加载耗时11ms”超出等待调用点与调度IO-wait证据，未有资源请求端点/对象；与系统等待点限制相冲突。

不据本轮断定缺陷由358引入。completion aggregate写2.020020的内部交接瑕疵留档，但未污染最终主窗。历史人工FAIL及HMC-02.4/18.4继续开放；稳定清单13/79已交付、66开放不变。
