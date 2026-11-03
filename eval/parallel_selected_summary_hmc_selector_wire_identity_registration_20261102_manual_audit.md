# HMC337 独立人工审计汇总

2026-11-02本地，main，冻结SHA `c2fcde4a53a968b13892eab365029fa5d7516cdd0223f5737305ea260df33210`。本批恰好两例并行、各n1，无第三例求绿；冻结后不改源码、测试、教学、helpers、case、oracle。审计/文档结果生成不冒新测试。源码`e068a13cffb696c0979d7aad98cda1ff523082bc`；79唯一/28完整/51开放，完整新增0，共享子能力1，测试精度工程项1另计。

## 1. 分层结论

| 审计层 | A原资源身份自然题 | B只读缺来源终态原负控 |
|---|---|---|
| 进程 | OS0；源前后true | OS0；源前后true |
| 原机器 | FAIL no_primary_text_regex_match，非CLI0推定 | 原harness PASS，非CLI0推定 |
| 原有限合同 | 03.2 FAIL | 持续18.5的限定负控PASS，不关闭父项 |
| 原生取数 | 3query；10标记＝5资源/5计数，原始身份保真 | 精确既存1断言、真实新invocation及source归属 |
| 最终解释/状态 | 完整FAIL，正确明细/尾注不抵销 | proof verified、来源缺终态，overall unverified正确 |
| 成文/图范围 | 一成文3条INIT；完整emit1、patch3；无图 | 无模型成文；脚本controller，不冒自然controller/fullCLI |
| 新定点提示 | NOT_TRIGGERED | NOT_TRIGGERED |

§336两原例机器PASS/人工FAIL和全仓FAIL保持原结论；本次全仓PASS不倒签历史。A CLI成功一度口误成机器通过已即时更正，原verdict始终FAIL未重写。旧充分事实后的采样/IO/代表解释失败仍留账，不以本轮B负控通过销掉。

## 2. A原件与全表面

原问题、数据、case与原README独立期望未改，标准CLI15steps/n1。原资源text2503B、SHA `841d772de200860d1a13ddbae65912995fa39e9043438671ac514393c0af5dfb`；预览2606B不能当原件大小。resource_stack在原窗不可展开不等于无资源操作；event_search边界容差[.00045,.00555]与[.0005,.0055)统计窗分别表述，匹配10行完整不代表整个采集完整。

10行最终有序列表保signed/hex、精确2^53以上整数、NULL/零、空串/SQL NULL/未解析子类；Heap三点8192→4096→4128、Mmap两点4096→8192各自分母/序列。合法端点差与未知单位可陈述，不把C序列合并、量级吻合升为单次I释放量。窗口外FD未混入。

人工FAIL具体为：精确大整数被断言典型占位/不能真实读取；.003子类8在正文又被说无编号；closed-running与缺测外推无竞争/调度无问题/硬件压力全零；共现先后升级同一资源生命周期并无依据判断幅度小。03.2有限FAIL来自原身份/解释合同，不追加图/完整栈/泄漏证明门槛。尾部“不能证明变慢成因”和正确优先级说明不免前文矛盾。

实际finalizer3消息112161/8427/129340B，SHA依次`4114985f42721460091c75553e9914e760600cf98eb779c0718de31797307e9c`、`8059aea39e684858e77532f084f3513689418cfdc1867abfe92dc3a6e243ee6a`、`86893ff65d15631115fb7fd70703819739812d57b12982e064ae683a0d8934e3`，三条均有效UTF-8；原810265B日志非UTF-8原样留存，不推断provider wire。调查只展示8单元的0/1、省4126B，未投坏解释不能直接冒污染源；源字段和缺测范围充分，正文错述独立留账。

原生复用尝试：完整answer table/event_selection被拒；patch1只改summary，又因两个summary结构拒绝。后一错误不证明ID/view/row已通过binder，可能先触发结构检查。patch2手写10行、patch3补member_set均受理。需精确公开复现修补交接，不能自动改kind或放宽断言。completion首次合法scheduler两view，定点提示未触发。

最终MD13162B SHA `c252875b7b2f23370732a546e8a77245c06e666a325a738b1d4e18af82550481`，HTML2791044B SHA `7ecb74a103fc672c5dfa3b3e977ffd24ff8135baa571e388d4d4b9065c4270ab`。正文、列表、覆盖说明、系统Trace补充及附件均审；仅1附件元数据表、含表头2行，MD/HTML格值诊断一致，surface markdown digest一致。无mermaid/SVG/canvas/iframe/img，图关系/主体/修补N/A；HTML源审非像素QA。无源码引文不构成新增义务。

独立完整审计：[A封存原审计](results/hmc_selector_wire_identity_registration_20261102_v1/sealed-v1/0169-codrax-hmc337-A-audit-v1.md)，SHA `156b4a03d9181dd34bb1fac8a6aa832ad0f79bb7a084ba3f57300929fcb8c325`。

