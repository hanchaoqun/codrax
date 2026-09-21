# HMC 原生身份交接：固定双例完整人工审计

日期：2026-09-21。构建revision `68730664c8cb`（源码`a15614f57`＋`bfa381902`），buildTime `2026-09-21T11:47:55Z`。runner40831正式exit0；2并行×1，每例1800秒，无第三次追绿。结果根`eval/results/hmc_native_identity_crossmode_20260921`；[机器摘要](parallel_selected_summary_hmc_native_identity_crossmode_20260921.md)。

| 场景 | 自动 | 完整人工 | 子结论 |
|---|---|---|---|
| nested_python_increment | FAIL，237秒 | FAIL：整体未闭环 | 实现、原测试保全、真实测试PASS和最终状态诚实分别通过；3个required仍缺绑定 |
| trace_query_business_marker_io_chain | PASS，253秒 | FAIL | 请求35与业务40补回；混尺、事件方向误述、系统用户根错位仍在 |

## 1. 写例：身份可见性命中，执行成功不等同合同证明

目录：`eval/results/hmc_native_identity_crossmode_20260921/nested_python_increment-20260921-044832`。检查计划、3次报告、delivery tree、完整请求/工具日志和最终输出，不以机器失败推定代码失败。

- 唯一交付diff为`packages/widget/widget.py`的`return value`→`return value + 1`。测试、setup配置、依赖均未改。测试文件seed/delivery SHA256均为`504b2535413cafa883be2802d92a698036735de8883384530caec4fa8375f322`；实现原/新hash为`a7cacc256c5c36de85546785b94088dc0fa61747baac427e0490342c6b2b3679`→`833556db828032d49efc4c58a380411c53465c917da89bf613a607a10cee2ba6`。
- 嵌套unittest真实执行3次，每次3方法通过，覆盖负数/零/正数及大整数。根目录zero_tests未覆盖整批PASS。共5次run_tests尝试：3次正式验证、1次planner非dry-run被拒、1次合法dry-run。
- 原计划3个required regression合同仍0/3。声明suite=`python@packages/widget::IncrementTest`且ID为短方法名；真实suite=`python/unittest@packages/widget::tests.test_widget.IncrementTest`，ID也带`python/unittest@packages/widget::`。声明确实不匹配，不是正确声明被validator错拒。
- 新snapshot实际到达7区块、21行完整身份：apply日志工具回包672/1105/2837，controller请求822/1255/1798/2994。source-free planner未命中snapshot，未回填旧报告。共享教学进入2次planner请求、共4处system/user文本；字段schema未在日志完整展开，不代签字段schema live命中。
- 首次controller把“原生PASS+存在PTO”误当系统映射问题申请all_verified，既有校验正确改为verify_batch。source-free新增PTO在2393行被拒；probe-only也没抹去required。终稿“测试通过／未完全验证”诚实，B2–B6未闭环。

下一高ROI接缝：`internal/types/write_context_pack.go:600–608`的声明摘要只有id/path/refs，漏了声明suite/id。该通道240-rune截断，不能简单拼长ID；需当前声明/当前观测分别完整、有界、带来源地并置，不自动匹配或放开source-free登记。

## 2. Trace：自动命中数字不代表解释正确

目录：`eval/results/hmc_native_identity_crossmode_20260921/trace_query_business_marker_io_chain-20260921-044832`。以下日志号为`run-1.logs.all.log`，正文号为`run-1.principal.md`；另审真实query blob、系统投影和根因JSON。

| 对象 | 窗口/耗时 | 独立计量 |
|---|---|---|
| OpenDocument | 1.000000..1.050000，50ms | main 5ms running＋1ms runnable＋44ms S |
| 查询 | 1.000000..1.051000，51ms | main 6＋1＋44；worker running 9.5ms |
| LoadDocumentIndex | 1.004500..1.044500，40ms | worker 8ms running＋1ms runnable＋31ms S |
| 请求 | issue1.005000→complete1.040000，35ms | 请求驻留，不等于睡眠 |
| worker闭合阻塞 | S1.009010→wake1.040010，31ms | S态也可被精确IO完成/唤醒证据关联 |
| 唤醒后调度 | worker1.040010..1.041010、main1.045..1.046 | 两段分别1ms；不重复计算别名 |
| backup | 请求驻留47ms | 无本次主链绑定，仅背景 |

