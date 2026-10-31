# HMC325：原生留存与完整业务账户交接——失败封存，未独立发布

批名日期为本地2026-10-30；实际运行在2026-10-31 UTC。持续main，发布基线`a8846881aaa1ba6cea42aeaad7dd86988165c553`。本代实现留在main工作树等待下一代修复，不把失败代码当验收通过推送。冻结后源、测试、注释、helper、输入停写，所有失败先封存后再推进。

后继状态注记（§327）：§327唯一full已PASS，修复后的34文件代码在main提交776c8683940a777ec99cfac94bc51ff2e5f42afa，交付范围仅3组有界子能力；该代552原件已封存，代码/文档相邻两笔与main远端由[§327 publication](results/hmc_native_publication_compat_20261030_v1/publication-v1.json)确认。此事不改变本报告的§325全仓/两例FAIL、未独立发布和821件原始封存，不补§325 publication，也不把§327数据写回旧代。当前总账仍79/27/52，完整能力新增0；后继状态见[§327汇总](parallel_selected_summary_hmc_native_publication_compat_20261030.md)。

## 计数与双轨

79唯一稳定项＝27已交付＋52开放（18部分、29待实施、3待验收、1验收中、1持续）。本批完整能力交付0、子能力交付0，closure=none。两个候选子能力有确定性/实际输入正向证据，不升为交付：原生整查询独立于对话历史留存；唯一精确查询owner/完整端点账户在既有展示预算内优先。代表完整能力仍失败，旧IO/采样/权限/图未触发继续留账。

优先顺序：显式证据生命周期回归与整份请求预算/同源复用 → 代表规格和已有真实材料的二进制/只读登记轮换 → 其余业务、IO/资源/图形、硬件与新域、写回/建议。稳定ID完整名册见[封存census](results/hmc_donghu_representative_native_handoff_20261030_v1/sealed-v1/0524-codrax-hmc325-census-plan-v1.md)。WAL未满足零副作用前不接生产。

## 对照与实现边界

实际阅读参考`core/preprocess/marker_ops.py`的完整node_map/roots、itid身份、节点状态交集、map身份绑定缓存；同源完整实例先于显示选择是可复用设计。没有复制缺值零默认、模糊同名关系或跨层相加。参考只读，无凭据读取/hash/copy；详情[参考记录](results/hmc_donghu_representative_native_handoff_20261030_v1/sealed-v1/0617-codrax-hmc325-reference-findings-v1.md)。

候选原生核心128查询/16MiB encoded边界，物理版本/准备代/真实查询签名分开；同源完整发布恢复Agent/Bus/fork消费者，留存不增因果或完成权限。真实公开压力47条→40条、1324758B→1041080B，完整self3.938从普通历史丢失而窄窗0仍在，原RED保留；没有改低上限/灌水summary/绕生产入口。该回归证明问题类，不倒签§323精确删除时点。完整账户导航不扩原16+4/128行合同。

## 验证与全仓失败

- 最终目标测试带真实启动/OS终态收据：3包、41顶层/105子例PASS；race2包、3顶层/5子例PASS。
- 早期agent目标测试仅重定向JSON，原intent和部分OS终态缺失；回顾索引如实记缺，不伪造历史收据。首轮容量夹具`/var`对比原生`/private/var`错误已纠正精确源路径，原失败保留。
- 冻结7545仓库输入/181外部/26仓外快照（24源+2二进制）。manifest SHA `060bfe9501d2733ccfefa84214c4276b663e503dbeaf7ce5d29c0bacb7780379`。CLI和operator各编译一次。
- 唯一全仓PID7838实际exit1，源前后不变：96包PASS/2FAIL/12无测试；28621顶层PASS/7FAIL/43SKIP；34030子例PASS/38FAIL/14SKIP；265840 JSON行，0非JSON/重复/未终态。见[完整统计](results/hmc_donghu_representative_native_handoff_20261030_v1/sealed-v1/0564-codrax-hmc325-full-stats-v1.json)。

7个顶层失败是copylocks审计及6组原有恢复/来源合同。3个新测试复制AgentContext的mutex；旧测试证明清空、物理源替换、去Replay/Stats和失败查询仍从旧池恢复。不能说旧断言过时、删除断言或放宽边界。下一代明确区分权威替换与历史压缩，并防旧fork复活撤销内容。

## 两例并行真实评测

| 原例 | 过程 | 原机器判定 | 人工 | 完整能力 |
|---|---|---|---|---|
| A：105版元宝activityStart完整实例 | exit0，15steps/n1 | smoke FAIL，无答案 | FAIL | 0 |
| B：按线程/窗口每组两份代表 | exit0，50steps/n1 | 原operator审计exit1 | FAIL | 0 |

A使用63659046B原始真实ftrace，SHA `be59c0648ac7e1d8a2d117f5dc36efa0c92b5f3686671719ada435c16713a20c`，原case SHA `c8920375d1b0cf7d1389e0598277fad84138f4e9965cd44e349a35be41db12e9`。29唯一原生JSON、6实际INIT、0HTML。四完整self6.770/21.605/3.938/83.143已到INIT，但两次provider2013上下文超限，未提交最终答案。消息和工具schema已合并统计却HARD后仍发请求；不是未计schema，第二轮也不能称完全同字节。原件读取三次错误路径不能误诊为合法来源误拒。

B四完整SQLite拷贝来自两份原夹具，不冒四独立设备。自然选择8计划/2组/4选/4未选已触发，但view错误；一次审计精确使用同次status返回的index/snapshot，未猜latest文件、未重跑operator。12原生JSON、3INIT、1HTML；表格事实完整，正文混用查询次数/地址/协议含义/栈关系，人工失败。后续附加resource查询不能替换已选计划规格。

启动器最初仅在预建空logs检查处失败，未spawn A/B。原记录保留，只把已核实的空目录可恢复移位后运行未改冻结pair，不是第二轮模型评测。原失败见[启动恢复说明](results/hmc_donghu_representative_native_handoff_20261030_v1/sealed-v1/0608-codrax-hmc325-prelaunch-recovery-v1.md)。A10/B27唯一完成请求配对、28歧义另留，实际重叠14.512秒；不称配对数为全部请求数。[配对原件](results/hmc_donghu_representative_native_handoff_20261030_v1/sealed-v1/0514-codrax-hmc325-actual-request-overlap-v1.json)。

## 封存与后继

821原件已封存，[index](results/hmc_donghu_representative_native_handoff_20261030_v1/sealed-index-v1.json) SHA `1f2120a4ca1c95188dcf34f4ed3beecbcf05f7dd4f8342b5ed565bbab30472b3`。此前26档9316件/606快照身份已核验，源快照在仓外/文本扩展名，不引回Go编译树。大型原件不以预览替代；本地归档不是Git上传承诺。

详见[本批人工审计](parallel_selected_summary_hmc_donghu_representative_native_handoff_20261030_manual_audit.md)。下一代可以修已证系统缺口，但不得改本代判定、重复本代模型追绿或宣称所有52项完成。只读登记/sample-only及100版抖音未运行，旧解释越权有限留账。
