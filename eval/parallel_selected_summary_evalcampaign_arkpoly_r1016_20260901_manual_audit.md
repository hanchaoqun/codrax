# r1016 ArkTS / Python–Rust 人工审计

- date: 2026-09-02T02:47:10Z
- sweep_start_ts: 20260901-194708
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

基线 `main@a3cec27ae` 的干净构建快照；恰好两路并行。机器通过不代替人工事实/过程审核。本批未运行额外 live case。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | arkts_repomap | PASS | eval/results/arkts_repomap-20260901-194710 | typed_inventory_rowset,answer_contains | none | 162s | 29 | read=0,repo_map=1,list=0,trace=0,source_lens=1 | midloop=1,inv=3/0,fin_reject=0,unavail=0,prune=0 | PASS | 4 Entry+2 Builder 精确；零最终拒绝；内部工具名露出仅 advisory |
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260901-194710 | answer_regex | none | 485s | 62 | read=10,repo_map=1,list=2,trace=0,source_lens=1 | midloop=15,inv=3/0,fin_reject=13,unavail=0,prune=0 | FAIL（内容部分正确） | B1551 混合修补权限丢失；B1552 结果字段晚报；模型错误终点判断，不能仅凭正则收账 |

## ArkTS 清单

终稿 `.codrax/output/20260901-194950.331-12331.md` 与 `internal/thirdparty/tree-sitter-arkts/corpus/sources` 对照：

- Entry：Index（01:5）、ParentComponent（03:32）、StyledPage（04:17）、ListPage（05:30），四个源码修饰符位置正确。
- Builder：defaultHeader（02:8）、GlobalCard（02:26），没有把同文件 BuilderParam 属性算入。
- 探索和成文上下文均携带完整 source-inventory rows；跨行修饰符和声明绑定正常，不是只猜文件名。
- 引用补齐为源码中真实修饰符行，六个列表项保留；首稿即接受，不要求此类清单强行画图。
- 终稿导语出现 `source_inventory` 是可读性残余，不能扫描正文强制替换或增加成文拒绝。

## 跨语言链与回退

终稿 `.codrax/output/20260901-195513.747-12378.md` 与本轮 `run-1.parent/{bindings-py,core-rs}` 对照：

- Python 的 `_HAVE_NATIVE`、UTF-8 编码、`_fastlex.tokenize_bytes` 和 `self._tokenize_slow` 分支正确；原生模块名正确。
- Rust 的 py.tokenize_bytes wrapper→super::tokenize_bytes→best_merge、MergeTable.from_triples / rank 已在源码和实际模型上下文中。
- 实际 finalizer log 3399–3402、3453–3466 已给出 register handoff、准确端点和可选组合示例；没有缺少 bridge 数据。
- 首稿误把导出绑定画成调用、多条内部操作画成无证 self-loop；校验拒绝这些边是正常的。模型后来选择删除无证边，
  最终保留图中的两段调用和回退，列表保留导出绑定。图缺乏桥接视觉解释，但用户未独立强制画完整图，不能靠系统补边。
- 最终称语义等价/返回相同结果、原生模块已编译导出，未充分区分条件源码与实际构建运行；本例未执行扩展构建或等价性测试。
- 最后 `conceptual_terminal_resolution` 选择 text.encode 并判断到达 Rust 概念目标，不受该证据支持。这是模型选择错误；
  不能替它选择别的候选或为得到指定答案扩大权限。

## 13 次拒绝的过程分解

日志 `run-1.logs/codrax-20260901-194711-000-12378.log`：

1. 首稿（19:50:08）：列表缺锚与图中六处关系错误同时出现。随后 producer 的4条列表 additions 被 lease 丢弃，
   输出 `failures=6/6 additions=0/4`；原错误却禁止 replace_blocks 并要求 addition_ref，当前 schema 的普通列表出口不同。B1551。
2. 第二轮模型自造 `diagram_edge_edits[].edge_anchors`；系统诚实回滚，修复提示含完整当前事务状态。
3. 第三轮只修图，列表 additions 下一轮才发布；第四轮列表四条补锚一次执行成功。这直接证明不是证据不足，而是混合同轮的组合缺口。
4. 后续列表缺 register handoff、部分整块替换丢端点身份、自造 block-field 操作，均未通过精确校验；不能全部归为系统自冲突。
5. 首稿已携带的非法终点选择直到第九轮持久化才报错，稳定块 id/当前选择行不足，进一步引发猜字段和重复整块替换。B1552 单列待修。
6. 第十四轮最终以合法但业务判断不恰当的 text.encode 选择通过；第十五轮补成员 facet。没有丢模型答案，未由系统替换正文。

## 本批处置与边界

- B1551：共享既有精确模型选中谓词，修复列表 additions 与图失败的同轮组合；提示服从实际 schema；不放开未选关系。
- B1552：下一小批处理已有精确结果字段的早期合并反馈/可执行修复方式，保留候选与结论的模型所有权。
- 模型终点误判、构建条件不充分、内部术语和可选图解释力暂作独立残余，不增加答案关键词硬门。
- 本轮最大上下文62%，没有 provider 中断、工具不可用或修补内容截断借口。活跃流持续接收期间没有按4ms/4分钟无可见答案降级。
- Trace 显式窗/投影/自动补齐、链上主因及占时/可消量双账户不在本批修改面，由全仓回归继续看护。
