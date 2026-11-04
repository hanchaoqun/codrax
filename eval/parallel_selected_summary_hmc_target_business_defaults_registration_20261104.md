# HMC349：查询目标默认展示、完整实采与只读自然终态

完整能力累计28/79，本批新增0；交付子能力1共享组；剩余51项、4组验收欠项。原状态18部分/29待实施/2待验收/1验收中/1持续不变，不以局部通过勾父项。main基线a268f17f70555cd9423ec0b88b0d38346a511f82，源码551e94a48d43c8c67999ce2cba0926d39c988ccf，6Go/343增7删（2生产/4测试）。全程main，源码和五文档分笔提交，远端同代由publication另核。

## 本批共享交付与参考发现

有精确查询TID的宽window_stats仍保全部线程span，但业务publisher原来全部DefaultPresentation=true，背景业务因而自动进入主展示。公开query→actual Finalizer→完整emit摘要/表/无边事实图三surface RED复现。抽取引擎既有TraceSpanMatchesTarget供publisher复用，只改默认布局，不新建相关性判断或过滤查询。

精确TID沿原线程匹配；显式process成员沿原TGID/marker PID规则，不将三种PID混用，也不扩大window_stats支持的scope。宽/未知/仅命名未解析目标保持旧默认。所有背景原生事实、完整Notes、显式选择、查询窗口/总体、来源/成员/完成/因果门仍独立；Trace因果投影、原4根因、闭合睡眠、链上业务/语义及自动补齐不变。无prompt/schema新增、问答关键词门或旧断言放宽。

参考span_analysis.py的thread.tid→itid精确筛选、同步真实打点及时间交集设计，复用物理身份，不搬原c.dur聚合当本项目裁窗时长，不搬名字前缀/固定主线程/首窗口规则。身份控制默认布局，不是因果关系或目标完成证据。

## 小测、完整材料与唯一full

原RED三surface真实失败；首次新green误设部分comm匹配、支持process统计，失败日志保留，仅修新增前提，原引擎精确comm/不支持scope断言不放宽。最终相关五包61顶层/163子、race×2四包22/60、原因果四包21/141 PASS；标准CLI/测试binary构建、零模型启动、原runner实参8项正反检查通过。

完整用户网易云ftrace 82838676 B/559592行，SHA12349db5bbdc5a4343360d9633c7d6c36f18c8be5f39fb64e78b53c9e345bfe4，没有切片替换原件。冻前公开双query/零模型保8业务量测表，仅1目标默认；11成员表及actual Finalizer背景导航保留，原投影不变。旧整源独立整数oracle仅供operator核root627.645/self194.581/子并集433.064ms、全部直接子2131及状态，不注入模型，不冒本轮重新独立计算全调度。

冻结7628输入/287外部/16仓外快照，SHA92dafb88465f264efe424c7a0da96a3be547edaa47e40e1aca3598daabb62697后源码/测试/教学/helper/case/oracle停写。唯一full UTC10:54:03.005878→11:08:14.954836，OS0/源前后true：98包PASS/12无测试、28760顶层PASS/54SKIP、34562子PASS/14SKIP；268330纯GoJSON，FAIL/非JSON/重复/未终态0。log SHA53bb6c6cc6bdd554b36c4200a972b015c5c97b3a0fba49f5017e0992b76ac168。

## 两例原始真实并行与人工审计

| 侧 | 原例，各n1 | 原机器 | root人工 |
|---|---|---|---|
| A | 完整网易云启动，原题/15steps | PASS；8query声明/7唯一JSON | 默认目标分支PASS；整答FAIL，新成员最终selector/图未选 |
| B | 原TestNativeRegistrationLiveNaturalControllerTerminal，12步planner/20m | PASS | 原有限自然controller/独立恢复/只读新执行与终态PASS；非fullCLI/模型写源码 |

机器2/2、有限完整人工1/2；精确已配对A10/B10、A另7歧义忙区排除，最大真实重叠12.713秒。不以barrier/进程同时存活证明模型并行，不按邻近日志硬配对，不追第三例或第二次full。

