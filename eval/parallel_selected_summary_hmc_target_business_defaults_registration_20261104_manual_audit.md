# HMC349 原两例人工审计

root结论：机器2/2 PASS；完整原合同主审1/2 PASS（B仅有限自然controller终态，非fullCLI），A完整答案FAIL。共享目标默认展示子能力1组、完整新增0，79/28/51不变。每例原题/原入口各n1，不重跑求绿、不回签旧失败或未触发。

原始主审：[A](results/hmc_target_business_defaults_registration_20261104_v1/sealed-v1/0266-codrax-hmc349-A-audit-v1.md)、[B](results/hmc_target_business_defaults_registration_20261104_v1/sealed-v1/0288-codrax-hmc349-B-audit-v1.md)、[root裁定](results/hmc_target_business_defaults_registration_20261104_v1/sealed-v1/0296-codrax-hmc349-acceptance-v1.json)。完整日志、参数、JSON、actual INIT及HTML/MD由[索引](results/hmc_target_business_defaults_registration_20261104_v1/sealed-index-v1.json)反查，汇总不代替原件。

## A：完整用户网易云

82838676B/559592行整件SHA12349db5bbdc5a4343360d9633c7d6c36f18c8be5f39fb64e78b53c9e345bfe4，原TID36434、5969.994941..5970.622586/15steps/n1，UTC10:54:02.391198→11:00:55.370683，OS0/源前后true。46工具声明与46参数独立列示，不合计成92次执行；8trace_query声明、7唯一JSON，重用同字节结果不冒独立新结果。

当前window_stats保所有线程span，actual Finalizer的8个业务量测selector均在场；完整最终默认业务表只剩bindApplication一组，7个背景表没有被默认升级。其它线程背景仍可显式选择；公开摘要/表/无边事实图绑定回归PASS不冒其自然最终命中。本例最终emit没有runtime_measurement selector或图，新成员最终选择NOT_TRIGGERED。

原数表627.645ms总量、194.581ms自身、2131直接子、子并集433.064ms正确；原独立整件整数oracle的Running412.174/Runnable11.069/S200.474/D3.928ms与当前原生一致，self122.521/1.498/70.312/0.250ms保真。D未标IO0.099+标IO3.829分开；S中未评估IO完成不当零，Binder索引不完整/闭合0不当无Binder。其它线程VerifyClass/JIT只作同期背景，没有升主因。旧嵌套睡眠相加和self一概纯CPU本次改善，不回签旧FAIL。

完整答案仍FAIL：正文称内attach是全线程“唯一”self睡眠过半。当前32原生节点至少7反例：Resources16.804/33.304、IDisplayManager transact7.235/7.352、setSystemFontMap2.652/4.720、IActivityManager transact4.283/4.363、InternTable锁4.153/4.197等。展示部分不能推出全集唯一；caveat承认省略也不能豁免。自身S70.312占总S200.474的35.07%，仍被概括睡眠“几乎全部在子阶段”。不新增词面硬门，保合法运算和机理未知。

实际第三条INIT315022B：两个query的16条完整业务账户只覆盖8个物理实例（每项重复两次），原32节点中的其它已测阶段被挤出。Resources完整状态16.804/33.304未投，只有关系名；因此不冒“所有反例已给模型”。精确省略/库存边界已给，仍不能支持唯一。这个重复预算缺口挂01.3/02.4/16.4/16.5/18.4，下一批应共享完整账户内容而保每query的来源/窗口/总体/权限，不能按名合并或强制扩预算。

两份新handoff各7解释单元、旧reason完整原文未见完整转入actual INIT（原文与JSON转义同时核）；上游有self/IO越权，但不据此猜本次污染。接受状态、私有叙述角色与实际可见文字分开。5个completion参数blob均可JSON解析；原日志2行非UTF-8、1条inline声明解析失败和3次completion reject全部保原，不能拿logger截断冒产品JSON调用错误。

