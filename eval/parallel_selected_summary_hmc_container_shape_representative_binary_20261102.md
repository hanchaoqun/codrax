# HMC334：共享容器形状诊断、代表原合同与sample-only验收

日期：2026-11-02（America/Los_Angeles；终态为11-03 UTC）。在main开发；代码`4a402833c77350f8605c2377725d286d928ff9af`，父`724ec7c7150c3505cc2e064144b708d346f3bcd1`。源码/相邻五文档/远端main及完整冻结输入的一致性由[publication](results/hmc_container_shape_representative_binary_20261102_v1/publication-v1.json)单独核验，不能以此签模型答案通过。

## 1. 核账与交付

稳定清单79条/79唯一：起点27完整/52开放；本批只关闭HMC-18.3，终点28完整/51开放（18部分、29待实施、2待验收、1验收中、1持续）。[root原有限决定](results/hmc_container_shape_representative_binary_20261102_v1/sealed-v1/0204-codrax-hmc334-acceptance-v1.json)与[任务清单](../docs/design/hmosperf_implementation_tasks_20260917.md)同口径。

| 分账 | 本批结果 | 边界 |
| --- | --- | --- |
| 完整能力交付 | **1：HMC-18.3** | 累计原CLI/REPL完整尾部/显式窗正例＋本轮原sample-only诚实边界；不是全部格式/平台/BRBE通过 |
| 已交付子能力 | **1组：共享容器形状拒绝诊断** | 共用解码、完整发射与局部修补复用，不拆成3个父能力 |
| 原代表验收 | 结构operator PASS；原11.2有限及整答FAIL | 分组/选中身份误述仍在原退出条件内，维持部分实施 |
| 剩余验收缺陷 | 3组已证问题，另2组未触发覆盖 | 详见§6；不因原生数表或全仓PASS销账 |

## 2. 已证共享缺陷与泛化方案

§333实际修补因数组被包成`{"item":...}`多次失败，旧错误只说第一个字段、没有索引。新公开full/patch及两种共享decoder反例在实现前得到3顶层/2子FAIL；[RED原件](results/hmc_container_shape_representative_binary_20261102_v1/sealed-v1/0259-codrax-hmc334-container-shape-red-v1.log)保留，不是坏夹具改绿。

新诊断仅在原owner已经以`json.UnmarshalTypeError`拒绝native object/array后生效：按同一目标类型及现有JSON字段解析器列原始索引路径、应有形状及实收形状。稳定排序、最多12条，额外数量显式；最多16384访问/64深度，触限说明名册不全。数组中的整数/字符串/对象等分型，特殊map键用JSON括号转义。RawMessage/interface/自定义JSON或Text解码器均保持opaque；null、json.Number、FlexInt与旧unknown-field/string-carrier路径保原owner。

共享、完整发射、patch使用同一反馈；不unwrap item、不猜值、不删业务事实、不自动修补/受理参数，不改变拒绝结论或现有根因selector提交顺序。结构化修复仅有字段路径与教学，不认证语义。7个Go文件（6生产/1新测试），381增5删，旧断言/问题/oracle不改；Trace投影、原4候选、闭合睡眠、自动补齐未改。

对照参考`core/batch/closure/select.py:14`的manifest驱动成员/软排序与`package.py:40`选择后独立打包：选择、材料、分析与完成分开；本项目保当前来源/线程/窗口/每组预算和同代执行凭据。没有照搬固定5/50为用户任务语义、缺字段补0或宽松substring证据授权，也不拿参考仓流程规则替本工作区明确main指令。实际`trace_catalog`查询schema本来已共享，不为一个错view再加硬编码路由。

## 3. 相关测试、真实材料与冻结

| 验证 | 实际结果 | 不能冒充 |
| --- | --- | --- |
| 最终相关测试 | 3实际测试包209顶层/1881子PASS、1可选SKIP；另1包无匹配测试 | 不把无测试包算代表套件通过 |
| race | 2包37顶层/1675子PASS | 不代全仓race |
| 原代表operator全套 | 7顶层/40子PASS | 不代自然采用和终稿 |
| 原sample-only模型外oracle＋普通CLI预检 | 1测试PASS、0模型；完整12000、窗1393、目标164/259 | 不是第二次CLI build或额外真实eval |
| 完整实采实际上下文 | 原HMC333测试1次原生query/0模型PASS | 不冒自然描述覆盖或新独立oracle |

