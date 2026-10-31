# HMC331：原生调度边界与IO有限合同复验

## 1. 交付、验收与剩余任务

本批在main上继续，基线`74334472616db2df4dd469c616b05cfc09b3a67d`。8个Go文件（5生产、3测试）已单独提交`fb5332c0fce27c65cc74e6304b4a947d5a2ecacb`，父提交即基线。稳定任务仍**79＝27已交付＋52开放**，18部分/3待验收/29待实施/1验收中/1持续，无勾选改变。

| 口径 | 本批结论 |
| --- | --- |
| 完整能力新增 | **0**；root明确`closure=none`，08.3原有限合同FAIL |
| 有界源码子能力 | **3组**：同刻/首次wake及代际状态分割；精确完成阶段原因属性共用；同源同实例属性到因果分支投影并重建量测域 |
| 原自然用例 | A/B各一次，均OS0、原smoke PASS；完整人工均FAIL |
| 剩余验收 | A原IO合同内解释错误；B五类量尺/成员/范围解释错误；新完成证明/链投影自然分支未触发 |
| 远端发布 | 代码与恰好5文档相邻提交、main远端一致及clean由[publication实际凭证](results/hmc_io_scheduler_boundary_20261031_v1/publication-v1.json)核验；本文不预填docs SHA或remote结果 |

旧IO九张原生表成功复验不增加第四组新子能力；原机器PASS、全仓PASS、封存完成也不代替人工验收。旧失败代不回签。三份共享文档为任务清单、统一gap、architecture，另加本汇总和[人工审计汇总](parallel_selected_summary_hmc_io_scheduler_boundary_20261031_manual_audit.md)。

## 2. 已证缺陷与泛化实现

1. **量测边界。** 同刻switch-out→wake在timeline遗漏等号，off-CPU统计/头部checkpoint却已转换；冗余wake还会覆盖首次转换。现保持首次已接纳wake，零长睡眠不铸闭合等待，下一周期/生命周期仍分割。不同范围消费者保持同一量尺，不以改旧断言求绿。
2. **完成原因属性。** 首次waking结束量测，与完成wake旁的blocked_reason是两件事。查询局部证明核相同物理源、目标及发起线程代次、首waking/唯一完成wake原行、原时间/行范围；仅在完成侧既有5µs包络且在下个调度/生命周期边界前补原因。坏字段、多完成/歧义、取消/跨源/换代不借权，中间未测间隙不计睡眠。timeline/off-CPU/state-churn及无窗入口共用，不扩旧direct-wake匹配。
3. **原实例属性投影。** 原查询完整等待与链上局部测量按源、代次、原状态、实际完整起止及物理端点绑定。正时长局部区间必须属于原实例，重复身份不投影，未知/歧义不被缩窗洗成确定IO；复制后重建局部量测域，不新增因果席位。原4候选、闭合睡眠和自动补齐合同保留。

两条chain RED V1是**有效投影元数据丢失回归**，前提曾被误判，最终恢复原目标的独立保护；V1/V2都保留。过程中tracequery guards V1另暴露3个旧回归（first-waking后合法S/IO原因失联），没有改原assert或冻后追写，修复后V2及最终全仓通过。

参考实现取舍见[实际静态审查](results/hmc_io_scheduler_boundary_20261031_v1/sealed-v1/0395-codrax-hmc331-reference-boundaries-v1.md)：marker_ops的callid/itid和正区间交集可复用，但不定义ftrace同刻/首次wake顺序；io_ops的发起桶计数不是在途深度。参考GOLDEN_RULES、architecture、window_utils/wakeup_chain_index、closure/select/package的身份/窗口/完整名册设计，不复制D→nonIO、未知补零、直接子时长求和、邻近因果或固定Top常数。冻结名册不是“所有参考行均已阅读”的声明。

## 3. 恰好两例、原问句与真实材料

