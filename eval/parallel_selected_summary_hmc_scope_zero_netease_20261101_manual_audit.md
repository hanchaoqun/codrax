# HMC332完整人工审计：过程、实际上下文、原生事实与全部答案表面

批号20261101。root逐例审计，不以OS0/原smoke或正确原生表替代整答。结论：A原18.3有限FAIL、A整答FAIL、B整答FAIL；完整能力新增0、子能力3组、79/27/52不变。冻结后不改源/测试/教学/helper/case/oracle，不追第三例。源码与发布身份见[批汇总](parallel_selected_summary_hmc_scope_zero_netease_20261101.md)。

## 1. 可复核证据入口

ROOT为`/Users/han/opt/codrax/eval/results/hmc_scope_zero_netease_20261101_v1`，下表相对ROOT。

| 材料 | 本代封存路径 / SHA256 |
|---|---|
| A完整原始手审 | sealed-v1/0223-codrax-hmc332-A-audit-v1.md / d3c9fa59a20e148d63f45109cb9a44558920793aacc44085a1a20655c0096791 |
| B完整原始手审 | sealed-v1/0244-codrax-hmc332-B-audit-v1.md / f81ef14523dee33b387541d8a478d71d77dee473b12c249634250523abe4b99b |
| B全源后置核对方法 | sealed-v1/0265-codrax-hmc332-B-post-audit-method-v2.txt / 5c2efc2d49ae3e3a788cbe4189e81d03b7c37b116bd8e3d87696fc99b96543fa |
| B全部16账户/3735行核对 | sealed-v1/0266-codrax-hmc332-B-post-audit-v2.json / e58e0d0ed880553f6d9eb54b8183fdbdc332be7ce4b9f894b35ee73e074f1ac9 |
| 模型前独立整数µs oracle | sealed-v1/0315-donghu-independent-oracle.json / a5620e7a1493989ee02f23feffcee90ce736726a268bb3d52ed78fb465fd6f12 |
| 实际并行请求配对 | sealed-v1/0268-codrax-hmc332-actual-request-overlap-v1.json / c363eeff6a84bfe8415bb8cf352c8d1dcbd5df621d018b5fe0a5bc079e5e41ff |
| A实际进程收据 | sealed-v1/0307-codrax-hmc332-live-A-v1-receipt.json / 7727863442d56ffb78756542ec7881f810344f932ab5b2fb04304a1a4faa8e78 |
| B实际进程收据 | sealed-v1/0311-codrax-hmc332-live-B-v1-receipt.json / 8c274c19a5e9298cfc3234822a685690b5c6904d7204e9574a2f1b5a9e5bf4e3 |
| 不销账决定 | sealed-v1/0267-codrax-hmc332-acceptance-v1.json / e5b660981f34890a9ce821a7222ff2829e49dd1ffe4c0976e78b23cd326629bd |

实际query、INIT、完整MD/HTML、tool声明/param blob、受理及拒绝稿、诊断错误和原始debug逐文件映射见[index](results/hmc_scope_zero_netease_20261101_v1/sealed-index-v1.json)，365件SHA`13444f60917d02d51c5ec0978823fe5e72b986c8617a009f3044314646b41b5e`。两例原日志均有无效UTF8，按字节留存，不替换后再称原件。A/B收集receipt分别为0227/0248-collection-receipt.json；收集时缺失的工具引用清单保留，不据missing-log-reference推原始Trace不可读。

## 2. A：原sample-only二进制，原有限合同仍失败

原case/question/assertions完全未改，普通CLI15steps/n1，run `A/trace_binary_sample_only-20261101-174501`。完整test_brbe.data为30062585B，SHA`da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`，无裁剪、stub转换器或oracle输入。official simpleperf adapter不可用/未尝试，raw parser真实尝试成功：12000物理样本完整进入可查询normalized perftrace；这不是BRBE/SPE、设备采集完备或已知on-CPU类型证明。

7探索trace_query成功，2个确定性assembly补查root_cause_rank/critical_blocking_calls成功，9唯一native JSON，不是9个独立来源。11条工具声明与11份参数blob独立计数，不加成22次执行。原7文件scalar oracle按HMC299身份复用，不冒本代重算或符号真实性独立验证。

