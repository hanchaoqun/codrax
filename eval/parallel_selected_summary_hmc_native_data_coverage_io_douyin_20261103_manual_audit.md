# HMC346人工审计：机器1/2，完整人工0/2，完整任务不销账

79唯一＝28完整＋51开放、原勾选/状态不变；完整新增0，源码8124fec5ba557afdec180caa6da7962872446d30交付2子能力/1共享组。两个原自然题各15步/n1真实并行，精确请求A11/B19、0歧义、最大重叠13.004s。源码全仓PASS、原生取数PASS、自然分支覆盖与整答人工判决分别记账。

## A — 原08.3自然IO题，有限及整答FAIL

原题、专家case import、EXPECT、1839B合成systrace及独立oracle不改。真实2query/2JSON，10工具声明/10参数不混计，2调查提交、1full emit/2patch。进程OS0/源前后true，但原case为`FAIL no_primary_regex_match:(歧义|无法唯一|不能唯一)`，不是机器PASS；初始口头误读进程退出已立即更正，原verdict保原。

四组（RQ8,0R/BIO8,0R/RQ8,0W/RQ8,1R）的峰/均/忙/面积/配对/发起分别为2/1.4/10/14/4/6，1/1/10/10/1/1，1/.6/6/6/2/2，1/.4/4/4/1/1，24项正确。8成员/13段、窗前起点.998、窗后终点1.012、零尾段和partial配对覆盖均保真；1未配对起点、1歧义cohort、pairing-suppressed计数2独立披露。封存手审中“two pairing-suppressed requests”应按原生抑制计数理解，不证明两个唯一逻辑请求；原稿不改。完整请求6ms与窗内4ms量尺未被统计器混合。

完整人工FAIL：primary第1行由有限配对推“任何设备都没有请求堆积”、单次服务延迟主导；第9/282行将RQ/BIO断为同一批IO；第275行把4个相交配对全作6个窗内发起的子集（其中一个在.998起点）。英文原生Notes不豁免缺中文歧义说明或前文越权。9次RQ是同源同层不交设备端点小计，合法，不误判为跨层相加；无图要求不添加图条件。

三实际INIT中仅unit0/6投递、省9803B，已含同类设备队列越权；原生限界同时到场，不能冒整套缺事实，也不能把未投坏单元当污染。全MD32485B/SHA78e4ade0c35ef7f6901743eedd081547dd256010d63e2b823e10e62f094c57de，HTML2813413B/SHA76993487d3d9475f336f6dbec381253141e85724ff2a280f812b5c727d3d5911，15表同值/摘要digest一致，无Mermaid/svg/canvas/iframe/img，仅源码审非像素QA。新不同覆盖数据复用在A未触发，不代签；08.3继续验收中，不加新退出门或追第三例。[完整原始手审](results/hmc_native_data_coverage_io_douyin_20261103_v1/sealed-v1/0245-codrax-hmc346-A-audit-v1.md)。

## B — 原完整抖音启动，数据复用限定PASS、整答FAIL

235422121B/1696481行用户原件SHA93b45bcca5c9e9b415478d81e176b633d70886328c49731b5258c806695de1ef，经原问题/预算自然查询；没有裁剪/改字节或源码分析。4query声明/3唯一JSON（两次window_stats同payload不同query），19工具声明/19参数不加总。generic shell拒绝保原，精确published-ref的read_file/grep允许；两个grep正命中Known Facts均runtime，补上代限定自然覆盖，不回签上代NOT_TRIGGERED或其它分支。

275884B实际INIT有1数据体/16严格内容/3完整查询凭证，三份timeline各2783保留/4预览/2779省略，差异覆盖Notes仍独立。新data-ref自然路径限定PASS；原selector发射绑定完整表，未用内部字典ID冒数据身份。全来源25139/目标2611/展示32/目标省2579和原直子1429不混用。32节点对旧独立整源marker oracle的160项精确起止/总/自身/直子检查零差，非重跑旧parser或新调度oracle。

总区间669.351ms、自身227.101ms与运行487.440/runnable8.714/S170.918/D未标IO2.245/标IO.034ms正确；attachBaseContext总151.430/自身107.397、总运行143.636/自身运行103.841分开。本轮没有把未知频率绑定成目标执行限制，审计不虚增旧错误。

整答人工FAIL：

- 第16/19/44行用11段类校验的首末包络宣称与dex加载重叠。精确同TID语义实例总48945000ns，包络[236627858747000,236628069540000)，dex区间[236627871880000,236627959747000)；11次真实区间交集全部0。家庭包络不是连续占用，更不证明并行/因果关系。
- 第1行把2.3ms D/标IO组合说为不可中断IO阻塞，第31行进一步排除IO主要矛盾；原生明确2.245ms未标IO，.034ms才带标，IO完成型等待本窗未评估。正确后文/原生表不能抵消前文机制升权。

“数百段毫秒级”是尺度松散措辞，不单独设硬FAIL；最初审稿的过强裁定已收回，初稿与更正并存。legacy调查reason有其它错误，但三INIT未含那些精确文字，也未转发结构化解释单元；保原错误，不臆判本次直接污染。

完整MD969348B/SHA765be3b38443054c810749a14c21099cf77c405f165de0c0e5f8f5e4faf9007e仍重复两份2783行timeline和阶段表；HTML4269216B/SHAcddaf41f3f019310f929e00dbb68b4713c466ed213c82cfeb9669c86a187c5c8含11表。原表比较false只两处内联反引号差异，未改原诊断；digest匹配、无Mermaid/像素QA。上下文预览复用不能签全输出预算闭环。[完整手审](results/hmc_native_data_coverage_io_douyin_20261103_v1/sealed-v1/0268-codrax-hmc346-B-audit-v1.md)、[保留首稿](results/hmc_native_data_coverage_io_douyin_20261103_v1/sealed-v1/0267-codrax-hmc346-B-audit-first-draft-v1.md)。

## 批判决与后继

冻结后停写源码/测试/教学/helper/case/oracle；唯一full正式OS0/源前后true，98包PASS/12无测试、28748顶层PASS/51SKIP、34432子PASS/14SKIP，零FAIL/非JSON/重复/未终态。原4根因/闭合睡眠、Trace因果投影、自动补齐、链上业务及合法计算重试保留；没有问答原文关键词门或旧断言放宽。

完整能力新增0；2交付子能力/1共享组不充完整父项。4组欠项是IO有限解释、区间集合/状态适用域、全局输出去重/预算、未轮换入口/协议平台覆盖，不新增稳定ID。只读登记、sample-only/其它二进制、代表/图/权限本批NOT_RUN，不拿前代绿补签；下一双轨优先已证共享缺口并轮换有真实材料的完整能力。500件原件保原、closure=none，全部51项ROI及相邻main提交/远端独立核见[汇总](parallel_selected_summary_hmc_native_data_coverage_io_douyin_20261103.md)。WAL未证零副作用前生产NO-GO。
