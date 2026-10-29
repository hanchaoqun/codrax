# HMC313：量测元数据隔离、原始合计量尺与代表规格复验

日期：2026-10-29（UTC批次日）。候选分支 `codex/hmc299-wait-interval-candidate`。基线 `9067124455afa46a59f5316ad51c6ecb70d41828`；main 保持 `788588e365d40a48b30827ffd8db08dae4e2ae9d`。

结论：**原量测域隔离回归修复，唯一全仓通过；两个完整答案仍失败，不合入 main。** 原问题、断言和旧失败未改，未追跑第三例求绿。

## 1. 计数与交付口径

| 口径 | 本批结果 |
| --- | --- |
| 稳定任务账本 | 79唯一ID＝26已实现交付＋53开放，18类 |
| 开放状态 | 18部分实施、29待实施、3待验收、2验收中、1持续 |
| 新增完整能力交付 | 0；11.2仍未通过原有限合同 |
| 候选分支有界子能力 | 2组：纯量测元数据隔离；原始段合计范围/量尺披露 |
| main新增 | 0 |
| 当前批剩余验收 | 3组产品原因＋1组评测精度；归原ID，不新增4项稳定任务 |
| 两例完整人工 | 0/2；正确子表/新说明不抵消错误正文或缺图 |

18类开放数量依次为2、1、1、5、2、2、4、6、3、2、1、5、3、5、3、2、2、4。根与独立代理按实际复选框核对；不是直接沿用上一轮口头数字。详见[统一实施清单](../docs/design/hmosperf_implementation_tasks_20260917.md)及[§313统一账本](../docs/design/hmosperf_capability_gap_audit_20260917.md)。

## 2. 对照参考仓的发现与修复

本地参考 `/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main` 只读：

- `core/query_engine.py:461–496` 保留结果对象，另作展示副本。对应本次缺陷不能通过删原生JSON/来源字段解决，必须在展示事实比较层精确分离已分类元数据。
- `core/preprocess/thread_sched_ops.py:306–319,338–350,365–382` 分开窗口裁剪、未知值与显式统计分母。包络不是连续执行证明，合法跨状态合计也不应被误判为错误数字。
- `core/batch/closure/select.py` 对已确定manifest软排序，不是自然问题的事实族分类器。不能因其存在就认为本项目自然意图到精确查询规格已闭环。

### 2.1 量测域隔离回归

先原样复现 `TestB1638B1ActualQueryPayloadPreservesMeasurementDomain/thread_timeline/0`。旧JSON往返测试的其它绿色叶子丢掉了private timeline fallback，不足以证明活体路径；新增真实parser/engine→原生查询发布的五view × 三种metadata-only对偶，六个真正事实变化负控。

`trace_query_native_timeline.go` 只在producer值拷贝的哈希输入中清 `MeasurementDomain`。原对象、原生JSON、`MeasurementSources`、独立查询记录、精度、端点见证及来源/窗口校验不变；没有删任意ID、按文本模糊去重或从JSON恢复私有权限。

五活体view为thread_timeline、window_stats、wakeup_chain、root_cause_rank、critical_blocking_calls。事实负控包含duration_precision、head_witness、interval_summary；可见JSON相同也不能把不同私有事实当同一显示凭证。原断言未放宽，唯一全仓五个旧叶子与父项全部PASS。

### 2.2 原始段合计与范围披露

公开定位后更正§311/312尚未决的诊断：**6ms/41.5ms不是上游算错**。原始段合计分别是5运行＋1就绪、9.5运行＋1就绪＋31睡眠；ActualWindow给出1.045–1.051与1.004–1.0455的原始段定位包络，不能当连续执行、同状态或业务窗总量。

新共享formatter在范围已发布且有限、非负、递增时说明原始合计/范围及量尺边界，正文树与摘要走同一处；缺失/非法范围保原兼容出口，不反推新范围。未改聚合器、原值、原JSON类型或合法2ms小计。公开query→投影→emit→noop patch保原4排序合同、31ms闭合睡眠/35ms请求驻留/47ms背景。

自然A恢复稿MD191/226、HTML337/376已使用新说明，限定PASS。不能拿恢复渲染子合同替代完整图或最终根因选择验收。

### 2.3 代表规格：有证据才改系统

独审确认 `trace_catalog_plan_schema.go` 已复用普通query的view枚举/短教学，本次模型明确提交event_search，编译器保留了该值。没有“已受理resource_stack被丢弃”的反例，所以本批没有伪造RED、重复添加枚举或用词扫描强制选工具。观察意图到精确view的更完整typed映射仍需分类及消费端审计，归11.2/16.4开放项。

