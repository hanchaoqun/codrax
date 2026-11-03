# HMC338：可读原生事实共享预览与完整抖音实采验收

2026-11-02本地（终态11-03 UTC），main，基线f3f1215234ab2969d2769e16bd19400712564d63。源码13c91b982ce67c680cc240610bbd283b06dd4edc。

核账79唯一＝28完整＋51开放，状态仍18部分/29待实施/2待验收/1验收中/1持续，原勾选不变。**完整能力新增0；已交付共享子能力1组；剩余本批验收欠项4组。** 两原例smoke PASS，完整人工均FAIL；14.1保持部分，原生正确表不代签模型解释，closure=none。原始失败、未触发与整能力交付分账。

## 1. 双轨、参考发现与实际实现

完整能力轨复验原资源统计二进制，轮换原完整抖音启动题；问题/原件/原检查器不改，不加图/全栈/全2611节点新门槛。共享缺陷轨处理§336已证opaque预检时间×100/帧数错读：已有严格解析字段因不属于scalar语义被可读展示跳过，模型只见编码而自行猜解。下游query正确，不能反证前置上下文充分。

对照参考`core/llm_contract.py`的源字段/manifest所有权、`core/preprocess/context_index.py`的原件导航，吸收“框架与源事实分层、自包含字段、可回查”，而非复制±0.5%容差、文本子串或fingerprint作授权门。复用本项目既有解析器而非复制参考分析器/引入七套诊断策略，是本批泛化方案。

6Go/692增17删（2生产/4测试）：

- 新`tracequery.ProjectTraceEventSourceRecordPreview`覆盖7族：资源事件/源帧、CPU量测区间、frame map/callstack/GPU、NAPI异步点、EBPF区间。资源/CPU沿严格wire owner，其余沿已解析typed字段；fresh marshal不可改源Event/缓存/版本。
- 旧scalar semantics优先；source_record与record_time_role仅内部展示JSONL，不是新工具schema或来源准入。精确零/负/未知时间、NULL/状态/编码保留；未知不借排序0，帧锚不冒GPU执行区间，源帧数/深度不冒完整展开或执行顺序。
- 扫描128KiB/256行、32完整行、整个展示16KiB，先预约教学/尾注/导航后选行；完整对象省略不裁字段/长ID，后面小行仍可进入。显式shown/omitted/scan_limited只说所给preview，不外推完整源。
- 没有新增SQL归档块解码或隐藏文件读取；不修改观测、selector、源准入、根因/配对/CPU权限。无问答原文关键词硬门、自动结论或新硬序列，Trace因果投影、自动补齐、合法计算/重试与链上业务线索不变。

## 2. 公开反例、相关测试与冻结全仓

公开旧renderer RED1顶层/10子，先由旧parser确认canonical fixture有效；资源/CPU零负未知/frame/NAPI/EBPF均缺可读字段。首轮green失败在JSONL fence内混入教学（invalid character s），完整日志保留；冻结前把教学移到fence外并修正整块预算，不放宽原JSON断言。green-v2 PASS，另含非法/新版本/无关prose/原DB归档隔离、UTF-8、整行/32行/共享16KiB/后续小行、fresh bytes和非准入负控。

最终同源相关7包113顶层/198子PASS、2可选SKIP（log SHA2d3d9a960b8f3947919489e5811d1b88c084bf64d0af3eaa419f7ba16cb27578）；race×2三包24/40 PASS、2可选SKIP（9892813598fef67396251fb6c1ecb5b31fba8566b92d059ed1abec828585c555）；原4根因/闭合睡眠等因果守护3包21/141 PASS。实际agent→adapter预检/探索初始消息由本地sentinel校验，0外部模型，不用手造prompt冒真实接缝。

完整用户抖音235422121B/1696481行、SHA93b45bcca5c9e9b415478d81e176b633d70886328c49731b5258c806695de1ef，真实1nativequery/0模型回归原HMC320标量与Extract/Finalize；不是小片段替代或第三模型case。独立整数微秒全源LIFO oracle只给operator，不注入模型：24336闭合target markers，窗内2611，原父669351/self227101us及makeApplication153066/self1078us一致，TID57874不混marker payload10266。未匹配E1条在主窗外单列，不把整源异常藏掉。

