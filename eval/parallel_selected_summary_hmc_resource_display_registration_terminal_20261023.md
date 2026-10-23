# HMC287：原件范围同源展示与只读登记缺来源终态

基线 `fea0c262e48a26167865b38f7fbaddeda446c77a`。独立按唯一任务 ID 重核：79 项、0 重复，25 项完整实现已交付、54 项开放，分属 18 类；31 待实施、17 部分实施、3 待验收、2 验收中、1 持续执行。本批完整能力新增 **0**；共享生产子能力 **2 组**。两组剩余整答/上下文验收欠项仍归既有任务，不新增父项或误减 54。

## 两条修复轨道

**原件范围和原查询总体分离。** 上批资源宽查询的窗内成员投影已交付，不能继续将其写成原生事实未送达。本批查明原件 `trace_range` 的完整起止声明没有进入受信元数据，50ms 转换事件包络与 100ms 原件声明容易混用。现在严格读取单行、非负 SQLite INTEGER 纳秒端点，分别保留起点、终点和合法端点差；缺失、NULL、非法存储类、负数、反序、多行不猜值。仅声明背景，不改变转换事件、实际查询窗口、分母、端点包含规则或完整性。

同一受信准备记录进入单源原生摘要/表、双侧对比和采集状态目录，正文、表及无边事实图共用绑定；JSON 往返、旧来源、跨源替换不能恢复私有来源凭证。原查询总体的说明改成“保留原查询范围和分母，以实际筛选条件为准”，既不把宽查询总数伪作投影总数，也不错误否定本就精确匹配请求的合法查询。原生补齐、Trace 因果投影、合法计算、链上业务线索保留，无问答原文关键词硬门。

**当前只读证明与来源完成分离。** 新分类只回答当前登记是否新增源码归属义务，不把它等同于“来源已验证”。精确 run/repo/plan/delivery/apply 身份齐全、来源确已应用且从未开始 controller verify 的独立登记可以运行本轮证明；来源整体仍缺终态。已有失败、未完成验证、悬空报告、显式非 verified 终态、新修改或额外 owner scope 仍走原保守边界。原严格 `WorkflowRegistrationReusesVerifiedSource` 不放宽；新 scope 在 Normalize 之前判定，避免显式空终态被归一化抹去。当前 proof verified 不升级整个 workflow。

参考仓核读 `core/query_engine.py` 的完整执行参数/总体与预览分离、`core/llm_contract.py` 的成员字段值校验、`config/indicators/memory/heap.yaml` 的资源分组/栈帧展开，以及 `core/hiperf_converter.py:33`、`core/extensions/log_extender.py:135` 同时读取原起止范围的设计。采用来源分层，不复制 `LIMIT 1` 歧义、固定层数推业务调用者或文本关键词门。五份参考原件纳入冻结。

## 确定性验证与真实执行

公开准备→查询→实际 Finalizer 的源范围反例先 RED，修后覆盖严格端点状态、两个原件独立范围、单源/双侧/目录、JSON 失凭证及三展示面无权限升格。只读恢复覆盖真实 apply、无来源 verify 的新进程，以及 17 个精确边界；旧 strict helper 作为 overlay 反事实仍复现不当当前 owner gate。最初“删 Completion 但保留成功 verify”的测试不是纯缺验证：既有 Normalize 合法补出 `attempts_backfill`，该初始失败保原，不能充当目标分支证据。

定向三包、竞态四包及独立 orchestrator 竞态通过。中间旧资源表数量期待和 HTML 数字实体匹配失败均保留；原 12 个资源视图/4 个默认视图等强断言未放宽，而把新增元数据独立验收。

首次 CLI 构建与最后一处源码注释修改时间重叠，独立复建所得 SHA 不同。该问题在零模型阶段发现：v1 冻结/构建原件保留，改用稳定复建 CLI 创建 v2，**不是重复运行 A**。正式冻结 7338 个输入，SHA `1f5292afcce671151f167b1514324b55aeb1ec20eb2663b819a73d34df95afe8`；实际 CLI SHA `5c7056f2bb645b4ba6603d0e969a2f98d4b64a89525aeb0e76ec6cc01f2c84c9`，恢复测试二进制 SHA `03a12d3b539ab3db937f1ba8857b70b67070ff30300ac23b68980a1006e373d5`。

