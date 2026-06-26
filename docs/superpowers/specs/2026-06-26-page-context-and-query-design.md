# Page Context 一等公民化 & 数据访问分层设计

> 2026-06-26 · 两个相关改动：(1) 把 `context` 提升为 page 的一等公民，提供 per-key 增删改查；(2) 重构数据访问的层级边界——`evolving` 管 roll 级（inner + 模板），`page query` 管 page 级（cwd outer）。

## 1. 背景与动机

### 1.1 架构层级

```
rollhub (多个 iroll 的注册中心)
  └─ iroll (一个包，roll 级)
       ├─ inner.db          roll 级蓝图（dna / loop seeds / skills / metadata / 模板 page）
       ├─ roll-outer.db     roll 级模板（新 cwd 复制的种子）
       └─ cwd (工作目录)
            └─ <cwd>/.iroll/<name>.db   cwd 级活数据（pages / memory / loop_runs）
                 └─ page (page_id)
                      └─ loop runs
```

**数据归属的不对称是核心：** inner 是 `1 : roll`（唯一），outer 是 `1 : (roll, cwd)`（多个）。

### 1.2 两个痛点

1. **context 只能整体替换。** `update-context --content '{...}'` 必须传完整 JSON，无法改单个键。Agent 想更新 `user_context.project` 必须重发整个 context——既冗余又容易丢字段。
2. **evolving 无法表达"改哪个 outer"。** evolving 现在打开 workspace 默认那一个活 outer，但一个 roll 有多个 cwd outer。它改的是某个活工作库，而非 roll 级蓝图——与"进化 agent 核心"的语义不符，还存在"选哪一个"的歧义。

## 2. 数据访问分层（SQLite 强制）

两个 SQL 逃生舱共享 `db.ExecuteAll` 引擎，区别只在**打开方式**：

| 命令 | main db | inner attached | 能改 |
|---|---|---|---|
| `roll evolving` | 模板 roll-outer.db (**rw**) | inner.db (**rw**) | 蓝图：`inner.dna` / `inner.loop` / `inner.metadata` / 模板 seed |
| `page query` | cwd outer.db (**rw**) | inner.db (**ro**) | 工作数据：`pages` / `memory` / `loop_runs`；`inner.*` 只读 |

- `page query` 把 inner 以**只读** attach（SQLite `?mode=ro` URI）→ `UPDATE inner.dna` 被 SQLite 拒绝，从底层守住"page 不碰蓝图"的边界。
- `inner.` 前缀是既有约定（模板 context 的 `@sql` 已用 `inner.metadata` / `inner.dna`，见 `examples/base-agent/init_data.sql`）。
- **关键不对称：**
  - evolving 改 inner → 所有现存 page **立即**生效（它们 live attach inner）。
  - evolving 改模板 roll-outer.db → **只影响之后新建的 cwd**（老 cwd 已从模板独立复制，见 `page.go:130` 的 copyFile 逻辑）。

## 3. Feature 1：context 一等公民

### 3.1 命令

移除 `page get-context`、`page update-context`。新增：

```
logos page get [path] [--page <id>] [--alias <name>] [--roll <name>] [--cwd .]
logos page set <path> <value> [--page <id>] [--alias <name>] [--roll <name>] [--cwd .]
logos page set --content '<json>' [targeting flags]
logos page unset <path> [--page <id>] [--alias <name>] [--roll <name>] [--cwd .]
logos page alias <name> [targeting flags]
logos page alias --clear [targeting flags]
```

targeting flags（`--page` / `--alias` / `--roll` / `--cwd`）复用现有 `resolvePageContext` 的解析优先级：`--page > --alias > --roll > 位置参数 > 当前 cwd`。

### 3.2 path 语法

点号表示对象嵌套：`user_context.project`、`system_prompt`。

- 只支持对象键，**不支持数组索引**（数组作为整体值设置/覆盖）。
- `set` 时中间路径不存在 → 自动创建中间对象（`set a.b.c v` 会创建 `a`、`a.b`）。
- `get` / `unset` 时路径不存在 → 错误 `key_not_found`。
- 保留键 `loop`：动态注入字段（见 loop-context spec）。在 raw context 上 `set loop.xxx` 技术上可写，但读取时会被动态注入覆盖，无意义。