## 3. B真实恢复、当前/来源终态与交付

原`TestNativeRegistrationLiveRestoredMissingTerminalReceipt`，真实planner两请求读取现有实现/测试，既存`test_value.ValueTest/test_increment`→increment-result精确登记；来源由原operator公共emit/apply/git/native执行准备，不冒自然源码创作。独立子进程scripted_verify_then_finish，验证为public_run_tests，不冒模型verifier/fullCLI；本变体无context-pack耐久验收。

source plan `plan-1793680080452726000-29118`，登记plan `plan-1793680089127128000-29118`，source commit/保留HEAD `d05c9ab78fbe0365e8c079c834481b95cea8cd0f`。当前invocation `native:7246:18e46e89c1fd1e50:1`不同于旧`native:71be:18e46e87b0b79e60:1`。1断言0失败、command_index1、原件数量分别记账。源码/测试/config/依赖/HEAD保持只读、授权撤销。

原receipt passed/proof_batch_verified=true，但source_terminal_verification=false；workflow_completion=unverified/missing_terminal_verify_verdict，durable_run_complete不升级verified。父子final/report均当前同代，报告7604B SHA `3dd8cef38f69df3c75da02f2b21a6b74b9c23e79975cfcb84fec1d833dd301d3`，历史2717B报告不覆盖。父子各4原件/1verify引用，逐个物理hash及final_report_id/path/report_plan_id/report_sha256四项匹配；同字节多副本不算多次执行。

补证通过但整体未验证的负向边界限定PASS，不关闭持续18.5，不签自然controller、其它框架/多来源或完整模型答案。父日志真实2request/response，脚本子恢复无模型日志不冒缺失模型证据。独立完整审计：[B封存原审计](results/hmc_selector_wire_identity_registration_20261102_v1/sealed-v1/0192-codrax-hmc337-B-audit-v1.md)，SHA `271c7f6adc9397aa5fdc460d4cee0dcd82fe240a0b1bffaed657a352c64a8705`。

## 4. 全仓、原4候选及完整ftrace保护

§336旧全仓FAIL不改，本代先用确定性反例复现整JSON查768的测试误报/漏报，再改为结构化精确selector-only检查；不放宽原禁止复制、值/无边/同源恢复/源不变断言。首次owner数量错误和race-v1源变化原件保留，不混作最终绿。最终相关6包126/372、race×2三包68/74通过，工程项不膨胀完整交付数。

完整用户网易云ftrace82838676B/559592行，SHA `12349db5bbdc5a4343360d9633c7d6c36f18c8be5f39fb64e78b53c9e345bfe4`，实际1nativequery/0模型再核627.645/194.581ms与当前消息；不是用缩略片段替代，不冒第三例、全语义审计或新生成oracle。Trace投影、原4根因、闭合睡眠/链上业务及自动补齐未降低，相关和唯一全仓保持原合同。

冻结7589/337/17后源码停写；唯一full OS0、源前后true，98包PASS/12无测试、28717顶层PASS/45SKIP、34316子PASS/14SKIP，267102 GoJSON、无FAIL/非JSON/重复/未终态。日志SHA `d987874562fc1789a4a7b212d9099a1513553fcfbbd8a5f78f756d1b55c59e43`。A11/B2真实请求精确唯一配对、未解/歧义0、最大重叠3.708秒，不凭进程barrier推断。

## 5. 留账与发布边界

本代5组剩余：03.2身份解释、缺测/业务关系范围、原生selector拒绝后手写回落、整单元预算/投递、新定点提示自然未触发。前两是已证终稿FAIL；第三仅行为链已证，不能越权断定工具原因。旧不透明预检可信可读性、expected版本不一致及采样/IO/权限/代表/图/更多平台债都挂原ID，下一先做高影响共享缺口，不追充分事实后单答案无限调优。

root无闭项[决定](results/hmc_selector_wire_identity_registration_20261102_v1/sealed-v1/0200-codrax-hmc337-acceptance-v1.json) SHA `4e71b19dca61d261dfb0dc7b2840791d807ef0a564ee0c415bf20997ce5470f5`，489件index SHA `c90ba198b433a18e778ee28700adb822b78e5837fb94f1557b8002dc909469f7`。旧38代15686保原、39代按代累计16175，不冒全局去重或全部旧payload重hash；最新335的508＋失败336的397物理重核，其余链头保留。

只提交7Go源码＋紧邻五份文档并推main，不创建新分支；最终clean/远端/逐blob和原件由[publication](results/hmc_selector_wire_identity_registration_20261102_v1/publication-v1.json)另核。源码交付不签A FAIL或新分支未触发为通过，忽略目录原件不冒已上传。WAL未满足零副作用前生产NO-GO。完整[51项ROI/下一双轨](parallel_selected_summary_hmc_selector_wire_identity_registration_20261102.md)与[统一台账](../docs/design/hmosperf_implementation_tasks_20260917.md)同步。
