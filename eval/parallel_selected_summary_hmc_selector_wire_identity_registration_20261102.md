# HMC337：原生坐标定点修补、严格测试精度与资源/只读负控轮换

2026-11-02本地（终态2026-11-03 UTC），main。基线`a014ec6d59987233f16df26c45b02c70494b7411`是§336失败审计文档提交；本批源码`e068a13cffb696c0979d7aad98cda1ff523082bc`。旧成功源码基线仍为§335，不能把§336文档推送当成候选源码已发布。

79个唯一任务＝28完整＋51开放，勾选不变；开放状态18部分实施/29待实施/2待验收/1验收中/1持续。**完整能力新增0；已交付共享子能力1组；测试精度修复1项另计。** A原资源身份有限及整答未通过，03.2不关闭；B只读缺来源终态负控限定通过，持续18.5不关闭。`closure=none`，不把相关/race/全仓或正确附表代签模型验收。

## 1. 双轨与参考实现

完整能力轨轮换原03.2自然题和既有只读登记负控，不改原问句/数据/检查器，不追加图、完整栈或设备材料门槛。共享缺陷轨承接§335真实丢命名空间反馈：原native completion拒绝只给字段/代码和全部当前pairs，缺少错误提交到当前候选的关联。

参考仓`core/llm_contract.py`以manifest及字段位置约束证据归属，`core/preprocess/context_index.py`提供来源导航/发现，而非把导航当事实权威。吸收精确归属与可操作错误坐标，不复制其文本子串检查、±0.5%数值容差、fingerprint等宽松授权方式。IO参考bucket计数/均值不能直接充当设备精确时间加权深度或压力证明；仍保旧IO解释债。

共享修补只作用于原binder已经拒绝的unknown_id/wrong_view：按精确失败索引列出submitted ID/view、完整current candidates和match_kind。优先相同完整ID，命名空间边界后缀仅建议；多个不同ID、无匹配、其它错误或预算省略时，保原全名册fallback，metadata始终保当前全名册。4KiB/16完整单元，显式omitted/unresolved，不截ID/字段/单个候选列表。合法当前对象仍由模型选择并重新过原source/view合同，不接受别名、不改ID/view/结论，不恢复失效来源或修改前次已受理状态。

没有用户/输出原文关键词硬门，不改变根因排序/链上资格、Trace投影、自动补齐、合法计算/重试。公开原生query→有效completion→缺命名空间/错view拒绝→同坏输入仍拒绝→精确当前坐标成功；私有负控覆盖歧义、整字段预算、namespace边界非任意子串、空ID、多view和无关错误。它是一组共享子能力，不是按入口/字段重复计多个完整任务。

## 2. 先闭全仓失败，不放宽原合同

§336原全仓实际1：HMC279 summary/table/diagram整份JSON查`768`报“复制”。本批另代确定性RED（SHA `85a55892978b6e4be54f76c3edc2479119da0e0afc8040145b127d7979c9a553`）证明该检查器既会误拒合法opaque observation ID/host reasoning metadata，又会漏掉其它值的内联复制。不声称已定位旧336每份失败JSON中具体哪一项含768——旧日志没有输出完整对象。

修复只在测试层：精确native块必须只序列化完整observation_id/view selector与既有typed呈现注释；未知/竞争payload字段、不同数值文本/行/图、selector内联行、错ID/view、缺selector/目标块均拒绝。模型块人口/独立lead保持，所有自动native块也逐一检查；由私有SystemGeneratedKind区分合法host补充，不按ID前缀。原显示值、无因果边、重绑定与源不变断言完整保留。不是生产放宽，也不是按一个数值拟合。

首次新增总块数前提错误，误计合法自动native补充，相关v1/owner诊断失败原样留存；race-v1有source_unchanged=false，不能计有效PASS。最终修正为按私有owner核模型人口；最终v2相关/race/实采/build使用同一源码SHA `0747f526b5e87d53fb15aef76fc1548def727a001f5645123cb5fe51cd375fba`，均在冻结前完成。7Go文件448增6删（3生产/4测试），测试精度工程项不冒第二个完整能力。

## 3. 冻结与唯一全仓