## 3. 小改验证、冻结与唯一全仓

所有原RED与夹具前提错误都留原件；代码冻结后不修源码、测试、注释或helper。

| 阶段 | 实际结果与边界 |
| --- | --- |
| 隔离原针RED | 原thread_timeline/0失败，原断言未改 |
| 隔离新活体RED-v2 | 五view×三纯元数据变体失败；六事实变化负控仍通过 |
| 隔离GREEN | 8顶层/79子例PASS，2包 |
| 隔离相关 / race | 83/426、14/103 PASS，各2包 |
| 量尺公开RED-v2 | 1顶层/2子例FAIL，新披露前置未满足 |
| 量尺相关 / race | 各3包286顶层/340子例PASS；属于最终措辞收窄前版本 |
| 最终量尺精确race | 4顶层/9子例PASS；最终措辞不声称总量必然混状态 |
| 标准CLI构建 | 一次普通go build，实际exit0、source_unchanged=true |
| 冻结后唯一全仓 | 98包PASS/12无测试；28527顶层/33773子例PASS；43/14 SKIP；零FAIL |

隔离初版新夹具对无private timeline的view与nil head的假设错误、量尺初版把rank0也计入正榜单、过宽筛选混入隔离RED等非最终输出均留档，不当产品失败根因或最后绿色证据。零放宽断言、零失败日志覆盖。

唯一命令 `go test -json ./... -count=1 -timeout=30m`；PID83006，父spawn前时间12:00:15.469389Z，真实结束12:11:45.990534Z，exit0、recovered=false。262177行全量JSON无非JSON、重复终态或未结束项。原B1638 thread_timeline/0在日志253596通过，其余四叶及父在253600–253614通过。

冻结7502仓库输入＋154外部＋16仓外快照，SHA256 `ea9f793c4d8bf2c68092f2fac050a5edf001c89ba2bbd68a92615859fb9e1e32`。唯一CLI SHA256 `ab1bcc57f09d741994ba70ada6aa09187103e6668386eb3aa21bc5a84782a0e1`；冻结后字节复核通过。provider只传路径，不读/hash/复制内容；快照全部仓外，Go副本以.go.txt封存。

## 4. 两例并行真实评测

| 项目 | A：OpenDocument业务IO时序 | B：代表选择有限合同 |
| --- | --- | --- |
| 原入口 | trace_business_io_sequence，原题＋原Trace | 普通命名captures目录，原自然题 |
| 模型预算 | 15步，n=1 | 50步，n=1 |
| 材料性质 | 既有构造文本Trace | 两既有构造SQLite源的四只读路径副本，不是四次设备采集 |
| CLI PID / 正式退出 | 83059，harness83014；exit0 | 83015；exit0 |
| 原判据 | FAIL degraded_answer_checks_skipped:1 | 原Go auditor真实exit1，规格/完整成员不符 |
| 完整人工 | FAIL：恢复稿、无合格时序图，正文范围/时序/优先级冲突 | FAIL：代表规格错、跨源身份、执行/关系解释混用 |
| 有限改善 | 新原始段量尺说明到场，31/35与2ms、47背景保留 | 六原生资源结果和16表正确；最终帧列表未再强拼调用链 |

barrier真实release12:00:20.286748Z，A/B exec分别20.997444/20.997440。真实模型请求重叠不只靠进程时间证明：A原日志2941–2948与B1301–1302的可证交集为12:01:38.975–12:02:00.208Z，**21.233秒**。

独审发现冻结overlap helper在同model多个pending时清空尚未响应项，原文件20A/32B、3排除记录不能全称严格唯一配对。保守关联为A20/B14；最大交集仍早于B首次歧义12:02:13.113，故并行事实成立。A区间自身结束12:02:42.880较晚但A全程唯一，不误说两个完整区间都早于歧义。保留原统计与[独审更正](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0217-codrax-hmc313-integrity-audit-v1.md)，未改helper/重跑模型求绿。

### B同代结构验收，不换代重试

末次普通status真实call `4351c0726e9c4fa48d80076d`，B原日志2611参数、2623成功返回30938B；完整result SHA `e84c7e86cebde39a5e3c6ae30d29b663ce6ec415e68ad6c4f22788da151b477d`。