认真使用用户提供基线：105版网易云完整82838676B/559592行，SHA12349db5bbdc5a4343360d9633c7d6c36f18c8be5f39fb64e78b53c9e345bfe4；实际查询[5969.994941,5970.622586)、TID36434、原77708–152774行完整bindApplication账户627.645ms inclusive/194.581ms self不变，Extract/Finalize实际消息回归通过。原§332独立µs oracle保血缘，没有重签或声称重新全审旧3735行。本轮该模型外测试使用完整原件，非缩短夹具；也不是第三例模型评测。

[冻结](results/hmc_container_shape_representative_binary_20261102_v1/sealed-v1/0274-codrax-hmc334-frozen-v1.json)7582输入/265外部/15仓外快照，SHA318526109a5aeb6b615eaf2aafca891702d73438128342dec45b43eb0d02e604；之后代码/测试/prompt/helper/case/oracle停写。原sample-only模型外测试唯一一次标准CLI build供两例共用，SHA4528561ca77312b4b8d19b9b9a0e6999567addf993aa4e547de251e9b099fb7b；代表operator另有独立构建。预检不调模型；未泄露/读取/哈希/复制providers，连本地拒绝模型的配置也不封存。

唯一全仓`go test -json ./... -count=1 -timeout=30m`实际OS0/前后true：98包PASS/12无测试，28709顶层PASS/45SKIP、34289子PASS/14SKIP；266960 GoJSON，FAIL/非JSON/重复/未终态全部0。[统计](results/hmc_container_shape_representative_binary_20261102_v1/sealed-v1/0289-codrax-hmc334-full-stats-v1.json)，原log SHA2c638dc3470413e83ab44202da5cbed344ad2f5b07c473e2260e072d2ee262cb。默认全仓实采opt-in为SKIP，不冒又跑模型/实采。

## 4. 恰好两例并行、各一次

| 项 | A 原代表自然题 | B 原sample-only自然题 |
| --- | --- | --- |
| 预算/输入 | 原50steps；4完整构造SQLite，非4设备录制 | 原15steps；完整30062585B真实参考二进制 |
| 进程/冻结 | OS0/前后true | OS0/前后true |
| 结构smoke | 原operator OS0/PASS | 原case PASS |
| 原有限合同/整答 | **FAIL/FAIL** | **PASS/PASS** |
| 原生/交付 | 15真实查询，11唯一JSON；8计划/2组/4选/4未选，7额外查询，同代tuple | 8真实查询，7唯一JSON；完整12000到目标164/259、unknown/cycles/CPU记录口径 |

实际请求配对A36/B13，歧义0，最大可证重叠53.203秒，见[精确配对](results/hmc_container_shape_representative_binary_20261102_v1/sealed-v1/0205-codrax-hmc334-actual-request-overlap-v1.json)；不是进程/barrier推断。B日志有截断导致的无效UTF8，原字节保留，配对只取精确唯一时间记录。A/B各1次成文、各3条INIT消息，不计三次成文；两个正常explore dispatch也不是第三例。

A自然选择resource_stack，线程101选b/d、202选c/a，4selected真实执行、4unselected保状态，结构和同代index/selection通过。8资源查询的14事件/40帧原字段逐源核对独立SQLite oracle一致，最终19张表中的原生资源/元数据正确；但正文把选择组换成a/c与b/d内容组、把额外查空说选中，并将深度数据写成方向调用链。实际INIT已有正确两组、4选中与状态总体，未选个体只展示3/4、另1显式省略；不冒全名册投递。错误unit0也被转发并复现到正文，解释冲突已证；一个未选导航省略不能解释已完整选中分组被改写。HTML与MD首表差异仅inline-code标记，非丢格；语义错误两面均在。封存后这一投递数量更正见手审§3，原封存不回改。

