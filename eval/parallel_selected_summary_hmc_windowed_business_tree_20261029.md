# HMC318：窗口化来源覆盖与业务树双轨批汇总

日期：2026-10-30 UTC / 2026-10-29本地。基线10237c5c45cbcdcc6505f6139f99f5206d4bea5b；按用户指令持续main。

## 交付与核账

79唯一稳定任务＝26已交付＋53开放，18类；18部分／29待实施／3待验收／2验收中／1持续。逐类开放数2/1/1/5/2/2/4/6/3/2/1/5/3/5/3/2/2/4。

- 完整能力新增：0；04.2、08.3不销账。
- 有界子能力交付：1组，真实文件完整扫描的同代每源事件边界，经账本/锚缓存复用到窗口化状态计量。
- 本批成文剩余：3组，A合法差值被禁、B自身计量解释、图业务参与者桥；新Windowed自然覆盖另为NOT_TRIGGERED。旧任务和未触发不删除。

两例机器2/2。A原题统计子项限定PASS，但整答语义有欠项；B原完整合同FAIL。不能记整批完整人工通过，也不能用正确原生表抵消正文错误。详见[人工审计](parallel_selected_summary_hmc_windowed_business_tree_20261029_manual_audit.md)。

## 实现与测试

参考thread_sched_ops.py先trace_range再求查询交集，marker_ops.py按同线程parent_id/self计量。本项目沿已有每源私有账本和锚缓存记录物理身份、完整扫描资格、hasEvent及最大事件时间；完整EOF、取消和文件同代校验后才发布。普通流/前缀/顺序证明不升格；混合typed、非法身份头、多源和非identity时钟不借合并最大值。保查询窗、真实关闭、合法零时间、31/35区分和原4候选/2ms小计/47背景；无问答关键词门。

先公开RED再修复。中间compile/夹具前提失败、旧Size/顺序保护失败、分配57.69超过原55上限全部留存；复用严格header submatch后54.69，旧断言不改。新9顶层测试，最终：

| 验证 | 真实结果 |
| --- | --- |
| protections-v3 | 92顶层/172子例PASS，零FAIL/SKIP |
| downstream-v2 | 239顶层/351子例、3包PASS，零FAIL/SKIP |
| race-v1 | 90顶层/300子例、3包PASS，零FAIL/SKIP |
| 唯一全仓 | exit0，98包PASS/12无测试；28569顶层/33909子例PASS；原43/14 SKIP；零FAIL/重复终态/未终态 |

全仓命令go test -json ./... -count=1 -timeout=30m；PID65426，04:43:15.404622Z→04:54:31.120848Z。原日志SHA2b6bc3b4773a9ee34f5c35cf52080425b42936fff8e98c30ced02c93d43f5734。仅一次标准CLI构建，SHA5ae6cee9726cda4c4e36be328675772f364774ac2cb822910089949e8a2a141a；冻结7518仓库/206外部/18仓外快照，SHAe30ed9abe0cd75dfd296a3dad80184d8a35b48a5420d75d72992d4369c98f2a7，之后源/测试/注释/helper/输入停写。原opt-in跳过及历史失败不签通过。

## 恰好两例真实评测

| 侧 | 输入与范围 | 终态/机器 | 人工与覆盖 |
| --- | --- | --- | --- |
| A | 新自然两窗状态题；trace_query_windowed_artifact_coverage；既有2312B文本Trace | shell65427 exit0，04:45:35.780926Z；PASS | 四target/window统计正确；不同窗时长不可相减文案留账；新Windowed NOT_TRIGGERED |
| B | 原trace_query_business_tree题/EXPECT；5–5.012s；既有1481B文本Trace | shell65428 exit0，04:46:09.081769Z；PASS | 原数值/身份/分段正确，self重叠解释FAIL；业务图身份桥冲突 |

各15步/n1，只两例；B专家显式约束不替代默认易用性证明。两原件是authored导出文本，不冒新增设备实采或二进制材料；oracle仅在操作员目录。实际请求各4唯一配对，A另7请求/6响应、B另11/10各一日志歧义组；最大可证交集18.820秒。日志欠响应不等于进程未终止；不猜配、无第三例追绿。

A八次真实trace_query、四去重payload；B八次模型查询加一次确定性补采、七去重payload。调用数、内容数、可唯一配对模型请求数分别计算。A完整提交一次受理；B两次完整提交拒绝，唯一boundary patch受理。两侧实际index_windowed=false；时间窗请求不等于窗口化索引分支被执行。

## 后续ROI与材料

1. 12.5/16.4：业务实例→请求主体精确桥及inclusive/self教学适用域。保来源、查询、线程、实例，不按同名认领背景，不放宽已证关系门。
2. 16.4/18.2：原生受信查询端点→共享标题；合法集合差/互斥self求和与未知范围分开。新Windowed自然覆盖保持独立验收债。
3. 完整能力槽轮换18.5只读负控、11.2代表规格；04.2/08.3/03.2充分事实下误述留账，不无限同题优化。
4. 17.6按新增真实格式/工具/宿主材料再扩二进制；WAL未零副作用前不接生产。

本批本地留存根：[results](results/hmc_windowed_business_tree_20261029_v1/)。A/B目录唯一run均为20261029-214318。388件[索引](results/hmc_windowed_business_tree_20261029_v1/sealed-index-v1.json)按来源路径+SHA保真，索引SHA8204f84fee94124a06d18e9895f48ec41496fed2e577dcd402f58e5af75b3f92；[封存收据](results/hmc_windowed_business_tree_20261029_v1/sealed-receipt-v1.json)含真实终态/全仓统计。旧19档5897件/410来源快照身份保真，18历史污染是子集；不把仓外快照恢复为仓内源码。raw results为本地留存，不冒称随Git上传。原非法UTF8、失败、完整INIT/参数/payload/MD/HTML及两独立审计保留；HTML源审不冒像素QA，凭据不读/哈希/复制。

实现提交cc974a5d7dad19413febb2a277ea36c38ced881d，随后相邻文档提交并普通推送main；最终精确文档SHA、远端相等、清洁工作区及全部冻结字节检查写入[publication](results/hmc_windowed_business_tree_20261029_v1/publication-v1.json)。它只证明发布完整性，closure=none，不倒签人工FAIL或未触发。
