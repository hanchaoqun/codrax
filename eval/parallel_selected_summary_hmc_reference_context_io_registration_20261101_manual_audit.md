# HMC333 两例人工审计

本代恰好两例各一次、并行。原始过程、实际INIT、全部拒绝/受理、最终表/正文、旁路和真实登记终态分别核对；不以smoke/OS0替代人工。批源码、冻结/测试和52项ROI见[汇总](parallel_selected_summary_hmc_reference_context_io_registration_20261101.md)。

## 1. 独立结果

| 维度 | A：原IO自然题 | B：原只读缺来源终态负控 |
| --- | --- | --- |
| 入口 | 原ordinary CLI，15steps/n1 | 原live planner、脚本恢复controller、新进程/public RunTests |
| OS/源 | 0；前后冻结一致 | 0；前后冻结一致 |
| 原smoke/harness | PASS | PASS |
| 原有限合同人工 | **FAIL**，HMC-08.3不销 | **限定PASS**，不代完整18.5 |
| 整答/交付人工 | **FAIL**，原题解释错误 | 原负控交付边界正确；非模型最终MD/自然fullCLI |
| 新描述自然投递 | 原预算全省略，NOT_TRIGGERED | 不属于此Trace分支 |
| 新格式导航自然行为 | 无结果读取，NOT_TRIGGERED | 不属于此Trace分支 |
| 图/像素 | 无图；HTML仅源审 | 无模型图要求 |

root明确[closure=none](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0203-codrax-hmc333-acceptance-v1.json)，完整新增0、子能力2组；所有旧FAIL不改签。两进程并行不替代实际请求重叠：当前严格配对A18/B2、歧义0、最大5.915秒，[原始配对](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0204-codrax-hmc333-actual-request-overlap-v1.json)。

## 2. A：数表正确，原题解释仍失败

原问题为“帮我分析这份 trace 在1.000到1.010秒里的IO请求情况。按RQ/BIO、设备和读写分别列出峰值和平均在途请求数、有请求在途的时长、请求数乘时间的面积，以及期间发起了多少次请求。这些结果能说明什么？”没有为凑绿添加单位/边界规则或降低断言。输入是原构造文本1839B/23行/21事件，不冒实采；oracle独立、不进模型。

实际一次window_stats query，源及[1,1.01)身份正确，原生JSON SHA`44f309be27f50b33956d71a9d78ede6a693e2c1b30ea0a2f55bfcfde005e9f28`。完整逐组核对：

| 组 | 峰 | 均 | busy ms | request·ms | 相交完整配对 | 窗内发起 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| RQ8,0读 | 2 | 1.4 | 10 | 14 | 4 | 6 |
| BIO8,0读 | 1 | 1 | 10 | 10 | 1 | 1 |
| RQ8,0写 | 1 | 0.6 | 6 | 6 | 2 | 2 |
| RQ8,1读 | 1 | 0.4 | 4 | 4 | 1 | 1 |

四RQ读完整请求驻留6/4/4/4ms，窗内4/4/4/2ms；原物理端点.998→1.004、1.002→1.006、1.006→1.010、1.008→1.012。深度1→2→1→2，等深段可跨成员更换。4相交配对不是6窗内发起的子集；.998发起在窗外，1.012完成仍有窗内2ms贡献。RQ发起6+2+1=9与BIO1分开，不能推出10个唯一逻辑IO。

block-family partial：8合格配对、1未配对发起、1歧义组、2抑制行；不补未完成尾部、不猜FIFO。原生表覆盖/来源版本/半开标题都正确，捕获完整性未知。RQ8,0起点8/32768已知B、8,1起点1/4096、BIO8,0起点1/4096，字节率是全10ms端点统计，不是设备带宽校准。

最终8张原生表保全部7个选定展示，并有RQ读members；RQ读summary自动附加未丢。附件大小1839B与预览1929B分开。HTML9表含附件、21数据行，MD/HTML格一致；没有Mermaid/SVG/canvas/iframe/img，未做像素QA。root-causes旁路为空/unavailable，原因no_selectable_typed_on_chain_candidates。

两组原合同失败均在实际正文：

- 摘要/解释说“IO不构成瓶颈”“可以排除存储排队导致延迟”，用低配对深度、10ms忙碌对4–6ms驻留推断“基本一发出就完成”。配对深度不能证明设备服务/队列或排除瓶颈，尤其有未配对与歧义、无校准。
- 列表说六次发起只有四次完整配对；末尾又说窗外端点如1.012不计窗内数字，与同答2ms贡献表矛盾。到达、相交配对、完成端点与裁窗量必须分开。

“这些结果能说明什么”是原题，不得将错误另称不相关解释欠项以关闭08.3。原生正确、非因果边界正确不取消错误。当前“RQ/BIO很可能为不同阶段”未被当已证物理对应，此审计不多造一个精确跨层配对错误；“当前仓库没有对应文件”也不自动等于已声称附件不可访问。

## 3. A：实际上下文/受理状态而非推测污染

7次completion：前两unknown source/path；第3重构ID；第4引用不在所选测量；第5展示超出所选；第6缺member_set降级；第7加原要求member_set后resolved。模型思考猜description用途，但没有实际unknown description拒绝，不能据此认定新schema要求错误输入。

受理9个advisory单元，实际Finalizer只投[0,1,2]三个完整单元，披露5851B省略。坏unit5单通道/无队列、unit6无积压、unit8无调度即D/IO零均未投递，不能冒“调查摘要污染”。投递unit0正确区分相交与到达，后续正文仍错，不能用没转发的坏文字作原因。

