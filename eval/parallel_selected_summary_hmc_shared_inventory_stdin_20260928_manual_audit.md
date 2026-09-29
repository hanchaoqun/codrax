# HMC §186：完整 stdin 与共享事件展示人工审计

日期：2026-09-28（本地）；快照 `d18a8dd25ba3-dirty`，dirty仅架构文档。24550固定2并行×1，正式exit0；没有第三次追跑。机器1/2、完整人工0/2。原机器判定、失败报告和日志不覆盖。

| 场景 | 自动结果 | 完整人工 | 接入/上下文结论 |
| --- | --- | --- | --- |
| `trace_stdin_sqlite_jank_inventory` | PASS，124秒 | FAIL | 完整二进制stdin→EOF收据→SQLite只读准备→全量检索命中；两条数值正确，线程名被说成进程名 |
| `trace_existing_sqlite_dictionary` | FAIL，190秒 | FAIL | 13条完整到达实际finalizer，四段8/4/4/5ms正确；未知域、未知启动名及跨类型混列仍错 |

## 1. 标准输入：能力通过，身份表述不通过

结果目录 `eval/results/trace_stdin_sqlite_jank_inventory-20260928-183013`。问题仅询问至少两帧的卡顿记录、应用、起止时间、帧数和持续时间；没有把时间单位、边界、因果资格等系统约束塞给用户。

- `.codrax/trace-input-04c4feccd8a07a3d0f542a41bc777490/input-stream.json`：49152字节、EOF=true、上限68719476736字节、完整SHA `f7fa073b0016e82ba8c5e1ad851802e64ead08bf828f4feb1947a8d9e5fb77a6`；流有独立源代次，未把预览当转换输入。
- 实际finalizer日志2711–2723：2份查询收据分别总数3和2，共享3个展示对象，引用成员分别3和2；零成员省略、零metadata省略。仍各保过滤条件和范围，未把两个查询计数相加。
- 正文准确列出1020000000–1040000000ns、2帧、20ms，以及1100000000–1160000000ns、4帧、60ms；appId均27599，1帧记录排除。没有时区转换或邻近业务晋升根因。
- 人工失败：正文称“进程名main”，表写`27599 (main)`。原SQLite的process.name是DocumentApp，thread.name才是main；实际库存只携comm/main、emitter身份和appid，不能把线程标签当进程名称。归01.3/16.4身份角色交接，不归stdin损坏，也不凭一次结果宣称模型波动。
- trace_query=3，read_file/repo_map/list/source_lens均0；finalizer_reject=0。root sidecar schema2空列表，`trace_root_cause_contract_not_active`，符合清单问题没有授根因。

## 2. 启动与系统事件：端点恢复，语义缺陷保留

结果目录 `eval/results/trace_existing_sqlite_dictionary-20260928-183013`。沿用原自然问题和机器oracle，未改旧问题、旧结果或断言追绿。

- 实际finalizer日志2039–2049：1次查询13/13成员全到场，32对象预算只占13、零省略；1.034和1.045两个E端点在row-7/row-9。真实答案四段均正确：1.010–1.018=8ms，1.022–1.026=4ms，1.030–1.034=4ms，1.040–1.045=5ms。较§185末两段错误有改善，但本次只查一次，不冒称live复现了9份重叠查询的完整压力；该压力由公共工具→实际adapter回归覆盖。
- 自动8ms/5ms正则失败均为单位在表头、单元格只写数字的oracle误报；机器FAIL原样保留。人工仍有独立实质FAIL，不能据此改成PASS。
- row-11的plugin.domain明确unavailable/null_reference，模型却写成`codrax_hisysevent`（物理载体名）。1.058的系统单点BOOTSTRAP又列入启动阶段表。两个AppStartup:startup没有披露名称未知：生产者仍把NULL/重复字典引用降成同一兜底标签，逐行状态未传递，归04.3/17.7/01.3/16.4。
- 系统覆盖附注说“展示上限省略尾部”导致无因果投影，实际event_search枚举和成员均完整；日志2080是通用`trace_query_result_compacted` refinement。`runtimeTraceCausalProjectionCoverageReasons`把整个结果的压缩标志直接转成因果缺证原因，未区分成员、元数据和未查询的因果视图。这是需结构化修复的系统展示问题，不能扫正文或删除合法压缩提示。
- 日志2234首emit已accepted；2237后的维度advisory发起一次patch。2273–2275真实拒绝未发布的`add_facet_id=member_set`两项操作，原稿未被改坏；fin_reject=1确为patch拒绝。当前教学已有“若发布则用，否则完整replace_blocks”的条件分支，尚不能证明schema授权错误；应减少未发布分支的条件教学和重复提示，而非放宽精确schema gate。
- trace_query=1，read_file/repo_map/list/source_lens均0；root sidecar schema2空列表、`no_selectable_typed_on_chain_candidates`，未从邻近信息补根因。两份报告均为表格/文字，无关系图；检查Markdown/HTML产物，不声称做过截图视觉验收。

## 3. 原始证据摘要

| 文件 | SHA-256 |
| --- | --- |
| dictionary `run-1.logs.all.log` | `713c281b1a6af07c7c977745e6d367f41945dcce530da6a47f234e2249ae8f2b` |
| stdin `run-1.logs.all.log` | `b63edc89da1e115bbb89ca0ebb4929c0232424031a74c46879d43eb8c6802cb2` |
| `.codrax/output/20260928-183320.699-24389.md` | `311d14c0078a51f1f05c7dabaa2454309e6f5782730492f6851afb224a530c13` |
| 对应dictionary root-causes | `4a2346a0ab933741f5fc321029bed58c55054781554fe36eabbf8429419b3a67` |
| `.codrax/output/20260928-183213.367-24391.md` | `1098715edd4a3652f19416a3f718f9795c2c354c3724a349f73d0c8cda814714` |
| 对应stdin root-causes | `5f8063f5882d6f6c5da9ef59d8ba0282f0fb2ef8c728bdcc77ef0249d2287e39` |

后续优先：当前写run/batch的只读登记投递；启动/事件/发起线程身份状态统一交接；覆盖原因按精确字段的范围投影。参考能力主线继续17.7一致快照与04.3启动实例，不能以这两份人工FAIL无限阻塞其他高ROI能力；03.2/04.2/08.3/08.4/18.2原验收父项保留。