### 2.1 已知量、总体和范围

目标TID1609/TGID1514，原窗486489.4–486489.6s：164条/259事件权重。IP0xffffff800b829d24为82条/174权重，占67.181467%；OnDataEvent_为82/85，占32.818533%；记录比例各50%。source-window1393条不是目标分母；完整source12000与显式宽窗486400–486600s的515条/1156权重是不同集合。另一次未显式给窗的perf_stats继承原窗，不冒整采集对照。

两根均为完整一帧，不能按深度推caller边。CPU1/IP、CPU0/sensor为已知记录坐标，sample_kind unknown100%，不能推执行状态或迁核。sample包络sensor486489.464088–.465612、IP.588096–.589609，两者间122.484ms无记录，不是测得的睡眠/阻塞/等待CPU。1ms四桶49/52、33/33、50/142、32/32（条数/权重）合计164/259；event_search30/164和IP12/82是另行成员预览，不否定完整聚合。权重不写成耗时。

### 2.2 实际成文上下文足够，不能伪造上游丢失

只有1次成文，3条INIT消息system/user/user；提取字节110457/7828/252760，logger framing差异不冒provider wire。INIT03确有两个完整热点、分母、unknown、权重规则和一帧树。预算展示3/4视图完整、预览行28/128是实际有限展示，但本次两热点关键事实完整；没有证据把错误归为丢字段、截断或OOM。

8个受理调查unit只投[0,1,2]，5单元/3047B省略，实际标记no_native_row_address。坏unit3“无样本即未运行/睡眠”、unit6和reason的供给排除未转发，**不是实际INIT污染**。最终仍自行产生执行断言，应保真实因果归因而不是为了系统修复编造污染。省略解释不等于原生unknown边界被删。

### 2.3 有限与整答FAIL，正向边界也保留

primary第3、52、58行把采样点说成“线程在CPU上跑/占用CPU/消耗CPU”；原生表同答第20、38行却是“未确立（样本类型未知）”。CPU坐标和hw-cpu-cycles名称不授sample_kind，后面正确说空隙未知/不能定根因也不撤销前面的确定执行。这正是18.3原有限合同内的旧反例，不另加新格式门，所以不能打勾。

其它成功：两热点/164/259/82/174/85/比例正确，未解析IP不命名，权重不转耗时，无具体阻塞者/唤醒者编造，缺blocking记录不说实际无等待。自动Trace投影明确unknown/context-only、链停止、无根因时长；背景没有冒主因。元数据空值/缺值/未知分开，不以hostname升设备身份。主文66行和完整178行MD、全部提交块/Notes/旁路均读，完整结果不是只审首段。

### 2.4 展示QA

本轮完整MD27330B SHA`fb86d180b284a179df2f5115c1693ff96c7da35b0e5104522431dc3f7ad871f2`；primary9780B SHA`622095ad7bf71495085dfaa3ba83ebb56dea14e2585b5e6bf02dac4c096f3e23`；HTML2814161B SHA`903e9de0cc7126f0a6a3d938dd44f0dfbf754812d60feb840a6d4ffb6fb0bb48`。5张表含header行数2/2/2/2/5，HTML与MD解码单元一致，surface MD digest吻合。无Mermaid/SVG/canvas/iframe/img，图身份/局部修补未触发，不能冒图验收或浏览器像素QA。

## 3. B：全源网易云业务启动，原生正确、正文仍失败

新case普通CLI15steps/n1，run `B/donghu_v105_netease_bind_application-20261101-174501`。完整82838676B/559592行，原件SHA`12349db5bbdc5a4343360d9633c7d6c36f18c8be5f39fb64e78b53c9e345bfe4`，不是旧合成IO夹具。header TID/TGID36434与marker payload PID4943不做无证映射；闭合bindApplication精确行77708–152774，5969.994941–5970.622586s。全源保留选中闭合区间外1个未配E、无选中open tail/PID冲突、11964目标scheduler事件，不据此称连续采集绝无缺失。

### 3.1 自然入口、工具结果和导航成本