| 侧 | 原入口与预算 | 实际过程和人工结果 |
| --- | --- | --- |
| A | [trace_io_inflight_overview.case](cases/trace_io_inflight_overview.case)，原共享case/断言不变，15steps/n1 | 3次真实trace_query、2份unique JSON；1次finalizer/3消息；9张原生量测表正确，08.3有限及整答FAIL |
| B | [donghu_v100_douyin_bind_application.case](cases/real_traces/donghu_v100_douyin_bind_application.case)，15steps/n1 | 12次真实trace_query、7份unique JSON；1次finalizer/3消息；2783主窗＋90窄窗时序正确，整答FAIL |

实际ROOT为`eval/results/hmc_io_scheduler_boundary_20261031_v1`。A运行子目录`A/trace_io_inflight_overview-20261031-035140`，B为`B/donghu_v100_douyin_bind_application-20261031-035140`。A只是1839B authored synthetic文本，SHA`7df22573b4ef6388cc19bcc0859a7152f17a0d3fc967cd65239d0f2938886e19`，不冒设备采集。

B原完整ftrace为`/Users/han/opt/donghu/基线数据/100版本/抖音极速版/应用后台销毁后冷启动/1、LOG_ftrace_20261019_9.55.40/ftrace-20261019_9.55.40.ftrace`，235422121B/1696481行，SHA`93b45bcca5c9e9b415478d81e176b633d70886328c49731b5258c806695de1ef`。复用同物理SHA的330整数微秒oracle，SHA`ad6df405a62f7762dc9041cdcb465cee7158ae98af3252d3db288013a0b5de68`，**不是331重新测量，不送模型、不作因果权威**。标尺包含root＋1429直接子共1430个带状态账户；本轮人工交叉root＋12，不冒全部逐项手审。

自然B验证两处同刻边界6＋10µs，Runnable8.714/S170.918/D-family2.279ms现与标尺一致。没有sched_waking完成凭证/因果链/图自然调用，公开测试PASS不能代该分支自然验收。1429确是direct children；root此前误读已撤回，不作为产品缺陷。主账户total669.351/self227.101/子并集442.250ms及合法D-family小计保留，解释错误不能反改正确原生数值。

A合同内失败涉及独立总体、完整驻留与裁窗贡献、设备/跨层身份越权及合法小计；B五类失败为self墙钟升执行、有限桶排除供给/存储、6行表与8项小计混用、预览/排名外推、全S计数套余集。受理advisory错误确有部分进入INIT，但A只unit0/5、B只前三个/7，其余省略不能冒已送达；正确原生事实充分，不能杜撰整体预算丢失或source权限失效。详见完整手审。

只读登记、sample-only二进制、代表选择均本批NOT_RUN，不追加第三例；旧有限PASS只保原代。真实Donghu目录75份/9995542762B的只读清点和网易云/蜂鸟未来候选记在[冻结规划](results/hmc_io_scheduler_boundary_20261031_v1/sealed-v1/0393-codrax-hmc331-plan-v1.md)，没有为这些候选新建oracle/模型用例。本批是修后新代，不是沿旧失败无变化追绿。

## 4. 唯一构建、冻结、全仓与真实并行

唯一普通CLI SHA`3b0792f025b2c20cb1e6f01d3887f767d0f35e1baae12e966e72bdc9de8bc478`；[构建收据](results/hmc_io_scheduler_boundary_20261031_v1/sealed-v1/0342-codrax-hmc331-build-cli-v1-receipt.json)为OS0/source_unchanged=true。无operator/testbin构建或额外查询准备。[预检](results/hmc_io_scheduler_boundary_20261031_v1/sealed-v1/0394-codrax-hmc331-preflight-v1-receipt.json)通过后冻结，7573 inputs/218 external/20仓外快照；[冻结清单](results/hmc_io_scheduler_boundary_20261031_v1/sealed-v1/0373-codrax-hmc331-frozen-v1.json) SHA`cd6d4264aa7165932cf2a20405496250eeaf271d1ca429e0f661ebf57c14f50a`。Go1.26.3 darwin/arm64、CGO1、SDK MacOSX26.5在清单中，不能冒其它平台已验。