### 3.3 值语义（json-or-text）

`page set` 的 value：能解析成 JSON 则存 JSON 值，否则存字符串。对齐 loop `plan`/`progress` 的 json-or-text 约定。

| 输入 | 存储值 |
|---|---|
| `page set user_context.project blog` | 字符串 `"blog"` |
| `page set user_context.active true` | 布尔 `true` |
| `page set user_context.tags '["a","b"]'` | 数组 `["a","b"]` |
| `page set greeting '{"@file":"Resources/greeting.txt"}'` | marker 对象（原样存） |

**写入存 raw（不解析），读取时解析 @file/@sql。** `page get` 复用 `ResolveContext` 的 `resolveValue` 对取出的值做解析。

### 3.4 各命令行为

| 命令 | 行为 | 输出 |
|---|---|---|
| `page get`（无 path） | 返回整个解析后 context（等同旧 get-context，**含动态注入的 `loop` 字段**） | data = 解析后 context + hints |
| `page get <path>` | 在**同一个**完整解析+注入的 context 上导航 path，返回该处值 | data = 该处值 + hints |

**`page get` 无 path vs 有 path：同一份 context。** 有 path 不是另读 raw，而是先做完整 `ResolveContext`（解析 @file/@sql + 注入 `loop`），再在结果上按 path 取切面。因此 `page get loop.focus.main.seed_name`、`page get user_context.project`、`page get system_prompt` 都能取到。数组索引仍不支持（`page get loop.focus.children` 返回整个数组，不能 `.0.seed_name`）。
| `page set <path> <value>` | 读改写 raw context，设单键 | data = page brief + hints |
| `page set --content '<json>'` | 整体替换 raw context | data = page brief + hints |
| `page unset <path>` | 读改写，删单键 | data = page brief + hints |
| `page alias <name>` | 设 alias（pages.alias + page_index） | data = page brief + hints |
| `page alias --clear` | 清 alias | data = page brief + hints |

**`page set` 参数校验：** 若 `--content` 已提供 → 整体替换模式（位置参数被忽略，或给出则报错）；否则要求恰好 2 个位置参数（path, value）。两者都不给 → 报错 `at least one of <path> <value> or --content is required`。

### 3.5 新增错误码

```go
ErrCodeKeyNotFound = "key_not_found"
```

完整错误码清单（8 个）：原 7 个 + `ErrCodeKeyNotFound`。用途：`page get`/`page unset` 的 path 不存在。

## 4. Feature 2：evolving 重构 + page query

### 4.1 evolving 重构

**签名不变：** `logos roll evolving [name:version] [sql] [--sql <stmt>] [--file <path>] [--dry-run]`。

**目标变更：**

| | 现在 | 改后 |
|---|---|---|
| outer 来源 | `WorkspaceOuterDbPath`（workspace 默认活 outer，不存在则从模板复制） | 直接打开模板 `~/.iroll/<name>/<version>/roll-outer.db` |
| inner | attached rw | attached rw（不变） |
| 碰活数据？ | 是（改的是 workspace 默认活库） | **否**（只碰模板 + inner） |

SQL 输入优先级（`--sql > 位置参数 > --file > stdin`）、dry-run、`ExecuteAll` 引擎均不变。复用现有 `OpenOuter(templateOuterPath, innerPath)`。

### 4.2 page query（新）

```
logos page query [sql] [--sql <stmt>] [--file <path>] [--dry-run] [--page <id>] [--alias <name>] [--cwd .]
```

- 解析目标 page（`--page` / `--alias` / `--cwd`）→ 经 `store.LookupPageByID` 拿到 cwd outer.db 路径。
- 打开 cwd-outer (**rw**) + inner (**ro**)，用新函数 `OpenOuterReadOnlyInner`。
- SQL 输入优先级同 evolving；**自由 SQL，不自动加 `WHERE page_id=?`**（"只看当前 page"由专用命令 `query-memory` / `loop show` 覆盖，逃生舱保持强大显式）。
- 复用 `db.ExecuteAll`，输出格式同 evolving（三段式 + results 数组）。

**首个位置参数是 SQL（非 tag）。** page query 通过 page 解析目标，不取 `name:version`。

## 5. db 层变更

