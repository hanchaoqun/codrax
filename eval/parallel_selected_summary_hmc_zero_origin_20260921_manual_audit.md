# 零起点修复固定双例人工审计

- date: 2026-09-21T13:59:15Z
- sweep_start_ts: 20260921-065914
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_zero_origin_20260921

冻结代码 `0b9e890f2`，2 并行 × 1，无第三次追绿。机器 2/2 通过；完整答案人工 0/2，物理计量/交付通过与整份答案语义失败分账。旧人工失败不倒签，本片不代表全部 HMC 父项完成。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_zero_origin_wait_account | PASS | eval/results/hmc_zero_origin_20260921/trace_query_zero_origin_wait_account-20260921-065916 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 154s | 33 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=1,inv=2/2,fin_reject=0,unavail=0,prune=0 | FAIL（计量/范围/清单通过） | 真实无窗 timeline 命中；7+1+2=10ms，IO 一段 2ms 正确。模型过度否定局部零值、未验证 Binder 写零、内部词/重复表；系统清单另有非 IO D 计数标签遗漏 |
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/hmc_zero_origin_20260921/trace_query_wakeup_causal_io_chain-20260921-065916 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 258s | 46 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL（因果投影/原生量保留） | 20ms 主窗、11ms IO、三条1ms供给候选、四节点路径及旁路均保留；三边误称四跳、唤醒方向反述、Binder零值越界排除、有效量误作可消除承诺；sidecar还误说相邻区间重叠 |

## 1. 零起点：确认修复真正到达模型和可见答案

读取全部 `run-1.principal.md`、`run-1.out`、相关日志及 `.codrax/output/20260921-070147.677-70155.root-causes.json`。实际模型仅调用一次无窗 `thread_timeline`（log1146），原生结果保 0..0.010；系统按已接受 full_artifact/named target 另补 `window_stats`（1334–1336），两个结果皆为 Running7、Runnable1、非IO-D0、IO2、Total10ms。IO完整一段为0.002..0.004秒，次数1，单一kernel caller不当对象/holder；最大状态段5ms。物理首段0..0.002未再丢失，最终输入2039/2040/2141与成文均实际保留，不仅是测试针。

模型侧留债：caveat把完整状态分区中sleep=0/非IO-D=0退化为“仅清单未匹配”，混淆真实测量零与证据缺失；没有独立验证的Binder清单写“0（未验证）”不是有效零事实。可见 `io_wait` 等内部字段和“无外部时间窗绑定”仍多；修订再加一份状态表而保留summary内同表。系统owned清单也写“D状态0、IO等待1”，实际前者是非IO-D桶，系另一确定展示缺口，已单独记账，不改物理量以迎合文本。

过程：analyzer接受有限事实/整工件/具名主线程，未再误判无目标。第一次调查完成因value1与4个members不一致被拒，第二次成功（1224–1266）；runner的inv计数字段保原样，不能据此认定两次实质拒绝。成文零硬拒绝；一次结构化member_set承载提示后模型新增typed table，属于软修订，不是畸形JSON或超时降级。可执行提示能否按typed承载形状减少重复，留P2审计，不能扫描summary原文自动造表。

schema2旁路实际存在，root_causes=[]、reason=trace_root_cause_contract_not_active。有限统计没有根因选择授权，空值合理；不为使旁路非空强开因果投影。

## 2. 明确时间窗：核心因果链保留，整篇语义仍失败

读取全部principal、out、日志及 `.codrax/output/20260921-070331.913-70140.root-causes.json`。本轮named app-100、explicit2.000..2.020、required causal_attribution正确；首次causal/fact_families不兼容被拒，模型仅删families后接受，没有降为finite。四个原生查询与最终输入保留完整20ms目标sleep、依赖17/14ms睡眠、threadpool11ms D侧IO/fscache，以及三个独立1ms供给候选。平台已发布Harmony优先级语义，prio20/CFS、prio52/RT的标签不能按Linux规则另判错。

可见核心量和四节点路径保留；但模型称四跳，实际列的是三条边；把cookie/network/threadpool说成被app唤醒，方向与其自己列出的threadpool→network→cookie→app相反；从无已关联Binder等待推断本次睡眠不是Binder传导，越过未观测边界。11ms有效归因不是已证明可消除的优化承诺；系统图表的潜力/可回收标签是否也存在同类歧义，另审精确生产语义，不只归模型错误。

旁路4项根因各自保同一请求/查询窗及0.011/0.001/0.001/0.001秒，三个priority候选保mechanism_qualifier与runnable1ms/running_deficit0，不丢链上席位。但model-owned descriptions也承接反向唤醒说法，并把threadpool IO[2.003,2.014)与Runnable[2.014,2.015)说成“重叠”，实际只是同一线程且相邻。不得因此扫描描述字符串做新硬门/系统重写结论；继续区分原生结构正确与模型语义失败。