唯一全仓`go test -json ./... -count=1 -timeout=30m`，清除LIVE/CODRAX评测覆盖，OS0/源前后true。结果：98包PASS/12无测试；28693顶层PASS/44SKIP；34255子例PASS/14SKIP；266760 GoJSON；FAIL/nonJSON/重复终态/未终态均0。[原统计](results/hmc_io_scheduler_boundary_20261031_v1/sealed-v1/0374-codrax-hmc331-full-stats-v1.json)、[原OS收据](results/hmc_io_scheduler_boundary_20261031_v1/sealed-v1/0375-codrax-hmc331-full-v1-receipt.json)绑定log SHA`06573c850353df073bb3f65be33c2d25fe7ecfd77ed742a5e26ba649468245a5`。定向/race先通过不替代全仓；原失败日志不删除。

两例均原进程OS0/源前后true：[A收据](results/hmc_io_scheduler_boundary_20261031_v1/sealed-v1/0385-codrax-hmc331-live-A-v1-receipt.json)、[B收据](results/hmc_io_scheduler_boundary_20261031_v1/sealed-v1/0389-codrax-hmc331-live-B-v1-receipt.json)。严格模型请求配对A8/B9，11歧义组排除，最大可证重叠18.914秒，见[真实请求区间](results/hmc_io_scheduler_boundary_20261031_v1/sealed-v1/0340-codrax-hmc331-actual-request-overlap-v1.json)。这不是“只有17次总请求”，不是启动屏障或进程时间重叠代替请求重叠。

## 5. 封存与旧ROOT完整名册

本代[sealed-index](results/hmc_io_scheduler_boundary_20261031_v1/sealed-index-v1.json)503件，SHA`2a64ba10f185b4652d4533bad95f4badfc74d8646be7e89668ca2e1babe2c8c9`。全部真实query/参数/INIT/受理与拒绝/MDHTML及invalidUTF8原日志保留，不按文件大小删掉答案；原大ftrace与同字节全文副本只登记、不复制入Git。Go原快照位于仓外，封存副本位于仓内忽略的结果目录并保留`.go.txt`后缀，不进入Go编译树或递归复制旧归档。冻结后没有源/测试/helper/计划/输入修改，文档是封存后独立收口。

旧32代计13013件；此计数由各原sealed-index独立复核。331实际只重核最新330的504物理原件并串接更早index/receipt/publication头，**不是重hash全部13013件，也不是重跑历史评测**。下表ROOT均相对`/Users/han/opt/codrax/eval/results/`，各自保留`sealed-index-v1.json`与`sealed-receipt-v1.json`；旧FAIL/未发布代不因本次全仓通过转绿。