### 5.1 新增函数

| 函数 | 作用 |
|---|---|
| `OpenOuterReadOnlyInner(outerPath, innerPath)` | 打开 outer (rw) + inner attached 只读（`ATTACH 'file:<inner>?mode=ro' AS inner`） |
| `GetContextKey(db, pageID, path, irollPath) (interface{}, error)` | 先完整 `ResolveContext`（解析 + 注入 loop），再在结果 map 上导航 path 返回值 |
| `SetContextKey(db, pageID, path, rawValue string) error` | 读改写 raw context，设 path（rawValue 已做 json-or-text 解析） |
| `UnsetContextKey(db, pageID, path) error` | 读改写，删 path |

### 5.2 path 导航 helper

在 `map[string]interface{}` 上操作的三个小工具（db 层内部）：

- `navigateGet(m, path) (value, found)` — 沿点号路径取值
- `navigateSet(m, path, value)` — 沿路径设值，中间不存在则建对象
- `navigateUnset(m, path) (found)` — 沿路径删键

### 5.3 evolving 复用

evolving 直接调用 `OpenOuter(templateOuterPath, innerPath)`（现有函数），传入模板路径。无需新 open 函数。

## 6. cmd 层变更

### 6.1 移除

- `getContextCmd`、`updateContextCmd` 及其 flag 变量（`getContextPage` 等 8 个）、`init()` 注册。
- 保留 `resolvePageContext`（被新命令复用）、`getContextHints`（重命名/复用为通用 hints）。

### 6.2 新增

- `pageGetCmd` / `pageSetCmd` / `pageUnsetCmd` / `pageAliasCmd` / `pageQueryCmd`，注册到 `pageCmd`。
- `page query` 的 SQL 输入路由复用 evolving 的 `resolveEvolvingSQL` 模式（去掉 tag 分支）。

### 6.3 hints 更新（机械批量）

所有 hint 中的命令名替换：

| 旧 | 新 |
|---|---|
| `logos page get-context` | `logos page get` |
| `logos page update-context --content ...` | `logos page set --content ...` |

涉及文件：`context.go`（新命令的 hints）、`page.go`、`loop_run.go`、`loop_seed.go`、`memory.go`、`evolving.go`、`query_dna.go` 中所有 hint 字符串。

## 7. 迁移与文档

- 删除 `get-context`/`update-context`，新增 `get`/`set`/`unset`/`alias`/`query`。
- 更新 `skills/logos-1/skill.md`：Startup Sequence 与 Command Reference 里的命令名。
- 更新 `README.md` / `docs/rebot-roll.md` 的命令表。
- evolving 测试（`evolving_test.go`）：操作 workspace 默认 outer 的用例改为操作模板 `roll-outer.db`。
- 新增错误码 `ErrCodeKeyNotFound`，更新 structured-output-v2 spec 的错误码清单。
- `logos-positioning-design.md` 中 `update-context` 命令名同步为 `page set`（概念不变）。
- **无向后兼容别名**（干净切除）。

## 8. 不在范围

- `query-memory` / `query-dna` 的命名调整（保持现状）。
- `chat_history` 表（已否决，见 positioning spec）。
- evolving 的 REPL / 语法高亮 / 跨 iroll 执行。
- path 的数组索引支持、JSON Pointer 标准语法（点号够用）。
- `page alias` 的"查看当前 alias"模式（用 `page list` 查看）。

## 9. 测试策略

- **path 导航**：get/set/unset 在嵌套对象上的各种路径（存在、不存在、中间缺失自动建、顶层、深层）。
- **值语义**：json-or-text 解析（字符串、数字、布尔、数组、对象、marker）。
- **读写一致性**：`set user_context.project blog` 后 `get user_context.project` 返回 `"blog"`。
- **保留性**：set 单键不破坏其他键；unset 单键不影响其他键。
- **evolving 边界**：evolving 改模板后，新 cwd 继承、老 cwd 不受影响；evolving 改 inner 后所有 page 立即生效。
- **page query 边界**：`UPDATE inner.dna` 被拒绝（read-only）；`SELECT * FROM memory` 正常；dry-run 不持久化。
- **错误码**：get/unset 缺失 path → `key_not_found`；targeting flags 解析失败 → 对应码。