本轮既有1800B预算保三个原单元，但任何optional reference context均无余量；实际明示3个context compact-or-omitted，文本没有context对象。因此新描述自然正向未触发，不能把模型外NetEase消息回归代签自然覆盖。关键四组原生事实、完整端点/裁窗、覆盖、量尺及unknown-not-zero规则已在实际INIT，当前未见OOM或关键事实总账丢失。没有结果read_file/grep，不宣称新格式说明已减少12次导航。

成文1次初始化3消息，1full emit拒绝、5patch四拒一受理。错误对象形状数组不staged，末次失败不覆盖此前23:00:30受理稿。成功局部patch保原s1/list1/s2/c1，图/关系修复未发生；自动补齐保RQ读summary。JSON形状/选择器重试仍是需简化的共享教学/易用性缺口，不放宽decoder或扫文字硬门。

完整人工原文：[A根审计](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0173-codrax-hmc333-A-audit-v1.md)；[完整最终MD](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0095-run-1.answer-transcript.md)、[完整日志](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0097-codrax-20261101-225125-000-82979.log)、[实际INIT03](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0182-log-01-finalizer-init-03.txt)、[原生query全字段](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0186-native-query-payloads.json)。MD SHA`f32cc1c187f74fefae6fd21b9c8749c5a01401fd9f46c640ab4be438c952e618`，日志SHA`0b63b9246c2cd583fe710074b5a7f075870136719a5cf3ea35f96137ecd9c752`。

## 4. B：真实零修改登记、新执行与缺来源终态

原入口`TestNativeRegistrationLiveRestoredMissingTerminalReceipt`未修改；原real planner/脚本恢复/child/public RunTests负控，不是自然controller/fullCLI。父PID82930、子83055；父15.86s、子.45s，OS0。实际planner两请求均attempt1/6；完整读取value.py2/2行、test_value.py4/4行，`changes:[]`加1既有测试观察，没有改源/测试/探针/配置/依赖。持久化changes:null是Go空slice，不是模型变更。

原assert为increment(4)==5，源return value+1。sourceplan`plan-1793602285825748000-82930`、实际commit`e216136bcb776744c86952de9f0e1fd9a7be2463`；proofplan`plan-1793602300953307000-82930`。旧`native:143f2:18e427c6b2c99fd0:1`与新`native:1446f:18e427ca44b7bc08:1`分开；新process真实执行1assertion、test_count1，编译25ms与unittest45ms两个命令，existing_test_executions1。probe_count2不是两个模型探针或两次断言。

独立子审计只读、HEAD/diff/status不变，源计划/登记/PTO不变；子attempt/verifications/completion不变，父只添context-pack token，context_pack_durability_verified=false。source仅apply/applied，无VerifyRef/Completion；proof verified/tests_passed而整体unverified/missing_terminal_verify_verdict。两批状态没有混写，未将注册本身当执行凭证，也未将workflow run完成当来源验证完成。原测试还断言撤销写授权，新执行未绕开隔离。

整份actual final与content-addressed report、父子两个归档逐字段/原字节核对通过。primary sourceplan及source_plan_with_later_validation_followup正确，final_plan_validation_only=true；materialization unavailable/apply_checkpoint_missing、observed_only不冒已证源码定位。四原件父子同字节不是四次执行。actual final SHA`1ec21788cb8fa208038aaf49e8c1cd28aa0e74de153506ba2bfcef205c1213a3`；report原字节SHA`28b2ec96d0f6a6b06431258eadbe76c8037d1fa41df90ba4ca05b2c81a2ad180`，canonical SHA`c259666ba333ecb165d9b0b883e4a93798c902b858e4559c287be9023411cb8c`不是同序列化身份。

限定原负控人工PASS；不证明任意输入、更多框架、自然恢复controller、模型最终MD/图、context-pack耐久或完整18.5。两份INIT是同次planner system/user；重复行为保持教学仍可精简，但无截断/OOM或当前失败因果证据。完整原文：[B根审计](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0195-codrax-hmc333-B-audit-v1.md)、[原harness outcomes](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0199-original-harness-outcomes.json)、[actual final](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0310-plan-1793602300953307000-82930.final.json)、[完整模型日志](results/hmc_reference_context_io_registration_20261101_v1/sealed-v1/0312-codrax-20261101-225125-000-82930.log)。

## 5. 已交付、未触发、旧债分别留账

两组有界源码能力通过public/related/race/唯一full。完整网易云原件用于模型外query/真实adapter消息验证：Extract compact、Finalizer full描述，精确实例627.645/194.581双账户；这是正向共享接缝证据，不是第三例模型eval，也不声称任意解释已正确。初始self表断言前提错、旧HMC315预算回归、helper语法草稿等失败全部原样封存，不弱化旧断言。

剩余5组中IO两组是已证原合同FAIL，schema/JSON形状是共享易用性债，两个新自然行为是NOT_TRIGGERED。总体27/52不变。旧采样、IO/权限覆盖、图主体/绑定、代表执行交付、二进制其它适配、去重和全局预算未在本代重验，不能用这次负控或新全仓补签。冻结后没有补源码/教学/helper，也没有追第三例求绿。WAL零副作用条件仍未达标，不接生产。

347件[原件索引](results/hmc_reference_context_io_registration_20261101_v1/sealed-index-v1.json)与[封存凭证](results/hmc_reference_context_io_registration_20261101_v1/sealed-receipt-v1.json)保留全过程；最新旧365件物理复核、其前历史只核链头，不冒全历史重验。main相邻代码/5文档/远端及冻结blob核验独立于人工verdict，以[实际publication](results/hmc_reference_context_io_registration_20261101_v1/publication-v1.json)为准。
