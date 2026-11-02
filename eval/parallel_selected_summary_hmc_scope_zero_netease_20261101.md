# HMC332：IO范围与查空登记、sample-only及网易云完整实采

批号20261101，终态核验2026-11-02 UTC。工作始终在main，基线`884f311e4f81734b260e7b1918fc29e0ba082d60`；实现`ddc7447768ce0309fd75282f580940c7856c84af`，父即基线。源码/测试/case先提交，本文及其它4文档为相邻后继提交；实际远端、干净状态与冻结一致性由[publication-v1.json](results/hmc_scope_zero_netease_20261101_v1/publication-v1.json)确认，不预填文档自身SHA或以本地提交冒已推送。

## 1. 分账结论

| 维度 | 本代终态 |
|---|---|
| 稳定任务 | 79唯一，27完整交付、52开放；18部分/29待实施/3待验收/1验收中/1持续，无勾选变更 |
| 完整能力新增 | **0**，root明确closure=none；原HMC-18.3有限合同FAIL |
| 已交付源码子能力 | **3组**：IO半开端点来源交接、同口径互斥端点小计教学、完整原生查空登记；不是3个父任务 |
| 定向 / race | 3包31顶层/55子例PASS；2包14顶层/40子例PASS |
| 唯一全仓 | OS0；98包PASS/12无测试，28699顶层PASS/44SKIP，34279子例PASS/14SKIP；零失败 |
| 恰好两例真实模型 | A/B各15steps、n=1，均OS0、原smoke PASS、源前后true；完整人工均FAIL |
| 本代剩余语义失败 | A未知采样升执行；B inclusive/self状态错绑；B预览/区间分布外推，共3组 |
| 未触发 / 旧债 | 新IO/catalog零分支未自然触发；只读/代表/图未跑；原采样、IO、权限等债不销 |
| 安全边界 | 冻结后停写源码/测试/教学/helper/case/oracle；WAL未零副作用前生产NO-GO |

总数不是此前53/54/55的沿用值：本代重新解析79条稳定ID，27打勾、52未打勾，重复0。源码交付、有限能力验收、自然触发、整答人工与推送五个结论彼此独立。完整账见[实施清单](../docs/design/hmosperf_implementation_tasks_20260917.md)、[统一gap §332](../docs/design/hmosperf_capability_gap_audit_20260917.md)。

## 2. 参考设计、已证缺口与实现边界

参考仓是本地只读归档，无Git元数据。本轮对照`core/preprocess/io_ops.py`：compute_io_concurrency（114行起）数每桶相交请求，compute_io_concurrency_agg（482行起）筛行后按发起桶聚合；两者都不是本项目瞬时峰值/时间加权在途深度。compute_read_write_ratio（250行起）体现兼容读写端点可相加，借其限定总体的意图，不照搬缺值补0。`marker_ops.py`的callid/itid、物理实例、每节点状态交集说明同一账户不可脱离主体与区间；保本项目子区间并集/self计算，不直接照搬直子耗时相减。静态参考不授实采因果资格。

7文件为4生产Go、2新公开测试、1自然case，528增/7删；没有旧测试/断言修改。

- IO activity receipt完成后，仅显式连续半开查询窗发布既有私有端点。正文/表/图的共用来源标题沿原凭证取窗，line优先与隐式capture末端包含范围不借权；io_activity只加入端点谓词白名单，不开放raw事实通道。可选Notes删除合法，JSON回灌、变代和mutation不能重签。量测值、闭合睡眠、首次wake和5µs完成侧属性证明不改。
- 同一条scope说明服务typed记录与3张原生IO表：同源/family/阶段/字节量尺/查询、互斥device或operation组可小计；不总计加成员、不重叠/跨总体，更不借小计推唯一请求、RQ/BIO物理对应、饱和或因果。去掉该处重复语句，不声称全prompt去重或全局预算重构已完成。
- catalog finish按实际注册view读取唯一完整typed零凭证，resource_stack available/MatchedEvents=0，event_search结构有效且scope/enumeration/rows完整、matched/emitted=0；外层不截断。空预览、未知view、重复/混合凭证或复合局部空不推整体empty。failure/cancel/stale优先，实际尝试/重试、source revision、payload、未执行名册与完成权限保持。

初始公开IO RED的端点夹具缺实际Mutable/replay上下文，line/implicit误要求通用固定事实权；**它不是纠正夹具后的旧码端点RED**。生产端点反例是§331 A实收标题/Notes冲突，本代最终source-context正负控验证修复。初始禁合法小计和catalog零→success是有效产品反例；“lost identity”报错过宽，实际身份未丢。后续HTML数字实体比较、binary SQL通过nil-material text binder是新夹具错误，第二finish只是边界重放。原失败和诊断保留，详见[封存证据更正](results/hmc_scope_zero_netease_20261101_v1/sealed-v1/0325-codrax-hmc332-public-evidence-review-v1.md)，其解释优先于未改动[冻结计划](results/hmc_scope_zero_netease_20261101_v1/sealed-v1/0317-codrax-hmc332-plan-v1.md)中初始过宽RED描述。