5真实原生查询：event_search找bindApplication并继承窗→span_window给完整配对→原生business_span_ref驱动window_stats精确线程/窗→target B事件查询→critical_blocking_calls。各5份唯一JSON；assembly补齐families_present跳过，无第6原生查询。window_stats拓扑15329/发布32/省略15297，完整账户计算在截断前完成；目标direct children是2131，不混用。事件查询matched2709/emitted200，物理timestamp扫描75067行，scope完整不等于成员全集。

12次结果导航为2exec/4grep/6read_file，不是12次源查询/失败；模型在描述TXT raw_ref里反复搜business_tree/inclusive，而可读payload_ref JSON已给。这是可泛化schema/导航成本债，不是Trace不可读、binary不完整或业务名专修理由。task→真实业务引用→原窗取数自然成功，不能为正文误述破坏正确自动补齐。

### 3.2 原件独立核对：16实例/80数值/3735时序行全部一致

全源模型前独立整数µs oracle只作operator标尺，冻结后未改。root627645 inclusive、194581 self、433064 child union，2131 direct children。whole Running412174/Runnable11069/S200474/D-family3928/Unknown0µs；root self对应122521/1498/70312/250µs。native无标D .099与带标IO3.829ms互斥相加3.928ms，只签粗D-family总量；独立标尺不背书IO标记关联或物理设备耗时。

| 完整实例业务名 | inclusive ms | self ms |
|---|---:|---:|
| bindApplication | 627.645 | 194.581 |
| makeApplication | 196.033 | 2.181 |
| 外层Application attach | 192.164 | 80.890 |
| 内层Application attach | 111.271 | 82.388 |
| APK zip | 70.223 | .964 |
| APK OpenDex | 68.356 | .550 |
| odex | 61.001 | .534 |
| AppImage | 60.456 | .377 |
| getResources | 39.160 | 33.304 |
| MapImage | 27.692 | .867 |
| core-oj OpenDex | 23.015 | .369 |
| OpenDexfile | 22.337 | .154 |
| VerifyDex | 22.183 | 22.183 |
| RelocateImage | 21.797 | .166 |
| SetupProxies | 19.565 | 4.028 |
| Fixup | 15.954 | 15.954 |

16实例按精确源行核，80数值＝每实例inclusive墙钟、self墙钟、inclusive Running/Sleeping/Runnable五项；另核物理端点、窗口及self状态，不把这些额外核对混入80计数。名字不替代身份。同名attach未合并，父子marker是合法业务线索，不自动变wake/根因链。后置核对重用冻结整数解析片段，明确post-model/non-model，不重执行准备主程序、不查生产、不改oracle/helper/input；方法/结果均保留。首个只读诊断用IO匹配I/O等待产生10假差异，精确标签映射后消失，坐标原本都正确，不登记产品状态回归。

全部3735行逐项对clip起止/时长、actual完整端点、物理begin/end行与粗状态，数目/顺序一致。头部实际Running始于5969.994902/行77701、查询clip为.994941；尾部实际结束5970.622836/152802、clip为.622586，不造端点wake。Unknown0仅属构造观测分区，不证明采集完备。

S1138＋无标D33＋带标IO10＝1181 wait inventory；粗D-family43段/3928µs，完整timeline保全部43，另occurrence receipt只发32。整数分布32段<20µs、8段20–180µs、3段>180µs，max1582µs，不能浮点阈值误数为33。S+D-family的sleep union204.402ms不能套S-only200.474。Binder verified0但scan不完整/1181 unresolved，不证明无Binder/无机制。

### 3.3 实际INIT和调查摘要冲突

1次成文/3消息，实际字节100509/16030/372897；INIT03行448–467有16完整账户、inclusive与self状态、实例/源/窗、32/15329和范围说明。预览只部分展示不等于关键账户丢失；后文完整timeline也实际渲染。未发现OOM或目标原生事实截断的证据。

7受理units实际只投[0,6]，6664B省略。unit0的root627.645/self194.581/direct2131引用却指trace_business_span:1（H:getframe/aptouch_daemon），不是root:2。current_exact_rows只校验当前行地址，旁边明确未验证文字/算术/因果，不能把它说成系统已签语义；主体/量尺交接须继续修。