完整MD1218064B（SHA7810876436fa64fb84223f15dc804eabea98ee4cf6f9ddd5bc522d12bbf4e966）、HTML4688065B（SHA15d87fee6702bb3735f4f5a825c9580c7bcd0573d9296ce19e45dba15c54cb29）；8表含头行2/9/8/3736/7/3736/9/14。两份3735条时序完全相同，单元内容SHA6d09dcd05faa0b90012d1bd1ca4d5f4fc93999ad1397363bb200e1426360281f。来源查询不合并，但共享展示仍应避免重复；完整输出预算未完成，不把少了背景表冒总字节降幅。HTML/MD单元诊断一致、surface digest正确；无图/SVG/Mermaid/像素QA。

## B：原只读登记自然控制器终态

原TestNativeRegistrationLiveNaturalControllerTerminal、原12步planner/20m/n1，UTC10:54:02.394886→10:55:27.983351，OS0/源前后true。真实planner及父controller、独立PID41819恢复子controller；源码准备沿原operator公开夹具，不是模型完成源码，也不是全CLI。原11声明＝8controller/2read_file/1emit plan，不是11次测试。

source plan plan-1793789646464161000-38827，登记plan plan-1793789711905152000-38827，applied commit96efa25502b4ee96b522f3305533a98418d0da1a。planner完整读value.py/test_value.py，使用原assertion_ref展开精确test_value.ValueTest/test_increment并绑定increment-result；空changes不绕来源或版本门。新invocation native:a35b:18e4d23eae38bbb8:1不同旧native:97ab:18e4d22e03f85538:1及来源终态执行，command_index1/1assertion/0failed匹配。2命令是编译预检与一次原生执行，不当两次行为证明。

value.py SHA833556db828032d49efc4c58a380411c53465c917da89bf613a607a10cee2ba6；test_value.py SHAec7d5c37376fa9c583ba402a196e94c9fa3d1c12c734195ccd18e8e7386a7864，HEAD与expected相等、Git status/diff空。原断言检查授权撤销、source/test不改及新鲜执行。工作流三batch complete、最终verified/all_batches_verified。当前verification1测试/2命令，与cumulative proof2测试/4命令、2contributing reports分开；旧weak proof与weak源码定位不隐藏，不授源码解释权。

父/子各7件typed原件（5ChangeReport文件/2真正WriteFinalReport），逐件bytes/SHA一致；各2条final→report link的plan/id/path/hash完全匹配。3条attempt引用不等执行数，历史报告不被新报告覆盖，final-report.json兼容别名不冒真正最终报告。child_memory/durable_raw/durable_loaded/round_trip1/2各6包、6唯一logical/6artifact身份，原内容与引用往返验证PASS。原有限合同人工PASS，不签其它框架/缺来源负控/模型写源码/fullCLI，也不重复算父项完整交付。

## 共同冻结、覆盖及交付

最大已证真实请求重叠12.713s，精确A10/B10；A的7个歧义忙区排除，不拿无键交错日志猜配对或覆盖所有请求。唯一full98包28760顶层/34562子PASS，54/14SKIP、12无测试包、268330纯JSON；零FAIL/非JSON/重复/未终态，源码前后true。冻结后代码/测试/教学/helper/case/oracle未改，仅post-terminal主审和批次文档。

517件index SHAd6134f2e0127f76a30d60b8dcf53c96c37cfd5d6c4ac10018c9494d8d511cf30、seal SHA141aeadc3836d24507c388d905d6b19a8085c42004c6de6de6c7458b2727f701。累计51代22924件按代非全局去重；旧失败不补publication。源码551e94a48d43c8c67999ce2cba0926d39c988ccf与五文档的相邻main/远端绑定另核[publication](results/hmc_target_business_defaults_registration_20261104_v1/publication-v1.json)。

后继4组：有限事实/状态到正文；共享账户预算和跨块完整输出重复；有材料二进制/IO、代表/权限/图及自然正负覆盖；其它协议平台/WAL。全部51项按[汇总ROI](parallel_selected_summary_hmc_target_business_defaults_registration_20261104.md)逐ID排序，旧采样/IO误述不无限阻塞其它高ROI。Trace因果投影、自动补齐、原4根因、闭合睡眠和链上业务语义保留；WAL零副作用未满足前生产NO-GO。