## 3. 两个真实输入与实际执行

| 项 | A 原sample-only | B 新完整网易云实采 |
|---|---|---|
| case | [trace_binary_sample_only.case](cases/trace_binary_sample_only.case)，原问题/断言未改 | [donghu_v105_netease_bind_application.case](cases/real_traces/donghu_v105_netease_bind_application.case)，自然问题无oracle值/防护答案提示 |
| 普通CLI | 15steps/n1，完整二进制附件 | 15steps/n1，完整ftrace附件 |
| run | A/trace_binary_sample_only-20261101-174501 | B/donghu_v105_netease_bind_application-20261101-174501 |
| 原生取数 | 真实准备12000条；7探索＋2确定性补查，9份唯一JSON | 5真实查询/5唯一JSON；补齐因families_present跳过，不造第6次 |
| 成文 | 1次，3条实际INIT消息 | 1次，3条实际INIT消息 |
| 进程/原smoke/人审 | 0/PASS/**FAIL**；18.3有限FAIL | 0/PASS/**FAIL**；新case不关闭父项 |

ROOT：`/Users/han/opt/codrax/eval/results/hmc_scope_zero_netease_20261101_v1`。

A完整原件为`/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/tests/fixtures/hiperf_brbe_001/test_brbe.data`，30062585B，SHA`da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`。official adapter不可用/未尝试，真实raw parser尝试成功，不冒BRBE/SPE；7份历史独立scalar oracle按HMC299封存复用，不重新计算、不作为模型输入。原case SHA`09bae0831fa02a8b90f4605a0b6231fca82d90add25ec37144ec515de0269780`。

B认真应用用户基线目录：完整原件`/Users/han/opt/donghu/基线数据/105版本/网易云音乐/应用后台销毁后冷启动/3、LOG_ftrace_20261019_17.55.02/ftrace-20261019_17.55.02.ftrace`，82838676B/559592行，SHA`12349db5bbdc5a4343360d9633c7d6c36f18c8be5f39fb64e78b53c9e345bfe4`。全源整数µs oracle在模型前独立生成、冻结且不进问题/上下文；header TID/TGID36434与marker payload4943分开，完整B/E物理行77708–152774。case SHA`9082919d60e3d72b5ba818df424182d00d54cadadb5ba8da47afcd4e097d7d6d`。目录普查75个ftrace/9995542762B/15场景是材料清单，不冒75个都完整解析/hash/评测。

两例同批各一次，无第三例或追绿重跑。严格按实际请求/响应配对A9/B20、歧义0，最大可证模型调用重叠10.84秒，见[实际并行凭证](results/hmc_scope_zero_netease_20261101_v1/sealed-v1/0268-codrax-hmc332-actual-request-overlap-v1.json)，不能仅凭双进程起跑称并行。

## 4. 完整人工结果与系统上下文审计

A原生目标164条/259权重，IP82/174、OnDataEvent_82/85，source-window1393与独立宽窗515分开；unknown sample_kind100%、CPU坐标、时间包络、完整一帧与权重非耗时均到INIT。正文仍断言采到即在CPU运行，与同答表“未确立（样本类型未知）”冲突，处于18.3原合同而非新增格式要求。坏“无样本即睡眠”和供给排除单元未实际转发，不虚构污染/OOM/原生丢失。正确空隙未知、无具体阻塞者和context-only因果投影保留，但不抵销执行状态误述。

B自然event_search→business_span_ref→span_window/window_stats贯通，线程与查询窗未丢。16完整实例/80数值、所有3735时序行与独立全源整数解析一致，root627.645/self194.581/child union433.064ms，Running412.174/Runnable11.069/S200.474/D-family3.928ms；2131是direct children，15329全查询拓扑和2709/200事件预览另列。粗D-family核对不背书IO-marker机制。正文将VerifyDex22.183写全Running，将inner attach inclusive Running62.683配self82.388，另误述D/IO区间分布，故FAIL。完整账户与self状态已在INIT，不改正确表迎合正文。

共享债另列：B当前row地址引用错主体，调查供给排除unit6确实转发；其它坏unit未投递不算污染，最终正文也没复述所有供给排除。重复16行表、7旁线程背景表、12次结果导航（2exec/4grep/6read_file）有噪声/成本，不误报为真实源查询失败或已证根因越权。native payload_ref已给，反复在描述TXT搜索JSON字段是可泛化导航欠项；本批没实现。解释全单元预算/去重与typed语义绑定亦未完成。

A5/B14张HTML表已做完整源级QA，B16个名称格backtick/code表示差异不算数值渲染错误；图/patch均NOT_TRIGGERED，不冒浏览器像素检查。详见[完整人工审计汇总](parallel_selected_summary_hmc_scope_zero_netease_20261101_manual_audit.md)、[A封存全审](results/hmc_scope_zero_netease_20261101_v1/sealed-v1/0223-codrax-hmc332-A-audit-v1.md)、[B封存全审](results/hmc_scope_zero_netease_20261101_v1/sealed-v1/0244-codrax-hmc332-B-audit-v1.md)及[后置全行核对](results/hmc_scope_zero_netease_20261101_v1/sealed-v1/0266-codrax-hmc332-B-post-audit-v2.json)。

## 5. 冻结、全仓、封存和发布

唯一CLI SHA`62acb5cd430faad3365c046feb736205af03f6a3ee6dd4bea64953a3cb1a887f`；[冻结清单](results/hmc_scope_zero_netease_20261101_v1/sealed-v1/0284-codrax-hmc332-frozen-v1.json) SHA`f719811c71f3af22c2c42d2da4bfcce0aa9515c9dd14ec0229d1b09760aeb771`，7576输入/162外部/13仓外快照。冻结后只做终态收集/只读手审/文档和提交，不改代码、测试、提示、helper、问题或oracle。

唯一`go test -json ./... -count=1 -timeout=30m`于01:45:02.181974Z–01:58:37.079051Z运行；OS0/源前后true，98包28699顶层34279子例PASS，12无测试、44顶层/14子例SKIP；266878 GoJSON，FAIL/非JSON/重复/未终态全0。[统计](results/hmc_scope_zero_netease_20261101_v1/sealed-v1/0285-codrax-hmc332-full-stats-v1.json) SHA`8fd5d4b27b0bec6992a24516865960651168be2505788a228749227929f3b7bb`，[OS收据](results/hmc_scope_zero_netease_20261101_v1/sealed-v1/0286-codrax-hmc332-full-v1-receipt.json)绑定log SHA`1043b5e6cd6d3de336b45f1cc49581b0ba0f98d70a4cbb2480228fa909421d7b`。最终[定向](results/hmc_scope_zero_netease_20261101_v1/sealed-v1/0351-codrax-hmc332-shared-related-v2.log)、[race](results/hmc_scope_zero_netease_20261101_v1/sealed-v1/0341-codrax-hmc332-shared-race-v1.log)独立留存，不代全仓或真例人审。

本代365件，[index](results/hmc_scope_zero_netease_20261101_v1/sealed-index-v1.json) SHA`13444f60917d02d51c5ec0978823fe5e72b986c8617a009f3044314646b41b5e`，原始失败/实际query/INIT/受理拒绝/MDHTML/无效UTF8 debug均保留。[root不销账决定](results/hmc_scope_zero_netease_20261101_v1/sealed-v1/0267-codrax-hmc332-acceptance-v1.json)与原件一并封存，不因完整full通过改签整答。外部大原件/同字节副本仅登记大小SHA，Go快照仓外并以.go.txt归档，无全仓包污染。

旧33代13516件各ROOT保持，逐代名单续接[§331旧ROOT清单](parallel_selected_summary_hmc_io_scheduler_boundary_20261031.md)。最新旧ROOT`hmc_io_scheduler_boundary_20261031_v1`的publication SHA`22ed70419e87a5e4bc547c633a5e3c966fe26e54e5424da7751f62efad97b079`、index SHA`2a64ba10f185b4652d4533bad95f4badfc74d8646be7e89668ca2e1babe2c8c9`保真；本代只物理重核其503件和更早链头，**不是重hash所有历史payload或重跑历史**。旧失败代不补publication。

## 6. 剩余52项：高ROI到低ROI，无重复无遗漏

以下均为HMC稳定ID，状态仍以原清单勾选为准；排序不是本批实施权限扩张。

1. 共享正确性7项：01.3、02.4、16.4、16.5、01.2、18.4、12.5。
2. 已有材料有限验收7项：18.3、18.5、11.2、03.2、14.1、08.3、18.2。
3. 原生扩展11项：04.3、04.5、08.1、08.5、12.2、12.3、17.6、17.7、05.2、10.2、14.2。
4. 来源/协议依赖21项：04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、09.3、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1。
5. 新域6项：07.1、07.2、07.3、07.4、15.2、15.3。

双轨后继：共享轨先公开复现原生账户与解释引用的主体/查询/窗口/inclusive-self/状态绑定，并改进原生payload导航与整组预算；完整能力轨选择已有真材料的原有限二进制验收或只读缺来源终态负控，代表规格也须真实执行再验。不从自由label/业务名推view，不从原文关键词硬拒绝或系统代写结论。IO端点/小计与catalog-zero本代仅公开覆盖，后继自然轮换才签该维度；无第三例。充分事实后的旧采样误述留账，不无限追同题阻塞其它ROI。旧IO/权限覆盖/图主体及局部修补、h2/h3、lower-cost-full、更多格式/平台及原重点验收债保持，WAL零副作用前不接生产。