unit6以小目标Runnable＋背景聚合排除CPU供给，确实转发，是共享解释污染；最终正文只发表较窄“排队贡献小”，不能把所有供给排除都挂给最终答案。其它坏unit（嵌套dex写等待式、恒频/RT推未抢占、D短推无存储）没投递，不算实际INIT污染。保留真实选择/省略，不用全部调查原文替代实际模型上下文。

### 3.4 整答两组FAIL和其它留账

B1账户量尺错误：primary32将VerifyDex22.183写“全Running”，同表实际21.760Running/.383Sleeping/.040Runnable；self≈inclusive只说明无同步子段，不说明无等待。inner attach self82.388错配inclusive Running62.683，其self真实38.783Running/41.701S/.290Runnable/1.614带标IO。getResources selfRunning15.289使用正确。全部正确事实已进INIT，不改原生表迎合正文。

B2分布外推：primary38说D/IO单段多在.02–.18ms，43段整数分布实际32/8/3，max1.582ms，连32条预览也含max。保留完整43/已展示32和原量尺，不把它解释为另一种D定义或物理设备结论。不是只修某个业务名/type的理由。

其它共享债不新增父退出门：summary手写16行表＋structured stage_table再次显示同16实例/80值并多长ID列；7旁线程原生表有各自身份/窗，最终正文没当主因，是相关性噪声/重复不是已证根因晋升。目标queue11.069ms占约1.8%是有效观测，“可以忽略”措辞偏强；不凭它伪造“最终明确排除全部CPU供给”。nested Application睡眠集中是可查业务方向，未写错误相加总数不加新失败。

正向保留：root完整量尺/2131直子、16阶段值、线程/窗、同名独立实例、dex校验/资源业务线索、8个CPU频率区间、原生D/IO互斥、sleep-IO子集、Binder未验证边界均在。无parent/child inclusive重复加，无具体阻塞者/wake源/IO设备编造。查询窗与完整业务配对区间分开，原事件和clip端点保真。没有因果链或Mermaid自然触发，不冒该维度PASS。

### 3.5 全部表面QA

完整MD632475B SHA`0a62f9d11d3f244dca34bcc8b1285a1e03c93b9a39ffd33fd6fdcaf745322df8`，primary11875B SHA`24b37acbb7342c6140ae77e7895e6f93bd96ae3eb3cd20d9a3dc131c0c9bb2de`，HTML3758928B SHA`7202d23cb370dbcba960d7d68b4614554be48ea620a09509c813cd3099226aec`。primary97行及全部其它正文/Notes/旁表读审；3735时序及16阶段矩阵另外完整自动独立核对，不声称3735行逐行人工目测。

14张HTML表含header行数：2、17、17、14、7、3736、9及7×14。首张手写阶段表16名称格backtick与HTML code表现差异，不是量测/时间错误；其它scalar/time一致，surface MD digest吻合。无Mermaid/SVG/canvas/iframe/img，图身份/局部修补/语法仍NOT_TRIGGERED；QA仅源级，未浏览器像素验收。

## 4. 跨例处置、未触发和下一步

两例OS0/原smoke PASS/源前后true，唯一全仓PASS也不能覆写3组本代语义失败或旧失败。IO端点/合法小计与catalog-zero本代公开正负回归通过，但自然模型均未触发；只读登记负向终态、代表规格、其它二进制平台和图修补本批未跑。上一代08.3失败不由本代修IO教学倒签。原4候选、闭合睡眠、Trace因果投影/自动补齐不放宽；WAL零副作用未证明前生产NO-GO。

后继共享轨优先可公开复现的账户/解释主体与量尺绑定、原生payload导航、整组去重预算；完整能力轨轮换有材料的有限合同与只读缺来源终态负控。对事实充分后的旧采样误述保原FAIL，不把未证实的波动归因当销账理由，也不无限复跑/收紧某题。旧IO、采样、权限/图及h2/h3、lower-cost-full等原债保持；不得扫描问答原文作硬门、代写结论或把背景当根因。完整52项排序与main发布凭证见批汇总。
