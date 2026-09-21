# 固定 93858fb94a05 双例人工审计

- date: 2026-09-21T01:49:42Z
- sweep_start_ts: 20260920-184942
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_business_ref_assertions_20260920

固定93858fb94a05，2并行×1。机器2/2，人工0/2。已读最终MD、原始查询/成文日志及schema2旁路；没有第三例追绿。其后逻辑工件别名兼容补正不在此二进制中，不能借本轮回放签该支线。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/hmc_business_ref_assertions_20260920/trace_query_wakeup_causal_io_chain-20260920-184942 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 174s | 43 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | 保20ms/11ms/投影；将400→300唤醒2.016写成2.014，并从调用点扩写具体缓存IO机理、将11ms解释成完整20ms等待的原因 |
| 1 | trace_query_business_marker_io_chain | PASS | eval/results/hmc_business_ref_assertions_20260920/trace_query_business_marker_io_chain-20260920-184942 | log_regex,typed_trace_projection_count,trace_attachment,primary_answer | perf_triage+trace_query | 274s | 46 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 50ms业务窗混52ms测量，7ms运行应为5ms；前文31ms当请求耗时、末文35/31又正确，内部字段泄漏；引用兼容支线未命中 |

## 业务答案及上下文

报告`.codrax/output/20260920-185414.830-5901.md`。本轮只用了四次模型trace查询，wakeup_chain明确传0.999..1.051；没有调用business_span_ref，completion也没有接受实例焦点。原生统计有完整业务对，补齐按原探索窗执行root_cause_rank/critical_blocking_calls，投影与JSON因此恢复，不能据此声称新“冗余字段安全兼容”已live命中或业务选窗已经闭环。

正面：正文自己写了LoadDocumentIndex40ms，本轮不是靠系统表骗过正文关键词检查。IO末段清楚列出request35ms、issuer blocked31ms，后台47ms不入链上根因。schema2 `status=available`，两项31ms IO和1ms目标调度等待；root文件存在且合法。

仍失败：正文承认查询窗52ms，但反复把其中7ms运行移入50ms业务窗，且称7ms发生在1.046..1.050的4ms段；真实业务窗运行5ms。前文把1.009010..1.040010的31ms睡眠叫“请求发出到完成”，末段又正确写1.005..1.040请求35ms，前后矛盾。将worker自己的醒后runnable写成“唤醒前”易混主/对端阶段；后台唤醒者名字混写storage-irq-81/backup-irq-81。`cross_direction_physical_relation = unresolved`、`request_residence`、`issuer blocked`、`subtotal`等内部字段混入正文；这不是JSON不能解析。

两项1ms在本fixture确为不同时间段，系统原方向组提供互斥合计2ms，因此不能沿用上一轮“无证相加”的FAIL理由；错误重点是窗口/请求尺和内部词。analyzer曾对source_quote及causal scope+fact_families组合拒绝，后续有合法出口；尚未证明必带必拒。

## 明确窗口答案

报告`.codrax/output/20260920-185233.785-5887.md`。目标20ms全睡眠、11ms链上IO、三项1ms候选和Trace投影保留，最终没有沿用explorer中2.020020秒当用户窗的草稿；JSON `status=available`，两项链上候选。不得把探索阶段错误全算作最终错误。

最终正文仍把threadpool-400→network-300唤醒写为2.014秒（真实2.016；2.014是上游irq→threadpool事件），同一错误还在列表重复。把调用位置扩成“等待文件系统缓存页面完成IO”，又断言11ms经链传导导致全部20ms等待，超出该证据的机理/定量边界。阶段标题顺序1/3/5/4且缺2，属于表达留债。本次“类别标签不赋予链上根因资格”是一般性限定而非明确撤销三项候选，不能直接复用78079的断言理由。已交付事实卡边界与原生时间证据均在最终上下文，未证系统改写了时间；暂不加散文硬门或针对固定时间修补答案。

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