- 同次index SHA `4fc3716fa7c98192b1d788ae0770476127ce2f2c6a0584a5877a0081a1ee0e63`。
- 同次selection snapshot SHA `dcbef4d2f86d9daa3d5fd664f9ed963a437874eb3470d7ed7a411360317c4558`。
- 原auditor实际PID5600，12:07:06.094610Z–12:07:06.645292Z，exit1：`cluster specification or full membership differs`。不是未执行；没有成功JSON也不能降为NOT_TRIGGERED。

8计划/2组/4选中/4未选仍是event_search，后补resource_stack不改变计划身份。当前17记录＝8声明＋9观察；25尝试＝23success＋2empty；13有尝试/4无尝试。4个选中计划各三次均success，不是第三次才成功。13去重payload不等于13调用。相同数字窗也有端点差异：event_search含10.050事件，资源统计为[10,10.05)。

## 5. 剩余缺陷与下一ROI

| 优先级/原ID | 已证边界与下一动作 |
| --- | --- |
| 高：16.4/18.2 | A首稿worker ID一位抄错；正确ensure_visible只改Body，旧RuntimeParticipantsV1仍在。用原稿＋fresh lease公开复现允许修补覆盖，不放宽身份门、猜长ID或先断言所有路线死。iter7已staged但未accepted。 |
| 高：11.2/16.4 | 任务观察意图到精确view/端点及执行归属；已有枚举不能误报缺失。只读登记原终态可轮换，避免同题无限局部优化。 |
| 中高：16.4/16.5/18.2 | A50ms业务/7ms宽窗运行、4.49ms跨唤醒及方向；B将C202借D、未选计划混另规格执行、配对/负载越权。复用来源/主体/窗口/量尺/关系，不扫描自由文字设门。 |
| 评测精度：18.4 | 请求歧义组清零前不得恢复唯一配对；补精确ID或保守生命周期自测。原21.233秒仍已证，原helper输出保留。 |

3产品组＋1评测组不是整个仓库所有欠项数。旧采样/IO解释、权限未触发、§310扩大race与未知范围兼容出口继续记录。二进制17.6只有具备新真实格式/工具/宿主材料才扩验收，同字节再包装不算新覆盖。WAL未满足零副作用前不接生产。Trace因果投影、自动补齐、合法重试/额外未选查询、原查询总体和因果权限未改变。

## 6. 封存、提交与推送

本批434件按原来源身份＋SHA封存，索引SHA256 `d66645d4f4b184db6a3cf531198463c01fddb0bf08be49a42f77b839e8cd3418`。旧14档4041件/320快照来源身份逐字节保全，其中18历史污染来源是320子集；旧迁移tmp过期只核封存映射，不恢复进仓。封存不代表验收通过。

- [封存索引](results/hmc_display_spec_representative_20261029_v1/sealed-index-v1.json) / [封存收据](results/hmc_display_spec_representative_20261029_v1/sealed-receipt-v1.json)
- [原全仓日志](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0216-codrax-hmc313-full-v1.log) / [全仓真实退出](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0213-codrax-hmc313-full-v1-receipt.json) / [完整统计](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0212-codrax-hmc313-full-stats-v1.json)
- [A独立人工审计](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0130-codrax-hmc313-A-audit-v1.md) / [B独立人工审计](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0149-codrax-hmc313-B-audit-v1.md)
- [B结构审计真实退出](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0167-codrax-hmc313-B-structural-audit-v1-receipt.json) / [原FAIL日志](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0170-codrax-hmc313-B-structural-audit-v1.log)
- [独立执行完整性审计](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0217-codrax-hmc313-integrity-audit-v1.md) / [代码复核](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0186-codrax-hmc313-code-review-v1.md) / [主代理交叉核验](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0411-codrax-hmc313-root-review-v1.md)
- [公开隔离证据说明](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0222-codrax-hmc313-isolation-handoff-v1.md) / [代表规格只读审计](results/hmc_display_spec_representative_20261029_v1/sealed-v1/0434-codrax-hmc313-spec-audit-v1.md)
- [人工表面汇总](parallel_selected_summary_hmc_display_spec_representative_20261029_manual_audit.md)

六文件代码提交 `34c819ad92d70aaf2ee904ea04f85674496ff27e`，随后五文档相邻分笔；普通推送及两提交全部冻结字节验证以[publication收据](results/hmc_display_spec_representative_20261029_v1/publication-v1.json)为准。收据存在且通过才称候选已推送，不预写不存在的docs SHA；该收据只证候选完整性，不能代签两例FAIL、未触发分支或main验收。
