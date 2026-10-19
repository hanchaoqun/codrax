# HMC260 双例人工审计

日期2026-10-18，正式进程时间使用UTC 2026-10-19。两例各一次并行，代码/问题/原件冻结后未改；没有第三例。原机器判定2/2保留，完整人工验收0/2，不能只看exit 0销账。

| 范围 | A：真实sample-only自然CLI | B：只读登记与独立恢复 |
|---|---|---|
| 已证子能力 | 两级search-only原件→完整准备→原生查询PASS | 3个精确报告原件/3个attempt、只读恢复新执行PASS |
| 完整人工结果 | FAIL：采样状态/耗时误述 | FAIL：最终交付载荷未保留，证据不足 |
| 仍未验收 | 正文语义、同源重复；其它实机格式/平台 | 真实WriteFinalReport及最终交付链接/说明 |

## A：原件完整，语义失败继续留账

原件30,062,585字节，SHA `da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`。自然CLI实跑时文件0400、两级目录0100；两例结束后才核身份并恢复自有目录权限。完整12000记录，指定窗1393，目标tid1609/tgid1514为164条/权重259；内核未符号化IP82/174、OnDataEvent_82/85，权重占比67.18%/32.82%，次数各50%。数据没有被40条预览替代，记录CPU0/1保留。

四次模型查询加两次自动补齐全部走真实完整材料，保显式窗和精确线程。首份sampling_tree包含两个完整节点，每节点82成员、预览4/省略78；第二份带合法事件过滤的树为unavailable而非零样本，统计仍164/259。独立集合与预览完整性未被统一抹成缺测。

但primary/principal、MD和HTML正文都说“两个热点均为on-CPU执行符号”“采样只能证明线程在运行”，并把174/259写成“实际耗时占比”。原生sample_kind为unknown，CPU已知只授权记录坐标，权重不是耗时；后附正确原生表不能抵销主答案错误。业务符号被扩写成RPC/锁/IPC机制，虽属条件排查而非已登记根因，仍缺对应机制证据。继续归18.3/16.4，不以本次没有再写4ms/196ms就关闭前批同类FAIL。

相同来源、窗、线程和164/259的质量段重复显示为采样总体1/2/3；没有真的相加为492，不能夸报数值翻倍，但共享展示噪音已证。实际finalizer拿到完整树、unknown、权重非时间边界；调查interpretation/reason又重复交入onCPU、全部CPU0和连续执行的错误解释。最终CPU范围已纠正为0/1，不把仅中间错误说成最终CPU错配。max_context_tokens_est=112689，finalizer用户消息216865字符；事实到场，不能归因原件缺失或未经证明的纯模型波动。

原生文本因果图保200ms请求窗、数据盲区与“不参与根因排序”，没有时长或确定链上主因；root-causes为空/unavailable。无模型Mermaid图，不冒签浏览器像素布局验收。投影/自动补齐本例保护有效，不因自由正文失败删除这些能力。

独立审计原件：`verification-receipts-v1/codrax-hmc260-sample-manual-audit-v1.md`，SHA `569fa4c69dc7265cb5cf5c1aacf546c784effc348cc65484da2876b928544b54`。主控另查primary/surfaces及原生结果并同意结论。公开结果MD SHA `15c37aade0769a2f3cca673dccf47f28785498be10c2dc8420e2c590b0989fd6`，HTML SHA `426b841e036dd321db0d98e2bd34db8eaaf5dbe1d7b87fe7153b12a63c0cb05c`。

## B：原件与恢复PASS，最后交付材料缺失

真实parent PID39715、planner及独立child PID44847运行，初始源码/合同/Git和历史执行由operator准备，公共RunTests执行而非verifier模型；不是从零自然CLI。planner完整读源码/测试后提交changes=0的现有测试登记，child恢复后产生新执行，再完成工作流；源码/测试hash、HEAD和git diff/status不变。

归档5个报告文件＝3个不可变原件＋2个current镜像；全部3个verify attempt精确绑定相应原件。独立核payload SHA/文件名/PlanID/canonical一致，2个有typed结果快照的attempt计数与引用完全匹配，另1个缺快照不补零。source同plan的第2/3次验证全文同时保留；current依归属batch选择第2次，而不是按mtime取最新第3次。

实际native测试执行4次＝operator初始1＋自然工作流3；子进程当前新执行1次，独立断言1个。语法预检、镜像和额外审计拷贝不增次数。旧unverified/verification_proof_incomplete尝试保原，后续完成另记superseded_by_verified_proof_followup，不把旧报告改写成新成功。parent中间“必须修改测试设施”误述被typed控制器纠正，不妨碍这组已证只读恢复子能力。

真实三份payload SHA分别为 `b09c4c158d9a09eb685be6f79e90fb3b8a4d7b95eaa84a9b35a22afe06dcd8d6`、`0abd5f148cfa977620497e135a015783f4a41c8984c18c298514f314275901e0`、`900906916c4f3fb2a533b08b1fe304c1f571ba853571267e5ca56c4640e0183d`。完整索引与invocation保本地，不用文件数推执行数。

未通过项是归档证据：名为`final-report.json`的文件实际是ChangeReport，helper仅保存`.report.json`；真实WriteFinalReport `.final.json`及其临时父目录已清理。child日志424行只能证明写入曾发生，没有最后`verification.report_id`、`artifacts.report_path`或成文载荷，不能以当前代码或重建文件补签。此为证据留存缺口，不是已证生产错链；定向最终投影回归通过也不等于本轮真实说明已验。

独立审计原件：`verification-receipts-v1/codrax-hmc260-registration-manual-audit-v1.md`，SHA `a2cce709ecc6ded2efa22e82ec8455d200c454a8f13e216606a32e48b147eb3b`。另一审计者独立核路径已删除，定位报告SHA `b3fd8693dac4c17d9bf5b3df58ce2a65035d7d3f1f8b06075a820fff57e2e863`；主控核报告/attempt索引和类型一致。

## 统一记账与下一顺序

完整能力新增0；生产子能力1组（不可变验证报告、精确恢复/选择/交付接缝），已实现二进制入口新增真实采集验收1组。当前剩余验收欠项2组，均含于55开放、不另加总：①采样typed状态/量尺与解释重复，含同源展示；②真实最终说明/引用产物留存与复验。旧图、自然入口及更多格式/平台欠项不删除。79唯一＝24实现已交付＋55开放。

下一轮先以统一typed工件manifest补齐父/恢复子在清理前的ChangeReport/WriteFinalReport原字节、原始/归档路径及精确链接目标；再以来源代次、查询总体、event/unit与对象的同源事实减少重复展示和未验证摘要竞争。不扫问答关键词硬门、不代写模型根因、不堆更多重复prompt；继续轮换二进制剩余真实格式/外部工具/原生平台，而非无限重跑本题求绿。

正式冻结、进程收据、全仓与提交以[批摘要](parallel_selected_summary_hmc_report_history_binary_20261018.md)及[统一账本§260](../docs/design/hmosperf_capability_gap_audit_20260917.md)为准；原失败不改写。