- 最终相关6包126顶层/372子例PASS，2可选SKIP；race×2三包68顶层/74子例PASS。原4候选、闭合睡眠及相邻因果/JSON合同由相关和全仓保护。
- 完整用户原件`/Users/han/opt/donghu/基线数据/105版本/网易云音乐/应用后台销毁后冷启动/3、LOG_ftrace_20261019_17.55.02/ftrace-20261019_17.55.02.ftrace`，82838676B/559592行，SHA `12349db5bbdc5a4343360d9633c7d6c36f18c8be5f39fb64e78b53c9e345bfe4`。实际1次native query/0模型，TID36434、原77708–152774行、[5969.994941,5970.622586)秒，627.645/194.581ms及实际Extract/Finalize交接通过。完整文件不是小片段替代，独立oracle仍HMC332；不冒新模型case或完整语义人工PASS。预冻结v1实采也保留，不将重复定向检查算额外模型评测。
- CLI及test binary在最终源上构建、启动检查无模型调用。冻结7589输入/337外部/17仓外快照，SHA `c2fcde4a53a968b13892eab365029fa5d7516cdd0223f5737305ea260df33210`后源码、测试、教学、helpers、cases、oracles停写。仅生成原始结果/审计及批次文档。
- 唯一`go test -json ./... -count=1 -timeout=30m`，04:27:56.785542Z→04:41:53.847105Z，OS0/源前后true，98包PASS/12无测试、28717顶层PASS/45SKIP、34316子PASS/14SKIP；267102 GoJSON，FAIL/非JSON/重复终态/未终态0。原日志SHA `d987874562fc1789a4a7b212d9099a1513553fcfbbd8a5f78f756d1b55c59e43`。统计只读原日志，不是第二次全仓。

## 4. 恰好两原例、各n1并行

| 范围 | 真实入口 | OS | 原机器/有限合同 | 人工 |
|---|---|---|---|---|
| A资源身份 | 原15steps普通CLI自然题 | 0 | 原smoke FAIL；03.2 FAIL | 完整FAIL |
| B只读缺来源终态 | 原真实planner＋脚本独立恢复＋public_run_tests | 0 | 原harness PASS | 限定负控PASS；非完整模型答案 |
| 全仓 | 本代唯一冻结运行 | 0 | 原Go断言PASS | 不替代A/B语义验收 |

实际日志A11/B2 request/response精确唯一配对、无未解请求/歧义，最大可证重叠3.708秒；不是由两个进程同时存在或barrier推断。A有11工具声明/11参数/11blob，这些是同次调用的材料视角，不能相加；B独立恢复仍同一个case，无第三例追绿。

### A：事实足够，解释仍失败，原生复用恢复另列系统欠项

原case SHA `6faa0ee061f9638da88a1e15a0d9ed275e7e01a55edc358019466b0634adfac9`、完整text2503B SHA `841d772de200860d1a13ddbae65912995fa39e9043438671ac514393c0af5dfb`保持。原3query为resource_stack不可展开、event_search完整10标记、window_stats原5ms。容差查找窗与请求半开窗分别展示，不冒统计分母；无栈不等于无资源活动。

最终有序明细保5资源操作＋5独立计数，signed/hex/大整数、空串/NULL/未解析子类、两计数序列正确；但正文把2^53+1等精确值断言占位/不能读真实量，.003子类8前后矛盾，缺测外推无竞争/调度无问题/硬件压力零，并把共现先后升级一条内存业务生命周期。03.2原合同即失败，不需新加图或栈要求。正确列表及末尾“不能证明响应变慢”不抵销。

CLI0不是机器PASS，原verdict明确FAIL no_primary_text_regex_match，原文件不改。工作中曾把CLI0口误为机器通过，已及时更正，以原verdict为准。A一个成文三条实际INIT 112161/8427/129340B，均UTF-8有效；原日志非UTF-8保字节，不冒provider wire。8解释单元仅0/1展示，省略4126B；未投坏解释不能直接归因成文污染，原生充分证据与已投解释分别审计。

完整answer带原生event_selection table被拒；patch1改summary后因两个summary再拒，这不证明selector已被binder接受。patch2回落手写10行、patch3补member_set受理：初答1次拒绝，patch3次（1拒/2受理）。后继必须公开精确复现ID/view/row/窗/kind/检查顺序及有效修补交接，不自动改kind或放宽原门。最终只有附件元数据表，无原生事件表/图；HTML源审不冒像素QA。新completion坐标提示首次即合法，所以自然NOT_TRIGGERED。

### B：当前补证通过，不倒签缺失来源终态

原`TestNativeRegistrationLiveRestoredMissingTerminalReceipt`未改。真实planner2请求后，既存test_value.ValueTest/test_increment精确登记increment-result；新进程restore并真实新执行，新invocation与source plan/commit、当前登记plan分别保留。1断言/0失败、command_index1与归档数分开。

