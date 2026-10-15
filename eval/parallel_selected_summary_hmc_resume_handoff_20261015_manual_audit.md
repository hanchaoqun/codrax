# HMC247 双例完整人工审计

本批严格两例并行各一次。A机器PASS、B独立index审计FAIL；两例CLI均exit0，完整人工均FAIL。未改问题/原件/最终答案，未追跑第三例。`resume`自然入口实际命中属于新增子能力收据，不等于HMC11.1完整交付。

## A：采样事实已齐，成文状态与分母仍错误

完整输入SHA `da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`。默认准备12000条，目标窗/TID1609为164记录、259 cycles权重。内核地址与回调各82记录；分别174/259=67.18%、85/259=32.82%权重，各占记录数50%。记录CPU1/0不授执行状态，sample_kind全部unknown。

实际finalizer原生summary/distribution两热点24行加元数据4行完整投递，预算28/128、0省略；状态依据明确“未确立（样本类型未知）”。完整热点首末时刻为486489.588096–486489.589609与486489.464088–486489.465612。原生字段、CPU、未知资格、记录/权重独立分母均有证据，不再将这次错误归为parser缺字段或预算漏发。

最终正文仍写“CPU时间集中分布”“周期性的CPU执行快照”“线程当时在消耗CPU”，并把174/259的权重比例说成“约67%的样本”。两段按桶边界写的范围又被称为样本首末时间包络。结尾承认无法定位停顿，不抵消前文状态越权和分母错误。

探索completion的持久化reason还错误声称122ms off-CPU等待/等待下次传感器唤醒；但本次实际finalizer没有重放该reason，旧runtime-only typed抑制有效，不能错误归责“该reason污染成文”。新可选`native_fact_handoff`没有被模型使用，记NOT_TRIGGERED；实际日志未保存完整wire工具schema，不能从未选用反推字段未广告。代码/实际adapter的schema回归和自然采用是两份不同证据。

实际explorer加载的旧通用教学仍把perf输出称作code-execution context，并泛称热点解释线程正在做什么，与unknown负资格冲突。后置修复`7daeb6e17`原位替换三处默认叙述；真实adapter和注册skill公开RED→GREEN、完整skill与旧教学保护通过，保链上running/算力供给不足的合法采样支持。不声称该教学是最终幻觉的唯一成因，不重跑模型倒签本例。

根因sidecar没有可选择的链上根因；Trace因果投影/自动补齐保留，没有制造调度、唤醒、D态或IO主因。自然采样失败重试、完整树、模型图未触发。当前样本不能代验所有二进制格式或全部图。原始报告：本地证据根`verification-receipts/codrax-hmc247-sampling-live-manual-audit-v1.md`。

## B：恢复已触发，原计划执行与事实交接未完成

operator独立进程准备的是两份既有构造SQLite，不是实机新采集或自然中断。原checkpoint为137行/4878字节、4计划/1历史attempt，SHA `c6fff458274873e7b613dd50e660d6cceb4b1568ac351af5910214c8934b6ab7`，运行前后不变。

本轮两次完整`read_file`均收到历史导航；模型实际执行`trace_catalog resume`成功，当前4计划/0attempt/4not_executed，历史单独留存。原成功没有被还原成当前成功，明确给出四个可直接执行的query引用。

| 原计划 | 本轮结果 | 本轮attempt |
| --- | --- | ---: |
| capture.data / thread101 | not_executed | 0 |
| nested/capture.data / thread101 | not_executed | 0 |
| capture.data / thread202 | success | 1 |
| nested/capture.data / thread202 | empty | 1 |

模型没有使用`catalog_query_ref`。其后两次all查询各success，属于不同参数计划；最终6计划/4attempt、3success+1empty+2not_executed。不能因为all结果包含101记录，就假记原101精确计划完成；也不能反向因101精确计划未完成而说已观察到的101事件不存在。

原生当前payload已包含root101的10.005分配512B、10.025释放512B、10.040分配256B，root202的10.015分配128B，nested101的10.005分配512B；保各自完整/缺深度/重复深度的原始帧，nested202已查空。终稿却多次称只有SQLite表结构、没有事件或调用位置。实际finalizer ledger的`deterministic_runtime_queries=0`，主行是read_file/schema/current_source；这是真实共享事实交接缺口，不应简单称模型波动。另将当前6计划归给未更新的checkpoint，仍混历史/当前来源。

只读追踪发现上述计数属于成文promptLedger，不是完整持久化ledger：命名目标投影尚未识别resource_stack的typed owner，通用Subject不足以通过线程归属检查，数据在进入专用资源展示之前被过滤。此为真实参数加代码路径定位，公开复现/修复留下一批；不能通过改显示名、谓词整包放行，或将一条匹配成员扩成全部成员来修补。精确查询的已查空结果亦须保当前来源和选择范围，不能因没有owner行就再次消失。

resume前四次trace_query确被当轮工具面拒绝；resume后实际工具数16→17、第二dispatch同17已执行查询。因此不能据模型自述“工具仍不可用”就放宽工具准入。已证旧不可用回执留在历史消息中，而既有工具提示只比较本轮base/effective缩小；跨轮变化时效提示作为后置修复单独验收，不倒签这次自然失败。

独立audit-index使用实际成功resume返回的saved_index，不以mtime选取；正式exit1，原因`current catalog differs from the requested input roster`。完整HMC11.1仍部分实施，源/调用位置展示缺口独立归原16.4/18.4，不给11.1增加代表选样/报告回写/零重算等新退出门。原始报告：本地证据根`verification-receipts/codrax-hmc247-resume-live-manual-audit-v1.md`。

## 任务与后续

当前仍79唯一任务、21实现交付、58开放。父能力本批不因机器或导航子路径通过而减少；已实现底座、自然采用、整答验收分别记录。

后继能力轨优先11.2代表样本/簇对照，在原范围内做完整交付；缺陷轨优先无附件目录查询的原生事实进入成文上下文，以及恢复计划与实际查询的精确状态交接。采样在事实已充分供给后仍误述的原件保留，不加原文关键词硬门或继续堆单句规则，不让它无限独占能力轨。09.4按原树合同与既有证据另行清算，不用本例代签展开/折叠；18.3的sample-only诚实成文仍未通过。

正式全仓、后置修复和归档收据见[中央账本§247](../docs/design/hmosperf_capability_gap_audit_20260917.md)。两份原始审计报告、模型调用参数/结果、所有成文sidecar、离线oracle及失败日志保留于本地忽略的`eval/results/hmc_resume_handoff_20261015/`。