A保8业务selector、最终仅默认目标bindApplication一组，背景没有删除/升级主因。原生数表正确，正文却称内attach为全线程唯一self睡眠过半，32原生节点至少7反例；自身S70.312/200.474ms与“几乎全部在子阶段”的概括也冲突。其它线程JIT/类校验只作背景、S/未测IO和Binder未知界限保留。充分证据后的误述保原，不加“唯一”等词面硬门。

actual INIT315022 B中两个查询的16业务账户行只覆盖8物理实例，Resources完整状态16.804/33.304未投（仅关系名）；不是32节点状态都已给模型。已有原生库存/省略说明不授权全集外推，重复预算使其它已测阶段失去成文槽。两个坏handoff各7单元及旧reason完整原文未见转发，不冒直接污染。

完整MD1218064 B/HTML4688065 B、8表中同一3735行时序完整出现两遍；不能把“默认业务表少了”冒总字节下降或整组预算完成。0图/Mermaid/像素QA。2行日志非UTF-8、1条inline声明不可解析保原，实际5个completion参数blob均可JSON解析；日志问题与产品调用失败分开。

B由原operator公开夹具准备已修源码，真实planner完整读原测试并用原assertion_ref登记精确suite/id，父controller和独立PID41819恢复子controller真实执行。新invocation native:a35b:18e4d23eae38bbb8:1不同旧及来源执行，绑定源计划/commit/测试SHA；源码、测试和Git不变，grant撤销、6包持久化往返通过。父子各7件typed原件、2条真正final→report链接逐件bytes/hash/plan匹配；当前1测试/2命令与累计2测试/4命令分开，旧weak proof不覆盖。详[手审](parallel_selected_summary_hmc_target_business_defaults_registration_20261104_manual_audit.md)。

## 剩余验收4组及全部51项ROI

1. 原生账户到正文的适用域、状态与有限/全集解释；旧采样未知执行态、IO机理等充分事实后的误述继续留原，不无限追同题。
2. **优先共享缺口**：完整物理业务账户内容复用、逐query独立库存/窗口/来源/权限、跨块完整答案去重与整组预算。不同查询不能合并，总体/覆盖不能借用；不能按业务名去重、删事实或抬预算掩盖。
3. 有实物的二进制/原有限IO、代表/权限/图及只读自然正负终态覆盖。本次只读正向有限PASS不替缺来源负控/全CLI/其它框架签收，最终新成员selector/图仍未触发。
4. 其余协议/平台/新域与WAL零副作用证据，未满足前生产NO-GO。

P0共享（7）：16.4、01.3、02.4、16.5、01.2、18.4、12.5。

P1有材料验收（6）：08.3、18.5、11.2、03.2、14.1、18.2。

P2复用能力（11）：04.3、04.5、08.1、08.5、12.2、12.3、17.6、17.7、05.2、10.2、14.2。

P3协议/材料（21）：04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、09.3、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1。

P4新域（6）：07.1、07.2、07.3、07.4、15.2、15.3。51个稳定开放ID无漏项/重复，旧FAIL及NOT_TRIGGERED均不销；参考仓共享gap和有材料二进制优先。

## 封存与main交付

517件原始材料，index SHAd6134f2e0127f76a30d60b8dcf53c96c37cfd5d6c4ac10018c9494d8d511cf30、seal SHA141aeadc3836d24507c388d905d6b19a8085c42004c6de6de6c7458b2727f701。此前50代22407件＋本代＝51代22924件，按代非全局去重；本轮物理重核348521件与失败344554件，其余48代链头。大原件只留引用，providers不读/哈希/复制，失败历史不补成功publication。

[封存索引](results/hmc_target_business_defaults_registration_20261104_v1/sealed-index-v1.json)、[收据](results/hmc_target_business_defaults_registration_20261104_v1/sealed-receipt-v1.json)、[发布绑定](results/hmc_target_business_defaults_registration_20261104_v1/publication-v1.json)。发布绑定只在两相邻main提交及本机Git实际推送、远端同代校验后生成；不改密钥/仓库权限、不走GitHub写API。代码/全仓与root有限验收分开，closure=none。