| 旧ROOT | 原件数 |
| --- | ---: |
| hmc_business_identity_registration_20261027_v1 | 225 |
| hmc_business_io_identity_20261029_v1 | 278 |
| hmc_business_pcap_20261028_v1 | 268 |
| hmc_business_pcap_admission_20261028_v1 | 266 |
| hmc_business_pcap_flow_20261029_v1 | 328 |
| hmc_business_pcap_script_20261029_v1 | 386 |
| hmc_business_registration_binding_20261029_v1 | 443 |
| hmc_business_registration_pcap_budget_20261029_v1 | 351 |
| hmc_business_representative_20261030_v1 | 447 |
| hmc_business_sample_only_20261029_v1 | 296 |
| hmc_business_wait_registration_20261027_v1 | 232 |
| hmc_display_representative_20261029_v1 | 422 |
| hmc_display_spec_representative_20261029_v1 | 434 |
| hmc_donghu_representative_native_handoff_20261030_v1 | 821 |
| hmc_douyin_representative_scoped_delivery_20261031_v1 | 504 |
| hmc_endpoint_sample_registration_20261030_v1 | 426 |
| hmc_io_donghu_handoff_20261030_v1 | 790 |
| hmc_io_donghu_measurement_20261030_v1 | 528 |
| hmc_native_lifecycle_prompt_budget_20261030_v1 | 889 |
| hmc_native_publication_compat_20261030_v1 | 552 |
| hmc_reference_scope_resource_20261029_v1 | 331 |
| hmc_registration_pcap_metadata_20261028_v1 | 257 |
| hmc_registration_sample_compat_20261030_v1 | 397 |
| hmc_repair_pairing_representative_20261029_v1 | 474 |
| hmc_sampling_gzip_entry_20261026_v1 | 209 |
| hmc_scheduler_registration_20261029_v1 | 288 |
| hmc_scoped_rank_sampling_20261027_v1 | 235 |
| hmc_tail_io_inflight_20261029_v1 | 290 |
| hmc_teaching_compat_binary_registration_20261031_v1 | 406 |
| hmc_temporal_registration_20261029_v1 | 327 |
| hmc_terminal_business_publication_20261030_v1 | 525 |
| hmc_windowed_business_tree_20261029_v1 | 388 |

最新旧ROOT的publication SHA`023bb03602f956e2a530afc6bc33360b102ae1af7c100dfc17f4ed768e68e0a7`、index SHA`111ec43148554d35614bab3c960f0297581a68c6fa4904abef65a312ca7c1523`保持。321/323/325/326/328失败封存不补publication；330两例人工及原operator FAIL不改签。

## 6. 全52项ROI名册与下一双轨

下列稳定ID均带`HMC-`前缀，分组互斥、7＋7＋11＋21＋6＝52；不是新任务或新关闭决定。原退出合同仍以[任务清单](../docs/design/hmosperf_implementation_tasks_20260917.md)为准。

| 优先层/数量 | 全部开放ID | 近期顺序和边界 |
| --- | --- | --- |
| 共享合同7 | 01.3、02.4、16.4、16.5、01.2、18.4、12.5 | 解释单元原生量尺/成员/权限与整组预算优先；再消除合法算术、端点、来源教学冲突，覆盖正文/表/图，不扫问答关键词 |
| 有材料有限验收7 | 18.5、18.3、11.2、03.2、14.1、08.3、18.2 | 轮换既有只读负控/二进制；11.2先任务语义到真实query规格；08.3待系统改动后再验，不同题追绿 |
| 共享原生扩展11 | 04.3、04.5、08.1、08.5、12.2、12.3、17.6、17.7、05.2、10.2、14.2 | 精确身份、来源/窗口及原生统计优先；具备真实材料再扩合同，WAL保持NO-GO |
| 来源/协议依赖21 | 04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、09.3、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1 | 先补协议/材料与已证关系，不凭名称/时间邻近授机制或因果 |
| 新域6 | 07.1、07.2、07.3、07.4、15.2、15.3 | 能力设计及依赖明确后推进，写入仍走原审批/隔离合同 |

[任务到查询接缝诊断](results/hmc_io_scheduler_boundary_20261031_v1/sealed-v1/0468-codrax-hmc331-task-query-gap-v1.md)是operator-only后继设计，不是已实施修复：330的event_search是显式模型选择；建议复用typed语义事实族/能力registry软导航，不能硬改合法view或从自由label猜路由。event_search已知零结果仍记success的分类欠项也不倒改旧7success＋1empty，不假称造成原operator规格FAIL。

充分原生事实下的self/供给/采样误述继续留账，不无限阻塞其它高ROI项。旧IO、采样、权限、图/patch、h2/h3、lower-cost-full及其它格式平台未触发/FAIL保持；无WAL零副作用证明前不接生产。不删链上业务线索，不用背景冒主因，不削弱Trace因果投影或自动补齐。