两例使用共同就绪屏障，恰好各一次。实际模型请求也重叠：A 首请求本地 02:04:29.071–37.644，B 两轮 02:04:26.944–30.225、30.227–34.453。不是只凭进程启动声称并行；本批没有第三例模型。

| 例 | 真实执行（UTC 2026-10-23） | 机器 / 人工 |
| --- | --- | --- |
| A 采集总体及两线程窗口资源 | wrapper PID20320，实际 CLI20373；09:04:24.385507–09:07:51.803335，exit0 | 机器 PASS；完整答案人工 FAIL |
| B 只读登记缺来源终态恢复 | 父20322、独立子21013；09:04:24.387987–09:04:35.086062，exit0 | 机器 PASS；限定负向目标人工 PASS，不签全部上下文统一 |

A 是既有构造 SQLite，自然题没有加入系统看护约束，不是实机或新二进制格式。实际既有精确 [10,10.05) 查询 4事件/11帧，也有 [0,60) 宽查询 5/14。主表用精确查询；完整 MD/HTML 自动补齐宽查询窗内 4 个成员并保原总体5、窗外排除1，不能把两条发布路径混写。线程101申请2/768bytes、释放1/512bytes；202申请1/128bytes、释放0，右界正确，完整原生表保11帧。

原件 100ms 的三字段已进入实际 Finalizer 和附加 MD/HTML，终稿没有再把原件时长写为50ms。仍有正文“负载完全在一条线程”与202事件矛盾、原深度升为调用上下层/业务发起关系，完整答案 FAIL；同成员多查询重复展示另记噪音债。实际投递调查单元为3/4，末单元整条省略；原生说明另有关系边界，不能声称全部边界缺失或仅凭共现断言唯一诱因。无实际图，图关系/时序与像素 QA 不代签。

B 是真实 planner 两轮完整读取→登记、脚本 verify/finish controller、真实原生执行与独立进程恢复，**不是完整 CLI/自然 controller**。真实来源 apply 两坐标齐全，来源快照一直无 verify attempt/ref/completion；本轮 proof verified、整体 `unverified / missing_terminal_verify_verdict`。旧与新 invocation 不同，源码/测试/HEAD只读，父子每份4原件＋1 verify引用逐 bytes/SHA 通过。1条断言通过、2条命令不是2条独立测试；历史测试不计本轮。

B 仍有 planner INIT 的旧 owner 提示，以及最终 loop 中 `observed_only/recommended_action=localize` 与当前证明完成冲突；此外本 fixture 明示 `no_durable_store`、context pack durability 未验证。限定通过不掩盖这些留项，不回签 HMC286 的缺 apply/缺终态失败。

## 全仓失败留存与后继验证

正式 v2 全仓发现旧默认表/selector 数量断言尚未把独立原件元数据分列，新增诊断 coverage 未进入固定 accounting 收据，同时新动态 `Metadata[field.Name]` 写法不符合共享结构审计可解析要求。ZIP 测试裸 `ts-` 子串误击合法 `end_ts-start_ts`，另须保真实私有路径泄露负控。所有 v2 原件、日志、真实评测及人工失败先封存，再进行有界后继修正；不重跑模型求绿。

后继 v3 仅一处生产等价改写：将诊断元数据的动态键换成三个显式键，便于既有结构审计精确解析；不增加门豁免。13组完整范围收据及真实转换输出的前后14项逐字节一致，转换结果40392bytes、SHA `5ef91177791e39ceb123d909608bd7efbee71d02d17443775422aff597c2a074`。其余为17个测试文件的新增或迁移：新增原件metadata独立逐字段验收，不计入原量测行数；旧来源、JSON失凭证、窗口外成员、原生关系与因果负控保留。固定accounting只在精确验证新增coverage后分离它，旧输出和旧收据哈希不变；ZIP负控改验本次真实私有路径身份，保9个正反例，不再用裸`ts-`误击合法公式。