原receipt passed=true/proof_batch_verified=true/source_terminal_verification=false，overall unverified/missing_terminal_verify_verdict；durable_run_complete只表示到终态。父/子各4原件/1verify引用，同字节7604B报告SHA `3dd8cef38f69df3c75da02f2b21a6b74b9c23e79975cfcb84fec1d833dd301d3`不冒多个执行；真实final/report的ID、path、plan及SHA全匹配当前代。Git/HEAD/源码/测试/config/依赖只读、授权撤销。限定PASS，不冒自然controller/fullCLI/自然源码创作/context-pack耐久或其它框架；新completion提示同样未触发。

## 5. 剩余验收缺陷与ROI

本代5组不合并销账：①03.2大整数/子类解释FAIL；②缺测/状态/业务关系范围FAIL；③原生event_selection受拒后手写回落，工具原因未证、需精确公开复现；④调查/原生整单元预算与投递范围；⑤新定点提示自然未触发。①②证据充分后的误述保原，不无限阻塞③④及其它完整能力。

统一挂原01.3/12.5/16.4/16.5/18.4，不新增稳定ID。§336不透明base64预检时间×100/帧数错误仍是P0可信可读预览缺口；错误未投本轮finalizer不能冒最终误述根因。手工statistics_expected的name unavailable与原NULL-ID→name NULL协议冲突留版本/未消费oracle债，不盲改生产或冻结预期。旧采样/IO/权限覆盖、代表选择、图/patch、h2/h3/lower-cost-full及更多格式/平台仍留原账，WAL零副作用前生产NO-GO。

51项逐ID无重复/无遗漏，按现有实现复用面、用户影响、真实材料和依赖分层排序：

| ROI序 | 项数 | 稳定ID（均HMC-前缀） | 推进重点 |
|---|---:|---|---|
| P0共享 | 7 | 16.4、01.3、02.4、16.5、01.2、18.4、12.5 | 可信事实可读预览、精确原生选择/修补、范围/预算交接 |
| P1现有材料验收 | 6 | 11.2、18.5、03.2、14.1、08.3、18.2 | 原有限合同轮换；18.5持续、失败不提前打勾 |
| P2能力复用 | 11 | 04.3、04.5、08.1、08.5、12.2、12.3、17.6、17.7、05.2、10.2、14.2 | 完整业务/量测/格式材料与既有原生路径 |
| P3来源/协议依赖 | 21 | 04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、09.3、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1 | 先核数据协议和独立oracle；WAL不可绕过零副作用 |
| P4新域 | 6 | 07.1、07.2、07.3、07.4、15.2、15.3 | 具备真实材料后进入，不用示例声明冒完整交付 |

完整原条目、状态及逐ID描述见[统一任务清单](../docs/design/hmosperf_implementation_tasks_20260917.md)和[本代核账快照](results/hmc_selector_wire_identity_registration_20261102_v1/sealed-v1/0259-codrax-hmc337-census-roi-v1.json)。下一双轨首先补共享可信可读转换预览/原生复用恢复，再轮换有材料的原合同/二进制/只读终态；不靠关键词硬门或删合法计算防误述。

## 6. 封存和main发布核验

ROOT：`eval/results/hmc_selector_wire_identity_registration_20261102_v1`。489件index SHA `c90ba198b433a18e778ee28700adb822b78e5837fb94f1557b8002dc909469f7`；sealed-receipt SHA `95304acd1d95efa65e9bce8c5aeb6448604e6cb9a6964304dd3a57016642a08b`。root无闭项决定SHA `4e71b19dca61d261dfb0dc7b2840791d807ef0a564ee0c415bf20997ce5470f5`。失败、非UTF-8日志、实际INIT、原提交/patch、原生JSON、report/final原件和全仓收据均保留；大原件只登记完整外部引用，不复制进Git。

旧38代15686件各在原ROOT保原；加本代39代按代累计16175件，不称全局去重。只物理重核最新335的508和失败336的397件，其余历史链头不冒全payload重hash。§336失败审计推送收据保留、未创建其成功publication。providers（含synthetic配置）不读/哈希/复制。

源码7Go提交后，恰好五份批次文档紧邻提交/main推送；[publication](results/hmc_selector_wire_identity_registration_20261102_v1/publication-v1.json)另核冻结输入在两个commit逐blob一致、原件hash、唯一full、稳定79ID/28勾、clean main与远端同HEAD。该收据只签源码/发布完整性，不能把A FAIL或新提示未触发改成PASS。原始结果保本地忽略目录，不声称全部原件已上传。完整[人工汇总](parallel_selected_summary_hmc_selector_wire_identity_registration_20261102_manual_audit.md)保逐层结论。