B原样本类型未知、cycles非时间、CPU0/1仅记录坐标、IP-only与无调度/因果证据均到终稿；两热点分别82/174与82/85，分母164/259，精确包络正确，最终明确不能解释停顿。两原生热点表同源，未用宽窗函数替主因。受理reason和省略unit3/4仍有CPU/时长误述但本次未传，不能冒成污染；已传unit1的负断言适用域错绑继续入共享账，终稿未重复。旧§238 A2 REPL尾部及§244–247完整投影正例累积已在案，本轮补齐原剩余sample-only反例边界，故只关闭18.3，不要求未来所有格式或把旧FAIL改绿。

## 5. 原件与发布

553件新封存，index SHA2b04c688bd75afcd16ac8efc144bdc235c5aaf5170f5ff4bb0bd398b4e107ae4；[index](results/hmc_container_shape_representative_binary_20261102_v1/sealed-index-v1.json)、[原A完整手审](results/hmc_container_shape_representative_binary_20261102_v1/sealed-v1/0160-codrax-hmc334-A-audit-v1.md)、[原B完整手审](results/hmc_container_shape_representative_binary_20261102_v1/sealed-v1/0182-codrax-hmc334-B-audit-v1.md)及[人工汇总](parallel_selected_summary_hmc_container_shape_representative_binary_20261102_manual_audit.md)。旧35代14228件保各原ROOT，只物理复核最新333的347件和更早链头，不冒全历史payload重hash；按代累计为36代14781件，不是全局内容去重计数。大原件同字节内容只引用，原件封存本地不冒上传Git。

## 6. 剩余验收与51项ROI

本代3组已证语义欠项：①A选择分组/执行身份与内容分组混用；②A源深度升调用关系；③B调查单元负断言适用域错绑，及未传CPU/时长误述。前两导致A有限/整答FAIL，第三保上游债、不谎称最终答案失败；A未选名册3/4投递另归共享预算覆盖债，不漏记或冒全到。2组自然覆盖仍未触发：本批container-shape拒绝反馈、HMC333引用描述的自然正向。结果导航仅有限读到JSON，不签全格式/成本效果。旧IO、权限覆盖、图/patch、重复展示/全局预算、h2/h3、lower-cost-full及更多平台材料仍在原账，不能由18.3关闭一并销掉。

以下每个开放ID恰好一次，51条无重漏；优先级基于影响面、材料和验收成本，不授任何事实/因果权限。

| ROI层 | 数量 | 稳定ID（层内顺序） |
| --- | ---: | --- |
| P0 共享事实、schema、成本与补齐 | 7 | HMC-16.4、HMC-01.3、HMC-02.4、HMC-16.5、HMC-01.2、HMC-18.4、HMC-12.5 |
| P1 有材料/原合同与持续轮换 | 6 | HMC-11.2、HMC-18.5、HMC-03.2、HMC-14.1、HMC-08.3、HMC-18.2 |
| P2 原生复用与较成熟能力 | 11 | HMC-04.3、HMC-04.5、HMC-08.1、HMC-08.5、HMC-12.2、HMC-12.3、HMC-17.6、HMC-17.7、HMC-05.2、HMC-10.2、HMC-14.2 |
| P3 来源/协议与安全前置 | 21 | HMC-04.4、HMC-04.6、HMC-05.3、HMC-06.2、HMC-06.3、HMC-08.6、HMC-08.7、HMC-08.8、HMC-09.1、HMC-09.2、HMC-09.3、HMC-10.3、HMC-12.4、HMC-12.6、HMC-13.1、HMC-13.2、HMC-13.3、HMC-14.3、HMC-14.4、HMC-14.5、HMC-15.1 |
| P4 新领域/材料依赖高 | 6 | HMC-07.1、HMC-07.2、HMC-07.3、HMC-07.4、HMC-15.2、HMC-15.3 |

下一双轨：原11.2有限成文出口与共享解释的分组/来源/关系适用域，充分事实后的偶发措辞另留档、不无限追同题；轮换只读登记缺来源终态负控、二进制其它具备材料路径及IO原合同。继续复用完整Donghu实采，不用邻近背景充当链上根因；WAL未证明零副作用前不进生产。
