# HMC331：两例完整人工审计汇总

本文件汇总已封存的两份独立人工审计，不是新增评测、模型提示或替代原始报告。两个原题各15steps/n1，固定输入、问句、断言不变；所有源码/测试/helper/计划/模型输入冻结后停写。无第三例、无事后status/query补签、无模型追跑。本文只更新批准的5份文档，不修改封存原件。

## 1. 分层裁决与原报告

| 项目 | A：原IO在途自然题 | B：100版抖音bindApplication |
| --- | --- | --- |
| 真实CLI / 原smoke | OS0 / PASS，源前后true | OS0 / PASS，源前后true |
| 实际trace_query / 唯一JSON | 3 / 2；第三次真实执行但复用内容寻址结果 | 12 / 7；business_span_ref复用主窗结果不算失败 |
| Finalizer | 一次初始化、三条system/user/user消息 | 一次初始化、三条system/user/user消息 |
| 原生事实/表面 | 4组完整统计/成员及9张原生量测表有限PASS | 2783主窗＋90窄窗行及root＋12账户交叉有限PASS |
| 完整人工 | FAIL | FAIL |
| 原完整能力合同 | **08.3有限FAIL，不销账** | 不由此关闭其它父任务 |
| 图/新自然链分支 | 未触发，不增图/源码门 | 未触发，不冒完成新证明/chain投影自然PASS |

Root正式`closure=none`；79唯一＝27已交付＋52开放，完整能力新增0。三组本批源码子能力另见[批汇总](parallel_selected_summary_hmc_io_scheduler_boundary_20261031.md)，旧IO九表复验不增加新子能力。唯一全仓通过不抵销完整答案FAIL。

原审计已完整复读，保留其错误归因、正向边界及全部引用：

- [A原独立审计](results/hmc_io_scheduler_boundary_20261031_v1/sealed-v1/0298-codrax-hmc331-A-audit-v1.md)，SHA`046a992900bf0a52abe880991b3e1d8af2583e4b3d51b1e6ae7812e2e19033c0`。
- [B原独立审计](results/hmc_io_scheduler_boundary_20261031_v1/sealed-v1/0319-codrax-hmc331-B-audit-v1.md)，SHA`9495118972dcc84329ade59698b090de3ef0c7c73ec1277a32bd2fdc832399b6`。

## 2. A：有限合同内失败，不加新门槛

原运行`results/hmc_io_scheduler_boundary_20261031_v1/A/trace_io_inflight_overview-20261031-035140`；原fixture是1839B、23行/21事件的构造文本，不是硬件录制。原生窗口[1,1.01)，均值分母完整10ms，来源/层/设备/方向分组、all_issuers与partial capture边界保留。

| 组 | 峰值request | 全窗均值request | busy ms | request·ms | 相交完整配对 | 窗内发起 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| RQ / 8,0 / R | 2 | 1.4 | 10 | 14 | 4 | 6 |
| BIO / 8,0 / R | 1 | 1 | 10 | 10 | 1 | 1 |
| RQ / 8,0 / W | 1 | 0.6 | 6 | 6 | 2 | 2 |
| RQ / 8,1 / R | 1 | 0.4 | 4 | 4 | 1 | 1 |

四RQ读的完整起止0.998→1.004、1.002→1.006、1.006→1.010、1.008→1.012；完整驻留6/4/4/4ms，裁窗贡献4/4/4/2ms。四相交配对含窗外起点，不是六窗内发起的子集。一个unpaired start、一个ambiguous cohort及两个suppressed配对保持，不把歧义FIFO猜配或缺尾补至窗口末端。独立端点总体和配对总体分开。

最终9张原生量测表的数值、身份、成员正确；HTML共11表＝附件1＋手写总结1＋原生9，逐单元格与MD一致。没有mermaid/SVG/canvas/iframe/img，不要求原题补图或根因。root-causes旁路unavailable/contract_not_active不计因果能力PASS。表面审计为文本/结构/数据，不称浏览器像素QA。

| 原MD位置 | 本次失败及边界 |
| --- | --- |
| L15 | RQ的6+2+1本为9，却写10再加BIO1；跨层发起端点和独立物理请求不得混计 |
| L31 | 四请求“每次各占4ms”同时错完整驻留与裁窗贡献，后面正确原生表不能抵销 |
| L33/L289 | 把4配对与6发起相减解释为缺失差额，又把数量同为4的完成端点与4配对一一认同；实际集合不同 |
| L30–31 | 由接受配对的深度≤2推“没有排队堆积/不是带宽或队列饱和”，越过数据完整度和设备权限 |
| L38/L295/L307 | 把RQ/BIO说成同一IO/同一批，原件没有跨层物理映射；只能保may describe，同层合法小计仍允许 |
| L38/L307 | 一律不可跨设备/阶段相加过宽；应核兼容量尺与已证明不重叠成员，不禁止合法RQ发起6+2+1=9 |
| L215/L248 | 来源标题“端点包含性未知”与同表Notes[1,1.01)冲突；真实INIT已有，属于共享展示接缝 |
| L309 | 无实现源码锚点被写成仓库没有材料文件，实际fixture可读；不能因此新增源码义务 |

