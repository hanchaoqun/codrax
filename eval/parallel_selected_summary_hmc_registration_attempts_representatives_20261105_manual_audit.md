# HMC354 分轨人工审计

root_operator；仅main。机器A/B各PASS，不改历史失败。HMC-11.2原有限能力PASS与A整答FAIL分开，B具名缺来源负向PASS与真实retry NOT_TRIGGERED分开；[汇总/全50项ROI](parallel_selected_summary_hmc_registration_attempts_representatives_20261105.md)。完整原审：[A](results/hmc_registration_attempts_representatives_20261105_v1/sealed-v1/0168-codrax-hmc354-A-audit-v1.md)、[B](results/hmc_registration_attempts_representatives_20261105_v1/sealed-v1/0191-codrax-hmc354-B-audit-v1.md)。

## A：原代表题、当前计划与成文

原题与四份authored SQLite原字节不改，新workspace准备M0/T0，无预选结果、旧答案或预期值喂模型。真实CLI50steps，UTC10:02:01.194711→10:05:26.591993，OS0/源前后true，原log SHAea2fecfd604057539827bcdbe2686a2b83c56aed4c0df98a9249c17e3bcacad6。

分析确实接受representative、per_specification_limit2、线程101/202、10..10.05s；未错误激活源码实现选择。自然discover request_plan=resource_stack编8计划/2精确规格组/每组2，选择早于首查询。当前同代selection/index不是latest别名或旧恢复状态，不把metadata当测量证据。

| 组 | 选中并实际查询 | 未选、尚未执行 | 本轮事实 |
| --- | --- | --- | --- |
| thread101、resource_stack、[10,10.05)s | session-c、session-b | session-a、session-d | 各3次成功尝试；c3事件/8帧，b1事件/3帧 |
| thread202、同视图/窗 | session-c、session-d | session-a、session-b | c3次成功、1事件/3帧；d3次实际查空、0事件/帧 |

8计划/4选/4预算未选，候选4、分组2、源跳过0/未匹配0/新增或范围外计划0独立保存；12实际attempt涉及4记录，不是12代表。3成功/1已查空/4未执行的当前状态与原生payload、正文、代表表、进度表、交付footer一致。未选未宣告正常，无内容重分组或额外空查冒选中。

同一终态logger回执的index SHA210f447bac6bd0dd12c65c47c84171ac83217d2db8ee009eb19c0ed6f645bcc0与selection SHA9a8c38f457a5c33da76b49a78d1120ed8ad0182a8ee29ebde31100de47fd7b6f均核物理原字节，原operator实际OS0/representatives_smoke_pass_human_pending，不修改该原status。root独立核所有选择相关surface；原task da4a33d5f:119只要求软排序、独立全集/未匹配/跳过、不代未选宣布正常、无同文率硬拒。当前自然链与既有公开边界矩阵满足原合同，故有限能力HMC-11.2 PASS；不暗加新算法/报告回写/WAL义务，也不把其它解释债无限挂到此项。

完整答案仍FAIL。5事件/14源帧与原oracle一致，大小/地址/缺失/歧义原字段保留，768分配/512释放是独立事件流量，不冒净存量/配对生命周期。MD324行和HTML main全部源审，17表单元格一致、Markdown摘要匹配，无Mermaid/main图片或图元素；仅源码QA，不冒像素查看。正文却用malloc→ImageCache::reserve→UIFrame::render及libeditor符号写有序调用；后附正确限定不撤销前文越权。另有“各采集”元数据口径概括，须保留原件适用域检查。归12.2/01.3/16.4/18.5，不能签整答PASS。

实际INIT三消息共179986B（67464/36839/75683），含正确组/名册/当前状态、全部帧与“不是有序调用链/完整帧不证明完整展开”的精确原生限定。不是已证缺上下文；调查/正文解释冲突继续留账。首个emit坏JSON仅11B，系统精确拒绝，后一次合法全发射＋一次局部修补，最终文档与当前终态有效；原坏参数保原，不扩大通用恢复交付。root-causes为空/unavailable，未赋予资源栈根因权。

## B：只读登记的缺来源终态负向

具名新retry-audited入口，一次实时planner＋scripted恢复verify→finish controller，非全CLI/自然controller。UTC10:02:01.197866→10:02:18.497270，OS0/源前后true，log SHA3f457f6f97f03d12dc33c2ccac6d20d325d84dab106b41c3b47fcebea600fd56。规划1dispatch/1最终零变更native登记，无规划期执行report、授权已撤；本次真实retry未触发，不倒签§352的旧strict-harness FAIL。

source plan plan-1793872924275310000-15017真实交付value+1，HEAD e823f4ac54f22deebed2dacdff296db3db9bc068。新registration plan plan-1793872937100001000-15017绑定同源plan/HEAD/patch/test/contract，digest b58dd657cd0a24d0df4257d25f65e1b910ddc722a2694e4583e170706c4f4ba2跨进程不变，自身不改源码或承诺应用commit。

独立child15085/parent15017，fresh invocation native:3aed:18e51deeabc99ef0:1不同于old native:3aa9:18e51deba38024d8:1；实际原unittest1断言/0失败，syntax preflight不是第二个原生测试。源SHA833556db828032d49efc4c58a380411c53465c917da89bf613a607a10cee2ba6、测试SHAec7d5c37376fa9c583ba402a196e94c9fa3d1c12c734195ccd18e8e7386a7864、HEAD/status/diff均不变，无执行授权泄漏。

proof batch completion=verified/tests_passed，但overall completion=unverified/missing_terminal_verify_verdict；workflow status=complete不冒源码行为验证通过，persistence_degraded=true如实留存。source_terminal/context_pack_durability=false，不虚签完整恢复链。最终报告SHA5a38fdef558a6391f249f26087acd2d5310ba05428598d7a0e05253fe11745f3的一个原生收据绑定新invocation、source/HEAD/patch/test/digest；parent/child历史归档final→report的ID/path/plan/SHA均matched，旧报告未覆盖。4原件+1验证引用是材料数，不是测试数。

有限缺来源负向PASS；B2–B6、自然controller、全CLI、context-pack durability及真实重试覆盖继续开放。新工装确定性拒绝→登记两次尝试与8负控PASS，旧入口/断言不改。新测试v1漏旧report落盘的前提错误保原，只修新setup；不因新工装成功改旧352收据。此lane无模型最终答案/HTML/图，不虚构成文或像素验收。

## 冻结、全仓、权限与原件

唯一full98包28777顶层/34606子PASS、268604纯JSON、零FAIL/重复/未终态，正式OS0/源前后true。恰好2实际模型例，唯一请求A10/B2，2歧义组排除，4已证重叠对/最大4.595s；无第三例追绿。冻结SHA5be02aa4fc443df5c7976dec846155f6d7fe82001297bff8b7e202c16991489e后未改源/测试/用例/helper。完整用户82.8MB/63.7MB Trace再跑原生query→Finalizer并保原数值/投影，零模型不充实际eval数。

[root有限决定](results/hmc_registration_attempts_representatives_20261105_v1/sealed-v1/0200-codrax-hmc354-acceptance-v1.json)仅close11.2；A全答FAIL、B实际retry NOT_TRIGGERED和所有其它50开放项独立保留。476件SHAabeb2264336a71ed02ba901e136ee17bd2d10e2880dafa312f5e4c40bf6f48ff及旧失败原件不删、不改、不倒签。仅main相邻代码/五文档提交推送；源/原件/远端/清洁另核，Trace因果投影、自动补齐、链上根因/业务及合法计算/重试保持，WAL生产NO-GO。