CLI正常构建/启动0模型0工具。冻结7594输入/256外部/14仓外快照，SHA e5f150278145e6c31cd29d7793dc9798522b389fc41c7f7b8755e96f29bd0a90后源码/测试/教学/helpers/cases/oracles停写。唯一`go test -json ./... -count=1 -timeout=30m` 06:44:03.086972Z→06:58:29.559380Z，OS0/源前后true；98包PASS/12无测试，28721顶层PASS/46SKIP、34331子PASS/14SKIP，267181行纯JSON，无FAIL/重复/未终态。log SHA d458bb0ded081ff3aaa3f408dc0bb92883fb2f6f5725e68e3b70aa3068846cd5；只读统计不是第二次全仓。

## 3. 恰好两原例、各n1并行

| 原例 | 完整真实入口 | OS | 原smoke | 完整人工 |
|---|---|---|---|---|
| A资源统计 | 构造SQLite53248B→默认共享准备→3原生query | 0 | PASS | FAIL，边界时间/调用角色解释 |
| B抖音启动 | 用户完整235422121B ftrace→7原生query | 0 | PASS | FAIL，完整业务窗/裁窗及self/Running错绑 |

两例实际request/response唯一精确配对A12/B15，歧义0，最大可证重叠12.552秒；不由barrier/进程并存推断。A12/B20工具声明与参数/blob不是可相加的人口。原smoke verdict均明确PASS，不由CLI0推定；Smoke非语义充分条件。详[逐层人工审计](parallel_selected_summary_hmc_native_carrier_preview_douyin_20261102_manual_audit.md)。

### A：自然命中共享预览，原统计仍不能代签正文

原资源case SHAe9be62ae952dcb73cf9caf6cbdbdf08f6010d459105cbbf6a4b267069ff93ff0，capture.data SHAa938230f8ec8bceaafd8038d16058057f11407e8ce1846ee43f4e307f6d42bf5不变。它是完整构造二进制，不冒用户实采。预检实际INIT直接获得10005000000ns/10.005s/end10025000000/ITID1/frame_count3，新copy在分析/探索也出现；emit证据不再重复旧×100/2误读。只签资源载体自然覆盖，其余六族仅确定性，旧§336失败不倒签。

窄[10,10.05)4事件/3组/11帧，768/128/512正确，宽[10,10.06)5/14，额外事件恰10.050000。终稿两次写10.055，释放未知符号/源深度又被断为确定外内层或业务调用方未采；正确native表和尾部限制不豁免。896=768+128合法，512未再错256，不将调用位置线索一概禁掉。HMC14.1仍部分，native_hook_statistic增量/累计/reset协议未证，手工expected的name unavailable与NULL-ID规则版本冲突独立留账。

A一个成文3条实际INIT86852/9927/164358B、均UTF-8；原非UTF-8日志保原。只投3解释单元中的unit0，3069B省略；unit1过排关系、unit2正确10.050边界都未投，不冒直接污染或单一根因。两个模型native selector-only表加自动补齐，6HTML表、无patch/图，主Markdown20351B。首次合法completion不触发新坐标拒绝修补。

### B：认真使用完整用户原件，精确账户够用但成文错绑

原case SHA75041e3155d0a78c6ecdd666ea29fc7a190dbe9df2edfceb5d7da69b62e911d8，原HMC320 manifest不改；历史case的NOT_RUN是旧登记，本代确已首次模型执行。原件235422121B与预览封装235422293B分开，后者多172B头不是文件增长。窗口索引用完整原件，不以preview替代全集。

父669.351/self227.101、makeApplication153.066/self1.078、attachBaseContext151.430/self107.397及主窗Running487.440/Ready8.714/Sleep170.918/D2.245/IO0.034正确。32个已发布主窗账户按原始起止行/名与独立oracle一致；另三窗各32物理成员相符。宽producer拓扑数不等于2611目标marker分母，原题也不要求所有节点显示。

正文中段却使用完整makeApplication至236628.184269、153.066ms，再套用仅到.184158的Running144.906；实际全段Running145.017，表格保裁窗152.955ms，与tail起点.184158形成0.111ms交叠。self107.397ms也被当CPU执行构成，实际self Running103.841，另有Ready/Sleep/D。实际INIT同时明确供给完整/裁窗账户；不是源缺失或容差问题，不可把parent/self/状态相近值混用。