以上前五项直接属于原题“这些结果能说明什么”，故08.3原有限合同FAIL，不是以未来平台/二进制/图/源码等附加门槛拒绝关闭。

### A上下文与调查交接

原生四组、完整成员、时间线、配对/独立端点边界均到达实际INIT，最终全部selector使用Q1，不重复计算Q2。系统INIT01明确允许精确操作数的有界计算、同量尺小计，并禁止量尺互换；因此不是系统整体禁止计算。动态IO activity note的“Do not add sources, layers, phases or byte calibers”过宽，与通用教学有具体冲突；两来源标题遗漏已知端点也已在INIT存在。

受理调查实际只转发unit0/5，省略4单元/3428B。唯一展示单元含“无排队堆积/队列饱和”越权结论；其它未展示错误句不能称已污染成文。`current_exact_rows`只核地址，不核解释文字、算术或因果。advisory没有被签成native事实，但它与充分正确事实同包，值得优先从结构化成员/量尺/边界和整组预算修复，不扫用户/答案关键词做硬门。原invalid JSON、unknown field、unknown_id、被拒提交和实际受理提交全部保留。

## 3. B：量测修正自然命中，五类解释继续留账

原运行`results/hmc_io_scheduler_boundary_20261031_v1/B/donghu_v100_douyin_bind_application-20261031-035140`。完整235422121B/1696481行真实ftrace，SHA`93b45bcca5c9e9b415478d81e176b633d70886328c49731b5258c806695de1ef`；header TID/TGID57874与marker payload PID10266独立，不猜namespace等价。主实例61440–167407，236627.820748–236628.490099秒；parent_status=unknown_prefix不证明全程序树根。

| 主实例量尺 | ms |
| --- | ---: |
| 完整inclusive / self墙钟 / 直接子区间并集 | 669.351 / 227.101 / 442.250 |
| Running / Runnable / S | 487.440 / 8.714 / 170.918 |
| 未带IO标记D桶 / 带IO标记D桶 / 合法D-family合计 | 2.245 / 0.034 / 2.279 |
| self内Running / Runnable / S / D-family | 186.880 / 1.618 / 36.431 / 2.172 |

同刻S out/wake到switch-in的6µs（118836→118840）及D out/wake的10µs（161366→161370）现均是Runnable；timeline、target_window_states、thread_cpu_load和cpu_constraints量尺一致。2783段state/实际端点/物理行与旧整数微秒oracle逐段相符，不继续挂旧16µs分歧。但本例无sched_waking完成事件、无wakeup_chain/root_cause_rank，不拿它验收完成新证明、坏候选、原4根因或闭合睡眠链分支。

旧oracle含1430个带状态账户，本轮手审交叉root＋12个已选直接子，inclusive/self与状态均对；不声称1430账户全量手审。**1429明确为direct children，native字段和独立direct_children数组相互支持。Root初读误当后代计数已撤回，不能列为产品失败。**

最终HTML12表：附件、主要业务阶段12行（root1/直子6/更深5）、原生摘要、主窗状态、独立窄窗90行、窄窗8CPU频率、5张旁线程独立业务摘要、主窗2783行。表格cells与MD一致；旁线程保各自身份/窄窗，属于有标签背景而非已证主因。主窗业务树25139/32/25107、窄窗6000/32/5968，独立计数不当目标子节点数；event_search2611/300预览边界保留。

| 失败类 | 实际错误与正确范围 |
| --- | --- |
| F1：self≠执行 | L19/L33把root self227.101、attachBaseContext self107.397说成自身CPU执行；精确self中含S/Runnable/D，事实已到INIT |
| F2：局部桶≠机制缺席 | L15/L33用低Runnable和0.034带标IO排除CPU供给/磁盘文件系统；不排除Running中的算力供给或未标记/未闭合的存储等待 |
| F3：小计成员错配 | L23/L129“表内8直子319.976”实际表内6直子317.900；另两具名VerifyClass1.033+1.043在INIT却未入表。8项319.976和余量122.274本身可算，但不属于当前6行表；6行余量124.350 |
| F4：预览/排序越界 | L19/L129把未列出说成不可命名，已有具名未展示直子；L28把attach self称全窗最大而root更大；base.apk inclusive87.867/self0.634混入self大头 |
| F5：余集套总体 | L29从总S170.918扣两段69.923得100.995后仍贴837段/均值0.204；扣除两个完整S实例后的正确余集为835段、约0.121均值 |

两锁业务原marker明确是ClassLinker classes lock和Runtime.loadLibrary0 contention，不把已有业务线索抹为纯猜测；但名字/owner payload不证明跨namespace物理owner或唤醒链。D-family两个互斥桶2.245+0.034合法相加；S内细分sleep_io_wait不能再叠加。

