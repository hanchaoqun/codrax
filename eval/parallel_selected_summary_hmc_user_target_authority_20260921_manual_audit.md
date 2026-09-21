# 用户目标授权固定双例人工审计（2026-09-21）

构建 `819dc5b91f74`，构建时间 `2026-09-21T12:16:26Z`。runner65962正式exit0，05:16:56同时开始，严格2并行×1，无第三次追绿。结果根 `eval/results/hmc_user_target_authority_20260921`，目录时间戳 `20260921-051656`。机器0/2，完整人工0/2；目标授权子片公开回归通过，不以局部正确抵销完整回答失败。

| case | 时间 | 机器 | 人工 | 结论 |
|---|---:|---|---|---|
| trace_query_business_marker_io_chain | 261s | FAIL | FAIL | 原生链与分析锚标签正确；业务/查询分尺、完成/唤醒时间、未证/否定仍误用，补齐未运行 |
| real_trace_h4_supply_thermal_witness | 156s | FAIL | FAIL（核心判断通过） | 策略频率范围与性能受限区分正确；旧IO两尺遗漏和内部枚举泄漏仍在 |

## 业务 IO：正确证据已供给，模型仍误用；补齐边界另立项

完整日志 `trace_query_business_marker_io_chain-20260921-051656/run-1.logs/codrax-20260921-051659-000-70502.log`；主答 `run-1.principal.md`，系统投影 `run-1.answer-transcript.md`。

- 成功分析在日志897–899声明 `no_named_target`、空targets，Entities仅OpenDocument/LoadDocumentIndex/backup；探索随后产生app100和worker200两个cursor。投影81行正确称app为“分析锚点线程”，86–89及146/160保留storage-irq→worker→app。成功分析没有worker/app弱实体，因此本轮不能反事实证明旧截链被阻止；该性质由真实公开回归验证。
- 主答1、16–20将较宽查询的7ms运行套入50ms业务窗，且误称app-main自己发IO；实际发起者是worker。最终输入2346–2349有完整OpenDocument **50=5+1+44**、LoadDocumentIndex **40=8+1+31**及各自起止。宽查询为0.999–1.052，共53ms，状态归账52=7+1+44，未归账1ms，不能继续引用上一批51ms。
- 主答3、26把请求完成写成1.040010（真实为1.040000，前者是唤醒）；28行又列正确区间。2362/2364行明确请求35ms与worker S等待31ms各自起止。主答未清楚说明两尺不可相加，也未单列worker唤醒后1ms调度等待。
- 主答5、29、31把backup的 `completion_woke_issuer=false` 写成没有发出唤醒；最终输入2361/2363已明示这是未证明，不是已证明未唤醒。另有 `s_sleep`、`issuer_blocked` 等内部词泄漏。结构选择 `no_causal_conclusion`（日志2581）而正文仍断言真正根因；不能用正文扫描或系统代写结论修补。
- 本轮六次query，没有root_cause_rank/critical_blocking_calls。成功调查接受business_span_ref（1552行），但1653行补齐因 `no_typed_target` 跳过，1677行supplements=0。既有full_artifact保护先排除业务focus，混PID cursor又不能代选目标。侧车 `.codrax/output/20260921-052115.543-70502.root-causes.json` 正常生成schema2空结果，原因 `no_selectable_typed_on_chain_candidates`；不是文件未生成，也不是本批resolver改坏IO测量。仍不能签IO排名能力本轮保持。
- 分析首次从工件猜用户quote被旧精确门拒绝；第二次已选no_named却留空壳targets，错误提示仍要求“修身份、不要省略”（810行），第三次模型自行清空后恢复。是已证教学冲突，不是无解门。应按现有结构profile统一恢复指引，不改准入/伪造用户身份。

## 真实 H4：有限结论正确，独立 IO 披露仍缺

目录 `real_trace_h4_supply_thermal_witness-20260921-051656`。用户明确17267及13762.791708–13763.024898窗口。首次named目标缺身份被拒，下一次修复；四次query、三次成功，CPU全局频率视图携pid的错误被修正。最终首次成文接受，无最终重试。

- 有限事实集按实际问题返回；工作/帧合同均未启用，零因果投影正确，不为追图扩权。
- 主答四状态157.248/5.604/70.338/0ms、合计233.190ms及8CPU清单正确。CPU4策略范围558000–2100000kHz、28条记录，最低样本558000在下界；未将存在策略上限说成已经证明性能受限。机器的上限词面检查未识别范围式写法，保留原FAIL、不改oracle追绿。
- 最终输入1937附近有至少四段闭合IO等待，已列段并集至少4.384ms（1.337+1.238+1.027+0.782），并非邻近算力4.384515。现有教学要求独立披露，但模型与系统附录均未完整呈现，只给调度D/IO为0的窄口径；覆盖上限仍需保留，不能报作完整全量。旧IO两尺缺口不销。
- 主答54行泄漏 `target_effect_unproven_no_slice_binding`。侧车 `.codrax/output/20260921-051929.891-70500.root-causes.json` 为schema2空结果、`trace_root_cause_contract_not_active`，与本轮有限事实合同一致。

## 后续任务与验收边界

1. 优先修空壳target/profile恢复教学矛盾，公开拒绝→按提示修正→成功交接，保named与明确窗正针。
2. 独立设计全工件范围、已接受业务实例、多cursor并存的自动补齐；不得删除原范围保护或把探索线程晋升用户目标。
3. 当前声明/真实观测原生suite/id完整并置；业务/查询、请求驻留/闭合阻塞同卡分尺，保边界、削重复，不继续堆同义教学。
4. 模型错用已完整供给事实、内部词、H4独立IO披露继续留债。旧FAIL不回写，不凭一次生成判为纯模型波动后销账。父账13/79交付、66开放，B2–B6仍开放。