成文3INIT97597/13901/321356B、UTF-8有效，只展示2/6单元、省2526B；未投错误CPU/短子段摘要不冒直接污染，正确phase unit也有省略。selected16views只8完整进prompt，8保全行/13保全producer notes分别记录，不冒采集缺测。无answer patch/图。6模型块中两手写表，连自动native表共28HTML表，Markdown1007301B；21格文本比较差异逐格只是代码/加粗语法，不谎报数值错列，也不据表存在签全表语义。HTML源审不是像素QA。

## 4. 剩余验收缺陷与51项ROI

本代4组分开留账：①精确事件时刻/调用角色解释FAIL；②完整业务窗/查询裁窗、self-wall/CPU-running绑定FAIL；③同源事实重复、整单元/全请求预算和已选view投递；④其它六carrier自然覆盖及图/patch/新坐标修补未触发。本代只读登记未运行，不将§337负控PASS复制为本代结论。挂原共享16.4/01.3/02.4/16.5/01.2/18.4/12.5，不增稳定ID。

优先公开复现可组合的实例＋查询窗＋量尺账户到正文表图绑定，及原生selector受拒后的精确恢复；保原查询总体，不以压缩合并queryID、不靠词门改答案。完整轨轮换已有材料原有限合同/二进制及只读正负终态；证据已足的偶发误述保原，不无限阻塞高ROI交付。旧采样/IO/权限/代表/图、h2/h3/lower-cost-full、native_hook_statistic协议、expected版本债和更多平台仍留账，WAL零副作用前生产NO-GO。

| ROI序 | 项数 | 稳定ID（HMC-前缀） | 下一推进重点 |
|---|---:|---|---|
| P0共享 | 7 | 16.4、01.3、02.4、16.5、01.2、18.4、12.5 | 精确范围/量尺绑定、原生复用修补和完整单元预算 |
| P1现有材料验收 | 6 | 11.2、18.5、03.2、14.1、08.3、18.2 | 按原有限退出条件轮换，不把持续18.5一次通过销账 |
| P2能力复用 | 11 | 04.3、04.5、08.1、08.5、12.2、12.3、17.6、17.7、05.2、10.2、14.2 | 完整业务/量测/格式材料与既有原生路径 |
| P3来源/协议 | 21 | 04.4、04.6、05.3、06.2、06.3、08.6、08.7、08.8、09.1、09.2、09.3、10.3、12.4、12.6、13.1、13.2、13.3、14.3、14.4、14.5、15.1 | 独立协议/材料先行，WAL不可绕零副作用 |
| P4新域 | 6 | 07.1、07.2、07.3、07.4、15.2、15.3 | 真实材料/依赖就绪后交付，不用示例冒完整 |

51ID无重漏见[本代核账快照](results/hmc_native_carrier_preview_douyin_20261102_v1/sealed-v1/0334-codrax-hmc338-census-roi-v1.json)，逐ID原合同仍以[统一任务清单](../docs/design/hmosperf_implementation_tasks_20260917.md)为准。

## 5. 封存与main发布完整性

ROOT：`eval/results/hmc_native_carrier_preview_douyin_20261102_v1`，469件index SHA009dddbf652acf9c7576a55e05e46f7e5acc89d74f59cf261ef2952ea1eedb88，seal receipt SHA31ad8134b1e053f8e37fa987ace530451b342f797cd5a52b7afa6037f9dc53e1。root无闭项决定SHA295012a82265cf3a3162506a6d607ab479ef7f1c932857a5a7680f9d0be4d23b，原失败/首green失败、actual INIT、native JSON、完整提交、最终MD/HTML/进程与全仓均留存；大用户原件只留完整外部引用，不复制入Git。

旧39代16175件各原ROOT不动；加本代40代按代16644件，非全局去重。本批只重hash最新337的489件及更旧链头，不冒所有旧payload重核；336失败仍docs-only无成功publication。providers（含synthetic配置）不读/哈希/复制。

6Go源码提交后，恰好五份批次文档相邻提交/main推送；[publication](results/hmc_native_carrier_preview_douyin_20261102_v1/publication-v1.json)另核逐blob等于冻结输入、原件/hash、唯一full、79ID/28勾、clean main和远端同HEAD。发布完整性不豁免两个人工FAIL或未触发，本地忽略目录的原件也不谎称已经上传。根因/自动补齐/闭合睡眠不放宽；完整能力新增0、共享交付1、51开放。