agent/tool/hitraceconv相关测试及各自race通过，稳定重建通过。中间targeted-v3的旧行号失败、Ruby不支持`filter_map`导致的equivalence-v3脚本失败均保原，修订后的targeted-v4/equivalence-v4独立通过。最终冻结7340输入，SHA `385a6489cce444bfc8689462bc8035224d600b8e2ace23e0c1c814ae8b2331a8`；末版CLI SHA `c79bcdf3bc579eac3e836d3694b7bce9fa1aca4d0537ff2bfb98c5308055ce2e`。实际两例仍是上面的v2二进制结果，**没有在v3重跑模型**，不能把等价和确定性回归写成第二次真实答案验收。

最终v3全仓`go test -json ./... -count=1 -timeout=30m`，PID46293，UTC2026-10-23 09:30:36.896858–09:45:33.638242，正式exit0：**97包PASS、12无测试、零FAIL及非JSON行**。测试事件61193 PASS/55 SKIP含子测试，不能冒充61193个独立场景。运行前后7340输入及实际末版CLI均匹配冻结；日志SHA `46c7e4c56a1908c7fbafbfb44cf6c00f94dc4673745c6b240e9f32eabdb15da0`。这是完整后继运行，不拼接首轮或定向结果。

正式v2原件173份先封存，索引SHA `d626125ddcba7246176d1628f6eca17571f2aa6123e3dc816bbec5bd445f4b04`；v3后继52份另封，索引SHA `dacc0fa498cf447d6ce313e2047b6093bbeebaea7af12d15b87810cc11990350`。合计225份逐文件核验，原失败和两例完整输出不覆盖。目录为`eval/results/hmc_resource_display_registration_terminal_20261023_v2`下`sealed-v2`/`sealed-v3`，索引分别为`artifact-sha256-v2.json`/`artifact-sha256-v3.json`；大原件本地留存，不伪称已上传Git。最终远端/clean/两笔提交树与冻结输入的一致性由同目录`publication-v1.json`发布凭证单独核验，不修改封存索引。

## 下一 ROI 与账务边界

1. 下一批完整能力轨优先 11.2：代表选择原矩阵及真实选择/未选名册/执行状态。现有选择算法已具备，先贯通自然工作流而非重写。17.6 有新真实格式、可执行工具、原生宿主和 oracle 时提级，不重复旧 ZIP/gzip 充完整能力。
2. 同批高影响缺陷轨 16.4/18.4：统一原生事实与调查/最终解释的范围、关系资格，减少同成员重复发布的噪音。优先共享 typed 事实/解释交接，不为一个栈符号或正文句子加硬门。
3. 18.4/18.5 当前只读 scope 贯通 planner 与最终 loop 摘要继续登记，按影响面轮换。此次旧提示未阻断真实执行或误闭合来源，不无限挡住前两项；自然 controller/完整 CLI 和生产持久化仍须独立验收。
4. 旧 IO/采样解释 FAIL、权限分支自然覆盖继续留账；14.1 statistic 协议、05.2/05.3 校准仍欠材料。活跃 WAL 未满足零副作用，生产 NO-GO，本批无接线。

11.2下一入口已只读复核，未实施或另跑模型：复用`eval/trace_catalog_representatives`的自然题、新目录和独立只读SQLite oracle，不预置计划或指定选中文件。原8目标/2组/4代表/4未选矩阵、真实同次selection/status与逐项查询必须保留；未触发仍记NOT_TRIGGERED，合法额外探索导致固定矩阵不满足也如实记录，不手工补选或删尝试求绿。参考`core/batch/closure/select.py`只对既有成员稳定排序并保预算/未选边界，不负责猜查询视图或把未选写正常；本项目当前精确查询规格选择已具备，不重写算法。下批若仍未触发，保原证后轮换其它具备材料的能力槽，不追第三例。

完整审计见[本批人工审计](parallel_selected_summary_hmc_resource_display_registration_terminal_20261023_manual_audit.md)。代码`fd67e3a5c4f851d509a4b7f3cf7a60041f1e4663`已普通推送；本文与两个统一账本、人工审计另笔提交。最终文档提交ID、远端main、clean、两提交树各7340输入及225件归档的逐SHA核验，统一记录在[发布凭证](results/hmc_resource_display_registration_terminal_20261023_v2/publication-v1.json)，不为写入自身提交ID再修改封存原件。机器PASS不代签整答。