最终输入正确信息已在场：日志2486/2487分别Open50=5+1+44、Load40=8+1+31，有起止、源ID和不得替换为宽查询总量说明；2340–2343/2513另列query51=6+1+44，2630列worker同查询9.5。2500正确并置request35/wait31，2499明确未证不等于已证明没有唤醒。

正文仍失败：3及25–29行把50业务窗填为6+1+44=51；31–35行将Load40的8与query9.5混成8–9.5；46行同句先称1.009010发请求又给真实1.005..1.040=35；56行把backup-irq完成执行者写成被唤醒者。两段1ms保留，但app scheduler_latency/runnable同一份额描述含混。表标题退化“列2…”且Axis A/B、completion_closed、issue_to_complete等内部词面外露。不是原始trace或模型输入缺这些事实，不新增原文硬门。

### 2.1 系统P1：通用实体被晋升为用户根并截断真链

唯一wakeup调用log1485目标app-main-100，`trace-query-result-370c9d2b.json`目标pid100、真实路径storage-irq→worker→app。analyzer log897却在`no_named_target`下给通用entities，把用户未点名的document-worker排在app-main前。

`internal/types/observation_ledger.go:767–769`将这些Entities放入AnchorUserEntities；`trace_causal_projection.go:2671`按实体序选举，2786允许bare comm，2696–2698截掉worker→app后缀并标UserElected。tree消费该标志，系统输出`.codrax/output/20260921-045243.605-50043.md:139`把worker标“用户关注线程”，主句/总览错称自身，但板锚/状态仍app-main。同轮最终输入2539正确原路径，2344已是截短投影。这不是另查worker的合法导航，也不只图布局；不能只换标签掩盖截链。

登记独立P1：对齐typed RuntimeTargetProfile与用户根选举权限，公开no_named_target＋业务对象先红；同源保护ledger和显示焦点，正保明确named target、别名、cursor排除及旧无profile；不扫原文、不按实体提及序推因果方向。

### 2.2 投影、旁路与重试分账

1个Trace因果投影仍在，业务50/40、链上IO31、两段调度1、背景47未删。新P1影响树根/相对位置，不据此判所有原始测量或排名失效。`.codrax/output/20260921-045243.605-50043.root-causes.json`为schema2/status available，3项worker IO .031、main调度 .001、worker调度 .001，窗口1..1.051；证据保app-main目标与链上层级，backup未进根因。模型description仍把1.040010唤醒叫完成中断，不以文件存在代签语义。侧车重编号1/2/3与正文原榜1/2/4不是新缺陷。

analyzer807→811拒bounded_effect/causal冲突，863保留causal＋effect两required，867另拒缺scenario，897成功；此前双角色修复提示实际命中。explorer1565拒虚构relation authority，1583移除后1591成功。finalizer2782拒summary缺caliber，2858只补typed字段、2869成功，正文未修。最终两轮2731/2843，无历史裁剪。自动oracle只测身份/数字出现、背景措辞等，未覆盖数字与窗口的绑定。

## 3. 收据与下一队列

实际消息/实际schema均有效RED→GREEN；公开Python/Go、8协议×3scope、相邻/race通过；独立只读复核PASS。末版全仓session17118正式exit0：87测试包、13无测试包、零FAIL，tool432.783/agent122.240/tracequery143.528秒。未改matcher、权限、原生producer、Trace测量或模型正文。

自动1/2，完整人工0/2。下一优先级：系统弱实体冒充用户根/截链P1→当前声明/观测完整并置→业务/查询同卡分尺；B2–B6安全来源授权/只读补绑定继续开放。父账仍13/79交付、66开放，不回写旧FAIL，不降低oracle、不追加第三例。
