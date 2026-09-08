# r1040 双窗 Trace / TypeScript 实现集人工审计（2026-09-08）

基线 `282d93003a8b` 已推送，clean make 12:43:38Z。从243个case按显式多窗风险、异构read、覆盖老化选择；上一批r1039已覆盖原生C++ write。严格两路并发，timeout1200s，不改case/oracle、不追加单题求绿。

| case | 自动判定 | 人工判定 |
|---|---|---|
| real_trace_e1_dual_window_normalized | PASS，160s（主流程158s） | PARTIAL：核心值正确，模型倍数错误；范围/即时上下文有系统gap |
| sr_ts_workspace_impls | PASS，53s（主流程52s） | 核心清单通过、整体PARTIAL：系统伪绑定终点诱导错误公式 |

## Trace：查询、上下文与答案

工件：`.codrax/output/20260908-054645.682-15354.{md,html,root-causes.json}`；日志：`eval/results/real_trace_e1_dual_window_normalized-20260908-054407/run-1.logs.all.log`。行号按合并日志。

| 指标 | A：34579.472865..34579.475857 | B：34579.475857..34579.505857 |
|---|---:|---:|
| 窗口宽度 | 2.992ms | 30ms |
| running | 0ms / 0% | 3.414ms / 11.38% |
| runnable | 0.014ms / 0.468% | 0.780ms / 2.60% |
| sleep | 2.978ms / 99.532% | 25.806ms / 86.02% |

- 两次模型查询（900/901）遵守原窗与TID59566。JSON `trace-query-result-6431f976.json` / `26871758.json` 中target_window_states与上表一致；B的CPU1/2/5/3合计1.971+.702+.496+.245=3.414ms。A运行从结束边界开始，B尾睡眠按窗剪裁。旧数值oracle未漂移。
- B1618-P2a生产正证：A、B及后来包络查询的三个零D/IO清单独立带范围（1833–1835、1920–1924）。不是500µs邻窗非零反例的live重放，该臂仍靠真实Execute测试。Binder未关联分别1/10，不因窄D/IO零断言所有等待或IO不存在。
- 最终MD41/45同时写“无穷倍”和“20–100倍”。零基数不提供该倍数；应说0%→11.38%或增加11.38个百分点。记模型数值解释观察，不加正文扫描硬门或系统代改结论。自动oracle没有覆盖这句。
- **B1624/P1：结果文本被当原trace再次查询。** 967允许registered raw_ref读审计；973/981/1016/1025却建议trace_query同blob path。builtin.go的原始capture提示未先识别已登记结果，又被结果内系统sched_switch文案触发。首grep转义字符串配fixed_string是模型误用，错误后续建议是独立系统问题。复用当前发布结果角色，统一grep/read_file成功、零匹配、宽结果/截断建议；保留真实附件回拉和权限，不按文件名前缀或整个.codrax特判。
- **B1625/P1：普通window_stats文本漏目标四态主账。** JSON与typed observation已有，完整raw文本只有全局TopN，目标不在榜内时翻页也找不到。trace_query.go:5588的完整formatter仅frame bundle使用，generic头仅等待/S/Binder预览，导致3轮6grep和约88k上下文。应复用同源target账户/CPU清单优先显示，不调大TopN、不从进程账重建线程值。
- **B1626/P1：多请求窗被单包络替代。** analyzer563把A/B填成32.992ms单窗；补采及1820–1822、1833–1835、1917等把包络称用户范围，A/B称补充查询。RuntimeArtifactScopeProfile只容一对端点。三个账户未混值，但范围身份错误，finalizer还据包络减A反推B。需要typed多窗成员域及共享补采/选择/披露，包络不能代表独立请求窗；禁止原文数字扫描补硬身份。待设计施工，不以最终模型碰巧答对销账。
- 本题是有限CPU状态比较，无帧因果/根因诉求，不强制全量投影。首轮两块接受，无JSON修补/旧稿恢复；必选根因旁路131字节，trace_root_cause_contract_not_active空数组合理。上下文44%，无工具不可用/剪枝/活跃流短时降级。

## TypeScript：集合正确，系统绑定解释错误

工件：`.codrax/output/20260908-054458.880-15358.{md,html}`；日志：`eval/results/sr_ts_workspace_impls-20260908-054407/run-1.logs.all.log`。

- 真值：retry.ts:3接口、:11 ExponentialBackoff、:30 FixedDelay；:47 JitterHelper只提供apply，非RetryPolicy。最终成员、路径、排除结论正确；不要求图。表由模型2075主动提交，系统只提供两成员证据骨架及引用修复，没有代写成员。
- **B1627/P1：嵌套调用被升级为确定性排他绑定/答案终点。** 源:51是 `delayMs + Math.floor(Math.random() * this.spreadMs)`，1011–1066已完整读入；系统1163/1182生成 `JitterHelper.apply binds ONLY Math.random() * this.spreadMs`，1688–1691要求当确定性主依据/不得反驳/ANSWER TERMINAL，1720/1849标verified/independently_proven。模型2075照此发布错公式，不能归模型独错。
- explorer.go:19925–19993按首括号取实参、大写token+call opener猜构造器，20043–20058按唯一命中升binds ONLY。泛化修向是语法片段、绑定、排他和终点证明分层，完整表达式/调用位置/闭合集各守资格；不能加Math黑名单或把一个调用等同唯一绑定。需跨语言嵌套工具调用、构造/注册正例及复合return反例，保留合法关系能力。
- JSON审计：2080–2085缺summary（2017已教）后2116–2128补齐接受；2165–2168模型选未发布add_facet_id，教学明确schema发布才用，否则replace_blocks；该表无row evidence_ids不满足原子分支。未证同一声明必带/必拒合同。最终保留已接受答案；2174–2175是修订回合终止，不是活跃流超时。
- P2展示：模型漏columns且label和首cell重复，兼容渲染给默认“项目/列2/列3/列4”；summary迟置且重复，implements称继承不准确。不扫描正文自动删列/代写。上下文29%，read1/repo_map1，无工具不可用/剪枝。

## 排队

本轮审计先单独推送；B1624以精确结果角色做独立小批，B1627事实强度及B1625目标主账排高优先级，B1626多窗单列设计。B1620/B1622/B1623/B1616b/B1561仍开放。不把机器2/2当人审2/2；模型正文/图/根因选择保留，根因仅已证链上，背景仅支持，不改历史工件或oracle。
