# tree-sitter-arkts

ArkTS 严格模式（API 12+ Stable）的自维护 tree-sitter grammar。

**状态**：PR-(-1).b 调研期 —— 仅 corpus 种子；grammar.js / parser.c 在 PR-0 中实现。

## 设计基线

- **目标**：ArkTS 严格模式，API 12+ Stable
- **基础**：参考 tree-sitter-typescript 但不直接 fork —— ArkTS 引入 `struct` 替代 class、ArkUI 链式调用 + trailing block、21 装饰器白名单，与 TS surface 差异显著
- **严格门**：grammar 层 reject `any` / `as` / `index_signature` / `Function` 类型；遇到立即 ERROR 节点（红线 L-ArkTS-3）
- **fallback**：codrax 主体 4 层（arkts → typescript → regex → path-only）；grammar 自身不放宽

## 文件布局（最终态，PR-0 完成时）

```
.
├── grammar.js              # tree-sitter grammar DSL
├── src/
│   ├── parser.c            # tree-sitter generate 产物（checkin）
│   ├── scanner.c           # 自定义 scanner（处理 ArkUI trailing block）
│   ├── grammar.json
│   └── tree_sitter/
├── corpus/
│   ├── sources/*.ets       # 原始公开样本（PR-(-1).b 调研期产物）
│   └── *.txt               # tree-sitter test 标准格式（PR-0 写）
├── package.json
├── binding.go              # cgo Go binding
└── README.md
```

## Corpus 来源（公开资料）

- developer.huawei.com/consumer/cn/doc/harmonyos-references/arkts-language-overview
- gitee.com/openharmony/applications_*
- gitee.com/openharmony/arkui_ace_engine
- developer.huawei.com codelabs

每个 .ets 文件首行注释标 source URL。

## 装饰器白名单（21 个）

ArkUI：`@Component @Entry @Preview @CustomDialog @Observed @Reusable @Builder @BuilderParam @Styles @Extend`
状态管理：`@State @Prop @Link @Provide @Consume @Watch @ObjectLink @StorageLink @StorageProp @LocalStorageLink @LocalStorageProp`

非白名单装饰器 → grammar parse error。