### B实际交接

1次finalizer的3消息大小100500/44963/336588B。INIT03发布16个精确query/实例业务账户，root、自身状态、主要子段及两个VerifyClass均在场；16是成文预算发布集，不是原树全集。没有证据将F1–F5归为关键typed事实丢失、OOM或source权限放宽。

三次调查提交是拒绝empty_selection→受理→拒绝not_current_selected_native_row_address，最后拒绝修补不覆盖原受理结果。受理解释实际展示前3/7单元，另4单元/2345B省略。前三者分别把self说成未命名子段、用状态比例排除供给并禁合法D-family合计、用标记IO排除磁盘；均advisory/no_native_row_address，不是原生事实授权。正确教学、完整关键事实与错误调查解释同时在场，最终沿用部分错误；优先结构化成员/作用域与解释来源交接，不把未投递单元说成污染。

审计脚本两次前置错误（Ruby无filter_map、字段start_ts误用）仅发生在只读审计并已纠正，不是产品RED、未改原数据。表面核验不含像素QA；本代无图，不追加图门。

## 4. 过程、原件、冻结与发布分开

唯一full实际OS0，98包PASS/12无测试，28693顶层PASS/44SKIP、34255子例PASS/14SKIP，266760 GoJSON，FAIL/nonJSON/重复/未终态0。冻结SHA`cd6d4264aa7165932cf2a20405496250eeaf271d1ca429e0f661ebf57c14f50a`，7573输入/218外部/20仓外快照，源前后true。full log SHA`06573c850353df073bb3f65be33c2d25fe7ecfd77ed742a5e26ba649468245a5`；[统计原件](results/hmc_io_scheduler_boundary_20261031_v1/sealed-v1/0374-codrax-hmc331-full-stats-v1.json)。两例严格模型请求配对A8/B9，11歧义组排除，最大可证重叠18.914秒；不能写成17个全部请求。

本代503件封存，index SHA`2a64ba10f185b4652d4533bad95f4badfc74d8646be7e89668ca2e1babe2c8c9`。以下是实际索引映射，不从临时文件名字猜归档编号：

| 原件 | 封存路径 / SHA |
| --- | --- |
| A最终MD | [0150](results/hmc_io_scheduler_boundary_20261031_v1/sealed-v1/0150-20261031-035406.624-3609.md) / `2ed5b65528bf39381bc5e0b64c4f6d90b5c4ee89044fb859bb3fa973c7f6de5b` |
| A最终HTML | [0149](results/hmc_io_scheduler_boundary_20261031_v1/sealed-v1/0149-20261031-035406.624-3609.html) / `a08482291f5a2438110a070cec80eeea0bf87a80c5d8299af1d27d13d8da459b` |
| B最终MD | [0154](results/hmc_io_scheduler_boundary_20261031_v1/sealed-v1/0154-20261031-040207.121-3610.md) / `de13add8e7516c83d1ec6388f43995c8d53d2871cf6072fb871b727c46f462f4` |
| B最终HTML | [0153](results/hmc_io_scheduler_boundary_20261031_v1/sealed-v1/0153-20261031-040207.121-3610.html) / `8c19b1b7752f5857fc8f6a8d7988849355b90949d5e424b335c6bae2682a759b` |
| 实际请求配对 | [0340](results/hmc_io_scheduler_boundary_20261031_v1/sealed-v1/0340-codrax-hmc331-actual-request-overlap-v1.json) / `bb2deefd7d15930ef108898a4ae49c929a9b8fa93d0956925dd81e78dd198612` |

A/B logger原字节含invalid UTF8，SHA分别`61f89379ddcefaeb70337fe99b80b3a8f2df74697bac3c27ad4507dd40ce0d59`、`046b6fe62067da03495b3722f5b1599d6df056f8fb300a1f2a2977e063951a6f`；显示层解码不替换原件。完整query/INIT/受理与拒绝/附件MDHTML均保留，ftrace只登记完整身份不复制进Git。旧32ROOT/13013件清单见批汇总，本批仅最新504件物理核验与更早链头核验，不冒全量历史重验。

本地代码提交`fb5332c0fce27c65cc74e6304b4a947d5a2ecacb`交付3组有界源码能力；相邻5文档、推送与远端同代以[publication实际凭证](results/hmc_io_scheduler_boundary_20261031_v1/publication-v1.json)为准，不预填docs SHA或已push。旧FAIL、未触发及WAL NO-GO继续保留。

后继先修共享解释单元的成员/量尺/权限边界与整组预算，再统一合法小计/端点/源码与材料教学；完整能力轨推进任务语义→真实查询规格，并轮换既有只读负控、具备材料二进制。充分事实后的误述不无限阻塞其它ROI项，不以关键字硬门、删除业务线索或限制合法补查换PASS。
